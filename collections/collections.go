// Package collections holds the material a case did not receive with a request: what an examiner
// collected at a scene on call, before any request formalized the examination (case_collections,
// db/migrations/019_scene_collection.sql), and the physical items of a case, received or collected,
// with the seals they were kept under (case_vestiges, case_vestige_seals).
//
// Every piece of material has one origin: the request that sent it in (case_files.case_request_id)
// or the collection that gathered it (case_files.collection_id). A scene collection is the one way
// to open a case without a request; the request that formalizes it joins the case later
// (requests.Register, requests.Create). See docs/scene-collection-plan.md.
package collections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/occurrences"
)

// ValidationError is a refusal for the input.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// ErrCaseNotFound is returned for an unknown case.
var ErrCaseNotFound = errors.New("case not found")

// Report modes (biometric_cases.report_mode): a case's analyses reported in one laudo, or each in its own.
const (
	ReportSeparate = "SEPARATE"
	ReportTogether = "TOGETHER"
)

// VestigeInput is one physical item: what it is, and the number of the seal it is kept under, if sealed.
type VestigeInput struct {
	Description string `json:"description"`
	SealNumber  string `json:"sealNumber"`
}

// RegisterInput is a scene examined on call: the occurrence (OccurrenceID, or Occurrence to create),
// the case it opens, who collected and when, and what was collected.
type RegisterInput struct {
	OccurrenceID     *int64
	Occurrence       *occurrences.CreateInput
	Description      string // the case's
	CollectedFrom    time.Time
	CollectedTo      *time.Time
	CollectedBy      int64 // the examiner who collected; checking they examine fingerprints is the caller's job
	NothingCollected bool
	Notes            string
	ReportMode       string // SEPARATE (default) or TOGETHER
	Vestiges         []VestigeInput
	CalledAt         *time.Time // when the team was called out
	OtherOfficers    string     // others present, who are not users
	Team             []int64    // users at the scene besides the collector
	// SceneNoteID is the field note (scene_notes) this collection is registered from, 0 for none:
	// the note is marked converted and its audio notes become the collection's.
	SceneNoteID int64
}

// Registered is what registering a scene made.
type Registered struct {
	CaseID       string    `json:"caseId"`
	CollectionID int64     `json:"collectionId"`
	OccurrenceID int64     `json:"occurrenceId"`
	Vestiges     []Vestige `json:"vestiges"`
}

// Collection is a scene examined on call, as the case page shows it.
type Collection struct {
	ID               int64       `json:"id"`
	Sequence         int16       `json:"sequence"`
	CollectedFrom    time.Time   `json:"collectedFrom"`
	CollectedTo      *time.Time  `json:"collectedTo,omitempty"`
	CollectedBy      string      `json:"collectedBy"`
	CollectedByID    int64       `json:"collectedById"`
	NothingCollected bool        `json:"nothingCollected"`
	Notes            string      `json:"notes,omitempty"`
	RegisteredBy     string      `json:"registeredBy,omitempty"`
	CreatedAt        time.Time   `json:"createdAt"`
	Place            string      `json:"place,omitempty"`   // when the scene is not the occurrence's place
	Request          *RequestRef `json:"request,omitempty"` // the request that formalized it; nil while awaited
	CalledAt         *time.Time  `json:"calledAt,omitempty"`
	OtherOfficers    string      `json:"otherOfficers,omitempty"`
	Team             []Member    `json:"team"`
	AudioNotes       []AudioNote `json:"audioNotes"`
}

// Member is a user who was at a scene besides its collector.
type Member struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// AudioNote is an audio the examiner recorded at the scene: a file of the case, not evidence.
type AudioNote struct {
	FileID      int64     `json:"fileId"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	SHA256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"createdAt"`
}

// RequestRef names a request by its document.
type RequestRef struct {
	ID                int64  `json:"id"`
	DocumentReference string `json:"documentReference"`
	ReceivedOn        string `json:"receivedOn"` // YYYY-MM-DD
}

// Vestige is a physical item of a case, numbered V1, V2, ... with its origin and its seals.
type Vestige struct {
	ID           int64   `json:"id"`
	Sequence     int16   `json:"sequence"`
	Label        string  `json:"label"` // "V1"
	Description  string  `json:"description"`
	RequestID    *int64  `json:"requestId,omitempty"`    // received with this request
	CollectionID *int64  `json:"collectionId,omitempty"` // or collected in this collection
	Seals        []Seal  `json:"seals"`
	AskedBy      []int64 `json:"askedBy"` // the requests whose questions are about it
}

