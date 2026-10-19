package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/rodfileto/trackid/db"
)

// UserProfile is the public representation of a registered user.
type UserProfile struct {
	ID          int64       `json:"id"`
	Nome        string      `json:"nome"`
	UltimoNome  string      `json:"ultimo_nome"`
	Matricula   string      `json:"matricula"`
	Cargo       string      `json:"cargo"`
	Username    string      `json:"username"`
	Email       string      `json:"email"`
	AccountRole AccountRole `json:"account_role"`
	// ExaminesFingerprint and ExaminesFacial say which biometrics the user may sign human
	// decisions on: either, both, or neither. See Examines.
	ExaminesFingerprint bool `json:"examines_fingerprint"`
	ExaminesFacial      bool `json:"examines_facial"`
	// IntelligenceAnalyst and Coordinator are the other two capabilities (migration 020); see
	// IntelligenceAccess and Coordinates.
	IntelligenceAnalyst bool `json:"intelligence_analyst"`
	Coordinator         bool `json:"coordinator"`
	IsActive            bool `json:"is_active"`
}

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

// profileFrom maps a users row onto a UserProfile. sqlc emits one row type per query; the
// profile-shaped ones (CreateUserRow, ListUsersRow, ...) convert to GetUserByIDRow.
func profileFrom(row db.GetUserByIDRow) UserProfile {
	return UserProfile{ID: row.ID, Nome: row.Nome, UltimoNome: row.UltimoNome, Matricula: row.Matricula, Cargo: row.Cargo,
		Username: row.Username, Email: row.Email, AccountRole: AccountRole(row.AccountRole), ExaminesFingerprint: row.ExaminesFingerprint, ExaminesFacial: row.ExaminesFacial,
		IntelligenceAnalyst: row.IntelligenceAnalyst, Coordinator: row.Coordinator, IsActive: row.IsActive}
}

func (r *Repository) CreateUser(ctx context.Context, params db.CreateUserParams) (UserProfile, error) {
	row, err := r.queries.CreateUser(ctx, params)
	if err != nil {
		return UserProfile{}, err
	}
	return profileFrom(db.GetUserByIDRow(row)), nil
}

func (r *Repository) FindByMatricula(ctx context.Context, matricula string) (authenticatedUser, error) {
	row, err := r.queries.GetUserByMatricula(ctx, matricula)
	if err != nil {
		return authenticatedUser{}, err
	}
	return authenticatedUser{
		Profile: profileFrom(db.GetUserByIDRow{ID: row.ID, Nome: row.Nome, UltimoNome: row.UltimoNome, Matricula: row.Matricula, Cargo: row.Cargo,
			Username: row.Username, Email: row.Email, AccountRole: row.AccountRole, ExaminesFingerprint: row.ExaminesFingerprint, ExaminesFacial: row.ExaminesFacial,
			IntelligenceAnalyst: row.IntelligenceAnalyst, Coordinator: row.Coordinator, IsActive: row.IsActive}),
		PasswordHash: row.PasswordHash,
	}, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (UserProfile, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		return UserProfile{}, err
	}
	return profileFrom(row), nil
}

func (r *Repository) ListUsers(ctx context.Context) ([]UserProfile, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]UserProfile, len(rows))
	for i, row := range rows {
		users[i] = profileFrom(db.GetUserByIDRow(row))
	}
	return users, nil
}

// UpdateAccess sets a user's account role, capabilities and active flag. Callers check the change
// with CheckAccountChange first.
func (r *Repository) UpdateAccess(ctx context.Context, id int64, access Access) (UserProfile, error) {
	row, err := r.queries.UpdateUserAccess(ctx, db.UpdateUserAccessParams{
		ID: id, AccountRole: string(access.AccountRole), ExaminesFingerprint: access.ExaminesFingerprint, ExaminesFacial: access.ExaminesFacial,
		IntelligenceAnalyst: access.IntelligenceAnalyst, Coordinator: access.Coordinator, IsActive: access.IsActive,
	})
	if err != nil {
		return UserProfile{}, err
	}
	return profileFrom(db.GetUserByIDRow(row)), nil
}

// SetPassword replaces a user's password (an admin resetting it). Callers check the change with
// CheckAccountChange first; it returns sql.ErrNoRows for an unknown id.
func (r *Repository) SetPassword(ctx context.Context, id int64, password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	n, err := r.queries.SetUserPassword(ctx, db.SetUserPasswordParams{ID: id, PasswordHash: string(passwordHash)})
	if err == nil && n == 0 {
		err = sql.ErrNoRows
	}
	return err
}

// NewUser is what creating an account takes, from an admin (POST /api/admin/users) or from
// cmd/create-superuser.
type NewUser struct {
	Nome       string
	UltimoNome string
	Matricula  string
	Cargo      string
	Username   string
	Password   string
	Access     Access
}

// MinPasswordLength is the shortest password an account may have.
const MinPasswordLength = 8

// ErrPasswordTooShort is returned for a password under MinPasswordLength.
var ErrPasswordTooShort = errors.New("password must have at least 8 characters")

// Create hashes the password and inserts the account. The username is lowercased and trimmed;
// the email is username@emailDomain, or the bare username when emailDomain is empty.
func (r *Repository) Create(ctx context.Context, user NewUser, emailDomain string) (UserProfile, error) {
	if len(user.Password) < MinPasswordLength {
		return UserProfile{}, ErrPasswordTooShort
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		return UserProfile{}, err
	}
	username := strings.ToLower(strings.TrimSpace(user.Username))
	email := username
	if emailDomain != "" {
		email = username + "@" + emailDomain
	}
	return r.CreateUser(ctx, db.CreateUserParams{
		Nome:                user.Nome,
		UltimoNome:          user.UltimoNome,
		Matricula:           user.Matricula,
		Cargo:               user.Cargo,
		Username:            username,
		Email:               email,
		PasswordHash:        string(passwordHash),
		AccountRole:         string(user.Access.AccountRole),
		ExaminesFingerprint: user.Access.ExaminesFingerprint,
		ExaminesFacial:      user.Access.ExaminesFacial,
		IntelligenceAnalyst: user.Access.IntelligenceAnalyst,
		Coordinator:         user.Access.Coordinator,
	})
}
