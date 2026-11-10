// Package follows lets intelligence analysts follow a person or a cluster and tells them when it changes
// (docs/follow-plan.md). Triggers on core's tables record what happened in subject_changes; a deployment records its
// own changes with RecordFeature; Fanout works out what each change means for the persons and clusters followed and
// writes the notifications.
package follows

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// Subject kinds.
const (
	KindPerson  = "PERSON"
	KindCluster = "CLUSTER"
)

// Changes core records. A deployment adds its own through RecordFeature.
const (
	ChangeDocumentAdded = "DOCUMENT_ADDED"
	ChangeMemberAdded   = "MEMBER_ADDED"
	ChangeMemberLeft    = "MEMBER_LEFT"
	ChangeMerged        = "MERGED"
	ChangeClusterJoined = "CLUSTER_JOINED"
	ChangeCaseLinked    = "CASE_LINKED"
)

// Channel wakes the fan-out; the triggers and RecordFeature signal it.
const Channel = "trackid_subject_changes"

const basisAutomatic = "AUTOMATIC"

// Errors returned to callers.
var (
	ErrInvalidKind = errors.New("a subject is a PERSON or a CLUSTER")
	ErrNotFound    = errors.New("no such person or cluster")
)

// Subject is a person or a cluster someone follows.
type Subject struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Follow is one subject a user follows.
type Follow struct {
	Subject
	Since time.Time `json:"since"`
}

// Resolve checks that a subject exists and returns what is followed for it. Only AUTOMATIC clusters change fast
// enough to follow, so a REVIEWED cluster resolves to the AUTOMATIC cluster it sits inside.
func Resolve(ctx context.Context, sqlDB *sql.DB, kind, id string) (Subject, error) {
	q := db.New(sqlDB)
	switch kind {
	case KindPerson:
		exists, err := q.PersonExists(ctx, id)
		if err != nil {
			return Subject{}, err
		}
		if !exists {
			return Subject{}, ErrNotFound
		}
		return Subject{Kind: KindPerson, ID: id}, nil
	case KindCluster:
		clusterID, err := strconv.ParseInt(id, 10, 64)
		if err != nil || clusterID <= 0 {
			return Subject{}, ErrNotFound
		}
		basis, err := q.ClusterBasis(ctx, clusterID)
		if errors.Is(err, sql.ErrNoRows) {
			return Subject{}, ErrNotFound
		}
		if err != nil {
			return Subject{}, err
		}
		if basis != basisAutomatic {
			clusterID, err = q.AutomaticClusterOfReviewed(ctx, clusterID)
			if errors.Is(err, sql.ErrNoRows) {
				return Subject{}, ErrNotFound
			}
			if err != nil {
				return Subject{}, err
			}
		}
		return Subject{Kind: KindCluster, ID: strconv.FormatInt(clusterID, 10)}, nil
	}
	return Subject{}, ErrInvalidKind
}

// Add makes the user follow the subject, which Resolve returned.
func Add(ctx context.Context, sqlDB *sql.DB, userID int64, s Subject) error {
	return db.New(sqlDB).InsertFollow(ctx, db.InsertFollowParams{UserID: userID, SubjectKind: s.Kind, SubjectID: s.ID})
}

// Remove stops the user following the subject.
func Remove(ctx context.Context, sqlDB *sql.DB, userID int64, s Subject) error {
	return db.New(sqlDB).DeleteFollow(ctx, db.DeleteFollowParams{UserID: userID, SubjectKind: s.Kind, SubjectID: s.ID})
}

// IsFollowing reports whether the user follows the subject.
func IsFollowing(ctx context.Context, sqlDB *sql.DB, userID int64, s Subject) (bool, error) {
	return db.New(sqlDB).IsFollowing(ctx, db.IsFollowingParams{UserID: userID, SubjectKind: s.Kind, SubjectID: s.ID})
}

// List returns what the user follows, latest first.
func List(ctx context.Context, sqlDB *sql.DB, userID int64) ([]Follow, error) {
	rows, err := db.New(sqlDB).ListFollowsOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Follow, len(rows))
	for i, r := range rows {
		out[i] = Follow{Subject: Subject{Kind: r.SubjectKind, ID: r.SubjectID}, Since: r.CreatedAt}
	}
	return out, nil
}

// RecordFeature records a deployment's change that concerns one biometric feature, such as camera hits found for a
// face, amount standing for how many. It reaches the feature's AUTOMATIC cluster, the persons whose KNOWN features
// sit in that cluster and, for a KNOWN feature, its own person. q should be the transaction of the action being
// recorded, so the change exists only if that action commits. A feature that reaches no subject records nothing.
func RecordFeature(ctx context.Context, q *db.Queries, featureID, change, ref string, amount int) error {
	if amount <= 0 {
		return nil
	}
	var subjects []Subject
	memberships, err := q.ListClusterMembershipsForFeatureIDs(ctx, db.ListClusterMembershipsForFeatureIDsParams{
		Basis: basisAutomatic, FeatureIds: []string{featureID},
	})
	if err != nil {
		return err
	}
	persons := map[string]bool{}
	if len(memberships) > 0 {
		clusterID := memberships[0].ClusterID
		subjects = append(subjects, Subject{Kind: KindCluster, ID: strconv.FormatInt(clusterID, 10)})
		ids, err := q.ListPersonsInClusters(ctx, []int64{clusterID})
		if err != nil {
			return err
		}
		for _, id := range ids {
			persons[id] = true
		}
	}
	if _, questioned := graph.ParseQuestionedFeatureID(featureID); !questioned {
		if fileID, err := strconv.ParseInt(featureID, 10, 64); err == nil {
			personID, err := q.GetPersonOfIdentityFile(ctx, fileID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if personID != "" {
				persons[personID] = true
			}
		}
	}
	for id := range persons {
		subjects = append(subjects, Subject{Kind: KindPerson, ID: id})
	}
	if len(subjects) == 0 {
		return nil
	}
	for _, s := range subjects {
		if _, err := q.InsertSubjectChange(ctx, db.InsertSubjectChangeParams{
			SubjectKind: s.Kind, SubjectID: s.ID, Change: change, Ref: ref, Amount: int32(amount),
		}); err != nil {
			return err
		}
	}
	return q.SignalSubjectChanges(ctx)
}