// Seal is one seal a vestige was kept under.
type Seal struct {
	ID        int64      `json:"id"`
	Number    string     `json:"number"`
	AppliedAt time.Time  `json:"appliedAt"`
	AppliedBy string     `json:"appliedBy,omitempty"` // empty when applied outside
	BrokenAt  *time.Time `json:"brokenAt,omitempty"`
	BrokenBy  string     `json:"brokenBy,omitempty"`
}

// Origins is a case's report mode, collections and vestiges.
type Origins struct {
	ReportMode  string       `json:"reportMode"`
	Collections []Collection `json:"collections"`
	Vestiges    []Vestige    `json:"vestiges"`
}

// Awaiting is a collection still awaiting the request that formalizes it.
type Awaiting struct {
	CollectionID     int64     `json:"collectionId"`
	Sequence         int16     `json:"sequence"`
	CaseID           string    `json:"caseId"`
	CollectedFrom    time.Time `json:"collectedFrom"`
	CollectedBy      string    `json:"collectedBy"`
	NothingCollected bool      `json:"nothingCollected"`
	Occurrence       string    `json:"occurrence,omitempty"` // TrackID's number, else the agency's, else the inquiry's
}

// maxAttempts bounds how often Register retries when a concurrent create took the number it gave
// the new case or occurrence.
const maxAttempts = 3

// Register records a scene examined on call, in one transaction: the occurrence (found by
// in.OccurrenceID, or in.Occurrence created now), a FINGERPRINT case of the occurrence's nature
// linked to it, the collection, and its vestiges with their seals, applied at the scene by the
// collector. registeredBy is who registers it, the collector or a coordinator.
func Register(ctx context.Context, sqlDB *sql.DB, in RegisterInput, registeredBy int64, now time.Time) (Registered, error) {
	if err := checkRegister(&in, now); err != nil {
		return Registered{}, err
	}
	for attempt := 1; ; attempt++ {
		out, err := register(ctx, sqlDB, in, registeredBy, now)
		if err == nil || attempt >= maxAttempts || !(cases.IsDuplicateNumber(err) || occurrences.IsDuplicateNumber(err)) {
			return out, err
		}
	}
}

func checkRegister(in *RegisterInput, now time.Time) error {
	in.Description = strings.TrimSpace(in.Description)
	in.Notes = strings.TrimSpace(in.Notes)
	switch {
	case in.OccurrenceID == nil && in.Occurrence == nil:
		return ValidationError("a scene belongs to an occurrence: choose one or register it")
	case in.CollectedBy == 0:
		return ValidationError("collectedBy is required")
	case in.CollectedFrom.IsZero():
		return ValidationError("collectedFrom is required")
	case in.CollectedFrom.After(now.Add(5 * time.Minute)):
		return ValidationError("collectedFrom cannot be in the future")
	case in.CollectedTo != nil && in.CollectedTo.Before(in.CollectedFrom):
		return ValidationError("collectedTo cannot be before collectedFrom")
	case in.NothingCollected && len(in.Vestiges) > 0:
		return ValidationError("a scene where nothing was collected has no vestiges")
	case in.CalledAt != nil && in.CalledAt.After(in.CollectedFrom):
		return ValidationError("the team is called out before it collects")
	}
	in.OtherOfficers = strings.TrimSpace(in.OtherOfficers)
	team := in.Team[:0:0]
	for _, id := range in.Team {
		if id != in.CollectedBy && id > 0 && !slices.Contains(team, id) {
			team = append(team, id)
		}
	}
	in.Team = team
	mode, err := ParseReportMode(in.ReportMode)
	if err != nil {
		return err
	}
	in.ReportMode = mode
	for i := range in.Vestiges {
		in.Vestiges[i].Description = strings.TrimSpace(in.Vestiges[i].Description)
		in.Vestiges[i].SealNumber = strings.TrimSpace(in.Vestiges[i].SealNumber)
		if in.Vestiges[i].Description == "" {
			return ValidationError("each vestige needs a description")
		}
	}
	return nil
}

// ParseReportMode reads a report mode, SEPARATE when not given.
func ParseReportMode(mode string) (string, error) {
	switch m := strings.ToUpper(strings.TrimSpace(mode)); m {
	case "":
		return ReportSeparate, nil
	case ReportSeparate, ReportTogether:
		return m, nil
	}
	return "", ValidationError("reportMode must be SEPARATE or TOGETHER")
}

