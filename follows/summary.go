package follows

import (
	"context"
	"database/sql"
	"slices"
	"strconv"

	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/notifications"
)

// maxRefs is how many references a summary lists for one kind of change.
const maxRefs = 5

// ChangeCount is how many of one change a notification reports. Refs lists the first cases, clusters or merge target
// it names, for the changes worth naming.
type ChangeCount struct {
	Change string   `json:"change"`
	Count  int      `json:"count"`
	Refs   []string `json:"refs,omitempty"`
}

// Summary is what a PERSON_CHANGED or CLUSTER_CHANGED notification reports, built when it is read.
type Summary struct {
	SubjectKind  string        `json:"subjectKind"`
	SubjectID    string        `json:"subjectId"`
	PersonNumber int64         `json:"personNumber,omitempty"`
	Name         string        `json:"name,omitempty"`
	MergedInto   int64         `json:"mergedInto,omitempty"`
	Changes      []ChangeCount `json:"changes"`
}

var named = map[string]bool{ChangeCaseLinked: true, ChangeClusterJoined: true, ChangeMerged: true}

// changeOrder puts core's changes first, in the order a reader wants them; a deployment's follow.
var changeOrder = []string{ChangeCaseLinked, ChangeClusterJoined, ChangeDocumentAdded, ChangeMerged, ChangeMemberAdded, ChangeMemberLeft}

// Summarize builds the summaries of the PERSON_CHANGED and CLUSTER_CHANGED notifications among items, by ID.
func Summarize(ctx context.Context, sqlDB *sql.DB, items []notifications.Item) (map[int64]Summary, error) {
	q := db.New(sqlDB)
	out := map[int64]Summary{}
	var personIDs []string
	for _, item := range items {
		kind := KindPerson
		switch item.Kind {
		case notifications.KindPersonChanged:
			personIDs = append(personIDs, item.Subject)
		case notifications.KindClusterChanged:
			kind = KindCluster
		default:
			continue
		}
		rows, err := q.ListSubjectChangesInRange(ctx, db.ListSubjectChangesInRangeParams{
			SubjectKind: kind, SubjectID: item.Subject, FirstID: item.FirstChangeID, LastID: item.LastChangeID,
		})
		if err != nil {
			return nil, err
		}
		out[item.ID] = summarize(kind, item.Subject, rows)
	}
	if len(personIDs) > 0 {
		labels, err := q.ListPersonLabels(ctx, personIDs)
		if err != nil {
			return nil, err
		}
		byID := map[string]db.ListPersonLabelsRow{}
		for _, l := range labels {
			byID[l.PersonID] = l
		}
		for id, s := range out {
			if l, ok := byID[s.SubjectID]; ok && s.SubjectKind == KindPerson {
				s.PersonNumber, s.Name = l.PersonNumber, l.Name
				out[id] = s
			}
		}
	}
	return out, nil
}

func summarize(kind, subject string, rows []db.ListSubjectChangesInRangeRow) Summary {
	s := Summary{SubjectKind: kind, SubjectID: subject, Changes: []ChangeCount{}}
	byChange := map[string]*ChangeCount{}
	var order []string
	for _, r := range rows {
		c, ok := byChange[r.Change]
		if !ok {
			c = &ChangeCount{Change: r.Change}
			byChange[r.Change] = c
			order = append(order, r.Change)
		}
		c.Count += int(r.Amount)
		if named[r.Change] && len(c.Refs) < maxRefs && !slices.Contains(c.Refs, r.Ref) {
			c.Refs = append(c.Refs, r.Ref)
		}
		if r.Change == ChangeMerged {
			if to, err := strconv.ParseInt(r.Ref, 10, 64); err == nil {
				s.MergedInto = to
			}
		}
	}
	rank := func(change string) int {
		if i := slices.Index(changeOrder, change); i >= 0 {
			return i
		}
		return len(changeOrder) + slices.Index(order, change)
	}
	slices.SortFunc(order, func(a, b string) int { return rank(a) - rank(b) })
	for _, change := range order {
		// A merged cluster's members all leave with the merge, which says it better.
		if change == ChangeMemberLeft && s.MergedInto != 0 {
			continue
		}
		s.Changes = append(s.Changes, *byChange[change])
	}
	return s
}
