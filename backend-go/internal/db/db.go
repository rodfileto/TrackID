// Package db handles the Postgres connection and the queries auth needs:
// local users (username + password_hash) and linked OAuth identities.
package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &DB{Pool: pool}, nil
}

// EnsureSchema creates the auth tables if they don't already exist. Schema is
// bootstrapped idempotently at startup, matching the Rust backend's approach
// (no migration tool).
func (d *DB) EnsureSchema(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id uuid PRIMARY KEY,
			username varchar UNIQUE,
			email varchar UNIQUE,
			password_hash varchar,
			created_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE TABLE IF NOT EXISTS user_identities (
			id uuid PRIMARY KEY,
			user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider varchar NOT NULL,
			provider_user_id varchar NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE (provider, provider_user_id)
		);
		ALTER TABLE users ADD COLUMN IF NOT EXISTS email varchar UNIQUE;
		ALTER TABLE users ALTER COLUMN username DROP NOT NULL;
		ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
	`)
	return err
}

type User struct {
	ID           string
	Username     *string
	Email        *string
	PasswordHash *string
}

var ErrNotFound = errors.New("not found")
var ErrAlreadyExists = errors.New("already exists")

func (d *DB) CreateUser(ctx context.Context, id, username, passwordHash string) error {
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash) VALUES ($1, $2, $3)`,
		id, username, passwordHash,
	)
	if isUniqueViolation(err) {
		return ErrAlreadyExists
	}
	return err
}

func (d *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := d.Pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash FROM users WHERE username = $1`, username,
	)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := d.Pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash FROM users WHERE id = $1`, id,
	)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// GetUserByIdentity looks up a user already linked to this OAuth provider account.
func (d *DB) GetUserByIdentity(ctx context.Context, provider, providerUserID string) (*User, error) {
	row := d.Pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.email, u.password_hash
		FROM users u
		JOIN user_identities i ON i.user_id = u.id
		WHERE i.provider = $1 AND i.provider_user_id = $2
	`, provider, providerUserID)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := d.Pool.QueryRow(ctx,
		`SELECT id, username, email, password_hash FROM users WHERE email = $1`, email,
	)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// CreateOAuthUser creates a user with no password (OAuth-only) and links the
// given provider identity to it, in one transaction.
func (d *DB) CreateOAuthUser(ctx context.Context, id, email, provider, providerUserID, identityID string) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var emailArg any
	if email != "" {
		emailArg = email
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, email) VALUES ($1, $2)`, id, emailArg,
	); err != nil {
		return err
	}
	if err := linkIdentity(ctx, tx, identityID, id, provider, providerUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LinkIdentity attaches a provider identity to an existing user (account linking).
func (d *DB) LinkIdentity(ctx context.Context, identityID, userID, provider, providerUserID string) error {
	return linkIdentity(ctx, d.Pool, identityID, userID, provider, providerUserID)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func linkIdentity(ctx context.Context, e execer, identityID, userID, provider, providerUserID string) error {
	_, err := e.Exec(ctx,
		`INSERT INTO user_identities (id, user_id, provider, provider_user_id) VALUES ($1, $2, $3, $4)`,
		identityID, userID, provider, providerUserID,
	)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