func register(ctx context.Context, sqlDB *sql.DB, in RegisterInput, registeredBy int64, now time.Time) (Registered, error) {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Registered{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)

	var occurrenceID int64
	if in.OccurrenceID != nil {
		occurrenceID = *in.OccurrenceID
	} else {
		o := *in.Occurrence
		o.CaseID = ""
		created, err := occurrences.CreateWith(ctx, q, o, now)
		var vErr occurrences.ValidationError
		if errors.As(err, &vErr) {
			return Registered{}, ValidationError(vErr.Error())
		}
		if err != nil {
			return Registered{}, err
		}
		occurrenceID = created.ID
	}
	natures, err := q.ListOccurrenceCaseTypes(ctx, occurrenceID)
	if err != nil {
		return Registered{}, err
	}
	if len(natures) == 0 {
		return Registered{}, ValidationError("occurrence not found")
	}
	description := in.Description
	if description == "" {
		description = "Exame de local"
	}
	c, caseRowID, err := cases.CreateWith(ctx, q, cases.CreateInput{CaseType: natures[0], Modality: "FINGERPRINT", Description: description})
	var cErr cases.ValidationError
	if errors.As(err, &cErr) {
		return Registered{}, ValidationError(cErr.Error())
	}
	if err != nil {
		return Registered{}, err
	}
	if _, err := q.SetBiometricCaseOccurrenceIfNone(ctx, db.SetBiometricCaseOccurrenceIfNoneParams{ID: caseRowID,
		OccurrenceID: sql.NullInt64{Int64: occurrenceID, Valid: true}}); err != nil {
		return Registered{}, err
	}
	if _, err := q.SetCaseReportMode(ctx, db.SetCaseReportModeParams{ID: caseRowID, ReportMode: in.ReportMode}); err != nil {
		return Registered{}, err
	}
	if _, err := q.LockBiometricCase(ctx, caseRowID); err != nil {
		return Registered{}, err
	}
	params := db.InsertCaseCollectionParams{BiometricCaseID: caseRowID, CollectedFrom: in.CollectedFrom, CollectedBy: in.CollectedBy,
		NothingCollected: in.NothingCollected, Notes: sql.NullString{String: in.Notes, Valid: in.Notes != ""},
		RegisteredBy:  sql.NullInt64{Int64: registeredBy, Valid: registeredBy != 0},
		OtherOfficers: sql.NullString{String: in.OtherOfficers, Valid: in.OtherOfficers != ""}}
	if in.CollectedTo != nil {
		params.CollectedTo = sql.NullTime{Time: *in.CollectedTo, Valid: true}
	}
	if in.CalledAt != nil {
		params.CalledAt = sql.NullTime{Time: *in.CalledAt, Valid: true}
	}
	col, err := q.InsertCaseCollection(ctx, params)
	if err != nil && strings.Contains(err.Error(), "case_collections_collected_by_fkey") {
		return Registered{}, ValidationError("the collector is not a user")
	}
	if err != nil {
		return Registered{}, fmt.Errorf("collections: insert: %w", err)
	}
	// Sealed at the scene, by whoever collected, when they finished.
	sealedAt := in.CollectedFrom
	if in.CollectedTo != nil {
		sealedAt = *in.CollectedTo
	}
	out := Registered{CaseID: c.CaseID, CollectionID: col.ID, OccurrenceID: occurrenceID, Vestiges: []Vestige{}}
	for _, v := range in.Vestiges {
		added, err := addVestige(ctx, q, caseRowID, v, sql.NullInt64{}, sql.NullInt64{Int64: col.ID, Valid: true},
			sealedAt, sql.NullInt64{Int64: in.CollectedBy, Valid: true}, registeredBy)
		if err != nil {
			return Registered{}, err
		}
		out.Vestiges = append(out.Vestiges, added)
	}
	for _, id := range in.Team {
		if err := q.InsertCollectionMember(ctx, db.InsertCollectionMemberParams{CollectionID: col.ID, UserID: id}); err != nil {
			if strings.Contains(err.Error(), "case_collection_members_user_id_fkey") {
				return Registered{}, ValidationError("a member of the team is not a user")
			}
			return Registered{}, fmt.Errorf("collections: team: %w", err)
		}
	}
	if in.SceneNoteID != 0 {
		if err := convertNote(ctx, q, in.SceneNoteID, caseRowID, col.ID, in.CollectedBy, registeredBy); err != nil {
			return Registered{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Registered{}, err
	}
	return out, nil
}

// convertNote marks a field note as the collection's and takes its audio notes as files of the
// case. The note must be the collector's or the registrar's, and not converted already.
func convertNote(ctx context.Context, q *db.Queries, noteID, caseRowID, collectionID, collector, registeredBy int64) error {
	note, err := q.GetSceneNote(ctx, noteID)
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the field note does not exist")
	}
	if err != nil {
		return err
	}
	if note.AuthorID != collector && note.AuthorID != registeredBy {
		return ValidationError("the field note is someone else's")
	}
	n, err := q.SetSceneNoteCollection(ctx, db.SetSceneNoteCollectionParams{ID: noteID, CollectionID: collectionID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ValidationError("the field note is already a collection")
	}
	audios, err := q.ListSceneNoteAudios(ctx, noteID)
	if err != nil {
		return err
	}
	for i, a := range audios {
		name := strings.TrimSpace(a.Title.String)
		if name == "" {
			name = fmt.Sprintf("nota-%d", i+1)
		}
		// The same recording twice in one case is one file.
		if _, err := q.InsertCollectionNoteFile(ctx, db.InsertCollectionNoteFileParams{BiometricCaseID: caseRowID, HashID: a.Sha256,
			Filename: name, StorageRef: a.StorageRef, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
			CollectionID: collectionID}); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("collections: audio note: %w", err)
		}
	}
	return nil
}

// addVestige numbers a vestige in the case (whose lock the caller holds) and records its seal.
func addVestige(ctx context.Context, q *db.Queries, caseRowID int64, v VestigeInput, request, collection sql.NullInt64,
	sealedAt time.Time, sealedBy sql.NullInt64, createdBy int64) (Vestige, error) {
	row, err := q.InsertCaseVestige(ctx, db.InsertCaseVestigeParams{BiometricCaseID: caseRowID, Description: v.Description,
		CaseRequestID: request, CollectionID: collection, CreatedBy: sql.NullInt64{Int64: createdBy, Valid: createdBy != 0}})
	if err != nil {
		return Vestige{}, fmt.Errorf("collections: insert vestige: %w", err)
	}
	out := Vestige{ID: row.ID, Sequence: row.Sequence, Label: fmt.Sprintf("V%d", row.Sequence), Description: v.Description,
		Seals: []Seal{}, AskedBy: []int64{}}
	if request.Valid {
		out.RequestID = &request.Int64
	}
	if collection.Valid {
		out.CollectionID = &collection.Int64
	}
	if v.SealNumber != "" {
		id, err := q.InsertVestigeSeal(ctx, db.InsertVestigeSealParams{VestigeID: row.ID, SealNumber: v.SealNumber, AppliedAt: sealedAt, AppliedBy: sealedBy})
		if err != nil {
			return Vestige{}, fmt.Errorf("collections: insert seal: %w", err)
		}
		out.Seals = append(out.Seals, Seal{ID: id, Number: v.SealNumber, AppliedAt: sealedAt})
	}
	return out, nil
}

// Origin is where a case's material came from: one of its requests or one of its collections, and
// optionally the vestige it was taken from (of that same origin).
type Origin struct {
	RequestID    int64
	CollectionID int64
	VestigeID    int64
}

// caseRow finds a case by its case_id.
func caseRow(ctx context.Context, q *db.Queries, caseID string) (int64, error) {
	c, err := q.GetCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrCaseNotFound
	}
	return c.ID, err
}

