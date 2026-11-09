package bulk_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/rodfileto/trackid-vision/vision"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/bulk"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/db/migrations"
	"github.com/rodfileto/trackid/embedding"
	"github.com/rodfileto/trackid/storage"
)

// testDB creates a throwaway en_US-collated database on the server named by
// TRACKID_TEST_DATABASE_URL, migrates it, and drops it when the test ends. The test is skipped
// when the variable is unset.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	adminURL := os.Getenv("TRACKID_TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TRACKID_TEST_DATABASE_URL not set")
	}
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	name := fmt.Sprintf("trackid_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(fmt.Sprintf(
		`CREATE DATABASE %s TEMPLATE template0 ENCODING 'UTF8' LC_COLLATE 'en_US.utf8' LC_CTYPE 'en_US.utf8'`, name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`) })
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	sqlDB, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(sqlDB, "."); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

const (
	et = embedding.EmbeddingType
	mv = embedding.ModelVersion
)

// axis is a 512-d vector with weights on the given axes, normalized.
func axis(weights map[int]float64) []float32 {
	v := make([]float32, bulk.Dim)
	var norm float64
	for i, w := range weights {
		v[i] = float32(w)
		norm += w * w
	}
	for i := range v {
		v[i] /= float32(math.Sqrt(norm))
	}
	return v
}

// photo is one synthetic face photo: a PNG whose first pixel names the face, which fakeEmbedder
// maps to a vector. Red 255 is a photo with no face.
func photo(t *testing.T, path string, face, variant uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 50))
	img.Set(0, 0, color.RGBA{R: face, G: variant, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeEmbedder stands in for trackid-vision: the vector of a photo is faces[(R, G)] of its first
// pixel.
type fakeEmbedder struct{ faces map[[2]uint8][]float32 }

func (f fakeEmbedder) DetectAndEmbed(r io.Reader) ([]vision.Face, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, err
	}
	c := color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
	v, ok := f.faces[[2]uint8{c.R, c.G}]
	if !ok {
		return nil, nil
	}
	return []vision.Face{{Box: [4]float32{5, 5, 35, 45}, Score: 0.9, Embedding: v}}, nil
}

// world is the synthetic register:
//   - persons 0..9 each have two documents (a renewal) under one fiscal number, one photo each:
//     genuine pairs at about 0.92;
//   - person 10's face also appears on person 11's document, under another fiscal number: the
//     fraud, at about 0.92;
//   - person 12 looks like person 0: about 0.68 to person 0's photos;
//   - one photo with no face, one undecodable file, one missing file;
//   - one case trace of person 3, at about 0.86.
type world struct {
	dir    string
	faces  map[[2]uint8][]float32
	lines  []bulk.ManifestEnrollment
	vector map[string][]float32 // by register number
}

func newWorld(t *testing.T) *world {
	w := &world{dir: t.TempDir(), faces: map[[2]uint8][]float32{}, vector: map[string][]float32{}}
	add := func(person int, fiscal, register string, face, variant uint8, v []float32) {
		path := filepath.Join(w.dir, register+".png")
		photo(t, path, face, variant)
		if v != nil {
			w.faces[[2]uint8{face, variant}] = v
			w.vector[register] = v
		}
		w.lines = append(w.lines, bulk.ManifestEnrollment{
			PersonID: fmt.Sprintf("P%d-%s", person, register), DocumentType: "RG", DocumentNumber: "D" + register,
			FiscalNumber: fiscal, RegisterNumber: register, Name: fmt.Sprintf("Person %d", person), BirthDate: "2000-01-01",
			Files: []bulk.ManifestFile{{FileType: "photo", URI: "file://" + path}},
		})
	}
	for p := range 10 {
		for j := range 2 {
			add(p, fmt.Sprintf("F%02d", p), fmt.Sprintf("R%02d-%d", p, j), uint8(p), uint8(j),
				axis(map[int]float64{p: 1, 100 + 2*p + j: 0.3}))
		}
	}
	add(10, "F10", "R10-0", 10, 0, axis(map[int]float64{10: 1, 120: 0.3}))
	add(11, "F11", "R11-0", 10, 1, axis(map[int]float64{10: 1, 121: 0.3})) // person 10's face
	add(12, "F12", "R12-0", 12, 0, axis(map[int]float64{0: 1, 12: 1, 124: 0.3}))
	add(13, "F13", "R13-0", 255, 0, nil) // no face
	w.lines = append(w.lines, bulk.ManifestEnrollment{PersonID: "P14", DocumentType: "RG", DocumentNumber: "D14", FiscalNumber: "F14",
		RegisterNumber: "R14-0", Name: "Person 14", Files: []bulk.ManifestFile{{FileType: "photo", URI: "file://" + filepath.Join(w.dir, "missing.png")}}})
	broken := filepath.Join(w.dir, "broken.png")
	if err := os.WriteFile(broken, []byte("\x89PNG\r\n\x1a\nnot really"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.lines = append(w.lines, bulk.ManifestEnrollment{PersonID: "P15", DocumentType: "RG", DocumentNumber: "D15", FiscalNumber: "F15",
		RegisterNumber: "R15-0", Name: "Person 15", Files: []bulk.ManifestFile{{FileType: "photo", URI: "file://" + broken}}})
	return w
}

func (w *world) manifest(t *testing.T) string {
	var b strings.Builder
	for _, l := range w.lines {
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	b.WriteString("{not json}\n")
	return b.String()
}

// TestBulkGallery runs the whole cycle: enroll and vectorize, match two batches, apply under
// three threshold versions, calibrate. It runs once with a brute-force Go matcher standing in
// for tools/bulk-match, and once with the real one when uv is installed.
func TestBulkGallery(t *testing.T) {
	t.Run("go-matcher", func(t *testing.T) {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		runGallery(t, []string{"env", "BULK_HELPER_MATCHER=1", exe})
	})
	t.Run("python-matcher", func(t *testing.T) {
		if _, err := exec.LookPath("uv"); err != nil {
			t.Skip("uv not installed")
		}
		tool, err := filepath.Abs("../tools/bulk-match")
		if err != nil {
			t.Fatal(err)
		}
		runGallery(t, []string{"uv", "run", "--quiet", "--project", tool, "python", filepath.Join(tool, "bulk_match.py")})
	})
}

func runGallery(t *testing.T, matcher []string) {
	ctx := context.Background()
	sqlDB := testDB(t)
	w := newWorld(t)
	store := storage.NewFileClient([]string{w.dir})

	// Rows: a dry run writes nothing; the real run loads every valid line.
	ingest := func(commit bool) bulk.IngestStats {
		lines := make(chan bulk.ManifestLine)
		go bulk.ReadManifest(strings.NewReader(w.manifest(t)), 1, lines)
		return bulk.Ingest(ctx, sqlDB, store, lines, bulk.IngestOptions{Workers: 3, Commit: commit})
	}
	if s := ingest(false); s.Ingested != len(w.lines) || s.Invalid != 1 {
		t.Fatalf("dry run ingest = %+v", s)
	}
	if s := ingest(true); s.Ingested != len(w.lines) || s.Invalid != 1 || s.Failed != 0 || s.External != len(w.lines) {
		t.Fatalf("ingest = %+v %v", s, s.Errors())
	}

	// One trace of person 3, matched by the per-face sync's path (SYNC).
	res, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: "BULK-1", CaseType: cases.CaseTypeCriminal, Modality: "FACIAL",
		Evidences: []cases.EvidenceInput{{Sequence: 1, Traces: []cases.TraceInput{{Sequence: 1, TraceType: "FACE_CAPTURE"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	trace := res.Evidences[0].Traces[0]
	traceVector := axis(map[int]float64{3: 1, 400: 0.5})
	if _, err := db.New(sqlDB).UpsertFeatureEmbedding(ctx, db.UpsertFeatureEmbeddingParams{BiometricFeatureID: trace.FeatureID,
		EmbeddingType: et, Embedding: embedding.VectorText(traceVector), ModelVersion: sql.NullString{String: mv, Valid: true}}); err != nil {
		t.Fatal(err)
	}

	// Vectorize, killed after one page and resumed.
	shards := filepath.Join(t.TempDir(), "shards")
	if err := bulk.OpenShardDir(shards, et, mv); err != nil {
		t.Fatal(err)
	}
	opts := bulk.VectorizeOptions{EmbeddingType: et, ModelVersion: mv, Embedders: []embedding.FaceEmbedder{fakeEmbedder{w.faces}},
		Workers: 4, Page: 10, Limit: 10, Commit: true, Shards: bulk.NewShardWriter(shards, 7)}
	if s, err := bulk.Vectorize(ctx, sqlDB, store, opts); err != nil || s.Done != 10 || s.Pending != int64(len(w.lines)) {
		t.Fatalf("first vectorize = %+v, %v", s, err)
	}
	opts.Shards.Flush()
	opts.Limit, opts.Shards = 0, bulk.NewShardWriter(shards, 7)
	s, err := bulk.Vectorize(ctx, sqlDB, store, opts)
	if err != nil || s.Done != len(w.lines)-10 {
		t.Fatalf("resumed vectorize = %+v, %v", s, err)
	}
	opts.Shards.Flush()
	outcomes := map[string]int{}
	rows, err := sqlDB.QueryContext(ctx, `SELECT outcome, COUNT(*) FROM bulk_enroll_outcomes GROUP BY outcome`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var o string
		var n int
		rows.Scan(&o, &n)
		outcomes[o] = n
	}
	rows.Close()
	want := map[string]int{"EMBEDDED": 23, "NO_FACE": 1, "FETCH_FAILED": 1, "UNDECODABLE": 1}
	if fmt.Sprint(outcomes) != fmt.Sprint(want) {
		t.Fatalf("outcomes = %v, want %v", outcomes, want)
	}
	// Nothing is left, and the BULK embeddings are out of the per-face sync's backlog.
	if s, _ := bulk.Vectorize(ctx, sqlDB, store, bulk.VectorizeOptions{EmbeddingType: et, ModelVersion: mv}); s.Pending != 0 {
		t.Fatalf("pending after vectorizing = %d", s.Pending)
	}
	if n, _ := db.New(sqlDB).CountUnmatchedFeatureEmbeddings(ctx, et); n != 1 {
		t.Fatalf("per-face sync backlog = %d, want the trace only", n)
	}
	if have, err := bulk.ShardIDs(shards); err != nil || len(have) != 23 {
		t.Fatalf("shards hold %d ids, %v", len(have), err)
	}
	// Lose a shard: match fills it back from Postgres.
	lost, _ := filepath.Glob(filepath.Join(shards, "*.ids.npy"))
	os.Remove(lost[0])

	// Two batches.
	mopts := bulk.MatchOptions{EmbeddingType: et, ModelVersion: mv, Size: 12, Seed: "s", Floor: 0.3, K: 50,
		ShardsDir: shards, Matcher: matcher, GPU: "cpu", Commit: true}
	r1, err := bulk.Match(ctx, sqlDB, mopts)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := bulk.Match(ctx, sqlDB, mopts)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Size != 12 || r2.Size != 11 || r2.Earlier != 12 || r1.Others != 1 || r1.Filled == 0 {
		t.Fatalf("batches = %+v / %+v", r1, r2)
	}
	if r1.Comparisons+r2.Comparisons != 23*22/2+23 {
		t.Fatalf("comparisons = %d + %d, want every pair once plus the trace", r1.Comparisons, r2.Comparisons)
	}
	if _, err := bulk.Match(ctx, sqlDB, mopts); err == nil {
		t.Fatal("a third batch was made with nothing left")
	}

	// The score store holds exactly the pairs at or above the floor, each once.
	byRegister, err := embeddingIDs(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	vectors := map[int64][]float32{}
	for reg, v := range w.vector {
		vectors[byRegister[reg]] = v
	}
	var traceEmbedding int64
	sqlDB.QueryRowContext(ctx, `SELECT id FROM feature_embeddings WHERE biometric_feature_id = $1`, trace.FeatureID).Scan(&traceEmbedding)
	vectors[traceEmbedding] = traceVector
	stored := map[[2]int64]float64{}
	rows, err = sqlDB.QueryContext(ctx, `SELECT embedding_a_id, embedding_b_id, score FROM bulk_match_scores`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b int64
		var s float64
		rows.Scan(&a, &b, &s)
		stored[[2]int64{a, b}] = s
	}
	rows.Close()
	expected := map[[2]int64]float64{}
	ids := make([]int64, 0, len(vectors))
	for id := range vectors {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			if s := dot(vectors[a], vectors[b]); s >= 0.3 {
				expected[[2]int64{a, b}] = s
			}
		}
	}
	if len(stored) != len(expected) {
		t.Fatalf("score store holds %d pairs, want %d", len(stored), len(expected))
	}
	for k, s := range expected {
		if got, ok := stored[k]; !ok || math.Abs(got-s) > 2e-3 {
			t.Fatalf("pair %v stored %v (%v), want %.4f", k, got, ok, s)
		}
	}

	// Apply refuses without a version, unless allowed.
	aopts := bulk.ApplyOptions{EmbeddingType: et, ModelVersion: mv, Fallback: embedding.DefaultFaceBand, Queue: 10,
		QueueOut: filepath.Join(t.TempDir(), "queue.csv"), MaxCluster: 2, Commit: true}
	if _, err := bulk.Apply(ctx, sqlDB, aopts); err == nil {
		t.Fatal("apply ran with no threshold version")
	}

	// v1: review 0.5, confirm 0.9. Genuine renewals (0.917) and the fraud are POSITIVE; the trace
	// (0.86 to both of person 3's photos) and the lookalike (0.66 to both of person 0's)
	// INCONCLUSIVE.
	v1, err := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.5, Confirm: 0.9}, "test v1", "")
	if err != nil {
		t.Fatal(err)
	}
	dry := aopts
	dry.Commit = false
	if r, err := bulk.Apply(ctx, sqlDB, dry); err != nil || r.Written["POSITIVE"] != 11 || r.Written["INCONCLUSIVE"] != 4 {
		t.Fatalf("dry apply v1 = %+v, %v", r.Written, err)
	}
	if n := count(t, sqlDB, `SELECT COUNT(*) FROM biometric_decisions`); n != 0 {
		t.Fatalf("dry run wrote %d decisions", n)
	}
	r, err := bulk.Apply(ctx, sqlDB, aopts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Written["POSITIVE"] != 11 || r.Written["INCONCLUSIVE"] != 4 || r.Written["NEGATIVE"] != 0 {
		t.Fatalf("apply v1 wrote %v", r.Written)
	}
	if r.Clusters.Clusters == 0 || r.Largest != 2 || r.MultiID != 1 || r.Alarm {
		t.Fatalf("apply v1 clusters: %+v largest %d multi-identity %d", r.Clusters, r.Largest, r.MultiID)
	}
	// The queue: the fraud first (different fiscal numbers, same face), then the lookalike.
	if r.QueueSize != 3 || r.Queue[0].Score < 0.9 || !strings.Contains(r.Queue[0].NameA+r.Queue[0].NameB, "Person 11") {
		t.Fatalf("queue = %d %+v", r.QueueSize, r.Queue)
	}
	if again, err := bulk.Apply(ctx, sqlDB, aopts); err != nil || len(again.Written) != 0 {
		t.Fatalf("apply v1 again wrote %v, %v", again.Written, err)
	}

	// v2: confirm 0.6 promotes the trace and the lookalikes to POSITIVE, without matching again.
	v2, _ := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.5, Confirm: 0.6}, "test v2", "")
	if r, err = bulk.Apply(ctx, sqlDB, aopts); err != nil || r.Written["POSITIVE"] != 4 || len(r.Written) != 1 {
		t.Fatalf("apply v2 wrote %v, %v", r.Written, err)
	}
	// Person 0's photos with the lookalike, and person 3's with the trace, form clusters of 3:
	// above the alarm's 2.
	if r.Largest != 3 || !r.Alarm {
		t.Fatalf("apply v2 largest = %d alarm %v", r.Largest, r.Alarm)
	}
	// v3: review 0.95 drops every pair: NEGATIVE where there was a decision.
	v3, err := biometricmatch.SetBand(ctx, sqlDB, et, biometricmatch.Band{Review: 0.95, Confirm: 0.97}, "test v3", "")
	if err != nil {
		t.Fatal(err)
	}
	if r, err = bulk.Apply(ctx, sqlDB, aopts); err != nil || r.Written["NEGATIVE"] != 15 || len(r.Written) != 1 {
		t.Fatalf("apply v3 wrote %v, %v", r.Written, err)
	}
	if r.Largest != 1 || r.Alarm {
		t.Fatalf("apply v3 left clusters: %+v largest %d", r.Clusters, r.Largest)
	}
	// A trace pair's chain shows every version that changed it.
	var chain string
	sqlDB.QueryRowContext(ctx, `SELECT string_agg(d.decision || '@' || d.related_reference, ' ' ORDER BY d.decided_at, d.id)
		FROM biometric_decisions d
		JOIN identity_files f ON f.id::text = d.feature_a_id
		JOIN identity_registers r ON r.id = f.register_id AND r.register_number = 'R03-0'
		WHERE d.feature_b_id = $1`, fmt.Sprintf("TRACE:%d#feature", trace.TraceID)).Scan(&chain)
	if chain != fmt.Sprintf("INCONCLUSIVE@%d POSITIVE@%d NEGATIVE@%d", v1.ID, v2.ID, v3.ID) {
		t.Fatalf("trace chain = %q", chain)
	}

	// Calibrate: 10 genuine renewals; impostor comparisons are every pair of the 23 faces with
	// different fiscal numbers (all but the 10 renewals).
	c, err := bulk.Calibrate(ctx, sqlDB, bulk.CalibrateOptions{EmbeddingType: et, ModelVersion: mv, IdentityKey: bulk.DefaultIdentityKey})
	if err != nil {
		t.Fatal(err)
	}
	if c.Gallery != 23 || c.GenuinePairs != 10 || c.ImpostorComparisons != 23*22/2-10 {
		t.Fatalf("calibration counts: gallery %d genuine %d impostor %v", c.Gallery, c.GenuinePairs, c.ImpostorComparisons)
	}
	at := func(th float64) bulk.CalibrationRow {
		for _, r := range c.Rows {
			if math.Abs(r.Threshold-th) < 1e-9 {
				return r
			}
		}
		t.Fatalf("no row at %v", th)
		return bulk.CalibrationRow{}
	}
	// At 0.9 every genuine pair passes, and one impostor pair (the fraud) does.
	if r := at(0.9); r.FNMR != 0 || r.Different != 1 {
		t.Fatalf("at 0.9: %+v", r)
	}
	// At 0.95 every genuine pair fails.
	if r := at(0.95); r.FNMR != 1 || r.Different != 0 {
		t.Fatalf("at 0.95: %+v", r)
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func count(t *testing.T, sqlDB *sql.DB, query string) int64 {
	t.Helper()
	var n int64
	if err := sqlDB.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// embeddingIDs maps register numbers to their photo's feature_embeddings id.
func embeddingIDs(ctx context.Context, sqlDB *sql.DB) (map[string]int64, error) {
	rows, err := sqlDB.QueryContext(ctx, `SELECT r.register_number, fe.id FROM feature_embeddings fe
		JOIN biometric_features bf ON bf.id = fe.biometric_feature_id
		JOIN identity_files f ON f.id = bf.identity_file_id
		JOIN identity_registers r ON r.id = f.register_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var reg string
		var id int64
		if err := rows.Scan(&reg, &id); err != nil {
			return nil, err
		}
		out[reg] = id
	}
	return out, rows.Err()
}

