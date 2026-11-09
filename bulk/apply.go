package bulk

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/rodfileto/trackid/biometricmatch"
	"github.com/rodfileto/trackid/cluster"
)

// ApplyOptions configures Apply.
type ApplyOptions struct {
	EmbeddingType string
	ModelVersion  string
	// Fallback is the band used when no threshold version is set, and only with AllowDefault: a
	// gallery's decisions should cite a version someone set (cmd/match-threshold).
	Fallback     biometricmatch.Band
	AllowDefault bool
	IdentityKey  IdentityKey
	// Chunk is how many embedding ids (by embedding_a_id) each write transaction covers.
	Chunk int64
	// Queue is how many review-queue pairs to list; QueueOut, when set, gets them all as CSV.
	Queue    int
	QueueOut string
	// MaxCluster raises Alarm when the largest AUTOMATIC cluster is bigger: the sign of chaining.
	MaxCluster int
	Commit     bool
}

// ApplyReport is what Apply did.
type ApplyReport struct {
	Version   biometricmatch.Version
	Written   map[string]int64 // SYSTEM decisions written (or that would be), by decision
	Clusters  cluster.Stats
	Sizes     []SizeBucket
	Largest   int64
	MultiID   int64 // AUTOMATIC clusters holding faces of more than one identity
	Queue     []QueuePair
	QueueSize int64
	Alarm     bool
}

// SizeBucket counts AUTOMATIC face clusters by size.
type SizeBucket struct {
	Label    string
	Clusters int64
}

// QueuePair is a review-queue entry: a stored pair at or above Review whose records say
// different people, ranked by score. Those are the fraud candidates, and what calibrates the next
// version.
type QueuePair struct {
	Score              float64
	Decision           string // the pair's latest SYSTEM decision
	FeatureA, FeatureB string
	PersonA, PersonB   string
	NameA, NameB       string
	DocumentA          string
	DocumentB          string
}

// Apply bands every stored score of a model version under the current threshold version and
// writes the SYSTEM decisions that changed: POSITIVE at or above Confirm, INCONCLUSIVE between
// Review and Confirm, and NEGATIVE for a pair that had a SYSTEM POSITIVE or INCONCLUSIVE and now
// scores below Review. A pair whose decision is unchanged keeps the row of the version that first
// gave it. Each decision cites the version (one SYSTEM row per pair and version), so the chain
// shows every version that changed the pair, and human decisions are untouched. Apply then runs
// clustering once and reports the clusters and the review queue.
//
// Writes go in chunks of embedding ids, each its own transaction; a rerun writes only what the
// last one did not.
func Apply(ctx context.Context, sqlDB *sql.DB, opts ApplyOptions) (ApplyReport, error) {
	report := ApplyReport{Written: map[string]int64{}}
	if len(opts.IdentityKey) == 0 {
		opts.IdentityKey = DefaultIdentityKey
	}
	if opts.Chunk <= 0 {
		opts.Chunk = 200_000
	}
	v, err := biometricmatch.CurrentBand(ctx, sqlDB, opts.EmbeddingType, opts.Fallback)
	if err != nil {
		return report, err
	}
	if v.ID == 0 && !opts.AllowDefault {
		return report, fmt.Errorf("bulk: no threshold version is set for %s; set one with cmd/match-threshold (or allow the default)", opts.EmbeddingType)
	}
	if err := v.Band.ValidateSimilarity(); err != nil {
		return report, err
	}
	report.Version = v
	var ref sql.NullString
	if v.ID != 0 {
		ref = sql.NullString{String: strconv.FormatInt(v.ID, 10), Valid: true}
	}

	var lo, hi sql.NullInt64
	if err := sqlDB.QueryRowContext(ctx, `SELECT MIN(embedding_a_id), MAX(embedding_a_id) FROM bulk_match_scores sc
		JOIN bulk_match_batches bt ON bt.id = sc.batch_id AND bt.embedding_type = $1 AND bt.model_version = $2`,
		opts.EmbeddingType, opts.ModelVersion).Scan(&lo, &hi); err != nil {
		return report, err
	}
	if lo.Valid {
		tail := `SELECT decision, COUNT(*) FROM changed GROUP BY decision`
		if opts.Commit {
			tail = `, written AS (
				INSERT INTO biometric_decisions (feature_a_id, feature_b_id, modality, role, decision, system_source,
				                                 confidence, threshold, related_reference, related_reference_kind)
				SELECT fa, fb, 'FACIAL', 'SYSTEM', decision, $1, score, threshold, $7::text,
				       CASE WHEN $7::text IS NULL THEN NULL ELSE 'match_threshold' END
				FROM changed
				ON CONFLICT DO NOTHING
				RETURNING decision
			)
			SELECT decision, COUNT(*) FROM written GROUP BY decision`
		}
		for start := lo.Int64; start <= hi.Int64; start += opts.Chunk {
			args := []any{opts.EmbeddingType, opts.ModelVersion, start, start + opts.Chunk, v.Band.Review, v.Band.Confirm}
			if opts.Commit {
				args = append(args, ref)
			}
			rows, err := sqlDB.QueryContext(ctx, bandSQL+tail, args...)
			if err != nil {
				return report, fmt.Errorf("bulk: band scores: %w", err)
			}
			for rows.Next() {
				var d string
				var n int64
				if err := rows.Scan(&d, &n); err != nil {
					rows.Close()
					return report, err
				}
				report.Written[d] += n
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return report, err
			}
		}
	}

	if opts.Commit {
		if report.Clusters, err = cluster.Run(ctx, sqlDB); err != nil {
			return report, err
		}
	} else if report.Clusters, err = cluster.Plan(ctx, sqlDB); err != nil {
		return report, err
	}
	if err := clusterReport(ctx, sqlDB, opts.IdentityKey, &report); err != nil {
		return report, err
	}
	if opts.MaxCluster > 0 && report.Largest > int64(opts.MaxCluster) {
		report.Alarm = true
	}
	if err := reviewQueue(ctx, sqlDB, opts, v.Band.Review, &report); err != nil {
		return report, err
	}
	return report, nil
}