// CheckOrigin reports whether o names exactly one origin of the case (one of its requests or one of
// its collections) and, if it names a vestige, one of the case's from that same origin.
func CheckOrigin(ctx context.Context, sqlDB *sql.DB, caseID string, o Origin) error {
	q := db.New(sqlDB)
	id, err := caseRow(ctx, q, caseID)
	if err != nil {
		return err
	}
	return checkOrigin(ctx, q, id, o)
}

func checkOrigin(ctx context.Context, q *db.Queries, caseRowID int64, o Origin) error {
	switch {
	case (o.RequestID == 0) == (o.CollectionID == 0):
		return ValidationError("say where the material came from: one of the case's requests or one of its collections")
	case o.RequestID != 0:
		if _, err := q.GetCaseRequestOfCase(ctx, db.GetCaseRequestOfCaseParams{CaseRequestID: o.RequestID, BiometricCaseID: caseRowID}); errors.Is(err, sql.ErrNoRows) {
			return ValidationError("the request is not one of this case")
		} else if err != nil {
			return err
		}
	default:
		col, err := q.GetCaseCollectionOfCase(ctx, db.GetCaseCollectionOfCaseParams{ID: o.CollectionID, BiometricCaseID: caseRowID})
		if errors.Is(err, sql.ErrNoRows) {
			return ValidationError("the collection is not one of this case")
		}
		if err != nil {
			return err
		}
		if col.NothingCollected {
			return ValidationError("nothing was collected at that scene")
		}
	}
	if o.VestigeID == 0 {
		return nil
	}
	v, err := q.GetCaseVestigeOfCase(ctx, db.GetCaseVestigeOfCaseParams{ID: o.VestigeID, BiometricCaseID: caseRowID})
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the vestige is not one of this case")
	}
	if err != nil {
		return err
	}
	if v.CaseRequestID.Int64 != o.RequestID || v.CollectionID.Int64 != o.CollectionID {
		return ValidationError("the vestige did not come from that origin")
	}
	return nil
}

