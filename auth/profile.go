package auth

import "errors"

// AccountRole is what a user may do to the system's accounts (users.account_role), and nothing
// else: it gives no access to cases or persons. What a user does is their capabilities, each
// independent of the role and of each other: what they examine (UserProfile.Examines), whether
// they analyse intelligence (IntelligenceAccess) and whether they coordinate the work
// (Coordinates). See db/migrations/001_users.sql.
type AccountRole string

const (
	AccountRoleSuperAdmin AccountRole = "SUPER_ADMIN"
	AccountRoleAdmin      AccountRole = "ADMIN"
	AccountRoleUser       AccountRole = "USER"
)

// Valid reports whether r is one of the account roles users.account_role accepts.
func (r AccountRole) Valid() bool {
	switch r {
	case AccountRoleSuperAdmin, AccountRoleAdmin, AccountRoleUser:
		return true
	}
	return false
}

// IsAdmin reports whether r may manage accounts at all (ADMIN or SUPER_ADMIN).
func (r AccountRole) IsAdmin() bool {
	return r == AccountRoleAdmin || r == AccountRoleSuperAdmin
}

// Modality is a biometric, in biometric_cases.modality's vocabulary.
type Modality string

const (
	ModalityFingerprint Modality = "FINGERPRINT"
	ModalityFacial      Modality = "FACIAL"
)

// Examines reports whether the user may sign human decisions on modality m: they examine it and
// their account is active.
func (p UserProfile) Examines(m Modality) bool {
	if !p.IsActive {
		return false
	}
	switch m {
	case ModalityFingerprint:
		return p.ExaminesFingerprint
	case ModalityFacial:
		return p.ExaminesFacial
	}
	return false
}

// IntelligenceAccess reports whether the user may read every biometric case and person -- full
// person pages, clusters, face search, occurrences -- and link occurrences to cases: they are an
// intelligence analyst and their account is active. Blind work suspends it (distribution.BlindAnywhere).
func (p UserProfile) IntelligenceAccess() bool { return p.IsActive && p.IntelligenceAnalyst }

// Coordinates reports whether the user may distribute analyses, reassign their steps and open
// comparisons: they are a coordinator and their account is active.
func (p UserProfile) Coordinates() bool { return p.IsActive && p.Coordinator }

// Access is the part of a profile an admin can change.
type Access struct {
	AccountRole         AccountRole
	ExaminesFingerprint bool
	ExaminesFacial      bool
	IntelligenceAnalyst bool
	Coordinator         bool
	IsActive            bool
}

// Errors CheckAccountChange returns.
var (
	ErrNotAdmin         = errors.New("only an admin can manage users")
	ErrInvalidRole      = errors.New("invalid account role")
	ErrNeedsSuperAdmin  = errors.New("only a super admin can create or change an admin")
	ErrOwnAccountAccess = errors.New("you cannot change your own account role or deactivate yourself")
)

// CheckAccountChange decides whether actor may give an account the access next. target is the
// account as it is now, or nil when it is being created. The rules:
//   - only an active ADMIN or SUPER_ADMIN manages accounts;
//   - only a SUPER_ADMIN creates or changes an account that is, or would become, ADMIN or
//     SUPER_ADMIN, so an ADMIN manages plain users only;
//   - nobody changes their own account role or deactivates themselves, so the last super admin
//     cannot lock everyone out (they may still change what they examine).
func CheckAccountChange(actor UserProfile, target *UserProfile, next Access) error {
	if !actor.IsActive || !actor.AccountRole.IsAdmin() {
		return ErrNotAdmin
	}
	if !next.AccountRole.Valid() {
		return ErrInvalidRole
	}
	if target != nil && target.ID == actor.ID {
		if next.AccountRole != target.AccountRole || !next.IsActive {
			return ErrOwnAccountAccess
		}
		return nil
	}
	if actor.AccountRole != AccountRoleSuperAdmin {
		if next.AccountRole.IsAdmin() || (target != nil && target.AccountRole.IsAdmin()) {
			return ErrNeedsSuperAdmin
		}
	}
	return nil
}