// TestMain lets this test binary be the go-matcher subtest's matcher command: asked to, it does
// what tools/bulk-match does, by brute force and with no cap, instead of running the tests.
func TestMain(m *testing.M) {
	if os.Getenv("BULK_HELPER_MATCHER") == "1" {
		if err := helperMatch(os.Args[len(os.Args)-1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helperMatch(work string) error {
	var params struct {
		Shards string  `json:"shards"`
		Floor  float64 `json:"floor"`
	}
	raw, err := os.ReadFile(filepath.Join(work, "params.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return err
	}
	vectors := map[int64][]float32{}
	idFiles, _ := filepath.Glob(filepath.Join(params.Shards, "*.ids.npy"))
	for _, f := range idFiles {
		ids, err := bulk.ReadInt64s(f)
		if err != nil {
			return err
		}
		m, err := bulk.ReadMatrix(strings.TrimSuffix(f, ".ids.npy") + ".vectors.npy")
		if err != nil {
			return err
		}
		for i, id := range ids {
			vectors[id] = m[i]
		}
	}
	batch, err := bulk.ReadInt64s(filepath.Join(work, "batch_ids.npy"))
	if err != nil {
		return err
	}
	earlier, err := bulk.ReadInt64s(filepath.Join(work, "earlier_ids.npy"))
	if err != nil {
		return err
	}
	others, err := bulk.ReadInt64s(filepath.Join(work, "others_ids.npy"))
	if err != nil {
		return err
	}
	otherVectors, err := bulk.ReadMatrix(filepath.Join(work, "others_vectors.npy"))
	if err != nil && len(others) > 0 {
		return err
	}
	for i, id := range others {
		vectors[id] = otherVectors[i]
	}
	var a, b []int64
	var s []float32
	consider := func(x, y int64) {
		vx, okx := vectors[x]
		vy, oky := vectors[y]
		if !okx || !oky {
			return
		}
		if score := dot(vx, vy); score >= params.Floor {
			a, b, s = append(a, x), append(b, y), append(s, float32(score))
		}
	}
	for i, x := range batch {
		for _, y := range batch[i+1:] {
			consider(x, y)
		}
		for _, y := range append(append([]int64{}, earlier...), others...) {
			consider(x, y)
		}
	}
	for name, v := range map[string][]int64{"pairs_a.npy": a, "pairs_b.npy": b, "hubs_ids.npy": {}} {
		if err := bulk.WriteInt64s(filepath.Join(work, name), v); err != nil {
			return err
		}
	}
	if err := bulk.WriteFloat32s(filepath.Join(work, "pairs_score.npy"), s); err != nil {
		return err
	}
	if err := bulk.WriteFloat32s(filepath.Join(work, "hubs_kth.npy"), nil); err != nil {
		return err
	}
	n := int64(len(batch))
	result := map[string]any{"comparisons": n*(n-1)/2 + n*int64(len(earlier)) + n*int64(len(others)),
		"pairs": len(a), "hubs": 0, "seconds": 0, "device": "go-helper"}
	data, _ := json.Marshal(result)
	return os.WriteFile(filepath.Join(work, "result.json"), data, 0o644)
}
