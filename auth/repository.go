package auth

import (
	"context"
	"database/sql"

	"github.com/rodfileto/trackid/db"
)

// Repository is auth's data-access layer, backed by the sqlc-generated
// Queries in the db package.
type Repository struct {
	queries *db.Queries
}

func NewRepository(sqlDB *sql.DB) *Repository {
	if sqlDB == nil {
		return nil
	}
	return &Repository{queries: db.New(sqlDB)}
}

type authenticatedUser struct {
	Profile      UserProfile
	PasswordHash string
}

func (r *Repository) CreateUser(ctx context.Context, params db.CreateUserParams) (UserProfile, error) {
	row, err := r.queries.CreateUser(ctx, params)
	if err != nil {
		return UserProfile{}, err
	}
	return UserProfile{ID: row.ID, Nome: row.Nome, UltimoNome: row.UltimoNome, Matricula: row.Matricula, Cargo: row.Cargo, Username: row.Username, Email: row.Email}, nil
}

func (r *Repository) FindByMatricula(ctx context.Context, matricula string) (authenticatedUser, error) {
	row, err := r.queries.GetUserByMatricula(ctx, matricula)
	if err != nil {
		return authenticatedUser{}, err
	}
	return authenticatedUser{
		Profile:      UserProfile{ID: row.ID, Nome: row.Nome, UltimoNome: row.UltimoNome, Matricula: row.Matricula, Cargo: row.Cargo, Username: row.Username, Email: row.Email},
		PasswordHash: row.PasswordHash,
	}, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (UserProfile, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		return UserProfile{}, err
	}
	return UserProfile{ID: row.ID, Nome: row.Nome, UltimoNome: row.UltimoNome, Matricula: row.Matricula, Cargo: row.Cargo, Username: row.Username, Email: row.Email}, nil
}
