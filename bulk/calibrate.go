package bulk

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/rodfileto/trackid/cluster"
)

// scoreBins is the histogram resolution: scores are counted in bins of 1/scoreBins.
const scoreBins = 1000

// CalibrateOptions configures Calibrate.
type CalibrateOptions struct {
	EmbeddingType string
	ModelVersion  string
	IdentityKey   IdentityKey
	// MaxGroup drops identity keys shared by more faces than this (a placeholder fiscal number,
	// say) from both curves: such a key would label millions of pairs SAME.
	MaxGroup int
	// FinalSize is the gallery size the mean false degree is projected to (0: every embedding of
	// the type and model, matched or not).
	FinalSize int64
	// Step is the threshold grid's spacing.
	Step float64
}

// Calibration is Calibrate's report.
type Calibration struct {
	EmbeddingType, ModelVersion string
	IdentityKey                 IdentityKey
	Gallery                     int64 // faces in MATCHED batches
	FinalSize                   int64
	Floor                       float64 // the highest floor any batch kept: curves hold above it
	Hubs                        int64
	GenuinePairs                int64
	ImpostorComparisons         float64
	DroppedKeys                 int64
	Rows                        []CalibrationRow
	Reviewed                    []ReviewedBin
}

// CalibrationRow is the curves at one threshold t.
type CalibrationRow struct {
	Threshold float64
	FNMR      float64 // genuine pairs scoring below t, over all genuine pairs
	FMR       float64 // impostor pairs stored at or above t, over impostor comparisons made
	Pairs     int64   // stored pairs at or above t, traces included: the review load at t
	Different int64   // of those, pairs whose records say different people: fraud candidates and false matches
	// MeanFalseDegree is FMR x FinalSize: each face's expected number of false links at the
	// final gallery size. Union-find chains the AUTOMATIC layer into one cluster past 1.
	MeanFalseDegree float64
}

// ReviewedBin is what examiners concluded on stored pairs scoring in [Low, Low+width).
type ReviewedBin struct {
	Low       float64
	Confirmed int
	Rejected  int
	Open      int // pending, disputed or inconclusive
}