// bandSQL selects, for the stored scores of one chunk ($3 <= embedding_a_id < $4) of embedding
// type $1 and model version $2, the SYSTEM decisions that change under the band ($5 review, $6
// confirm) as the CTE changed (fa, fb, decision, score, threshold).
var bandSQL = `
WITH s AS (
	SELECT LEAST(` + graphIDSQL("ba") + ` COLLATE "C", ` + graphIDSQL("bb") + ` COLLATE "C") AS fa,
	       GREATEST(` + graphIDSQL("ba") + ` COLLATE "C", ` + graphIDSQL("bb") + ` COLLATE "C") AS fb,
	       sc.score::float8 AS score
	FROM bulk_match_scores sc
	JOIN bulk_match_batches bt ON bt.id = sc.batch_id AND bt.embedding_type = $1 AND bt.model_version = $2
	JOIN feature_embeddings ea ON ea.id = sc.embedding_a_id
	JOIN biometric_features ba ON ba.id = ea.biometric_feature_id
	JOIN feature_embeddings eb ON eb.id = sc.embedding_b_id
	JOIN biometric_features bb ON bb.id = eb.biometric_feature_id
	WHERE sc.embedding_a_id >= $3 AND sc.embedding_a_id < $4
),
latest AS (
	SELECT DISTINCT ON (d.feature_a_id, d.feature_b_id) d.feature_a_id, d.feature_b_id, d.decision
	FROM s JOIN biometric_decisions d ON d.feature_a_id = s.fa AND d.feature_b_id = s.fb AND d.role = 'SYSTEM'
	ORDER BY d.feature_a_id, d.feature_b_id, d.decided_at DESC, d.id DESC
),
banded AS (
	SELECT s.fa, s.fb, s.score,
	       CASE WHEN s.score >= $6 THEN 'POSITIVE' WHEN s.score >= $5 THEN 'INCONCLUSIVE' ELSE 'NEGATIVE' END AS decision,
	       CASE WHEN s.score >= $6 THEN $6::float8 ELSE $5::float8 END AS threshold,
	       l.decision AS previous
	FROM s LEFT JOIN latest l ON l.feature_a_id = s.fa AND l.feature_b_id = s.fb
),
changed AS (
	SELECT fa, fb, decision, score, threshold FROM banded
	WHERE (decision <> 'NEGATIVE' AND previous IS DISTINCT FROM decision)
	   OR (decision = 'NEGATIVE' AND previous IN ('POSITIVE', 'INCONCLUSIVE'))
)
`

