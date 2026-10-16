package auth

import (
	"errors"
	"testing"
)

func TestCheckAccountChange(t *testing.T) {
	super := UserProfile{ID: 1, AccountRole: AccountRoleSuperAdmin, IsActive: true}
	admin := UserProfile{ID: 2, AccountRole: AccountRoleAdmin, IsActive: true}
	user := UserProfile{ID: 3, AccountRole: AccountRoleUser, IsActive: true}
	inactiveAdmin := UserProfile{ID: 4, AccountRole: AccountRoleAdmin, IsActive: false}

	examiner := Access{AccountRole: AccountRoleUser, ExaminesFingerprint: true, IsActive: true}
	asAdmin := Access{AccountRole: AccountRoleAdmin, IsActive: true}

	tests := []struct {
		name   string
		actor  UserProfile
		target *UserProfile
		next   Access
		want   error
	}{
		{"user cannot manage", user, nil, examiner, ErrNotAdmin},
		{"inactive admin cannot manage", inactiveAdmin, nil, examiner, ErrNotAdmin},
		{"admin creates a user", admin, nil, examiner, nil},
		{"admin makes a user an examiner", admin, &user, examiner, nil},
		{"admin deactivates a user", admin, &user, Access{AccountRole: AccountRoleUser}, nil},
		{"admin cannot create an admin", admin, nil, asAdmin, ErrNeedsSuperAdmin},
		{"admin cannot promote a user", admin, &user, asAdmin, ErrNeedsSuperAdmin},
		{"admin cannot change another admin", admin, &inactiveAdmin, asAdmin, ErrNeedsSuperAdmin},
		{"admin cannot demote a super admin", admin, &super, examiner, ErrNeedsSuperAdmin},
		{"super admin creates an admin", super, nil, asAdmin, nil},
		{"super admin demotes an admin", super, &admin, examiner, nil},
		{"admin makes a user examine both", admin, &user, Access{AccountRole: AccountRoleUser, ExaminesFingerprint: true, ExaminesFacial: true, IsActive: true}, nil},
		{"invalid role", super, nil, Access{AccountRole: "ROOT", IsActive: true}, ErrInvalidRole},
		{"super admin cannot demote themselves", super, &super, asAdmin, ErrOwnAccountAccess},
		{"admin cannot deactivate themselves", admin, &admin, Access{AccountRole: AccountRoleAdmin}, ErrOwnAccountAccess},
		{"admin may make themselves an examiner", admin, &admin, Access{AccountRole: AccountRoleAdmin, ExaminesFacial: true, IsActive: true}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckAccountChange(tt.actor, tt.target, tt.next); !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExamines(t *testing.T) {
	both := UserProfile{ExaminesFingerprint: true, ExaminesFacial: true, IsActive: true}
	facial := UserProfile{ExaminesFacial: true, IsActive: true}
	inactive := UserProfile{ExaminesFingerprint: true, ExaminesFacial: true}
	tests := []struct {
		name string
		user UserProfile
		m    Modality
		want bool
	}{
		{"both, fingerprint", both, ModalityFingerprint, true},
		{"both, facial", both, ModalityFacial, true},
		{"facial only, facial", facial, ModalityFacial, true},
		{"facial only, fingerprint", facial, ModalityFingerprint, false},
		{"inactive", inactive, ModalityFacial, false},
		{"decisions' FACE spelling", both, "FACE", false},
		{"neither", UserProfile{IsActive: true}, ModalityFacial, false},
	}
	for _, tt := range tests {
		if got := tt.user.Examines(tt.m); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
