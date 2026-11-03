package articles

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rodfileto/trackid/db"
)

// Target is a mention target as the @ menu and a mention chip show it: a label, the case or
// cases it belongs to (to link it), and a few facts for the chip's card.
type Target struct {
	Kind  Kind   `json:"kind"`
	Ref   string `json:"ref"`
	Label string `json:"label"`
	// CaseIDs are the case a target is (CASE) or belongs to (REPORT), or the cases a request
	// opened or joined (REQUEST).
	CaseIDs     []string   `json:"caseIds,omitempty"`
	Description string     `json:"description,omitempty"`
	CaseType    string     `json:"caseType,omitempty"`
	Modality    string     `json:"modality,omitempty"`
	Status      string     `json:"status,omitempty"`
	Basis       string     `json:"basis,omitempty"`
	ReportType  string     `json:"reportType,omitempty"`
	Date        *time.Time `json:"date,omitempty"`
	// Count is an inquiry's occurrences or a cluster's members.
	Count int `json:"count,omitempty"`
}

// Resolved is one mention as it reads now. Found is false for a target that no longer exists;
// a cluster merged into another is not found, and MergedInto names the cluster that holds its
// members now, whose Target is given.
type Resolved struct {
	MentionKey
	Found      bool    `json:"found"`
	MergedInto string  `json:"mergedInto,omitempty"`
	Target     *Target `json:"target,omitempty"`
}

func dateOf(t time.Time) *time.Time { return &t }

func caseTarget(r db.ListMentionCasesRow) Target {
	return Target{Kind: KindCase, Ref: r.CaseID, Label: r.CaseID, CaseIDs: []string{r.CaseID},
		Description: r.Description, CaseType: r.CaseType, Modality: r.Modality, Date: dateOf(r.CreatedAt)}
}

// occurrenceLabel is the police report number with its agency, else TrackID's number, else the
// inquiry the occurrence was registered by.
func occurrenceLabel(r db.ListMentionOccurrencesRow) string {
	switch {
	case r.OccurrenceNumber.Valid:
		if r.Agency.Valid && r.Agency.String != "" {
			return r.Agency.String + " " + r.OccurrenceNumber.String
		}
		return r.OccurrenceNumber.String
	case r.OccurrenceYear.Valid && r.OccurrenceSeq.Valid:
		return fmt.Sprintf("%d.%04d", r.OccurrenceYear.Int32, r.OccurrenceSeq.Int32)
	case r.InquiryNumber.Valid:
		return "IPL " + r.InquiryNumber.String
	}
	return strconv.FormatInt(r.ID, 10)
}

func occurrenceTarget(r db.ListMentionOccurrencesRow) Target {
	return Target{Kind: KindOccurrence, Ref: strconv.FormatInt(r.ID, 10), Label: occurrenceLabel(r),
		Description: r.Description, Status: r.Status, Date: dateOf(r.OccurredFrom)}
}

func inquiryTarget(r db.ListMentionInquiriesRow) Target {
	return Target{Kind: KindInquiry, Ref: r.InquiryNumber, Label: r.InquiryNumber,
		Count: int(r.OccurrenceCount), Date: dateOf(r.LastOccurred)}
}

func personTarget(r db.ListMentionPersonsRow) Target {
	label := r.Name
	if label == "" {
		label = r.PersonID
	}
	return Target{Kind: KindPerson, Ref: r.PersonID, Label: label, Description: r.PersonID}
}

func clusterTarget(r db.ListMentionClustersRow) Target {
	id := strconv.FormatInt(r.ID, 10)
	return Target{Kind: KindCluster, Ref: id, Label: "#" + id, Modality: r.Modality, Basis: r.Basis,
		Count: int(r.MemberCount), Date: dateOf(r.CreatedAt)}
}

