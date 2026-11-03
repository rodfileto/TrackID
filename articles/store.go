package articles

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rodfileto/trackid/db"
)

// maxSummaryLen is the longest edit summary kept, in characters.
const maxSummaryLen = 300

// Author is who wrote a revision.
type Author struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// Revision is one saved version of an article. Body and Text are empty for a revision whose
// text was redacted (Redacted), and in the history's list.
type Revision struct {
	ID          int64     `json:"id"`
	Number      int       `json:"number"`
	ParentID    int64     `json:"parentId,omitempty"`
	SHA256      string    `json:"sha256,omitempty"`
	Chars       int       `json:"chars"`
	SizeDelta   int       `json:"sizeDelta"`
	EditSummary string    `json:"editSummary,omitempty"`
	Author      Author    `json:"author"`
	CreatedAt   time.Time `json:"createdAt"`
	Redacted    bool      `json:"redacted,omitempty"`
	// TemplateID is the draft generated from the records this revision was saved from, if any
	// (GenerateTemplate).
	TemplateID int64           `json:"templateId,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Text       string          `json:"text,omitempty"`
}

// Article is a subject's article as read: its current revision (nil while nobody has written
// one) and each of its mentions as it reads now.
type Article struct {
	Subject   Subject    `json:"subject"`
	Revision  *Revision  `json:"revision"`
	Mentions  []Resolved `json:"mentions"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

func authorName(first, last string) string { return strings.TrimSpace(first + " " + last) }

// subjectExists checks the subject is there: the person, for a person's article.
func subjectExists(ctx context.Context, q *db.Queries, s Subject) error {
	switch s.Kind {
	case SubjectPerson:
		if strings.TrimSpace(s.Ref) == "" {
			return ErrSubjectNotFound
		}
		if _, err := q.GetPersonByPersonID(ctx, s.Ref); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSubjectNotFound
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("articles: unknown subject kind %q", s.Kind)
}

// Get reads a subject's article. A subject nobody has written about yet has an Article with no
// Revision. Returns ErrSubjectNotFound if the subject does not exist.
func Get(ctx context.Context, sqlDB *sql.DB, s Subject) (Article, error) {
	q := db.New(sqlDB)
	if err := subjectExists(ctx, q, s); err != nil {
		return Article{}, err
	}
	return load(ctx, q, s)
}

func load(ctx context.Context, q *db.Queries, s Subject) (Article, error) {
	out := Article{Subject: s, Mentions: []Resolved{}}
	row, err := q.GetArticleBySubject(ctx, db.GetArticleBySubjectParams{SubjectKind: string(s.Kind), SubjectRef: s.Ref})
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return Article{}, err
	}
	out.CreatedAt, out.UpdatedAt = &row.CreatedAt, &row.UpdatedAt
	if !row.CurrentRevisionID.Valid {
		return out, nil
	}
	number, err := q.GetArticleRevisionNumber(ctx, row.CurrentRevisionID.Int64)
	if err != nil {
		return Article{}, err
	}
	rev, err := revision(ctx, q, row.ID, int(number))
	if err != nil {
		return Article{}, err
	}
	out.Revision = &rev
	if len(rev.Body) > 0 {
		var doc Node
		if err := json.Unmarshal(rev.Body, &doc); err != nil {
			return Article{}, fmt.Errorf("articles: stored body of revision %d: %w", rev.ID, err)
		}
		var keys []MentionKey
		for _, m := range MentionsIn(doc) {
			keys = append(keys, m.MentionKey)
		}
		if out.Mentions, err = Resolve(ctx, q, keys); err != nil {
			return Article{}, err
		}
	}
	return out, nil
}

func revision(ctx context.Context, q *db.Queries, articleID int64, number int) (Revision, error) {
	row, err := q.GetArticleRevision(ctx, db.GetArticleRevisionParams{ArticleID: articleID, RevisionNumber: int32(number)})
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrRevisionNotFound
	}
	if err != nil {
		return Revision{}, err
	}
	rev := Revision{
		ID: row.ID, Number: int(row.RevisionNumber), ParentID: row.ParentRevisionID.Int64,
		SHA256: hex.EncodeToString(row.BodySha256), Chars: int(row.BodyChars), EditSummary: row.EditSummary.String,
		Author:     Author{ID: row.AuthorID, Username: row.AuthorUsername, Name: authorName(row.AuthorFirstName, row.AuthorLastName)},
		CreatedAt:  row.CreatedAt,
		Redacted:   !row.Body.Valid,
		TemplateID: row.TemplateID.Int64,
	}
	if row.Body.Valid {
		rev.Body = row.Body.RawMessage
		rev.Text = row.BodyText.String
	}
	return rev, nil
}