// clusterReport fills the AUTOMATIC face clusters' size distribution, the largest, and how many
// hold more than one identity (by identity key, or else document).
func clusterReport(ctx context.Context, sqlDB *sql.DB, key IdentityKey, report *ApplyReport) error {
	rows, err := sqlDB.QueryContext(ctx, `
		WITH sizes AS (
			SELECT c.id, COUNT(*) AS n FROM clusters c JOIN cluster_members m ON m.cluster_id = c.id AND m.basis = c.basis
			WHERE c.basis = 'AUTOMATIC' AND c.modality = 'FACIAL' GROUP BY c.id
		)
		SELECT CASE WHEN n = 1 THEN '1' WHEN n = 2 THEN '2' WHEN n = 3 THEN '3' WHEN n < 10 THEN '4-9'
		            WHEN n < 100 THEN '10-99' ELSE '100+' END AS bucket, COUNT(*), MAX(n)
		FROM sizes GROUP BY bucket ORDER BY MIN(n)`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b SizeBucket
		var largest int64
		if err := rows.Scan(&b.Label, &b.Clusters, &largest); err != nil {
			rows.Close()
			return err
		}
		report.Sizes = append(report.Sizes, b)
		report.Largest = max(report.Largest, largest)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	return sqlDB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT m.cluster_id
			FROM cluster_members m
			JOIN clusters c ON c.id = m.cluster_id AND c.basis = m.basis AND c.modality = 'FACIAL'
			JOIN identity_files f ON f.id = CASE WHEN m.feature_id ~ '^[0-9]+$' THEN m.feature_id::bigint END
			JOIN identity_registers r ON r.id = f.register_id
			JOIN identity_documents d ON d.id = r.document_id
			LEFT JOIN persons p ON p.id = d.person_id
			WHERE m.basis = 'AUTOMATIC'
			GROUP BY m.cluster_id
			HAVING COUNT(DISTINCT COALESCE(`+key.keyExpr()+`, 'document:' || d.id)) > 1
		) t`).Scan(&report.MultiID)
}

// reviewQueue lists the stored pairs at or above review whose records say different people and
// that no person has decided yet (a pair already in an examination is left out), ranked by score.
func reviewQueue(ctx context.Context, sqlDB *sql.DB, opts ApplyOptions, review float64, report *ApplyReport) error {
	query := `WITH ` + opts.IdentityKey.labelsCTE() + `,
		q AS (
			SELECT sc.score::float8 AS score,
			       LEAST(fa.id::text COLLATE "C", fb.id::text COLLATE "C") AS ga,
			       GREATEST(fa.id::text COLLATE "C", fb.id::text COLLATE "C") AS gb
			FROM bulk_match_scores sc
			JOIN labels x ON x.id = sc.embedding_a_id
			JOIN labels y ON y.id = sc.embedding_b_id
			JOIN feature_embeddings ea ON ea.id = x.id JOIN biometric_features ba ON ba.id = ea.biometric_feature_id
			JOIN identity_files fa ON fa.id = ba.identity_file_id
			JOIN feature_embeddings eb ON eb.id = y.id JOIN biometric_features bb ON bb.id = eb.biometric_feature_id
			JOIN identity_files fb ON fb.id = bb.identity_file_id
			WHERE sc.score >= $3 AND ` + relationSQL + ` = 'DIFFERENT'
		)
		SELECT q.score,
		       COALESCE((SELECT d.decision FROM biometric_decisions d WHERE d.feature_a_id = q.ga AND d.feature_b_id = q.gb
		                 AND d.role = 'SYSTEM' ORDER BY d.decided_at DESC, d.id DESC LIMIT 1), ''),
		       q.ga, q.gb, pa.person_id, pb.person_id, ra.name, rb.name,
		       da.document_type || ' ' || da.document_number, db.document_type || ' ' || db.document_number
		FROM q
		JOIN identity_files fa ON fa.id = q.ga::bigint JOIN identity_registers ra ON ra.id = fa.register_id
		JOIN identity_documents da ON da.id = ra.document_id LEFT JOIN persons pa ON pa.id = da.person_id
		JOIN identity_files fb ON fb.id = q.gb::bigint JOIN identity_registers rb ON rb.id = fb.register_id
		JOIN identity_documents db ON db.id = rb.document_id LEFT JOIN persons pb ON pb.id = db.person_id
		WHERE NOT EXISTS (SELECT 1 FROM biometric_decisions h WHERE h.feature_a_id = q.ga AND h.feature_b_id = q.gb AND h.role <> 'SYSTEM')
		ORDER BY q.score DESC, q.ga, q.gb`
	rows, err := sqlDB.QueryContext(ctx, query, opts.EmbeddingType, opts.ModelVersion, review)
	if err != nil {
		return fmt.Errorf("bulk: review queue: %w", err)
	}
	defer rows.Close()

	var w *csv.Writer
	if opts.QueueOut != "" {
		f, err := os.Create(opts.QueueOut)
		if err != nil {
			return err
		}
		defer f.Close()
		w = csv.NewWriter(f)
		defer w.Flush()
		w.Write([]string{"score", "decision", "feature_a", "feature_b", "person_a", "person_b", "name_a", "name_b", "document_a", "document_b"})
	}
	for rows.Next() {
		var q QueuePair
		var pa, pb sql.NullString
		if err := rows.Scan(&q.Score, &q.Decision, &q.FeatureA, &q.FeatureB, &pa, &pb, &q.NameA, &q.NameB, &q.DocumentA, &q.DocumentB); err != nil {
			return err
		}
		q.PersonA, q.PersonB = pa.String, pb.String
		report.QueueSize++
		if len(report.Queue) < opts.Queue {
			report.Queue = append(report.Queue, q)
		}
		if w != nil {
			w.Write([]string{strconv.FormatFloat(q.Score, 'f', 4, 64), q.Decision, q.FeatureA, q.FeatureB, q.PersonA, q.PersonB,
				q.NameA, q.NameB, q.DocumentA, q.DocumentB})
		}
	}
	return rows.Err()
}

// Print writes the report as text.
func (r ApplyReport) Print(w io.Writer, commit bool) {
	verb := "would write"
	if commit {
		verb = "wrote"
	}
	fmt.Fprintf(w, "threshold version %d (%s): review %.4f, confirm %.4f\n", r.Version.ID, r.Version.Source, r.Version.Band.Review, r.Version.Band.Confirm)
	fmt.Fprintf(w, "  SYSTEM decisions %s: %d POSITIVE, %d INCONCLUSIVE, %d NEGATIVE\n", verb,
		r.Written["POSITIVE"], r.Written["INCONCLUSIVE"], r.Written["NEGATIVE"])
	fmt.Fprintf(w, "  clusters: %d automatic (%d created, %d merged), %d reviewed\n",
		r.Clusters.Clusters, r.Clusters.Created, r.Clusters.Merged, r.Clusters.Reviewed.Clusters)
	if !commit {
		fmt.Fprintln(w, "  (dry run: the cluster counts and the figures below are the current state, before these decisions)")
	}
	fmt.Fprint(w, "  automatic face clusters by size:")
	for _, b := range r.Sizes {
		fmt.Fprintf(w, "  %s: %d", b.Label, b.Clusters)
	}
	fmt.Fprintf(w, "\n  largest: %d   holding more than one identity: %d\n", r.Largest, r.MultiID)
	if r.Alarm {
		fmt.Fprintf(w, "  ALARM: the largest cluster is above the limit; links may be chaining (raise Confirm, or review its hubs)\n")
	}
	fmt.Fprintf(w, "  review queue: %d pairs at or above review whose records say different people\n", r.QueueSize)
	for _, q := range r.Queue {
		fmt.Fprintf(w, "    %.4f %-12s %s (%s, %s) ~ %s (%s, %s)\n", q.Score, q.Decision, q.FeatureA, q.NameA, q.DocumentA, q.FeatureB, q.NameB, q.DocumentB)
	}
}