func requestTarget(r db.ListMentionRequestsRow) Target {
	return Target{Kind: KindRequest, Ref: strconv.FormatInt(r.ID, 10), Label: r.DocumentReference,
		CaseIDs: r.CaseIds, Description: r.Authority, Date: dateOf(r.ReceivedOn)}
}

func reportTarget(r db.ListMentionReportsRow) Target {
	t := Target{Kind: KindReport, Ref: strconv.FormatInt(r.ID, 10), CaseIDs: []string{r.CaseID}, ReportType: r.ReportType}
	t.Label = fmt.Sprintf("%d/%d", r.ReportNumber.Int32, r.ReportYear.Int32)
	if r.IssuedAt.Valid {
		t.Date = dateOf(r.IssuedAt.Time)
	}
	return t
}

// Resolve reads each mention target as it is now: found, merged into another cluster, or gone.
// The result has one entry per distinct key, in the order given.
func Resolve(ctx context.Context, q *db.Queries, keys []MentionKey) ([]Resolved, error) {
	found, err := lookup(ctx, q, keys)
	if err != nil {
		return nil, err
	}

	// A cluster that is gone may have been merged: follow its merges to the cluster that holds
	// its members now.
	var gone []int64
	for _, k := range keys {
		if k.Kind == KindCluster && found[k] == nil {
			gone = append(gone, k.id())
		}
	}
	mergedInto := map[int64]int64{}
	if len(gone) > 0 {
		mergedInto, err = followMerges(ctx, q, gone)
		if err != nil {
			return nil, err
		}
		var survivors []MentionKey
		for _, to := range mergedInto {
			survivors = append(survivors, MentionKey{Kind: KindCluster, Ref: strconv.FormatInt(to, 10)})
		}
		more, err := lookup(ctx, q, survivors)
		if err != nil {
			return nil, err
		}
		for k, t := range more {
			found[k] = t
		}
	}

	seen := map[MentionKey]bool{}
	out := make([]Resolved, 0, len(keys))
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		r := Resolved{MentionKey: k, Target: found[k], Found: found[k] != nil}
		if !r.Found && k.Kind == KindCluster {
			if to, ok := mergedInto[k.id()]; ok {
				r.MergedInto = strconv.FormatInt(to, 10)
				r.Target = found[MentionKey{Kind: KindCluster, Ref: r.MergedInto}]
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// followMerges maps each removed cluster to the cluster its members are in now, following merge
// after merge (a bounded number of times, against a cycle). A cluster with no merge record is
// left out: it was removed.
func followMerges(ctx context.Context, q *db.Queries, ids []int64) (map[int64]int64, error) {
	next := map[int64]int64{}
	frontier := ids
	for hop := 0; hop < 32 && len(frontier) > 0; hop++ {
		rows, err := q.ListClusterMergesFrom(ctx, frontier)
		if err != nil {
			return nil, err
		}
		frontier = nil
		for _, r := range rows {
			if _, done := next[r.FromClusterID]; done { // the latest merge comes first
				continue
			}
			next[r.FromClusterID] = r.ToClusterID
			if _, known := next[r.ToClusterID]; !known {
				frontier = append(frontier, r.ToClusterID)
			}
		}
	}
	out := map[int64]int64{}
	for _, id := range ids {
		at, ok := next[id]
		if !ok {
			continue
		}
		for hop := 0; hop < 32; hop++ {
			to, ok := next[at]
			if !ok {
				break
			}
			at = to
		}
		out[id] = at
	}
	return out, nil
}

// lookup loads the targets that exist among keys, by kind, one query each.
func lookup(ctx context.Context, q *db.Queries, keys []MentionKey) (map[MentionKey]*Target, error) {
	byKind := map[Kind][]string{}
	idsByKind := map[Kind][]int64{}
	for _, k := range keys {
		if numericKinds[k.Kind] {
			idsByKind[k.Kind] = append(idsByKind[k.Kind], k.id())
		} else {
			byKind[k.Kind] = append(byKind[k.Kind], k.Ref)
		}
	}
	out := map[MentionKey]*Target{}
	put := func(t Target) { out[MentionKey{Kind: t.Kind, Ref: t.Ref}] = &t }

	if refs := byKind[KindCase]; len(refs) > 0 {
		rows, err := q.ListMentionCases(ctx, refs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(caseTarget(r))
		}
	}
	if refs := byKind[KindInquiry]; len(refs) > 0 {
		rows, err := q.ListMentionInquiries(ctx, refs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(inquiryTarget(r))
		}
	}
	if refs := byKind[KindPerson]; len(refs) > 0 {
		rows, err := q.ListMentionPersons(ctx, refs)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(personTarget(r))
		}
	}
	if ids := idsByKind[KindOccurrence]; len(ids) > 0 {
		rows, err := q.ListMentionOccurrences(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(occurrenceTarget(r))
		}
	}
	if ids := idsByKind[KindCluster]; len(ids) > 0 {
		rows, err := q.ListMentionClusters(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(clusterTarget(r))
		}
	}
	if ids := idsByKind[KindRequest]; len(ids) > 0 {
		rows, err := q.ListMentionRequests(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(requestTarget(r))
		}
	}
	if ids := idsByKind[KindReport]; len(ids) > 0 {
		rows, err := q.ListMentionReports(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			put(reportTarget(r))
		}
	}
	return out, nil
}

// Search limits.
const (
	DefaultPerKind = 5
	MaxPerKind     = 20
	maxQueryLen    = 100
)

var digits = regexp.MustCompile(`^[0-9]*$`)

// Search finds mention targets for the @ menu: up to perKind of each of the given kinds (every
// kind when none is given) whose key or text contains q, newest first; with an empty q, the
// newest of each. Persons need a term (an empty one would scan every register), and clusters
// are found by the digits of their id.
func Search(ctx context.Context, q *db.Queries, term string, kinds []Kind, perKind int) ([]Target, error) {
	term = strings.TrimSpace(term)
	if utf8.RuneCountInString(term) > maxQueryLen {
		term = string([]rune(term)[:maxQueryLen])
	}
	if perKind <= 0 {
		perKind = DefaultPerKind
	}
	if perKind > MaxPerKind {
		perKind = MaxPerKind
	}
	if len(kinds) == 0 {
		kinds = Kinds
	}
	want := map[Kind]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	limit := int32(perKind)
	var out []Target

	if want[KindCase] {
		rows, err := q.SearchMentionCases(ctx, db.SearchMentionCasesParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, caseTarget(db.ListMentionCasesRow(r)))
		}
	}
	if want[KindOccurrence] {
		rows, err := q.SearchMentionOccurrences(ctx, db.SearchMentionOccurrencesParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, occurrenceTarget(db.ListMentionOccurrencesRow(r)))
		}
	}
	if want[KindInquiry] {
		rows, err := q.SearchMentionInquiries(ctx, db.SearchMentionInquiriesParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, inquiryTarget(db.ListMentionInquiriesRow(r)))
		}
	}
	if want[KindPerson] && term != "" {
		rows, err := q.SearchMentionPersons(ctx, db.SearchMentionPersonsParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, personTarget(db.ListMentionPersonsRow(r)))
		}
	}
	if want[KindCluster] && digits.MatchString(term) {
		rows, err := q.SearchMentionClusters(ctx, db.SearchMentionClustersParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, clusterTarget(db.ListMentionClustersRow(r)))
		}
	}
	if want[KindRequest] {
		rows, err := q.SearchMentionRequests(ctx, db.SearchMentionRequestsParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, requestTarget(db.ListMentionRequestsRow(r)))
		}
	}
	if want[KindReport] {
		rows, err := q.SearchMentionReports(ctx, db.SearchMentionReportsParams{Q: term, MaxRows: limit})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, reportTarget(db.ListMentionReportsRow(r)))
		}
	}
	return out, nil
}
