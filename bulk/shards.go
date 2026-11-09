package bulk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// Vector shards are a copy of BULK embeddings on local disk, FP16, so bulk-match reads files
// instead of pulling every vector out of Postgres at every batch. Postgres stays the source of
// truth: a shard lost, or rows committed by a run that died before flushing its shard, are filled
// back from Postgres before matching (FillShards).
//
// A shard directory holds meta.json (the embedding type and model version every shard in it
// belongs to) and pairs of files <name>.vectors.npy (float16, n x Dim) and <name>.ids.npy (int64
// feature_embeddings ids, the same n, in the same order). The ids file is written last, so a
// shard counts only once both are complete.

// Dim is the embedding dimension shards hold.
const Dim = 512

// ShardMeta is meta.json.
type ShardMeta struct {
	EmbeddingType string `json:"embeddingType"`
	ModelVersion  string `json:"modelVersion"`
	Dim           int    `json:"dim"`
	DType         string `json:"dtype"`
}

const (
	shardIDsSuffix     = ".ids.npy"
	shardVectorsSuffix = ".vectors.npy"
)

// OpenShardDir creates dir if needed and checks (or writes) its meta.json: shards of different
// models are never mixed, since their scores are not comparable.
func OpenShardDir(dir, embeddingType, modelVersion string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	want := ShardMeta{EmbeddingType: embeddingType, ModelVersion: modelVersion, Dim: Dim, DType: "float16"}
	path := filepath.Join(dir, "meta.json")
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data, _ := json.MarshalIndent(want, "", "  ")
		return os.WriteFile(path, append(data, '\n'), 0o644)
	}
	if err != nil {
		return err
	}
	var got ShardMeta
	if err := json.Unmarshal(raw, &got); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if got != want {
		return fmt.Errorf("%s holds %s %s shards, not %s %s: use another directory", dir, got.EmbeddingType, got.ModelVersion, embeddingType, modelVersion)
	}
	return nil
}

// ShardWriter buffers embeddings and writes a shard every Rows rows (and on Flush).
type ShardWriter struct {
	dir  string
	rows int
	run  string

	mu      sync.Mutex
	seq     int
	ids     []int64
	vectors [][]float32
	written int
}

// NewShardWriter writes shards into dir (opened with OpenShardDir) of rows rows each.
func NewShardWriter(dir string, rows int) *ShardWriter {
	if rows <= 0 {
		rows = 1 << 20
	}
	return &ShardWriter{dir: dir, rows: rows, run: strconv.FormatInt(time.Now().UnixNano(), 36)}
}

// Add buffers one embedding, writing a shard when the buffer is full.
func (w *ShardWriter) Add(id int64, vector []float32) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ids = append(w.ids, id)
	w.vectors = append(w.vectors, vector)
	if len(w.ids) >= w.rows {
		return w.flushLocked()
	}
	return nil
}

// Flush writes whatever is buffered.
func (w *ShardWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

// Written is the number of rows written to shards so far.
func (w *ShardWriter) Written() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

func (w *ShardWriter) flushLocked() error {
	if len(w.ids) == 0 {
		return nil
	}
	w.seq++
	name := filepath.Join(w.dir, fmt.Sprintf("shard-%s-%05d", w.run, w.seq))
	if err := WriteMatrix(name+shardVectorsSuffix, w.vectors, Dim, true); err != nil {
		return err
	}
	if err := WriteInt64s(name+shardIDsSuffix, w.ids); err != nil {
		return err
	}
	w.written += len(w.ids)
	w.ids, w.vectors = nil, nil
	return nil
}

// ShardIDs lists every complete shard in dir and returns the set of ids they hold.
func ShardIDs(dir string) (map[int64]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, shardIDsSuffix) || strings.HasPrefix(name, ".") {
			continue
		}
		base := strings.TrimSuffix(name, shardIDsSuffix)
		if _, err := os.Stat(filepath.Join(dir, base+shardVectorsSuffix)); err != nil {
			continue
		}
		ids, err := ReadInt64s(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			out[id] = true
		}
	}
	return out, nil
}

// FillShards writes, from Postgres, a shard of every id in want that no shard in dir holds yet,
// and returns how many it wrote.
func FillShards(ctx context.Context, sqlDB *sql.DB, dir string, want []int64) (int, error) {
	have, err := ShardIDs(dir)
	if err != nil {
		return 0, err
	}
	var missing []int64
	for _, id := range want {
		if !have[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return 0, nil
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	w := NewShardWriter(dir, 1<<20)
	const page = 5000
	for start := 0; start < len(missing); start += page {
		end := min(start+page, len(missing))
		ids, vectors, err := loadVectors(ctx, sqlDB, `SELECT id, embedding::text FROM feature_embeddings WHERE id = ANY($1) ORDER BY id`,
			pq.Array(missing[start:end]))
		if err != nil {
			return 0, err
		}
		if len(ids) != end-start {
			return 0, fmt.Errorf("bulk: %d of %d embeddings to shard are gone from feature_embeddings", end-start-len(ids), end-start)
		}
		for i := range ids {
			if err := w.Add(ids[i], vectors[i]); err != nil {
				return 0, err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return 0, err
	}
	return w.Written(), nil
}

// loadVectors runs query, which selects (id, embedding::text), and parses the vectors.
func loadVectors(ctx context.Context, sqlDB *sql.DB, query string, args ...any) ([]int64, [][]float32, error) {
	rows, err := sqlDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	var vectors [][]float32
	for rows.Next() {
		var id int64
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, nil, err
		}
		v, err := ParseVector(text)
		if err != nil {
			return nil, nil, fmt.Errorf("bulk: feature_embeddings %d: %w", id, err)
		}
		ids = append(ids, id)
		vectors = append(vectors, v)
	}
	return ids, vectors, rows.Err()
}

// ParseVector reads pgvector's text output ("[v1,v2,...]").
func ParseVector(text string) ([]float32, error) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return nil, fmt.Errorf("not a vector: %.20q", text)
	}
	parts := strings.Split(text[1:len(text)-1], ",")
	out := make([]float32, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, err
		}
		out[i] = float32(f)
	}
	return out, nil
}
