package distribution_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/distribution"
	"github.com/rodfileto/trackid/notifications"
)

// TestAssignmentNotifies: whoever is given work is told, in the assignment's own transaction,
// except when they assigned it to themselves or nothing was actually assigned.
func TestAssignmentNotifies(t *testing.T) {
	ctx := context.Background()
	sqlDB := testDB(t)
	repo := auth.NewRepository(sqlDB)
	user := func(name string, role auth.AccountRole, facial bool) auth.UserProfile {
		t.Helper()
		u, err := repo.Create(ctx, auth.NewUser{FirstName: name, LastName: "Test", StaffNumber: name, JobTitle: "test", Username: name,
			Password: "password-1", Access: auth.Access{AccountRole: role, ExaminesFacial: facial, IsActive: true}}, "")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	coordinator := user("coord", auth.AccountRoleSuperAdmin, false)
	ana := user("ana", auth.AccountRoleUser, true)
	review := func(caseID string) int64 {
		t.Helper()
		if _, err := cases.Ingest(ctx, sqlDB, cases.CaseInput{CaseID: caseID, CaseType: cases.CaseTypeCriminal, Modality: "FACIAL", Description: "test"}); err != nil {
			t.Fatal(err)
		}
		return mustReview(t, sqlDB, caseID)
	}
	wantSubjects := func(step string, userID int64, want ...int64) {
		t.Helper()
		items, unread, err := notifications.List(ctx, sqlDB, userID)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != len(want) || unread != int64(len(want)) {
			t.Fatalf("%s: %d notifications, %d unread, want %d", step, len(items), unread, len(want))
		}
		for i, id := range want { // newest first
			if items[i].Kind != notifications.KindAnalysisAssigned || items[i].Subject != strconv.FormatInt(id, 10) {
				t.Fatalf("%s: item %d = %+v, want analysis %d", step, i, items[i], id)
			}
		}
	}

	first := review("MANUAL")
	if err := distribution.Assign(ctx, sqlDB, first, &ana.ID, coordinator.ID); err != nil {
		t.Fatal(err)
	}
	wantSubjects("manual assignment", ana.ID, first)

	if err := distribution.Assign(ctx, sqlDB, first, nil, coordinator.ID); err != nil {
		t.Fatal(err)
	}
	wantSubjects("unassigning", ana.ID, first)

	self := review("SELF")
	if err := distribution.Assign(ctx, sqlDB, self, &ana.ID, ana.ID); err != nil {
		t.Fatal(err)
	}
	wantSubjects("taking it oneself", ana.ID, first)

	automatic := review("AUTO")
	result, err := distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 10, DryRun: true})
	if err != nil || len(result.Assigned) == 0 {
		t.Fatalf("dry run = %+v, %v", result, err)
	}
	wantSubjects("a dry run", ana.ID, first)

	result, err = distribution.Distribute(ctx, sqlDB, distribution.Request{Count: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := make([]int64, 0, len(result.Assigned)+1)
	for i := len(result.Assigned) - 1; i >= 0; i-- {
		want = append(want, result.Assigned[i].AnalysisID)
	}
	want = append(want, first)
	if len(want) != 3 || !contains(want, automatic) {
		t.Fatalf("distributed %+v", result.Assigned)
	}
	wantSubjects("distribution", ana.ID, want...)
}

func contains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