// Attach records where a file of the case, with no origin yet, came from (CheckOrigin), and the
// vestige it was taken from. A file another request sent in or a collection gathered is refused.
func Attach(ctx context.Context, sqlDB *sql.DB, caseID string, fileID int64, o Origin) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	id, err := caseRow(ctx, q, caseID)
	if err != nil {
		return err
	}
	if err := checkOrigin(ctx, q, id, o); err != nil {
		return err
	}
	f, err := q.GetCaseFileOrigin(ctx, db.GetCaseFileOriginParams{ID: fileID, BiometricCaseID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the file is not an evidence file of this case")
	}
	if err != nil {
		return err
	}
	switch {
	case f.CaseRequestID.Valid && f.CaseRequestID.Int64 == o.RequestID, f.CollectionID.Valid && f.CollectionID.Int64 == o.CollectionID:
		// Already from there: only the vestige may change.
	case f.CaseRequestID.Valid || f.CollectionID.Valid:
		return ValidationError("the file already came from somewhere else")
	case o.RequestID != 0:
		if _, err := q.AttachRequestFiles(ctx, db.AttachRequestFilesParams{CaseRequestID: o.RequestID, BiometricCaseID: id, FileIds: []int64{fileID}}); err != nil {
			return err
		}
	default:
		if _, err := q.AttachCollectionFiles(ctx, db.AttachCollectionFilesParams{CollectionID: o.CollectionID, BiometricCaseID: id, FileIds: []int64{fileID}}); err != nil {
			return err
		}
	}
	if o.VestigeID != 0 {
		if _, err := q.SetFileVestige(ctx, db.SetFileVestigeParams{ID: fileID, BiometricCaseID: id, VestigeID: sql.NullInt64{Int64: o.VestigeID, Valid: true}}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddVestigeInput is a vestige added to a case after it was opened: what it is, its seal, and its
// origin (one of the case's requests or collections; a vestige of its own has no VestigeID).
type AddVestigeInput struct {
	VestigeInput
	RequestID    int64 `json:"requestId"`
	CollectionID int64 `json:"collectionId"`
}

// AddVestige records a physical item of a case: one a request sent in (sealed by the agency that
// sent it, so its seal has no applier here) or one more from a collection (sealed by its collector).
func AddVestige(ctx context.Context, sqlDB *sql.DB, caseID string, in AddVestigeInput, by int64, now time.Time) (Vestige, error) {
	in.Description = strings.TrimSpace(in.Description)
	in.SealNumber = strings.TrimSpace(in.SealNumber)
	if in.Description == "" {
		return Vestige{}, ValidationError("a vestige needs a description")
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return Vestige{}, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	id, err := caseRow(ctx, q, caseID)
	if err != nil {
		return Vestige{}, err
	}
	if err := checkOrigin(ctx, q, id, Origin{RequestID: in.RequestID, CollectionID: in.CollectionID}); err != nil {
		return Vestige{}, err
	}
	if _, err := q.LockBiometricCase(ctx, id); err != nil {
		return Vestige{}, err
	}
	request := sql.NullInt64{Int64: in.RequestID, Valid: in.RequestID != 0}
	collection := sql.NullInt64{Int64: in.CollectionID, Valid: in.CollectionID != 0}
	sealedBy := sql.NullInt64{}
	if collection.Valid {
		sealedBy = sql.NullInt64{Int64: by, Valid: by != 0}
	}
	v, err := addVestige(ctx, q, id, in.VestigeInput, request, collection, now, sealedBy, by)
	if err != nil {
		return Vestige{}, err
	}
	return v, tx.Commit()
}

// vestigeOfCase checks a vestige belongs to the case.
func vestigeOfCase(ctx context.Context, q *db.Queries, caseID string, vestigeID int64) error {
	id, err := caseRow(ctx, q, caseID)
	if err != nil {
		return err
	}
	if _, err := q.GetCaseVestigeOfCase(ctx, db.GetCaseVestigeOfCaseParams{ID: vestigeID, BiometricCaseID: id}); errors.Is(err, sql.ErrNoRows) {
		return ValidationError("the vestige is not one of this case")
	} else if err != nil {
		return err
	}
	return nil
}

// BreakSeal records that by opened the vestige's seal, to examine it. A vestige with no unbroken
// seal is refused.
func BreakSeal(ctx context.Context, sqlDB *sql.DB, caseID string, vestigeID, by int64, now time.Time) error {
	q := db.New(sqlDB)
	if err := vestigeOfCase(ctx, q, caseID, vestigeID); err != nil {
		return err
	}
	n, err := q.BreakVestigeSeal(ctx, db.BreakVestigeSealParams{VestigeID: vestigeID, BrokenAt: sql.NullTime{Time: now, Valid: true}, BrokenBy: by})
	if err != nil {
		return err
	}
	if n == 0 {
		return ValidationError("the vestige has no unbroken seal")
	}
	return nil
}

// Reseal records that by sealed the vestige under number, breaking the seal it had, if any: the lab
// seals again after examining.
func Reseal(ctx context.Context, sqlDB *sql.DB, caseID string, vestigeID int64, number string, by int64, now time.Time) error {
	number = strings.TrimSpace(number)
	if number == "" {
		return ValidationError("a seal needs its number")
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if err := vestigeOfCase(ctx, q, caseID, vestigeID); err != nil {
		return err
	}
	if _, err := q.BreakVestigeSeal(ctx, db.BreakVestigeSealParams{VestigeID: vestigeID, BrokenAt: sql.NullTime{Time: now, Valid: true}, BrokenBy: by}); err != nil {
		return err
	}
	if _, err := q.InsertVestigeSeal(ctx, db.InsertVestigeSealParams{VestigeID: vestigeID, SealNumber: number, AppliedAt: now,
		AppliedBy: sql.NullInt64{Int64: by, Valid: by != 0}}); err != nil {
		return err
	}
	return tx.Commit()
}

// SetReportMode changes whether the case's analyses are reported together or separately, until its
// first laudo is issued.
func SetReportMode(ctx context.Context, sqlDB *sql.DB, caseID, mode string) error {
	m, err := ParseReportMode(mode)
	if err != nil {
		return err
	}
	if strings.TrimSpace(mode) == "" {
		return ValidationError("reportMode must be SEPARATE or TOGETHER")
	}
	q := db.New(sqlDB)
	id, err := caseRow(ctx, q, caseID)
	if err != nil {
		return err
	}
	issued, err := q.CountIssuedCaseReports(ctx, id)
	if err != nil {
		return err
	}
	if issued > 0 {
		return ValidationError("the case already has an issued laudo")
	}
	_, err = q.SetCaseReportMode(ctx, db.SetCaseReportModeParams{ID: id, ReportMode: m})
	return err
}

// List returns a case's report mode, collections and vestiges with their seals.
func List(ctx context.Context, sqlDB *sql.DB, caseID string) (Origins, error) {
	q := db.New(sqlDB)
	c, err := q.GetBiometricCaseByCaseID(ctx, caseID)
	if errors.Is(err, sql.ErrNoRows) {
		return Origins{}, ErrCaseNotFound
	}
	if err != nil {
		return Origins{}, err
	}
	out := Origins{ReportMode: c.ReportMode, Collections: []Collection{}, Vestiges: []Vestige{}}
	cols, err := q.ListCaseCollections(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	members, err := q.ListCaseCollectionMembers(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	team := map[int64][]Member{}
	for _, m := range members {
		team[m.CollectionID] = append(team[m.CollectionID], Member{ID: m.UserID, Username: m.Username,
			Name: strings.TrimSpace(m.FirstName + " " + m.LastName)})
	}
	noteFiles, err := q.ListCollectionNoteFiles(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	audio := map[int64][]AudioNote{}
	for _, f := range noteFiles {
		audio[f.CollectionID.Int64] = append(audio[f.CollectionID.Int64], AudioNote{FileID: f.ID, Filename: f.Filename.String,
			ContentType: f.ContentType.String, SizeBytes: f.SizeBytes.Int64, SHA256: f.HashID.String, CreatedAt: f.CreatedAt})
	}
	for _, r := range cols {
		col := Collection{ID: r.ID, Sequence: r.Sequence, CollectedFrom: r.CollectedFrom, CollectedBy: r.CollectedByName,
			CollectedByID: r.CollectedBy, NothingCollected: r.NothingCollected, Notes: r.Notes.String,
			RegisteredBy: r.RegisteredByName.String, CreatedAt: r.CreatedAt, OtherOfficers: r.OtherOfficers.String,
			Team: team[r.ID], AudioNotes: audio[r.ID]}
		if col.Team == nil {
			col.Team = []Member{}
		}
		if col.AudioNotes == nil {
			col.AudioNotes = []AudioNote{}
		}
		if r.CollectedTo.Valid {
			col.CollectedTo = &r.CollectedTo.Time
		}
		if r.CalledAt.Valid {
			col.CalledAt = &r.CalledAt.Time
		}
		if r.LocationID.Valid {
			col.Place = strings.TrimSpace(strings.Join([]string{r.LocationName.String, r.LocationAddress.String}, " "))
		}
		if r.CaseRequestID.Valid {
			col.Request = &RequestRef{ID: r.CaseRequestID.Int64, DocumentReference: r.RequestDocument.String,
				ReceivedOn: r.RequestReceivedOn.Time.Format("2006-01-02")}
		}
		out.Collections = append(out.Collections, col)
	}
	vestiges, err := q.ListCaseVestiges(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	seals, err := q.ListCaseVestigeSeals(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	asked, err := q.ListCaseRequestVestiges(ctx, caseID)
	if err != nil {
		return Origins{}, err
	}
	for _, r := range vestiges {
		v := Vestige{ID: r.ID, Sequence: r.Sequence, Label: fmt.Sprintf("V%d", r.Sequence), Description: r.Description,
			Seals: []Seal{}, AskedBy: []int64{}}
		if r.CaseRequestID.Valid {
			v.RequestID = &r.CaseRequestID.Int64
		}
		if r.CollectionID.Valid {
			v.CollectionID = &r.CollectionID.Int64
		}
		for _, s := range seals {
			if s.VestigeID != r.ID {
				continue
			}
			seal := Seal{ID: s.ID, Number: s.SealNumber, AppliedAt: s.AppliedAt, AppliedBy: s.AppliedByName.String, BrokenBy: s.BrokenByName.String}
			if s.BrokenAt.Valid {
				seal.BrokenAt = &s.BrokenAt.Time
			}
			v.Seals = append(v.Seals, seal)
		}
		for _, a := range asked {
			if a.VestigeID == r.ID {
				v.AskedBy = append(v.AskedBy, a.CaseRequestID)
			}
		}
		out.Vestiges = append(out.Vestiges, v)
	}
	return out, nil
}

// ListAwaiting returns every collection still awaiting the request that formalizes it, oldest first.
func ListAwaiting(ctx context.Context, sqlDB *sql.DB) ([]Awaiting, error) {
	rows, err := db.New(sqlDB).ListAwaitingCollections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Awaiting, len(rows))
	for i, r := range rows {
		a := Awaiting{CollectionID: r.ID, Sequence: r.Sequence, CaseID: r.CaseID, CollectedFrom: r.CollectedFrom,
			CollectedBy: r.CollectedByName, NothingCollected: r.NothingCollected}
		switch {
		case r.OccurrenceYear.Valid && r.OccurrenceSeq.Valid:
			a.Occurrence = fmt.Sprintf("%04d.%04d", r.OccurrenceYear.Int32, r.OccurrenceSeq.Int32)
		case r.OccurrenceNumber.Valid:
			a.Occurrence = strings.TrimSpace(r.Agency.String + " " + r.OccurrenceNumber.String)
		case r.InquiryNumber.Valid:
			a.Occurrence = r.InquiryNumber.String
		}
		out[i] = a
	}
	return out, nil
}
