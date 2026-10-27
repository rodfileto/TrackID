// Package reports writes an analysis's report (docs/facial-reports-plan.md, phase 4): the lead
// report of a FACIAL_REVIEW and the laudo of a FACIAL_COMPARISON.
//
// The server generates the draft as a .docx from the template (template.go) and the data
// (Build): everything that comes from the database is locked text, and the sections the examiner
// writes are editable regions. The document is protected read-only with those regions open, which
// WordCraft enforces while the examiner edits it in the analysis page; every save is a new version
// (analysis_report_versions). The server does not trust the editor: a save is refused unless its
// locked content is the generated draft's, and issuing is refused unless the latest version's
// locked content still matches a draft generated from the data now (otherwise the examiner
// refreshes it, keeping their text).
//
// Issuing happens on the server, in one transaction with numbering (analyses.IssueReport): the
// data is frozen as the snapshot, the number and date replace their placeholders in the
// examiner's document, the PDF is rendered from it by WordCraft's own engine (CLIRenderer, the
// same layout the editor showed), and the .docx and PDF are kept as forensic_report files with
// their SHA-256. An issued report never changes; a correction is a new report that complements it.
package reports

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/auth"
	"github.com/rodfileto/trackid/db"
)

// DocxType is a .docx file's content type.
const DocxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// Errors returned to callers.
var (
	ErrNoRenderer     = errors.New("reports: no PDF renderer is configured (WORDCRAFT_CLI)")
	ErrNoStore        = errors.New("reports: object storage is not configured")
	ErrLockedChanged  = errors.New("the document changes text outside its editable sections")
	ErrRegionsChanged = errors.New("the document's editable sections do not match the report's")
	ErrStale          = errors.New("the report's data changed since this draft was generated: refresh the draft (your text is kept)")
	ErrNotIssued      = errors.New("the report is not issued yet")
	ErrNoDocument     = errors.New("the report has no such document")
	ErrPlaceholders   = errors.New("the document lost the placeholders for its number and date")
)

// MissingSectionsError is Issue's refusal while required sections are empty.
type MissingSectionsError struct{ Sections []string }

func (e MissingSectionsError) Error() string {
	return "these sections cannot be empty: " + strings.Join(e.Sections, ", ")
}

// Store keeps files (storage.Client).
type Store interface {
	Upload(ctx context.Context, objectKey string, content []byte, contentType string) (string, error)
	Download(ctx context.Context, storageRef string) ([]byte, error)
}

// Renderer turns a .docx into a PDF.
type Renderer interface {
	PDF(ctx context.Context, docx []byte) ([]byte, error)
}

// CLIRenderer renders with wordcraft-cli convert: the engine the examiner's editor runs, so the
// PDF breaks pages where the editor did.
type CLIRenderer struct{ Path string }

// PDF converts docx through a temporary directory.
func (r CLIRenderer) PDF(ctx context.Context, docx []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "trackid-report-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "report.docx"), filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(in, docx, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Path, "convert", in, out, "--update-fields")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reports: wordcraft-cli: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(out)
}

