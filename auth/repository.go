package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/rodfileto/trackid/db"
)

// UserProfile is the public representation of a registered user.
type UserProfile struct {
	ID          int64       `json:"id"`
	FirstName   string      `json:"first_name"`
	LastName    string      `json:"last_name"`
	StaffNumber string      `json:"staff_number"`
	JobTitle    string      `json:"job_title"`
	Username    string      `json:"username"`
	Email       string      `json:"email"`
	AccountRole AccountRole `json:"account_role"`
	// ExaminesFingerprint and ExaminesFacial say which biometrics the user may sign human
	// decisions on: either, both, or neither. See Examines.
	ExaminesFingerprint bool `json:"examines_fingerprint"`
	ExaminesFacial      bool `json:"examines_facial"`
	// IntelligenceAnalyst and Coordinator are the other two capabilities (db/migrations/001_users.sql); see
	// IntelligenceAccess and Coordinates.
	IntelligenceAnalyst bool `json:"intelligence_analyst"`
	Coordinator         bool `json:"coordinator"`
	IsActive            bool `json:"is_active"`
	// AvatarUpdatedAt is when the user's picture was last set, nil while they have none; the
	// picture is GET /api/users/{id}/avatar.
	AvatarUpdatedAt *time.Time `json:"avatar_updated_at,omitempty"`
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
	profile := UserProfile{ID: row.ID, FirstName: row.FirstName, LastName: row.LastName, StaffNumber: row.StaffNumber, JobTitle: row.JobTitle,
		Username: row.Username, Email: row.Email, AccountRole: AccountRole(row.AccountRole), ExaminesFingerprint: row.ExaminesFingerprint, ExaminesFacial: row.ExaminesFacial,
		IntelligenceAnalyst: row.IntelligenceAnalyst, Coordinator: row.Coordinator, IsActive: row.IsActive}
	if row.AvatarUpdatedAt.Valid {
		at := row.AvatarUpdatedAt.Time
		profile.AvatarUpdatedAt = &at
	}
	return profile
}

func (r *Repository) CreateUser(ctx context.Context, params db.CreateUserParams) (UserProfile, error) {
	row, err := r.queries.CreateUser(ctx, params)
	if err != nil {
		return UserProfile{}, err
	}
	return profileFrom(db.GetUserByIDRow(row)), nil
}

func (r *Repository) FindByStaffNumber(ctx context.Context, staffNumber string) (authenticatedUser, error) {
	row, err := r.queries.GetUserByStaffNumber(ctx, staffNumber)
	if err != nil {
		return authenticatedUser{}, err
	}
	return authenticatedUser{
		Profile: profileFrom(db.GetUserByIDRow{ID: row.ID, FirstName: row.FirstName, LastName: row.LastName, StaffNumber: row.StaffNumber, JobTitle: row.JobTitle,
			Username: row.Username, Email: row.Email, AccountRole: row.AccountRole, ExaminesFingerprint: row.ExaminesFingerprint, ExaminesFacial: row.ExaminesFacial,
			IntelligenceAnalyst: row.IntelligenceAnalyst, Coordinator: row.Coordinator, IsActive: row.IsActive, AvatarUpdatedAt: row.AvatarUpdatedAt}),
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

// Details are what users may change about themselves: their name and job title. Username, staff
// number and email are fixed (decisions name their decider by username).
type Details struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	JobTitle  string `json:"job_title"`
}

// ErrInvalidDetails is returned for an empty name or a field over 100 characters.
var ErrInvalidDetails = errors.New("first and last name are required, and no field may exceed 100 characters")

// ErrWrongPassword is returned when the current password given to ChangePassword is not the
// account's.
var ErrWrongPassword = errors.New("the current password is not correct")

// UpdateDetails sets a user's name and job title, trimmed.
func (r *Repository) UpdateDetails(ctx context.Context, id int64, d Details) (UserProfile, error) {
	d.FirstName, d.LastName, d.JobTitle = strings.TrimSpace(d.FirstName), strings.TrimSpace(d.LastName), strings.TrimSpace(d.JobTitle)
	if d.FirstName == "" || d.LastName == "" || len(d.FirstName) > 100 || len(d.LastName) > 100 || len(d.JobTitle) > 100 {
		return UserProfile{}, ErrInvalidDetails
	}
	row, err := r.queries.UpdateUserDetails(ctx, db.UpdateUserDetailsParams{ID: id, FirstName: d.FirstName, LastName: d.LastName, JobTitle: d.JobTitle})
	if err != nil {
		return UserProfile{}, err
	}
	return profileFrom(db.GetUserByIDRow(row)), nil
}

// ChangePassword replaces a user's own password, given the current one.
func (r *Repository) ChangePassword(ctx context.Context, id int64, current, next string) error {
	hash, err := r.queries.GetUserPasswordHash(ctx, id)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return ErrWrongPassword
	}
	return r.SetPassword(ctx, id, next)
}

// NewUser is what creating an account takes, from an admin (POST /api/admin/users) or from
// cmd/create-superuser.
type NewUser struct {
	FirstName   string
	LastName    string
	StaffNumber string
	JobTitle    string
	Username    string
	Password    string
	Access      Access
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
		FirstName:           user.FirstName,
		LastName:            user.LastName,
		StaffNumber:         user.StaffNumber,
		JobTitle:            user.JobTitle,
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