// GetRevision reads one revision of a subject's article by its number, text included.
func GetRevision(ctx context.Context, sqlDB *sql.DB, s Subject, number int) (Revision, error) {
	q := db.New(sqlDB)
	row, err := q.GetArticleBySubject(ctx, db.GetArticleBySubjectParams{SubjectKind: string(s.Kind), SubjectRef: s.Ref})
	if errors.Is(err, sql.ErrNoRows) {
		return Revision{}, ErrRevisionNotFound
	}
	if err != nil {
		return Revision{}, err
	}
	return revision(ctx, q, row.ID, number)
}

// History lists an article's revisions, newest first, without their text. Each one's SizeDelta
// is its size less its parent's. An article nobody has written has an empty history.
func History(ctx context.Context, sqlDB *sql.DB, s Subject) ([]Revision, error) {
	q := db.New(sqlDB)
	row, err := q.GetArticleBySubject(ctx, db.GetArticleBySubjectParams{SubjectKind: string(s.Kind), SubjectRef: s.Ref})
	if errors.Is(err, sql.ErrNoRows) {
		return []Revision{}, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.ListArticleRevisions(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	chars := map[int64]int{}
	for _, r := range rows {
		chars[r.ID] = int(r.BodyChars)
	}
	out := make([]Revision, len(rows))
	for i, r := range rows {
		out[i] = Revision{
			ID: r.ID, Number: int(r.RevisionNumber), ParentID: r.ParentRevisionID.Int64,
			Chars: int(r.BodyChars), SizeDelta: int(r.BodyChars) - chars[r.ParentRevisionID.Int64],
			EditSummary: r.EditSummary.String,
			Author:      Author{ID: r.AuthorID, Username: r.AuthorUsername, Name: authorName(r.AuthorFirstName, r.AuthorLastName)},
			CreatedAt:   r.CreatedAt,
			Redacted:    !r.HasBody,
			TemplateID:  r.TemplateID.Int64,
		}
	}
	return out, nil
}

// SaveInput is one edit of an article.
type SaveInput struct {
	Subject Subject
	// BaseRevisionID is the revision the edit started from: 0 for a first article.
	BaseRevisionID int64
	Body           json.RawMessage
	EditSummary    string
	AuthorID       int64
	// TemplateID names the generated draft (GenerateTemplate) the edit started from, 0 for none.
	// It must be a draft of the same subject.
	TemplateID int64
}

// Save writes an edit as the article's new current revision, in one transaction: the body is
// checked against the schema (ParseBody), every new mention must name a target that exists
// (mentions already in the current revision may stay, even if their target is gone), and the
// article's mentions are rewritten. An edit that changes nothing writes no revision and returns
// created false.
//
// Errors: ErrSubjectNotFound; a *BodyError; a *MentionError; ErrConflict when BaseRevisionID is
// not the current revision (another analyst saved first): the caller reloads, merges and saves
// again.
func Save(ctx context.Context, sqlDB *sql.DB, in SaveInput) (rev Revision, created bool, err error) {
	body, err := ParseBody(in.Body)
	if err != nil {
		return Revision{}, false, err
	}
	summary := strings.TrimSpace(in.EditSummary)
	if !utf8.ValidString(summary) || strings.ContainsRune(summary, 0) {
		return Revision{}, false, &BodyError{Reason: "edit summary is not valid text"}
	}
	if utf8.RuneCountInString(summary) > maxSummaryLen {
		summary = string([]rune(summary)[:maxSummaryLen])
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Revision{}, false, err
	}
	defer tx.Rollback()
	q := db.New(tx)

	if err := subjectExists(ctx, q, in.Subject); err != nil {
		return Revision{}, false, err
	}
	kind, ref := string(in.Subject.Kind), in.Subject.Ref
	if err := q.EnsureArticle(ctx, db.EnsureArticleParams{SubjectKind: kind, SubjectRef: ref}); err != nil {
		return Revision{}, false, err
	}
	article, err := q.LockArticle(ctx, db.LockArticleParams{SubjectKind: kind, SubjectRef: ref})
	if err != nil {
		return Revision{}, false, err
	}
	if article.CurrentRevisionID.Int64 != in.BaseRevisionID {
		return Revision{}, false, ErrConflict
	}

	number := 1
	if article.CurrentRevisionID.Valid {
		n, err := q.GetArticleRevisionNumber(ctx, article.CurrentRevisionID.Int64)
		if err != nil {
			return Revision{}, false, err
		}
		current, err := revision(ctx, q, article.ID, int(n))
		if err != nil {
			return Revision{}, false, err
		}
		if current.SHA256 == hex.EncodeToString(body.SHA256[:]) {
			return current, false, nil
		}
		number = int(n) + 1
	}

	templateID := sql.NullInt64{}
	if in.TemplateID != 0 {
		tpl, err := q.GetArticleTemplate(ctx, in.TemplateID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (tpl.SubjectKind != kind || tpl.SubjectRef != ref)) {
			return Revision{}, false, ErrTemplateNotFound
		}
		if err != nil {
			return Revision{}, false, err
		}
		templateID = sql.NullInt64{Int64: tpl.ID, Valid: true}
	}

	if err := checkMentions(ctx, q, article.ID, body.Mentions); err != nil {
		return Revision{}, false, err
	}

	inserted, err := q.InsertArticleRevision(ctx, db.InsertArticleRevisionParams{
		ArticleID: article.ID, RevisionNumber: int32(number), ParentRevisionID: article.CurrentRevisionID,
		BodySha256: body.SHA256[:], BodyChars: int32(body.Chars),
		EditSummary: sql.NullString{String: summary, Valid: summary != ""}, AuthorID: in.AuthorID,
		TemplateID: templateID,
	})
	if err != nil {
		return Revision{}, false, err
	}
	if err := q.InsertArticleRevisionBody(ctx, db.InsertArticleRevisionBodyParams{
		RevisionID: inserted.ID, Body: body.Canonical, BodyText: body.Text,
	}); err != nil {
		return Revision{}, false, err
	}
	if err := q.SetArticleCurrentRevision(ctx, db.SetArticleCurrentRevisionParams{
		ID: article.ID, CurrentRevisionID: sql.NullInt64{Int64: inserted.ID, Valid: true},
	}); err != nil {
		return Revision{}, false, err
	}
	if err := q.DeleteArticleMentions(ctx, article.ID); err != nil {
		return Revision{}, false, err
	}
	if len(body.Mentions) > 0 {
		params := db.InsertArticleMentionsParams{ArticleID: article.ID}
		for _, m := range body.Mentions {
			params.Kinds = append(params.Kinds, string(m.Kind))
			params.Refs = append(params.Refs, m.Ref)
			params.Counts = append(params.Counts, int32(m.Count))
		}
		if err := q.InsertArticleMentions(ctx, params); err != nil {
			return Revision{}, false, err
		}
	}

	rev, err = revision(ctx, q, article.ID, number)
	if err != nil {
		return Revision{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Revision{}, false, err
	}
	return rev, true, nil
}

// checkMentions refuses mentions of targets that do not exist, other than those the current
// revision already holds: a target removed since must not block every later edit.
func checkMentions(ctx context.Context, q *db.Queries, articleID int64, mentions []MentionCount) error {
	if len(mentions) == 0 {
		return nil
	}
	held, err := q.ListArticleMentionKeys(ctx, articleID)
	if err != nil {
		return err
	}
	had := map[MentionKey]bool{}
	for _, h := range held {
		had[MentionKey{Kind: Kind(h.Kind), Ref: h.Ref}] = true
	}
	var fresh []MentionKey
	for _, m := range mentions {
		if !had[m.MentionKey] {
			fresh = append(fresh, m.MentionKey)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	resolved, err := Resolve(ctx, q, fresh)
	if err != nil {
		return err
	}
	var missing []MentionKey
	for _, r := range resolved {
		if !r.Found && r.MergedInto == "" {
			missing = append(missing, r.MentionKey)
		}
	}
	if len(missing) > 0 {
		return &MentionError{Missing: missing}
	}
	return nil
}

// Backlink is one article whose current revision mentions a target.
type Backlink struct {
	Subject      Subject            `json:"subject"`
	SubjectLabel string             `json:"subjectLabel"`
	Mentions     []MentionCountJSON `json:"mentions"`
	UpdatedAt    time.Time          `json:"updatedAt"`
}

// MentionCountJSON is one target a backlink's article mentions, and how many times.
type MentionCountJSON struct {
	Kind  Kind   `json:"kind"`
	Ref   string `json:"ref"`
	Count int    `json:"count"`
}

// Backlinks lists the articles that mention any of the targets, most recently edited first, one
// entry per article with the targets it mentions. An article never links back to itself.
func Backlinks(ctx context.Context, sqlDB *sql.DB, targets []MentionKey, self *Subject) ([]Backlink, error) {
	out := []Backlink{}
	if len(targets) == 0 {
		return out, nil
	}
	q := db.New(sqlDB)
	params := db.ListBacklinksParams{}
	for _, t := range targets {
		params.Kinds = append(params.Kinds, string(t.Kind))
		params.Refs = append(params.Refs, t.Ref)
	}
	rows, err := q.ListBacklinks(ctx, params)
	if err != nil {
		return nil, err
	}
	index := map[Subject]int{}
	var persons []string
	for _, r := range rows {
		s := Subject{Kind: SubjectKind(r.SubjectKind), Ref: r.SubjectRef}
		if self != nil && s == *self {
			continue
		}
		i, ok := index[s]
		if !ok {
			i = len(out)
			index[s] = i
			out = append(out, Backlink{Subject: s, SubjectLabel: s.Ref, UpdatedAt: r.UpdatedAt})
			if s.Kind == SubjectPerson {
				persons = append(persons, s.Ref)
			}
		}
		out[i].Mentions = append(out[i].Mentions, MentionCountJSON{Kind: Kind(r.Kind), Ref: r.Ref, Count: int(r.MentionCount)})
	}
	if len(persons) > 0 {
		names, err := q.ListMentionPersons(ctx, persons)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if i, ok := index[PersonSubject(n.PersonID)]; ok && n.Name != "" {
				out[i].SubjectLabel = n.Name
			}
		}
	}
	return out, nil
}

// CaseTargets lists what an article can mention of one case: the case itself, its occurrence and
// that occurrence's inquiry, its requests and its issued laudos. Backlinks to them all are the
// case's backlinks. Empty for a case that does not exist.
func CaseTargets(ctx context.Context, sqlDB *sql.DB, caseID string) ([]MentionKey, error) {
	rows, err := db.New(sqlDB).ListCaseMentionTargets(ctx, caseID)
	if err != nil {
		return nil, err
	}
	out := make([]MentionKey, len(rows))
	for i, r := range rows {
		out[i] = MentionKey{Kind: Kind(r.Kind), Ref: r.Ref}
	}
	return out, nil
}
