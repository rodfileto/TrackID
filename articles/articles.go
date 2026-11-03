// Package articles is the analytical layer's written part (docs/person-article-plan.md): what
// intelligence analysts write about a subject, one shared article per subject in the style of a
// Wikipedia page, with @ mentions of cases, occurrences, inquiries, persons, clusters, requests
// and laudos. Only persons have articles for now.
//
// An article is analysis, not data. Nothing in the matcher, the clusters, the person views or
// the reports reads it, and it asserts no biometric link. Every save is a new revision, kept
// forever (db/migrations/010_articles.sql makes the revisions append-only); the article's current
// mentions are kept apart (article_mentions) for backlinks.
//
// Who may read and write is the API's business: intelligence analysts, none of them blind
// (docs/access-profiles-plan.md). Every intelligence user reaches every subject and target
// today; once organizations and units exist (docs/organizations-plan.md), Search, Resolve and
// Backlinks are where their scope will filter.
package articles

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SubjectKind is what an article is about.
type SubjectKind string

// SubjectPerson is a person's article; its ref is person.person_id.
const SubjectPerson SubjectKind = "PERSON"

// Subject names the subject of one article.
type Subject struct {
	Kind SubjectKind `json:"kind"`
	Ref  string      `json:"ref"`
}

// PersonSubject is the subject of a person's article.
func PersonSubject(personID string) Subject { return Subject{Kind: SubjectPerson, Ref: personID} }

// Kind is a kind of mention target.
type Kind string

// The mention targets and their keys (MentionKey.Ref).
const (
	KindCase       Kind = "CASE"       // biometric_cases.case_id
	KindOccurrence Kind = "OCCURRENCE" // occurrences.id
	KindInquiry    Kind = "INQUIRY"    // occurrences.inquiry_number: the occurrences sharing it
	KindPerson     Kind = "PERSON"     // person.person_id
	KindCluster    Kind = "CLUSTER"    // clusters.id
	KindRequest    Kind = "REQUEST"    // case_requests.id
	KindReport     Kind = "REPORT"     // analysis_reports.id, issued laudos only
)

// Kinds lists every mention kind, in the order the @ menu groups them.
var Kinds = []Kind{KindCase, KindOccurrence, KindInquiry, KindPerson, KindCluster, KindRequest, KindReport}

// numericKinds key on a row id.
var numericKinds = map[Kind]bool{KindOccurrence: true, KindCluster: true, KindRequest: true, KindReport: true}

// MentionKey names one mention target.
type MentionKey struct {
	Kind Kind   `json:"kind"`
	Ref  string `json:"ref"`
}

func (k MentionKey) less(o MentionKey) bool {
	if k.Kind != o.Kind {
		return k.Kind < o.Kind
	}
	return k.Ref < o.Ref
}

func (k MentionKey) id() int64 {
	id, _ := strconv.ParseInt(k.Ref, 10, 64)
	return id
}

// ParseKind checks a mention kind.
func ParseKind(kind string) (Kind, error) {
	for _, k := range Kinds {
		if string(k) == kind {
			return k, nil
		}
	}
	return "", fmt.Errorf("unknown mention kind %q", kind)
}

// ParseKey checks a mention target's kind and key: a known kind, a key of at most 200
// characters, and a positive row id for the kinds keyed on one.
func ParseKey(kind, ref string) (MentionKey, error) {
	k, err := ParseKind(kind)
	if err != nil {
		return MentionKey{}, err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" || len(ref) > maxRefLen || strings.ContainsRune(ref, 0) {
		return MentionKey{}, fmt.Errorf("mention %s has an invalid key", kind)
	}
	if numericKinds[k] {
		id, err := strconv.ParseInt(ref, 10, 64)
		if err != nil || id <= 0 {
			return MentionKey{}, fmt.Errorf("mention %s key %q is not an id", kind, ref)
		}
		ref = strconv.FormatInt(id, 10)
	}
	return MentionKey{Kind: k, Ref: ref}, nil
}

var (
	// ErrSubjectNotFound is a subject that does not exist (no such person).
	ErrSubjectNotFound = errors.New("articles: subject not found")
	// ErrRevisionNotFound is a revision number the article does not have.
	ErrRevisionNotFound = errors.New("articles: revision not found")
	// ErrTemplateNotFound is a generated draft that does not exist or is another subject's.
	ErrTemplateNotFound = errors.New("articles: generated draft not found")
	// ErrConflict is a save based on a revision that is no longer the current one.
	ErrConflict = errors.New("articles: the article changed since this edit began")
)

// MentionError is a save that mentions targets that do not exist.
type MentionError struct{ Missing []MentionKey }

func (e *MentionError) Error() string {
	parts := make([]string, len(e.Missing))
	for i, k := range e.Missing {
		parts[i] = string(k.Kind) + " " + k.Ref
	}
	return "articles: mentioned targets not found: " + strings.Join(parts, ", ")
}
