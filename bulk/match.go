package bulk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lib/pq"
)

// MatchOptions configures Match.
type MatchOptions struct {
	EmbeddingType string
	ModelVersion  string
	// Size is how many BULK embeddings a new batch takes, chosen at random (Seed decides the
	// order, so the same seed picks the same faces).
	Size int
	Seed string
	// Floor and K are what the score store keeps per face: the neighbours scoring at least Floor,
	// at most K of them. A resumed batch keeps the values it was selected with.
	Floor float64
	K     int
	// ShardsDir holds the vector shards; WorkDir the files exchanged with the matcher (default
	// <ShardsDir>/work/batch-<id>).
	ShardsDir string
	WorkDir   string
	// Matcher is the command that runs the GPU matcher; Match appends "--work <dir>". See
	// tools/bulk-match.
	Matcher []string
	// GPU selects the matcher's device: "auto" (a GPU when FAISS has one), "cpu", or device ids
	// ("0,1").
	GPU    string
	Commit bool
}

// MatchReport is what Match did.
type MatchReport struct {
	Batch       int64
	Resumed     bool
	Size        int
	Earlier     int // faces of earlier batches compared against the batch
	Others      int // embeddings outside the gallery (traces) compared against the batch
	Filled      int // embeddings written to shards from Postgres first
	Comparisons int64
	Scores      int64
	Hubs        int
	Seconds     float64
	Matcher     string
	Pending     int64 // dry run: BULK embeddings not in any batch yet
}

// matcherResult is the matcher's result.json.
type matcherResult struct {
	Comparisons int64   `json:"comparisons"`
	Pairs       int64   `json:"pairs"`
	Hubs        int     `json:"hubs"`
	Seconds     float64 `json:"seconds"`
	Device      string  `json:"device"`
}

