package bulk

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"math"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/rodfileto/trackid-vision/vision"
	xdraw "golang.org/x/image/draw"

	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/identity"
	"github.com/rodfileto/trackid/storage"
)

// Outcomes recorded in bulk_enroll_outcomes.
const (
	OutcomeEmbedded      = "EMBEDDED"
	OutcomeMultipleFaces = "MULTIPLE_FACES"
	OutcomeNoFace        = "NO_FACE"
	OutcomeUndecodable   = "UNDECODABLE"
	OutcomeFetchFailed   = "FETCH_FAILED"
)

// IngestStats counts what Ingest did with a manifest.
type IngestStats struct {
	Lines     int // enrollments read
	Invalid   int // lines that did not decode or validate
	Failed    int // enrollments Ingest (or an upload) refused
	Ingested  int // enrollments written (or, in a dry run, that would be)
	Files     int
	Uploaded  int // local files uploaded to core's bucket
	External  int // files kept where they are, by URI
	LastLine  int // the highest line number handled, for -start-line on a rerun
	Duration  time.Duration
	firstErrs []string
}

// Errors is a sample of the first errors, for the report.
func (s IngestStats) Errors() []string { return s.firstErrs }

// IngestOptions configures Ingest.
type IngestOptions struct {
	Workers   int
	StartLine int
	Commit    bool
}

// Ingest writes every manifest enrollment's rows with identity.Ingest, in parallel and with no
// enqueuer: bulk-enroll vectorizes on its own (Vectorize), so no per-feature task is queued. Local
// files are uploaded into core's bucket (store); files with a URI are stored as references.
// Ingest upserts, so a rerun over the same manifest rewrites nothing that changed and resumes
// where a killed run stopped. Without Commit it only reads and validates the manifest.
func Ingest(ctx context.Context, sqlDB *sql.DB, store *storage.Client, lines <-chan ManifestLine, opts IngestOptions) IngestStats {
	start := time.Now()
	workers := max(opts.Workers, 1)
	var mu sync.Mutex
	var stats IngestStats
	record := func(f func(*IngestStats)) {
		mu.Lock()
		f(&stats)
		mu.Unlock()
	}
	fail := func(line int, err error, invalid bool) {
		record(func(s *IngestStats) {
			if invalid {
				s.Invalid++
			} else {
				s.Failed++
			}
			if len(s.firstErrs) < 20 {
				s.firstErrs = append(s.firstErrs, fmt.Sprintf("line %d: %v", line, err))
			}
		})
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for line := range lines {
				record(func(s *IngestStats) {
					s.Lines++
					s.LastLine = max(s.LastLine, line.Number)
				})
				if line.Err != nil {
					fail(line.Number, line.Err, true)
					continue
				}
				m := line.Enrollment
				refs := make([]string, len(m.Files))
				uploaded, external := 0, 0
				ok := true
				for i, f := range m.Files {
					if f.URI != "" {
						refs[i] = f.URI
						external++
						continue
					}
					if !opts.Commit {
						continue
					}
					data, err := readLocal(f.Path)
					if err != nil {
						fail(line.Number, err, false)
						ok = false
						break
					}
					in, err := identity.UploadFile(ctx, store, m.RegisterNumber, f.FileType, f.Sequence, data)
					if err != nil {
						fail(line.Number, err, false)
						ok = false
						break
					}
					refs[i] = in.StorageRef
					if m.Files[i].ContentType == "" {
						m.Files[i].ContentType = in.ContentType
					}
					if m.Files[i].SizeBytes == 0 {
						m.Files[i].SizeBytes = in.SizeBytes
					}
					uploaded++
				}
				if !ok {
					continue
				}
				if opts.Commit {
					if _, err := identity.Ingest(ctx, sqlDB, m.Enrollment(refs)); err != nil {
						fail(line.Number, err, false)
						continue
					}
				}
				record(func(s *IngestStats) {
					s.Ingested++
					s.Files += len(m.Files)
					s.Uploaded += uploaded
					s.External += external
				})
			}
		}()
	}
	wg.Wait()
	stats.Duration = time.Since(start)
	return stats
}

