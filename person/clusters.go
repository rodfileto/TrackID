package person

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/graph"
)

// ClusterMember is one biometric sample -- an enrollment photo or a
// crime-scene trace -- a cluster groups together. Kind says which: "KNOWN"
// populates the enrollment fields (PersonID..ContentType), "QUESTIONED"
// populates the case-evidence fields (CaseID..ThumbnailBox). A cluster on a
// person's own profile always has at least one KNOWN member (this person);
// any QUESTIONED members are crime-scene evidence a CONFIRMED decision has
// matched to them.
type ClusterMember struct {
	Kind string `json:"kind"`

	// KNOWN fields -- an enrollment photo.
	PersonID       string `json:"personId,omitempty"`
	Name           string `json:"name,omitempty"`
	RegisterNumber string `json:"registerNumber,omitempty"`
	DocumentType   string `json:"documentType,omitempty"`
	DocumentNumber string `json:"documentNumber,omitempty"`
	IdentityFileID int64  `json:"identityFileId,omitempty"`
	ContentType    string `json:"contentType,omitempty"`

	// QUESTIONED fields -- a case_trace, thumbnail resolved the same way as
	// CaseFaceSearchResult (see its doc comment).
	CaseID          string        `json:"caseId,omitempty"`
	Modality        string        `json:"modality,omitempty"`
	Description     string        `json:"description,omitempty"`
	TraceID         int64         `json:"traceId,omitempty"`
	ThumbnailFileID int64         `json:"thumbnailFileId,omitempty"`
	ThumbnailBox    *ThumbnailBox `json:"thumbnailBox,omitempty"`
}

// Cluster is one biometric cluster a person's enrolled (KNOWN) features have
// resolved into: a group of same-modality biometric samples -- enrollment
// records and/or crime-scene evidence -- that a confirmed decision chain has
// linked together (see cluster.Run). See MODEL.md section 4.
//
// HasCaseEvidence is true when at least one member is QUESTIONED -- this
// person's biometric has been matched to real crime-scene evidence, not just
// to another enrollment record.
type Cluster struct {
	ClusterID       int64           `json:"clusterId"`
	Modality        string          `json:"modality"`
	CreatedAt       time.Time       `json:"createdAt"`
	MemberCount     int             `json:"memberCount"`
	HasCaseEvidence bool            `json:"hasCaseEvidence"`
	Members         []ClusterMember `json:"members"`
}

// ListClusters returns every biometric cluster resolved to the given
// person -- one entry per cluster a CONFIRMED decision has placed one of the
// person's enrolled features into. A person with documents/registers of more
// than one modality can resolve into several clusters; most resolve into at
// most one per modality. Returns ErrNotFound if no person row matches.
func ListClusters(ctx context.Context, sqlDB *sql.DB, personID string) ([]Cluster, error) {
	if sqlDB == nil {
		return nil, fmt.Errorf("person: nil db")
	}
	if personID == "" {
		return nil, fmt.Errorf("person: personID is required")
	}

	_, _, clusterIDs, err := loadPersonAndClusterIDs(ctx, sqlDB, personID)
	if err != nil {
		return nil, err
	}
	if len(clusterIDs) == 0 {
		return nil, nil
	}

	q := db.New(sqlDB)
	clusterRows, err := q.ListClustersByIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}
	memberRows, err := q.ListClusterMembersByClusterIDs(ctx, clusterIDs)
	if err != nil {
		return nil, err
	}

	return buildClusters(ctx, q, clusterRows, memberRows)
}

// buildClusters assembles the Cluster list -- each with its resolved member
// details -- from an already-loaded set of cluster rows and their combined
// membership. The reusable core both ListClusters and GetProfile build on.
func buildClusters(ctx context.Context, q *db.Queries, clusterRows []db.ListClustersByIDsRow, memberRows []db.ListClusterMembersByClusterIDsRow) ([]Cluster, error) {
	var identityFileIDs, traceIDs []int64
	seenIdentityFile := map[int64]bool{}
	seenTrace := map[int64]bool{}
	for _, m := range memberRows {
		if traceID, ok := graph.ParseQuestionedFeatureID(m.FeatureID); ok {
			if !seenTrace[traceID] {
				seenTrace[traceID] = true
				traceIDs = append(traceIDs, traceID)
			}
			continue
		}
		id, err := strconv.ParseInt(m.FeatureID, 10, 64)
		if err != nil {
			continue
		}
		if !seenIdentityFile[id] {
			seenIdentityFile[id] = true
			identityFileIDs = append(identityFileIDs, id)
		}
	}

	knownByFile := map[int64]db.ListKnownClusterMembersRow{}
	if len(identityFileIDs) > 0 {
		rows, err := q.ListKnownClusterMembers(ctx, identityFileIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			knownByFile[r.IdentityFileID] = r
		}
	}

	questionedByTrace := map[int64]db.ListQuestionedClusterMembersRow{}
	if len(traceIDs) > 0 {
		rows, err := q.ListQuestionedClusterMembers(ctx, traceIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			questionedByTrace[r.CaseTraceID] = r
		}
	}

	membersByCluster := map[int64][]ClusterMember{}
	for _, m := range memberRows {
		if traceID, ok := graph.ParseQuestionedFeatureID(m.FeatureID); ok {
			d, ok := questionedByTrace[traceID]
			if !ok {
				// The trace's evidence was excluded/deleted since clustering.
				continue
			}
			member := ClusterMember{
				Kind:        "QUESTIONED",
				CaseID:      d.CaseID,
				Modality:    d.Modality,
				Description: d.Description,
				TraceID:     traceID,
			}
			switch {
			case d.TraceCropFileID.Valid:
				member.ThumbnailFileID = d.TraceCropFileID.Int64
			case d.EvidenceFileID.Valid && d.BoxX1.Valid:
				member.ThumbnailFileID = d.EvidenceFileID.Int64
				member.ThumbnailBox = &ThumbnailBox{
					X1: d.BoxX1.Float64,
					Y1: d.BoxY1.Float64,
					X2: d.BoxX2.Float64,
					Y2: d.BoxY2.Float64,
				}
			}
			membersByCluster[m.ClusterID] = append(membersByCluster[m.ClusterID], member)
			continue
		}

		id, err := strconv.ParseInt(m.FeatureID, 10, 64)
		if err != nil {
			continue
		}
		d, ok := knownByFile[id]
		if !ok {
			continue
		}
		membersByCluster[m.ClusterID] = append(membersByCluster[m.ClusterID], ClusterMember{
			Kind:           "KNOWN",
			PersonID:       d.PersonID,
			Name:           d.Name,
			RegisterNumber: d.RegisterNumber,
			DocumentType:   d.DocumentType,
			DocumentNumber: d.DocumentNumber,
			IdentityFileID: d.IdentityFileID,
			ContentType:    d.ContentType.String,
		})
	}

	out := make([]Cluster, 0, len(clusterRows))
	for _, c := range clusterRows {
		members := membersByCluster[c.ID]
		hasCaseEvidence := false
		for _, m := range members {
			if m.Kind == "QUESTIONED" {
				hasCaseEvidence = true
				break
			}
		}
		out = append(out, Cluster{
			ClusterID:       c.ID,
			Modality:        c.Modality,
			CreatedAt:       c.CreatedAt,
			MemberCount:     len(members),
			HasCaseEvidence: hasCaseEvidence,
			Members:         members,
		})
	}
	return out, nil
}