// Match takes a batch of BULK embeddings not matched yet (or resumes the SELECTED one), has the
// GPU matcher compare it exactly against itself, every earlier batch and every embedding outside
// the gallery, loads the scores it kept into the score store, and marks the batch's embeddings
// matched. Every pair of gallery faces is thus compared exactly once across all batches.
func Match(ctx context.Context, sqlDB *sql.DB, opts MatchOptions) (MatchReport, error) {
	var report MatchReport
	if opts.EmbeddingType == "" || opts.ModelVersion == "" {
		return report, fmt.Errorf("bulk: embedding type and model version are required")
	}

	batch, err := selectedBatch(ctx, sqlDB, opts)
	if err != nil {
		return report, err
	}
	if !opts.Commit {
		err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM feature_embeddings fe
			WHERE fe.embedding_type = $1 AND fe.model_version = $2 AND fe.match_path = 'BULK' AND fe.matched_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM bulk_match_batch_members m WHERE m.feature_embedding_id = fe.id)`,
			opts.EmbeddingType, opts.ModelVersion).Scan(&report.Pending)
		report.Batch, report.Resumed = batch.id, batch.id != 0
		report.Size = batch.size
		return report, err
	}
	if batch.id == 0 {
		if opts.Size <= 0 || opts.K <= 0 {
			return report, fmt.Errorf("bulk: a new batch needs a size and k")
		}
		if batch, err = newBatch(ctx, sqlDB, opts); err != nil {
			return report, err
		}
		if batch.size == 0 {
			return report, fmt.Errorf("bulk: no BULK %s %s embedding is waiting for a batch", opts.EmbeddingType, opts.ModelVersion)
		}
	} else {
		report.Resumed = true
	}
	report.Batch, report.Size = batch.id, batch.size

	members, err := int64Column(ctx, sqlDB, `SELECT feature_embedding_id FROM bulk_match_batch_members WHERE batch_id = $1 ORDER BY 1`, batch.id)
	if err != nil {
		return report, err
	}
	earlier, err := int64Column(ctx, sqlDB, `SELECT m.feature_embedding_id
		FROM bulk_match_batch_members m JOIN bulk_match_batches b ON b.id = m.batch_id
		WHERE b.status = 'MATCHED' AND b.embedding_type = $1 AND b.model_version = $2 ORDER BY 1`,
		opts.EmbeddingType, opts.ModelVersion)
	if err != nil {
		return report, err
	}
	report.Earlier = len(earlier)

	// Everything outside the gallery: the traces, and any KNOWN face the per-face sync owns. Few,
	// so read from Postgres every batch rather than sharded.
	otherIDs, otherVectors, err := loadVectors(ctx, sqlDB, `SELECT id, embedding::text FROM feature_embeddings
		WHERE embedding_type = $1 AND match_path = 'SYNC' ORDER BY id`, opts.EmbeddingType)
	if err != nil {
		return report, err
	}
	report.Others = len(otherIDs)

	if err := OpenShardDir(opts.ShardsDir, opts.EmbeddingType, opts.ModelVersion); err != nil {
		return report, err
	}
	if report.Filled, err = FillShards(ctx, sqlDB, opts.ShardsDir, append(append([]int64{}, members...), earlier...)); err != nil {
		return report, err
	}

	work := opts.WorkDir
	if work == "" {
		work = filepath.Join(opts.ShardsDir, "work", fmt.Sprintf("batch-%d", batch.id))
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return report, err
	}
	for _, f := range []string{"pairs_a.npy", "pairs_b.npy", "pairs_score.npy", "hubs_ids.npy", "hubs_kth.npy", "result.json"} {
		os.Remove(filepath.Join(work, f))
	}
	params := map[string]any{
		"shards": opts.ShardsDir, "floor": batch.floor, "k": batch.k, "batch": batch.id,
		"gpu": opts.GPU, "dim": Dim,
	}
	if err := writeJSON(filepath.Join(work, "params.json"), params); err != nil {
		return report, err
	}
	if err := WriteInt64s(filepath.Join(work, "batch_ids.npy"), members); err != nil {
		return report, err
	}
	if err := WriteInt64s(filepath.Join(work, "earlier_ids.npy"), earlier); err != nil {
		return report, err
	}
	if err := WriteInt64s(filepath.Join(work, "others_ids.npy"), otherIDs); err != nil {
		return report, err
	}
	if err := WriteMatrix(filepath.Join(work, "others_vectors.npy"), otherVectors, Dim, false); err != nil {
		return report, err
	}

	if len(opts.Matcher) == 0 {
		return report, fmt.Errorf("bulk: no matcher command")
	}
	cmd := exec.CommandContext(ctx, opts.Matcher[0], append(opts.Matcher[1:], "--work", work)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return report, fmt.Errorf("bulk: matcher %q: %w", strings.Join(opts.Matcher, " "), err)
	}
	var result matcherResult
	raw, err := os.ReadFile(filepath.Join(work, "result.json"))
	if err != nil {
		return report, fmt.Errorf("bulk: matcher wrote no result: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return report, err
	}

	if err := loadScores(ctx, sqlDB, batch.id, work); err != nil {
		return report, err
	}
	report.Comparisons, report.Hubs, report.Seconds, report.Matcher = result.Comparisons, result.Hubs, result.Seconds, result.Device
	err = sqlDB.QueryRowContext(ctx, `SELECT scores FROM bulk_match_batches WHERE id = $1`, batch.id).Scan(&report.Scores)
	if err != nil {
		return report, err
	}
	_, err = sqlDB.ExecContext(ctx, `UPDATE bulk_match_batches SET comparisons = $2, match_seconds = $3, matcher = $4 WHERE id = $1`,
		batch.id, result.Comparisons, result.Seconds, result.Device)
	return report, err
}

type batchRow struct {
	id    int64
	size  int
	floor float64
	k     int
}

// selectedBatch is the SELECTED batch of this embedding type and model version, if any.
func selectedBatch(ctx context.Context, sqlDB *sql.DB, opts MatchOptions) (batchRow, error) {
	var b batchRow
	err := sqlDB.QueryRowContext(ctx, `SELECT id, size, score_floor, neighbours FROM bulk_match_batches
		WHERE status = 'SELECTED' AND embedding_type = $1 AND model_version = $2 ORDER BY id LIMIT 1`,
		opts.EmbeddingType, opts.ModelVersion).Scan(&b.id, &b.size, &b.floor, &b.k)
	if errors.Is(err, sql.ErrNoRows) {
		return batchRow{}, nil
	}
	return b, err
}

func newBatch(ctx context.Context, sqlDB *sql.DB, opts MatchOptions) (batchRow, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return batchRow{}, err
	}
	defer tx.Rollback()
	b := batchRow{floor: opts.Floor, k: opts.K}
	selection := fmt.Sprintf("random, seed %q", opts.Seed)
	if err := tx.QueryRowContext(ctx, `INSERT INTO bulk_match_batches (embedding_type, model_version, size, selection, score_floor, neighbours)
		VALUES ($1, $2, 0, $3, $4, $5) RETURNING id`, opts.EmbeddingType, opts.ModelVersion, selection, opts.Floor, opts.K).Scan(&b.id); err != nil {
		return batchRow{}, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO bulk_match_batch_members (feature_embedding_id, batch_id)
		SELECT fe.id, $3 FROM feature_embeddings fe
		WHERE fe.embedding_type = $1 AND fe.model_version = $2 AND fe.match_path = 'BULK' AND fe.matched_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM bulk_match_batch_members m WHERE m.feature_embedding_id = fe.id)
		ORDER BY md5(fe.id::text || $4), fe.id
		LIMIT $5`, opts.EmbeddingType, opts.ModelVersion, b.id, opts.Seed, opts.Size)
	if err != nil {
		return batchRow{}, err
	}
	n, _ := res.RowsAffected()
	b.size = int(n)
	if _, err := tx.ExecContext(ctx, `UPDATE bulk_match_batches SET size = $2 WHERE id = $1`, b.id, b.size); err != nil {
		return batchRow{}, err
	}
	if b.size == 0 {
		return b, nil // rolled back
	}
	return b, tx.Commit()
}