// VectorizeOptions configures Vectorize.
type VectorizeOptions struct {
	EmbeddingType string
	ModelVersion  string
	// Embedders are the trackid-vision services to use, one per GPU; workers share them round
	// robin.
	Embedders []embedding.FaceEmbedder
	Workers   int // concurrent fetch, decode and embed
	Page      int // features per keyset page and per write transaction
	Limit     int // stop after this many features (0: all)
	// Pad presents each photo to the detector padded (embedding.FacesInBox), for registers whose
	// photos are cropped tight to the face; it costs a decode and a PNG encode per photo.
	Pad bool
	// Crops stores a face crop per embedded photo in core's bucket.
	Crops bool
	// RetryFailed retries features whose last outcome was FETCH_FAILED.
	RetryFailed bool
	Shards      *ShardWriter // nil: no shards written
	Commit      bool
	// Stop, when closed, ends the run after the page in flight (so no photo is cut off midway and
	// recorded as a failure).
	Stop <-chan struct{}
	// Progress, when set, is called after every page.
	Progress func(VectorizeStats)
}

// VectorizeStats counts what Vectorize did.
type VectorizeStats struct {
	Pending  int64 // features waiting at the start (dry run: all it would do)
	Done     int
	Outcomes map[string]int
	Duration time.Duration
}

type pendingFeature struct {
	id  int64
	ref string
}

type faceResult struct {
	featureID int64
	outcome   string
	faces     int
	score     float64
	relSize   float64
	width     int
	height    int
	vector    []float32
	cropRef   string
	detail    string
	transient bool // a failure not to record: the feature stays pending for the next run
}

// Vectorize detects and embeds the face of every KNOWN FACE_RECORD feature with no embedding of
// opts.EmbeddingType and no final outcome, paging by keyset on biometric_features.id. Each page is
// fetched and embedded in parallel, then its embeddings (match_path BULK) and outcomes are written
// in one transaction; a killed run loses at most the page in flight, and the next run resumes
// where it stopped. Embeddings also go to opts.Shards after the commit.
func Vectorize(ctx context.Context, sqlDB *sql.DB, store *storage.Client, opts VectorizeOptions) (VectorizeStats, error) {
	start := time.Now()
	stats := VectorizeStats{Outcomes: map[string]int{}}
	if err := sqlDB.QueryRowContext(ctx, pendingCountQuery, opts.EmbeddingType, opts.RetryFailed).Scan(&stats.Pending); err != nil {
		return stats, fmt.Errorf("bulk: count pending features: %w", err)
	}
	if !opts.Commit {
		stats.Duration = time.Since(start)
		return stats, nil
	}
	if len(opts.Embedders) == 0 {
		return stats, fmt.Errorf("bulk: no face embedder configured")
	}
	if store == nil {
		return stats, fmt.Errorf("bulk: storage is not configured")
	}
	page := opts.Page
	if page <= 0 {
		page = 512
	}
	workers := max(opts.Workers, 1)

	var after int64
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if stopped(opts.Stop) {
			break
		}
		n := page
		if opts.Limit > 0 {
			if stats.Done >= opts.Limit {
				break
			}
			n = min(page, opts.Limit-stats.Done)
		}
		batch, err := pendingPage(ctx, sqlDB, opts.EmbeddingType, opts.RetryFailed, after, n)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			break
		}
		after = batch[len(batch)-1].id

		results := make([]faceResult, len(batch))
		jobs := make(chan int)
		var wg sync.WaitGroup
		for w := range workers {
			embedder := opts.Embedders[w%len(opts.Embedders)]
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range jobs {
					results[i] = vectorizeOne(ctx, store, embedder, batch[i], opts)
				}
			}()
		}
		for i := range batch {
			jobs <- i
		}
		close(jobs)
		wg.Wait()

		ids, err := writePage(ctx, sqlDB, opts, results)
		if err != nil {
			return stats, err
		}
		if opts.Shards != nil {
			for i, r := range results {
				if ids[i] != 0 {
					if err := opts.Shards.Add(ids[i], r.vector); err != nil {
						return stats, fmt.Errorf("bulk: write shard: %w", err)
					}
				}
			}
		}
		for _, r := range results {
			if r.transient {
				stats.Outcomes["RETRY_LATER"]++
				continue
			}
			stats.Outcomes[r.outcome]++
		}
		stats.Done += len(batch)
		if opts.Progress != nil {
			stats.Duration = time.Since(start)
			opts.Progress(stats)
		}
	}
	stats.Duration = time.Since(start)
	return stats, nil
}

func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

const pendingWhere = `
	bf.feature_type = 'FACE_RECORD' AND bf.identity_file_id IS NOT NULL AND bf.withdrawn_at IS NULL
	AND NOT EXISTS (SELECT 1 FROM feature_embeddings fe WHERE fe.biometric_feature_id = bf.id AND fe.embedding_type = $1)
	AND NOT EXISTS (SELECT 1 FROM bulk_enroll_outcomes o WHERE o.biometric_feature_id = bf.id AND o.embedding_type = $1
	                AND NOT ($2 AND o.outcome = 'FETCH_FAILED'))`