// Calibrate reports, for a model version and over every stored score, the genuine and impostor
// curves from identity records, examiners' decisions on stored pairs, and per threshold the
// review load and the projected mean false degree. It writes nothing.
//
// Genuine pairs are scored directly from the embeddings, so pairs below the floor count too. Each
// impostor pair was compared exactly once, so FMR(t) is the impostor pairs stored at or above t
// over the impostor comparisons made. That holds above the floor and while few faces are hubs
// (Hubs): a hub's list was cut at k.
func Calibrate(ctx context.Context, sqlDB *sql.DB, opts CalibrateOptions) (Calibration, error) {
	c := Calibration{EmbeddingType: opts.EmbeddingType, ModelVersion: opts.ModelVersion, IdentityKey: opts.IdentityKey}
	if len(opts.IdentityKey) == 0 {
		opts.IdentityKey = DefaultIdentityKey
		c.IdentityKey = opts.IdentityKey
	}
	if opts.MaxGroup <= 0 {
		opts.MaxGroup = 100
	}
	if opts.Step <= 0 {
		opts.Step = 0.01
	}
	args := []any{opts.EmbeddingType, opts.ModelVersion, opts.MaxGroup}
	with := `WITH ` + opts.IdentityKey.labelsCTE() + `,
		sizes AS (SELECT key, COUNT(*) AS n FROM labels WHERE key IS NOT NULL GROUP BY key),
		lab AS (SELECT l.id, l.document_id, CASE WHEN s.n > $3 THEN NULL ELSE l.key END AS key
		        FROM labels l LEFT JOIN sizes s ON s.key = l.key)`

	row := sqlDB.QueryRowContext(ctx, with+`
		SELECT (SELECT COUNT(*) FROM lab),
		       (SELECT COUNT(*) FROM sizes WHERE n > $3),
		       (SELECT COALESCE(MAX(score_floor), 0) FROM bulk_match_batches WHERE status = 'MATCHED' AND embedding_type = $1 AND model_version = $2),
		       (SELECT COUNT(DISTINCT h.feature_embedding_id) FROM bulk_match_hubs h JOIN bulk_match_batches bt ON bt.id = h.batch_id
		         WHERE bt.embedding_type = $1 AND bt.model_version = $2)`, args...)
	if err := row.Scan(&c.Gallery, &c.DroppedKeys, &c.Floor, &c.Hubs); err != nil {
		return c, fmt.Errorf("bulk: calibrate counts: %w", err)
	}
	c.FinalSize = opts.FinalSize
	if c.FinalSize <= 0 {
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM feature_embeddings fe JOIN biometric_features bf ON bf.id = fe.biometric_feature_id
			WHERE fe.embedding_type = $1 AND (fe.model_version = $2 OR fe.match_path = 'SYNC') AND bf.provenance = 'KNOWN'`,
			opts.EmbeddingType, opts.ModelVersion).Scan(&c.FinalSize); err != nil {
			return c, err
		}
	}

	// Impostor comparisons made: pairs of matched faces whose records say different people. Per
	// key field, all pairs with that field's keys, minus pairs sharing a key, minus pairs sharing
	// a document but not a key.
	if err := sqlDB.QueryRowContext(ctx, with+`,
		kinds AS (SELECT split_part(key, ':', 1) AS kind, document_id, key FROM lab WHERE key IS NOT NULL)
		SELECT
		  (SELECT COALESCE(SUM(n * (n - 1) / 2), 0) FROM (SELECT COUNT(*)::numeric AS n FROM kinds GROUP BY kind) t)
		- (SELECT COALESCE(SUM(n * (n - 1) / 2), 0) FROM (SELECT COUNT(*)::numeric AS n FROM kinds GROUP BY key) t)
		- (SELECT COALESCE(SUM(n * (n - 1) / 2), 0) FROM (SELECT COUNT(*)::numeric AS n FROM kinds GROUP BY document_id, kind) t)
		+ (SELECT COALESCE(SUM(n * (n - 1) / 2), 0) FROM (SELECT COUNT(*)::numeric AS n FROM kinds GROUP BY document_id, key) t)`,
		args...).Scan(&c.ImpostorComparisons); err != nil {
		return c, fmt.Errorf("bulk: calibrate impostor comparisons: %w", err)
	}

	genuine, err := histogram(ctx, sqlDB, with+`,
		pairs AS (
			SELECT x.id AS a, y.id AS b FROM lab x JOIN lab y ON y.document_id = x.document_id AND x.id < y.id
			UNION
			SELECT x.id, y.id FROM lab x JOIN lab y ON y.key = x.key AND x.id < y.id
		)
		SELECT LEAST(GREATEST(floor((1 - (ea.embedding <=> eb.embedding)) * `+fmt.Sprint(scoreBins)+`), 0), `+fmt.Sprint(scoreBins)+`)::int AS bin, COUNT(*)
		FROM pairs
		JOIN feature_embeddings ea ON ea.id = pairs.a
		JOIN feature_embeddings eb ON eb.id = pairs.b
		GROUP BY bin`, args...)
	if err != nil {
		return c, fmt.Errorf("bulk: calibrate genuine scores: %w", err)
	}
	impostor, err := histogram(ctx, sqlDB, with+`
		SELECT LEAST(GREATEST(floor(sc.score * `+fmt.Sprint(scoreBins)+`), 0), `+fmt.Sprint(scoreBins)+`)::int AS bin, COUNT(*)
		FROM bulk_match_scores sc
		JOIN lab x ON x.id = sc.embedding_a_id
		JOIN lab y ON y.id = sc.embedding_b_id
		WHERE `+relationSQL+` = 'DIFFERENT'
		GROUP BY bin`, args...)
	if err != nil {
		return c, fmt.Errorf("bulk: calibrate impostor scores: %w", err)
	}
	stored, err := histogram(ctx, sqlDB, `
		SELECT LEAST(GREATEST(floor(sc.score * `+fmt.Sprint(scoreBins)+`), 0), `+fmt.Sprint(scoreBins)+`)::int AS bin, COUNT(*)
		FROM bulk_match_scores sc
		JOIN bulk_match_batches bt ON bt.id = sc.batch_id AND bt.embedding_type = $1 AND bt.model_version = $2
		GROUP BY bin`, opts.EmbeddingType, opts.ModelVersion)
	if err != nil {
		return c, fmt.Errorf("bulk: calibrate stored scores: %w", err)
	}

	for _, n := range genuine {
		c.GenuinePairs += n
	}
	// Cumulative counts from the top: at or above bin i.
	above := func(h []int64) []int64 {
		out := make([]int64, len(h)+1)
		for i := len(h) - 1; i >= 0; i-- {
			out[i] = out[i+1] + h[i]
		}
		return out
	}
	genuineAbove, impostorAbove, storedAbove := above(genuine), above(impostor), above(stored)

	start := math.Ceil(c.Floor/opts.Step-1e-6) * opts.Step // the floor is stored as REAL
	for t := start; t <= 1+1e-9; t += opts.Step {
		bin := int(math.Round(t * scoreBins))
		bin = min(max(bin, 0), scoreBins)
		r := CalibrationRow{Threshold: math.Round(t*1e6) / 1e6, Pairs: storedAbove[bin], Different: impostorAbove[bin]}
		if c.GenuinePairs > 0 {
			r.FNMR = 1 - float64(genuineAbove[bin])/float64(c.GenuinePairs)
		}
		if c.ImpostorComparisons > 0 {
			r.FMR = float64(impostorAbove[bin]) / c.ImpostorComparisons
		}
		r.MeanFalseDegree = r.FMR * float64(c.FinalSize)
		c.Rows = append(c.Rows, r)
	}

	if c.Reviewed, err = reviewedBins(ctx, sqlDB, opts.EmbeddingType, opts.ModelVersion); err != nil {
		return c, err
	}
	return c, nil
}

// histogram runs a query returning (bin, count) rows into a slice of scoreBins+1 counts.
func histogram(ctx context.Context, sqlDB *sql.DB, query string, args ...any) ([]int64, error) {
	rows, err := sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, scoreBins+1)
	for rows.Next() {
		var bin int
		var n int64
		if err := rows.Scan(&bin, &n); err != nil {
			return nil, err
		}
		out[min(max(bin, 0), scoreBins)] += n
	}
	return out, rows.Err()
}

// reviewWidth is the width of the examiner-decision bins.
const reviewWidth = 0.05

// reviewedBins reads every stored pair people decided on, derives its status from its full
// decision chain (cluster.DeriveEdgeStatus), and counts the outcomes by score.
func reviewedBins(ctx context.Context, sqlDB *sql.DB, embeddingType, modelVersion string) ([]ReviewedBin, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		WITH hp AS (
			SELECT DISTINCT feature_a_id AS fa, feature_b_id AS fb FROM biometric_decisions
			WHERE role <> 'SYSTEM' AND modality = 'FACIAL'
		),
		ends AS (
			SELECT hp.fa, hp.fb, ea.id AS ea, eb.id AS eb
			FROM hp
			LEFT JOIN biometric_features ka ON ka.identity_file_id = CASE WHEN hp.fa !~ '^TRACE:' THEN hp.fa::bigint END
			LEFT JOIN biometric_features qa ON qa.case_trace_id = substring(hp.fa from '^TRACE:(\d+)#feature$')::bigint
			LEFT JOIN biometric_features kb ON kb.identity_file_id = CASE WHEN hp.fb !~ '^TRACE:' THEN hp.fb::bigint END
			LEFT JOIN biometric_features qb ON qb.case_trace_id = substring(hp.fb from '^TRACE:(\d+)#feature$')::bigint
			JOIN feature_embeddings ea ON ea.biometric_feature_id = COALESCE(ka.id, qa.id) AND ea.embedding_type = $1
			JOIN feature_embeddings eb ON eb.biometric_feature_id = COALESCE(kb.id, qb.id) AND eb.embedding_type = $1
		),
		sp AS (
			SELECT ends.fa, ends.fb, sc.score FROM ends
			JOIN bulk_match_scores sc ON sc.embedding_a_id = LEAST(ends.ea, ends.eb) AND sc.embedding_b_id = GREATEST(ends.ea, ends.eb)
			JOIN bulk_match_batches bt ON bt.id = sc.batch_id AND bt.model_version = $2
		)
		SELECT sp.score, d.feature_a_id, d.feature_b_id, d.role, d.decision, COALESCE(d.case_analysis_id, 0),
		       COALESCE(ca.analysis_type, ''), COALESCE(d.conclusion, '')
		FROM sp
		JOIN biometric_decisions d ON d.feature_a_id = sp.fa AND d.feature_b_id = sp.fb
		LEFT JOIN case_analyses ca ON ca.id = d.case_analysis_id
		ORDER BY d.feature_a_id, d.feature_b_id, d.decided_at, d.id`, embeddingType, modelVersion)
	if err != nil {
		return nil, fmt.Errorf("bulk: calibrate reviewed pairs: %w", err)
	}
	defer rows.Close()
	type pair struct{ a, b string }
	scores := map[pair]float64{}
	chains := map[pair][]cluster.ChainEntry{}
	for rows.Next() {
		var p pair
		var score float64
		var e cluster.ChainEntry
		var role, decision, conclusion string
		if err := rows.Scan(&score, &p.a, &p.b, &role, &decision, &e.Analysis, &e.AnalysisType, &conclusion); err != nil {
			return nil, err
		}
		e.Role, e.Decision, e.Conclusion = cluster.Role(role), cluster.Decision(decision), cluster.Conclusion(conclusion)
		scores[p] = score
		chains[p] = append(chains[p], e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	bins := map[int]*ReviewedBin{}
	for p, chain := range chains {
		i := int(math.Floor(scores[p] / reviewWidth))
		b := bins[i]
		if b == nil {
			b = &ReviewedBin{Low: math.Round(float64(i)*reviewWidth*100) / 100}
			bins[i] = b
		}
		status, _ := cluster.DeriveEdgeStatus(chain)
		switch {
		case status == cluster.EdgeStatusConfirmed && cluster.Reviewed(chain):
			b.Confirmed++
		case status == cluster.EdgeStatusRejected && !systemOnly(chain):
			b.Rejected++
		default:
			b.Open++
		}
	}
	out := make([]ReviewedBin, 0, len(bins))
	for _, b := range bins {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Low < out[j].Low })
	return out, nil
}

// systemOnly reports whether the deciding round was settled by the SYSTEM decision alone.
func systemOnly(chain []cluster.ChainEntry) bool {
	_, _, by, ok := cluster.DecidingRoundBy(chain)
	return ok && by == cluster.RoleSystem
}

// Print writes the report as text.
func (c Calibration) Print(w io.Writer) {
	fmt.Fprintf(w, "calibration of %s %s, identity by %v\n", c.EmbeddingType, c.ModelVersion, c.IdentityKey)
	fmt.Fprintf(w, "  gallery matched: %d faces (projected to %d)\n", c.Gallery, c.FinalSize)
	fmt.Fprintf(w, "  genuine pairs: %d   impostor comparisons: %.4g   floor: %.3f   hubs: %d", c.GenuinePairs, c.ImpostorComparisons, c.Floor, c.Hubs)
	if c.DroppedKeys > 0 {
		fmt.Fprintf(w, "   identity keys dropped as too common: %d", c.DroppedKeys)
	}
	fmt.Fprintln(w)
	if c.Hubs > 0 && c.Gallery > 0 && float64(c.Hubs) > 0.01*float64(c.Gallery) {
		fmt.Fprintf(w, "  warning: %.1f%% of faces hit the cap k; FMR near the floor is underestimated\n", 100*float64(c.Hubs)/float64(c.Gallery))
	}
	fmt.Fprintf(w, "\n  %9s %10s %12s %12s %12s %14s\n", "threshold", "FNMR", "FMR", "pairs>=t", "different>=t", "false degree")
	for _, r := range c.Rows {
		fmt.Fprintf(w, "  %9.3f %10.4f %12.3g %12d %12d %14.3g\n", r.Threshold, r.FNMR, r.FMR, r.Pairs, r.Different, r.MeanFalseDegree)
	}
	if len(c.Reviewed) > 0 {
		fmt.Fprintf(w, "\n  examiner decisions on stored pairs\n  %11s %10s %10s %6s %10s\n", "score", "confirmed", "rejected", "open", "precision")
		for _, b := range c.Reviewed {
			precision := "-"
			if b.Confirmed+b.Rejected > 0 {
				precision = fmt.Sprintf("%.3f", float64(b.Confirmed)/float64(b.Confirmed+b.Rejected))
			}
			fmt.Fprintf(w, "  %.2f-%.2f %10d %10d %6d %10s\n", b.Low, b.Low+reviewWidth, b.Confirmed, b.Rejected, b.Open, precision)
		}
	}
	fmt.Fprintln(w, "\n  A band: Review where the examiners' capacity meets pairs>=t; Confirm where the false degree is well below 1 (0.1).")
}