// loadScores copies the matcher's pairs and hubs into staging, then in one transaction moves
// them into the score store, marks the batch's embeddings matched and the batch MATCHED. A rerun
// after a failure anywhere before the commit redoes the batch from scratch.
func loadScores(ctx context.Context, sqlDB *sql.DB, batchID int64, work string) error {
	a, err := ReadInt64s(filepath.Join(work, "pairs_a.npy"))
	if err != nil {
		return err
	}
	b, err := ReadInt64s(filepath.Join(work, "pairs_b.npy"))
	if err != nil {
		return err
	}
	score, err := ReadFloat32s(filepath.Join(work, "pairs_score.npy"))
	if err != nil {
		return err
	}
	if len(a) != len(b) || len(a) != len(score) {
		return fmt.Errorf("bulk: matcher pairs disagree in length (%d, %d, %d)", len(a), len(b), len(score))
	}
	hubIDs, err := ReadInt64s(filepath.Join(work, "hubs_ids.npy"))
	if err != nil {
		return err
	}
	hubKth, err := ReadFloat32s(filepath.Join(work, "hubs_kth.npy"))
	if err != nil {
		return err
	}
	if len(hubIDs) != len(hubKth) {
		return fmt.Errorf("bulk: matcher hubs disagree in length")
	}

	staging := fmt.Sprintf("bulk_match_staging_%d", batchID)
	if _, err := sqlDB.ExecContext(ctx, `DROP TABLE IF EXISTS `+staging); err != nil {
		return err
	}
	if _, err := sqlDB.ExecContext(ctx, `CREATE UNLOGGED TABLE `+staging+` (a BIGINT NOT NULL, b BIGINT NOT NULL, score REAL NOT NULL)`); err != nil {
		return err
	}
	defer sqlDB.ExecContext(context.WithoutCancel(ctx), `DROP TABLE IF EXISTS `+staging)

	i := 0
	if _, err := copyRows(ctx, sqlDB, staging, []string{"a", "b", "score"}, func() []any {
		if i == len(a) {
			return nil
		}
		x, y := a[i], b[i]
		if x > y {
			x, y = y, x
		}
		row := []any{x, y, score[i]}
		i++
		return row
	}); err != nil {
		return err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO bulk_match_scores (embedding_a_id, embedding_b_id, score, batch_id)
		SELECT a, b, MAX(score), $1 FROM `+staging+` WHERE a <> b GROUP BY a, b
		ON CONFLICT (embedding_a_id, embedding_b_id) DO NOTHING`, batchID)
	if err != nil {
		return fmt.Errorf("bulk: store scores: %w", err)
	}
	stored, _ := res.RowsAffected()
	if len(hubIDs) > 0 {
		kth := make([]float64, len(hubKth))
		for j, v := range hubKth {
			kth[j] = float64(v)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO bulk_match_hubs (batch_id, feature_embedding_id, kth_score)
			SELECT $1, t.id, MIN(t.kth) FROM unnest($2::bigint[], $3::float8[]) AS t(id, kth) GROUP BY t.id
			ON CONFLICT DO NOTHING`, batchID, pq.Array(hubIDs), pq.Array(kth)); err != nil {
			return fmt.Errorf("bulk: store hubs: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE feature_embeddings fe SET matched_at = NOW(), updated_at = NOW()
		FROM bulk_match_batch_members m WHERE m.batch_id = $1 AND fe.id = m.feature_embedding_id`, batchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bulk_match_batches SET status = 'MATCHED', matched_at = NOW(), scores = $2,
		hubs = (SELECT COUNT(*) FROM bulk_match_hubs WHERE batch_id = $1) WHERE id = $1`, batchID, stored); err != nil {
		return err
	}
	return tx.Commit()
}

func int64Column(ctx context.Context, sqlDB *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