const pendingCountQuery = `SELECT COUNT(*) FROM biometric_features bf WHERE ` + pendingWhere

func pendingPage(ctx context.Context, sqlDB *sql.DB, embeddingType string, retryFailed bool, after int64, limit int) ([]pendingFeature, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT bf.id, f.storage_ref
		FROM biometric_features bf
		JOIN identity_files f ON f.id = bf.identity_file_id
		WHERE bf.id > $3 AND `+pendingWhere+`
		ORDER BY bf.id
		LIMIT $4`, embeddingType, retryFailed, after, limit)
	if err != nil {
		return nil, fmt.Errorf("bulk: list pending features: %w", err)
	}
	defer rows.Close()
	var out []pendingFeature
	for rows.Next() {
		var p pendingFeature
		if err := rows.Scan(&p.id, &p.ref); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// vectorizeOne fetches, detects and embeds one photo. Of several faces it embeds the largest:
// on an identity photo the subject is the largest face, a bystander or a printed photo smaller.
func vectorizeOne(ctx context.Context, store *storage.Client, embedder embedding.FaceEmbedder, p pendingFeature, opts VectorizeOptions) faceResult {
	r := faceResult{featureID: p.id}
	data, err := store.Download(ctx, p.ref)
	if err != nil {
		r.outcome, r.detail = OutcomeFetchFailed, err.Error()
		return r
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		r.outcome, r.detail = OutcomeUndecodable, err.Error()
		return r
	}
	r.width, r.height = config.Width, config.Height

	var decoded image.Image
	decode := func() (image.Image, error) {
		if decoded == nil {
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			decoded = img
		}
		return decoded, nil
	}

	var faces []vision.Face
	if opts.Pad {
		img, err := decode()
		if err != nil {
			r.outcome, r.detail = OutcomeUndecodable, err.Error()
			return r
		}
		faces, err = embedding.FacesInBox(embedder, img, img.Bounds())
		if err != nil {
			r.transient, r.detail = true, err.Error()
			return r
		}
	} else {
		faces, err = embedder.DetectAndEmbed(bytes.NewReader(data))
		if err != nil {
			// The header decoded, so a failure here is the full decode (a truncated file) or the
			// inference itself. A truncated file is final; anything else is retried next run.
			if _, derr := decode(); derr != nil {
				r.outcome, r.detail = OutcomeUndecodable, derr.Error()
				return r
			}
			r.transient, r.detail = true, err.Error()
			return r
		}
	}

	r.faces = len(faces)
	if len(faces) == 0 {
		r.outcome = OutcomeNoFace
		return r
	}
	best := faces[0]
	for _, f := range faces[1:] {
		if area(f.Box) > area(best.Box) {
			best = f
		}
	}
	r.outcome = OutcomeEmbedded
	if len(faces) > 1 {
		r.outcome = OutcomeMultipleFaces
	}
	r.vector = best.Embedding
	r.score = float64(best.Score)
	if total := float64(config.Width) * float64(config.Height); total > 0 {
		r.relSize = area(best.Box) / total
	}

	if opts.Crops {
		img, err := decode()
		if err == nil {
			if crop, err := faceCrop(img, best.Box); err == nil && crop != nil {
				key := fmt.Sprintf("crops/%s/%d.jpg", opts.EmbeddingType, p.id)
				if ref, err := store.Upload(ctx, key, crop, "image/jpeg"); err == nil {
					r.cropRef = ref
				} else {
					log.Printf("bulk: biometric_features %d: upload crop: %v", p.id, err)
				}
			}
		}
	}
	return r
}

func area(box [4]float32) float64 {
	return math.Max(0, float64(box[2]-box[0])) * math.Max(0, float64(box[3]-box[1]))
}

// cropSize is the side of the stored face crop: the recognizer's input size, enough to show and
// compare a face without the original.
const cropSize = 112

// faceCrop is the face's box with vision.DefaultCropPadding around it, scaled to fit cropSize,
// as a JPEG. It is a box crop, not the recognizer's 5-point alignment, which trackid-vision does
// not expose.
func faceCrop(img image.Image, box [4]float32) ([]byte, error) {
	region := vision.Crop(img, box, vision.DefaultCropPadding)
	if region == nil {
		return nil, nil
	}
	b := region.Bounds()
	scale := math.Min(float64(cropSize)/float64(b.Dx()), float64(cropSize)/float64(b.Dy()))
	w, h := max(1, int(float64(b.Dx())*scale)), max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), region, b, xdraw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writePage writes a page's embeddings and outcomes in one transaction and returns, per result,
// the new feature_embeddings id (0 where none was written).
func writePage(ctx context.Context, sqlDB *sql.DB, opts VectorizeOptions, results []faceResult) ([]int64, error) {
	var embFeatures []int64
	var embVectors []string
	var oFeature, oFaces, oWidth, oHeight []int64
	var oOutcome, oCrop, oDetail []string
	var oScore, oSize []float64
	for _, r := range results {
		if r.transient {
			continue
		}
		if r.vector != nil {
			embFeatures = append(embFeatures, r.featureID)
			embVectors = append(embVectors, embedding.VectorText(r.vector))
		}
		score, size := math.NaN(), math.NaN()
		if r.vector != nil {
			score, size = r.score, r.relSize
		}
		detail := r.detail
		if len(detail) > 500 {
			detail = detail[:500]
		}
		oFeature = append(oFeature, r.featureID)
		oOutcome = append(oOutcome, r.outcome)
		oFaces = append(oFaces, int64(r.faces))
		oScore = append(oScore, score)
		oSize = append(oSize, size)
		oWidth = append(oWidth, int64(r.width))
		oHeight = append(oHeight, int64(r.height))
		oCrop = append(oCrop, r.cropRef)
		oDetail = append(oDetail, detail)
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	byFeature := map[int64]int64{}
	if len(embFeatures) > 0 {
		rows, err := tx.QueryContext(ctx, `
			INSERT INTO feature_embeddings (biometric_feature_id, embedding_type, embedding, model_version, match_path)
			SELECT t.feature_id, $3, t.vector::vector, $4, 'BULK'
			FROM unnest($1::bigint[], $2::text[]) AS t(feature_id, vector)
			ON CONFLICT (biometric_feature_id, embedding_type) DO NOTHING
			RETURNING id, biometric_feature_id`,
			pq.Array(embFeatures), pq.Array(embVectors), opts.EmbeddingType, opts.ModelVersion)
		if err != nil {
			return nil, fmt.Errorf("bulk: insert embeddings: %w", err)
		}
		for rows.Next() {
			var id, feature int64
			if err := rows.Scan(&id, &feature); err != nil {
				rows.Close()
				return nil, err
			}
			byFeature[feature] = id
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if len(oFeature) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO bulk_enroll_outcomes (biometric_feature_id, embedding_type, model_version, outcome, faces,
			                                  detector_score, relative_size, image_width, image_height, crop_ref, detail)
			SELECT t.feature_id, $10, $11, t.outcome, t.faces, NULLIF(t.score, 'NaN'), NULLIF(t.size, 'NaN'),
			       NULLIF(t.width, 0), NULLIF(t.height, 0), NULLIF(t.crop, ''), NULLIF(t.detail, '')
			FROM unnest($1::bigint[], $2::text[], $3::int[], $4::float8[], $5::float8[], $6::int[], $7::int[], $8::text[], $9::text[])
			     AS t(feature_id, outcome, faces, score, size, width, height, crop, detail)
			ON CONFLICT (biometric_feature_id, embedding_type) DO UPDATE SET
			    model_version = EXCLUDED.model_version, outcome = EXCLUDED.outcome, faces = EXCLUDED.faces,
			    detector_score = EXCLUDED.detector_score, relative_size = EXCLUDED.relative_size,
			    image_width = EXCLUDED.image_width, image_height = EXCLUDED.image_height,
			    crop_ref = EXCLUDED.crop_ref, detail = EXCLUDED.detail, created_at = NOW()`,
			pq.Array(oFeature), pq.Array(oOutcome), pq.Array(oFaces), pq.Array(oScore), pq.Array(oSize),
			pq.Array(oWidth), pq.Array(oHeight), pq.Array(oCrop), pq.Array(oDetail),
			opts.EmbeddingType, opts.ModelVersion); err != nil {
			return nil, fmt.Errorf("bulk: record outcomes: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ids := make([]int64, len(results))
	for i, r := range results {
		if r.vector != nil && !r.transient {
			ids[i] = byFeature[r.featureID]
		}
	}
	return ids, nil
}

// RebuildShards writes shards, from Postgres, for every BULK embedding of embeddingType that no
// shard in dir holds yet.
func RebuildShards(ctx context.Context, sqlDB *sql.DB, dir, embeddingType, modelVersion string) (int, error) {
	if err := OpenShardDir(dir, embeddingType, modelVersion); err != nil {
		return 0, err
	}
	rows, err := sqlDB.QueryContext(ctx, `SELECT id FROM feature_embeddings
		WHERE embedding_type = $1 AND match_path = 'BULK' AND model_version = $2 ORDER BY id`, embeddingType, modelVersion)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return FillShards(ctx, sqlDB, dir, ids)
}