// Service is the reports' dependencies.
type Service struct {
	DB       *sql.DB
	Store    Store
	Images   Images
	Renderer Renderer
	Now      func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Version is one saved version of a draft.
type Version struct {
	Version   int32     `json:"version"`
	Generated bool      `json:"generated"` // the server built it from the data
	CreatedBy string    `json:"createdBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Stale     bool      `json:"stale"` // the data changed since: refresh before issuing
}

// generate builds a fresh draft from the data, with regions' texts when given (a refresh).
func (s Service) generate(ctx context.Context, q *db.Queries, analysisID int64, regions []string) (Data, []byte, *doc, error) {
	d, imgs, err := Build(ctx, q, analysisID, s.Images, s.now())
	if err != nil {
		return Data{}, nil, nil, err
	}
	out, err := renderWith(d, imgs, regions)
	if err != nil {
		return Data{}, nil, nil, err
	}
	b, err := out.bytes()
	return d, b, out, err
}

// renderWith renders d, filling its regions with texts in order when there are as many.
func renderWith(d Data, imgs map[string]Image, regions []string) (*doc, error) {
	if regions == nil {
		return render(d, imgs)
	}
	probe, err := render(d, imgs)
	if err != nil {
		return nil, err
	}
	if len(probe.regions) != len(regions) {
		return nil, ErrRegionsChanged
	}
	// Re-render with the texts: regions are written in order, so render with a filler.
	return renderFilled(d, imgs, regions)
}

// storeFile uploads data under the case and records it as a case_files row of category.
func (s Service) storeFile(ctx context.Context, q *db.Queries, caseID string, biometricCaseID int64, category, filename, contentType string, data []byte) (int64, error) {
	if s.Store == nil {
		return 0, ErrNoStore
	}
	hash := sha(data)
	ref, err := s.Store.Upload(ctx, fmt.Sprintf("reports/%s/%s", caseID, hash), data, contentType)
	if err != nil {
		return 0, fmt.Errorf("reports: upload: %w", err)
	}
	row, err := q.UpsertCaseFile(ctx, db.UpsertCaseFileParams{BiometricCaseID: biometricCaseID, Category: category,
		MediaType: sql.NullString{String: "document", Valid: true}, HashID: sql.NullString{String: hash, Valid: true},
		Filename: sql.NullString{String: filename, Valid: true}, StorageRef: sql.NullString{String: ref, Valid: true},
		ContentType: sql.NullString{String: contentType, Valid: true}, SizeBytes: sql.NullInt64{Int64: int64(len(data)), Valid: true}})
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

func draftName(t analyses.Target, version int32) string {
	return fmt.Sprintf("%s-%s-rascunho-v%d.docx", strings.ReplaceAll(t.CaseID, "/", "-"), strings.ToLower(string(t.Type)), version)
}

// latest returns a draft's latest version and its bytes; ok false when it has none.
func (s Service) latest(ctx context.Context, q *db.Queries, reportID int64) (db.LatestReportVersionRow, []byte, bool, error) {
	v, err := q.LatestReportVersion(ctx, reportID)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil, false, nil
	}
	if err != nil {
		return v, nil, false, err
	}
	if s.Store == nil {
		return v, nil, false, ErrNoStore
	}
	b, err := s.Store.Download(ctx, v.StorageRef.String)
	if err != nil {
		return v, nil, false, fmt.Errorf("reports: download draft: %w", err)
	}
	return v, b, true, nil
}

// addVersion stores data as the draft's next version.
func (s Service) addVersion(ctx context.Context, q *db.Queries, t analyses.Target, reportID int64, data []byte, generated bool, by int64) (Version, error) {
	v, err := q.LatestReportVersion(ctx, reportID)
	next := int32(1)
	if err == nil {
		next = v.Version + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Version{}, err
	}
	fileID, err := s.storeFile(ctx, q, t.CaseID, t.BiometricCaseID, "report_draft", draftName(t, next), DocxType, data)
	if err != nil {
		return Version{}, err
	}
	row, err := q.InsertReportVersion(ctx, db.InsertReportVersionParams{AnalysisReportID: reportID, CaseFileID: fileID, Generated: generated,
		CreatedBy: sql.NullInt64{Int64: by, Valid: by != 0}})
	if err != nil {
		return Version{}, err
	}
	return Version{Version: row.Version, Generated: generated, CreatedAt: s.now()}, nil
}

// draftTarget reads the analysis and its report, refusing an issued one.
func draftTarget(ctx context.Context, q *db.Queries, analysisID int64) (analyses.Target, db.GetAnalysisReportRow, error) {
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return t, db.GetAnalysisReportRow{}, err
	}
	r, err := q.GetAnalysisReport(ctx, analysisID)
	if err != nil {
		return t, r, err
	}
	if r.Status != analyses.ReportDraft {
		return t, r, analyses.ErrNotDraft
	}
	return t, r, nil
}

// canWrite is analyses.CanWork at the REPORT stage: only the first examiner, once every pair is
// settled, writes the report.
func canWrite(user auth.UserProfile, t analyses.Target) error {
	if t.Stage != analyses.StageReport {
		return analyses.ErrNotReportStage
	}
	_, err := analyses.CanWork(user, t)
	return err
}

// Draft returns the latest version of a report's draft for its examiner to edit, generating the
// first one from the data when there is none. Stale says the data changed since that version was
// generated.
func (s Service) Draft(ctx context.Context, user auth.UserProfile, analysisID int64) (Version, []byte, error) {
	q := db.New(s.DB)
	t, r, err := draftTarget(ctx, q, analysisID)
	if err != nil {
		return Version{}, nil, err
	}
	if err := canWrite(user, t); err != nil {
		return Version{}, nil, err
	}
	v, data, ok, err := s.latest(ctx, q, r.ID)
	if err != nil {
		return Version{}, nil, err
	}
	_, fresh, _, err := s.generate(ctx, q, analysisID, nil)
	if err != nil {
		return Version{}, nil, err
	}
	if !ok {
		nv, err := s.addVersion(ctx, q, t, r.ID, fresh, true, user.ID)
		return nv, fresh, err
	}
	out := Version{Version: v.Version, Generated: v.Generated, CreatedBy: v.CreatedBy.String, CreatedAt: v.CreatedAt}
	stale, err := differs(data, fresh)
	if err != nil {
		return Version{}, nil, err
	}
	out.Stale = stale
	return out, data, nil
}

// differs reports whether two documents' locked content differs.
func differs(a, b []byte) (bool, error) {
	pa, err := parseDocx(a)
	if err != nil {
		return false, err
	}
	pb, err := parseDocx(b)
	if err != nil {
		return false, err
	}
	return pa.locked != pb.locked || len(pa.regions) != len(pb.regions), nil
}

// Save stores the examiner's edited document as the draft's next version, refusing one whose
// locked content or set of sections differs from the version it was edited from.
func (s Service) Save(ctx context.Context, user auth.UserProfile, analysisID int64, data []byte) (Version, error) {
	q := db.New(s.DB)
	t, r, err := draftTarget(ctx, q, analysisID)
	if err != nil {
		return Version{}, err
	}
	if err := canWrite(user, t); err != nil {
		return Version{}, err
	}
	edited, err := parseDocx(data)
	if err != nil {
		return Version{}, err
	}
	_, base, ok, err := s.latest(ctx, q, r.ID)
	if err != nil {
		return Version{}, err
	}
	if !ok {
		if _, base, _, err = s.generate(ctx, q, analysisID, nil); err != nil {
			return Version{}, err
		}
	}
	from, err := parseDocx(base)
	if err != nil {
		return Version{}, err
	}
	if len(from.regions) != len(edited.regions) {
		return Version{}, ErrRegionsChanged
	}
	if from.locked != edited.locked {
		return Version{}, ErrLockedChanged
	}
	v, err := s.addVersion(ctx, q, t, r.ID, data, false, user.ID)
	if err != nil {
		return Version{}, err
	}
	v.CreatedBy = user.Username
	return v, nil
}

// Refresh regenerates the draft from the data now, keeping the text of each section from the
// latest version (as plain paragraphs: the examiner's formatting in them is not carried over).
func (s Service) Refresh(ctx context.Context, user auth.UserProfile, analysisID int64) (Version, error) {
	q := db.New(s.DB)
	t, r, err := draftTarget(ctx, q, analysisID)
	if err != nil {
		return Version{}, err
	}
	if err := canWrite(user, t); err != nil {
		return Version{}, err
	}
	_, data, ok, err := s.latest(ctx, q, r.ID)
	if err != nil {
		return Version{}, err
	}
	var regions []string
	if ok {
		p, err := parseDocx(data)
		if err != nil {
			return Version{}, err
		}
		regions = p.regions
	}
	_, fresh, _, err := s.generate(ctx, q, analysisID, regions)
	if err != nil {
		return Version{}, err
	}
	v, err := s.addVersion(ctx, q, t, r.ID, fresh, true, user.ID)
	if err != nil {
		return Version{}, err
	}
	v.CreatedBy = user.Username
	return v, nil
}

// Issue issues the report from its latest draft (analyses.IssueReport): see the package comment.
func (s Service) Issue(ctx context.Context, user auth.UserProfile, analysisID int64) (analyses.Report, error) {
	if s.Renderer == nil {
		return analyses.Report{}, ErrNoRenderer
	}
	q := db.New(s.DB)
	t, r, err := draftTarget(ctx, q, analysisID)
	if err != nil {
		return analyses.Report{}, err
	}
	if t.Status == analyses.StatusOpen && t.Stage != analyses.StageReport {
		return analyses.Report{}, analyses.ErrNotReportStage
	}
	if _, err := analyses.CanWork(user, t); err != nil {
		return analyses.Report{}, err
	}
	_, data, ok, err := s.latest(ctx, q, r.ID)
	if err != nil {
		return analyses.Report{}, err
	}
	d, fresh, _, err := s.generate(ctx, q, analysisID, nil)
	if err != nil {
		return analyses.Report{}, err
	}
	if !ok {
		data = fresh
	}
	edited, err := parseDocx(data)
	if err != nil {
		return analyses.Report{}, err
	}
	now, err := parseDocx(fresh)
	if err != nil {
		return analyses.Report{}, err
	}
	if edited.locked != now.locked || len(edited.regions) != len(now.regions) {
		return analyses.Report{}, ErrStale
	}
	probe, err := render(d, nil)
	if err != nil {
		return analyses.Report{}, err
	}
	var missing []string
	for _, key := range d.requiredRegions() {
		for i, k := range probe.regions {
			if k == key && strings.TrimSpace(edited.regions[i]) == "" {
				missing = append(missing, regionTitle(key))
			}
		}
	}
	if len(missing) > 0 {
		return analyses.Report{}, MissingSectionsError{Sections: missing}
	}

	return analyses.IssueReport(ctx, s.DB, analysisID, user, s.now(), func(ctx context.Context, q *db.Queries, t analyses.Target, rep analyses.Report) (analyses.Issued, error) {
		frozen, _, err := Build(ctx, q, analysisID, nil, s.now())
		if err != nil {
			return analyses.Issued{}, err
		}
		frozen.Number, frozen.IssuedAt = rep.Number, rep.IssuedAt
		frozen.Examiners.Issuer = frozen.Examiners.Examiner
		// The images the document shows, by the hashes the draft recorded.
		shown := map[string]string{}
		for _, x := range d.Questioned {
			shown[x.FeatureID] = x.ImageSHA256
		}
		for _, x := range d.References {
			shown[x.FeatureID] = x.ImageSHA256
		}
		for i := range frozen.Questioned {
			frozen.Questioned[i].ImageSHA256 = shown[frozen.Questioned[i].FeatureID]
		}
		for i := range frozen.References {
			frozen.References[i].ImageSHA256 = shown[frozen.References[i].FeatureID]
		}
		final, counts, err := replaceInDocx(data, map[string]string{
			numberPlaceholder: rep.Number,
			datePlaceholder:   rep.IssuedAt.In(saoPaulo).Format("02/01/2006"),
		})
		if err != nil {
			return analyses.Issued{}, err
		}
		if counts[numberPlaceholder] == 0 || counts[datePlaceholder] == 0 {
			return analyses.Issued{}, ErrPlaceholders
		}
		pdf, err := s.Renderer.PDF(ctx, final)
		if err != nil {
			return analyses.Issued{}, err
		}
		base := fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(rep.Type, "_", "-")), strings.ReplaceAll(rep.Number, ".", "-"))
		docxID, err := s.storeFile(ctx, q, t.CaseID, t.BiometricCaseID, "forensic_report", base+".docx", DocxType, final)
		if err != nil {
			return analyses.Issued{}, err
		}
		pdfID, err := s.storeFile(ctx, q, t.CaseID, t.BiometricCaseID, "forensic_report", base+".pdf", "application/pdf", pdf)
		if err != nil {
			return analyses.Issued{}, err
		}
		snapshot, err := json.Marshal(Snapshot{Data: frozen, DocxSHA256: sha(final), PDFSHA256: sha(pdf)})
		if err != nil {
			return analyses.Issued{}, err
		}
		return analyses.Issued{Snapshot: snapshot, DocxFileID: docxID, PDFFileID: pdfID}, nil
	})
}

// Snapshot is what an issued report freezes (analysis_reports.snapshot): its data and its
// documents' hashes.
type Snapshot struct {
	Data       Data   `json:"data"`
	DocxSHA256 string `json:"docxSha256"`
	PDFSHA256  string `json:"pdfSha256"`
}

func regionTitle(key string) string {
	var n int
	switch {
	case strings.HasPrefix(key, "exam."):
		fmt.Sscanf(key, "exam.%d", &n)
		return fmt.Sprintf("Exame do par %d", n)
	case strings.HasPrefix(key, "answer."):
		fmt.Sscanf(key, "answer.%d", &n)
		return fmt.Sprintf("Resposta ao quesito %d", n)
	}
	return key
}

// Overview is a report as the analysis page shows it: its summary (from the snapshot once
// issued, preliminary before), its draft's latest version, its documents and deliveries.
type Overview struct {
	Status     string     `json:"status"`
	Number     string     `json:"number,omitempty"`
	Summary    Summary    `json:"summary"`
	Draft      *Version   `json:"draft,omitempty"`
	DocxSHA256 string     `json:"docxSha256,omitempty"`
	PDFSHA256  string     `json:"pdfSha256,omitempty"`
	Deliveries []Delivery `json:"deliveries"`
	Required   []string   `json:"required"` // sections a laudo cannot be issued without
}

// Get returns a report's Overview.
func (s Service) Get(ctx context.Context, analysisID int64) (Overview, error) {
	q := db.New(s.DB)
	r, err := q.GetAnalysisReport(ctx, analysisID)
	if errors.Is(err, sql.ErrNoRows) {
		return Overview{}, analyses.ErrAnalysisNotFound
	}
	if err != nil {
		return Overview{}, err
	}
	o := Overview{Status: r.Status, Deliveries: []Delivery{}, Required: []string{}}
	if r.Status == analyses.ReportIssued {
		var snap Snapshot
		if err := json.Unmarshal(r.Snapshot.RawMessage, &snap); err != nil {
			return Overview{}, err
		}
		o.Number, o.Summary, o.DocxSHA256, o.PDFSHA256 = snap.Data.Number, Summarize(snap.Data), snap.DocxSHA256, snap.PDFSHA256
	} else {
		d, _, err := Build(ctx, q, analysisID, nil, s.now())
		if err != nil {
			return Overview{}, err
		}
		o.Summary = Summarize(d)
		for _, key := range d.requiredRegions() {
			o.Required = append(o.Required, regionTitle(key))
		}
		if v, err := q.LatestReportVersion(ctx, r.ID); err == nil {
			o.Draft = &Version{Version: v.Version, Generated: v.Generated, CreatedBy: v.CreatedBy.String, CreatedAt: v.CreatedAt}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Overview{}, err
		}
	}
	rows, err := q.ListReportDeliveries(ctx, r.ID)
	if err != nil {
		return Overview{}, err
	}
	for _, d := range rows {
		o.Deliveries = append(o.Deliveries, Delivery{DeliveredOn: d.DeliveredOn.Format("2006-01-02"), Method: d.Method, Recipient: d.Recipient,
			Notes: d.Notes.String, RecordedBy: d.RecordedBy, RecordedAt: d.CreatedAt})
	}
	return o, nil
}

// Document returns an issued report's PDF or .docx.
func (s Service) Document(ctx context.Context, analysisID int64, kind string) ([]byte, string, string, error) {
	q := db.New(s.DB)
	r, err := q.GetAnalysisReport(ctx, analysisID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", "", analyses.ErrAnalysisNotFound
	}
	if err != nil {
		return nil, "", "", err
	}
	if r.Status != analyses.ReportIssued {
		return nil, "", "", ErrNotIssued
	}
	id := r.CaseFileID
	if kind == "docx" {
		id = r.DocxFileID
	}
	if !id.Valid {
		return nil, "", "", ErrNoDocument
	}
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return nil, "", "", err
	}
	f, err := q.GetCaseFile(ctx, db.GetCaseFileParams{ID: id.Int64, BiometricCaseID: t.BiometricCaseID})
	if err != nil {
		return nil, "", "", err
	}
	if s.Store == nil {
		return nil, "", "", ErrNoStore
	}
	data, err := s.Store.Download(ctx, f.StorageRef.String)
	if err != nil {
		return nil, "", "", err
	}
	return data, f.Filename.String, f.ContentType.String, nil
}

// Delivery is a record of an issued report being sent.
type Delivery struct {
	DeliveredOn string    `json:"deliveredOn"` // YYYY-MM-DD
	Method      string    `json:"method"`      // e.g. "SEI", "e-mail", "em mãos"
	Recipient   string    `json:"recipient"`
	Notes       string    `json:"notes,omitempty"`
	RecordedBy  string    `json:"recordedBy,omitempty"`
	RecordedAt  time.Time `json:"recordedAt"`
}

// ValidationError is a delivery refused for its input.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// Deliver records that an issued report was sent. Who may is the caller's check.
func (s Service) Deliver(ctx context.Context, user auth.UserProfile, analysisID int64, in Delivery) error {
	q := db.New(s.DB)
	r, err := q.GetAnalysisReport(ctx, analysisID)
	if errors.Is(err, sql.ErrNoRows) {
		return analyses.ErrAnalysisNotFound
	}
	if err != nil {
		return err
	}
	if r.Status != analyses.ReportIssued {
		return ErrNotIssued
	}
	on, err := time.Parse("2006-01-02", strings.TrimSpace(in.DeliveredOn))
	if err != nil {
		return ValidationError("deliveredOn must be a date (YYYY-MM-DD)")
	}
	if strings.TrimSpace(in.Method) == "" || strings.TrimSpace(in.Recipient) == "" {
		return ValidationError("method and recipient are required")
	}
	_, err = q.InsertReportDelivery(ctx, db.InsertReportDeliveryParams{AnalysisReportID: r.ID, DeliveredOn: on, Method: strings.TrimSpace(in.Method),
		Recipient: strings.TrimSpace(in.Recipient), Notes: sql.NullString{String: strings.TrimSpace(in.Notes), Valid: strings.TrimSpace(in.Notes) != ""},
		RecordedBy: user.ID})
	return err
}

// IssuedBy returns the username who issued a report, empty while it is a draft.
func IssuedBy(ctx context.Context, sqlDB *sql.DB, analysisID int64) (string, error) {
	r, err := db.New(sqlDB).GetAnalysisReport(ctx, analysisID)
	if err != nil {
		return "", err
	}
	return r.IssuedBy.String, nil
}
