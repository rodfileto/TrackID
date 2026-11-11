// Package storagecheck checks the files TrackID's database refers to against object storage:
// each must be there and, where the database recorded its SHA-256 (case_files.hash_id), have it.
// It is the storage half of the chain of custody: run after restoring a backup, to prove the copy
// whole, and from time to time on the live deployment (cmd/verify-storage).
package storagecheck

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/rodfileto/trackid/storage"
)

// Digester reads a stored file's SHA-256 and size (storage.Client.Digest).
type Digester interface {
	Digest(ctx context.Context, storageRef string) (sha256Hex string, size int64, err error)
}

// The kinds of problem.
const (
	Missing      = "MISSING"       // the reference names nothing
	HashMismatch = "HASH_MISMATCH" // the file's SHA-256 isn't the one recorded
	SizeMismatch = "SIZE_MISMATCH" // the file's size isn't the one recorded
	ReadError    = "READ_ERROR"    // the file couldn't be read, for another reason
)

// Problem is one file that failed. It names the row, not the file's content.
type Problem struct {
	Table  string
	ID     int64
	Kind   string
	Detail string
}

// Report counts a run.
type Report struct {
	Checked  int // references read
	Verified int // of them, matched against a recorded SHA-256
	Skipped  int // references that aren't to storage (placeholders such as sim://), not read
	Problems []Problem
}

// OK reports whether every file read was there and as recorded.
func (r Report) OK() bool { return len(r.Problems) == 0 }

// Options tune a run.
type Options struct {
	// Workers is how many files are read at once; 4 when zero.
	Workers int
	// Limit stops after this many references, for a quick sample; 0 checks all.
	Limit int
}

type reference struct {
	table  string
	id     int64
	ref    string
	sha256 string // empty when the table records none
	size   int64  // 0 when unknown
}

// The tables that refer to stored files. case_files and scene_note_audios record a SHA-256; the
// others are checked for presence and, when recorded, size.
const references = `
SELECT 'case_files', id, storage_ref, COALESCE(hash_id, ''), COALESCE(size_bytes, 0)
  FROM case_files WHERE COALESCE(storage_ref, '') <> ''
UNION ALL
SELECT 'identity_files', id, storage_ref, '', COALESCE(size_bytes, 0)
  FROM identity_files WHERE storage_ref <> ''
UNION ALL
SELECT 'scene_note_audios', id, storage_ref, sha256, size_bytes
  FROM scene_note_audios
UNION ALL
SELECT 'bulk_enroll_outcomes', biometric_feature_id, crop_ref, '', 0
  FROM bulk_enroll_outcomes WHERE COALESCE(crop_ref, '') <> ''
ORDER BY 1, 2`

// Run checks every file the database refers to.
func Run(ctx context.Context, sqlDB *sql.DB, store Digester, opts Options) (Report, error) {
	query := references
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}
	rows, err := sqlDB.QueryContext(ctx, query)
	if err != nil {
		return Report{}, err
	}
	var refs []reference
	for rows.Next() {
		var r reference
		if err := rows.Scan(&r.table, &r.id, &r.ref, &r.sha256, &r.size); err != nil {
			rows.Close()
			return Report{}, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Report{}, err
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 4
	}
	var (
		report Report
		mu     sync.Mutex
		wg     sync.WaitGroup
		work   = make(chan reference)
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range work {
				checked, verified, problem := check(ctx, store, r)
				mu.Lock()
				if checked {
					report.Checked++
				} else {
					report.Skipped++
				}
				if verified {
					report.Verified++
				}
				if problem != nil {
					report.Problems = append(report.Problems, *problem)
				}
				mu.Unlock()
			}
		}()
	}
	for _, r := range refs {
		select {
		case work <- r:
		case <-ctx.Done():
		}
	}
	close(work)
	wg.Wait()
	return report, ctx.Err()
}

// check reads one file. checked is false for a reference that isn't to storage.
func check(ctx context.Context, store Digester, r reference) (checked, verified bool, problem *Problem) {
	if scheme, _, ok := strings.Cut(r.ref, "://"); ok && scheme != "s3" && scheme != "file" {
		return false, false, nil
	}
	fail := func(kind, detail string) *Problem {
		return &Problem{Table: r.table, ID: r.id, Kind: kind, Detail: detail}
	}
	sum, size, err := store.Digest(ctx, r.ref)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return true, false, fail(Missing, "")
	case err != nil:
		return true, false, fail(ReadError, err.Error())
	case r.sha256 != "" && !strings.EqualFold(sum, r.sha256):
		return true, false, fail(HashMismatch, fmt.Sprintf("recorded %s, stored %s", r.sha256, sum))
	case r.size > 0 && size != r.size:
		return true, r.sha256 != "", fail(SizeMismatch, fmt.Sprintf("recorded %d bytes, stored %d", r.size, size))
	}
	return true, r.sha256 != "", nil
}
