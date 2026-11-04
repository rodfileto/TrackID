package bulk

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"
)

// copyRows streams rows into table with COPY. next returns the next row's values, or nil at the
// end. It works on both drivers core runs on: pgx (the commands, through internal/database) and
// lib/pq (the database tests).
func copyRows(ctx context.Context, sqlDB *sql.DB, table string, columns []string, next func() []any) (int64, error) {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	var n int64
	usedPgx := false
	err = conn.Raw(func(driverConn any) error {
		c, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return nil
		}
		usedPgx = true
		n, err = c.Conn().CopyFrom(ctx, pgx.Identifier{table}, columns, &rowSource{next: next})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("bulk: copy into %s: %w", table, err)
	}
	if usedPgx {
		return n, nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(table, columns...))
	if err != nil {
		return 0, fmt.Errorf("bulk: copy into %s: %w", table, err)
	}
	for row := next(); row != nil; row = next() {
		if _, err := stmt.ExecContext(ctx, row...); err != nil {
			stmt.Close()
			return 0, fmt.Errorf("bulk: copy into %s: %w", table, err)
		}
		n++
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		stmt.Close()
		return 0, fmt.Errorf("bulk: copy into %s: %w", table, err)
	}
	if err := stmt.Close(); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// rowSource adapts next to pgx.CopyFromSource.
type rowSource struct {
	next func() []any
	row  []any
}

func (s *rowSource) Next() bool {
	s.row = s.next()
	return s.row != nil
}

func (s *rowSource) Values() ([]any, error) { return s.row, nil }

func (s *rowSource) Err() error { return nil }
