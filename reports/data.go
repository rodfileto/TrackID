package reports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/db"
	"github.com/rodfileto/trackid/examination"
	"github.com/rodfileto/trackid/graph"
	"github.com/rodfileto/trackid/requests"
)

// Data is everything a report states that comes from the database: the sections the examiner
// cannot edit. It is built fresh for a draft and frozen, as JSON, when the report is issued
// (analysis_reports.snapshot). Images are referenced by their SHA-256, so the snapshot says which
// image a report showed without holding it.
type Data struct {
	AnalysisID     int64              `json:"analysisId"`
	AnalysisType   analyses.Type      `json:"analysisType"`
	ReportType     string             `json:"reportType"`
	Number         string             `json:"number,omitempty"` // empty while a draft
	IssuedAt       *time.Time         `json:"issuedAt,omitempty"`
	Complements    string             `json:"complements,omitempty"` // the number of the report it complements
	Case           CaseInfo           `json:"case"`
	Request        *RequestInfo       `json:"request,omitempty"`
	Questioned     []TraceInfo        `json:"questioned"`
	References     []ReferenceInfo    `json:"references"`
	Method         Method             `json:"method"`
	Pairs          []PairInfo         `json:"pairs"`
	Faces          []FaceResult       `json:"faces,omitempty"` // a review's result per questioned face
	Examiners      Examiners          `json:"examiners"`
	Previous       []PriorReport      `json:"previous"`
	Identification cluster.Conclusion `json:"identificationLevel"` // the lowest level that identifies
	GeneratedAt    time.Time          `json:"generatedAt"`
}

// CaseInfo is the case a report is about.
type CaseInfo struct {
	CaseID          string `json:"caseId"`
	CaseType        string `json:"caseType"`
	Description     string `json:"description"`
	Occurrence      string `json:"occurrence,omitempty"` // TrackID number and/or police number
	OccurrenceTypes string `json:"occurrenceTypes,omitempty"`
}

// RequestInfo is the request a report answers.
type RequestInfo struct {
	Sequence          int16      `json:"sequence"`
	Authority         string     `json:"authority"`
	DocumentReference string     `json:"documentReference"`
	DocumentDate      string     `json:"documentDate,omitempty"`
	ReceivedOn        string     `json:"receivedOn"`
	DueOn             string     `json:"dueOn,omitempty"`
	CitedReference    string     `json:"citedReference,omitempty"`
	Questions         []string   `json:"questions"`
	Files             []FileInfo `json:"files"`
}

// FileInfo is one received file.
type FileInfo struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

// TraceInfo is one face of the case: a questioned trace, or a reference sent in.
type TraceInfo struct {
	FeatureID        string `json:"featureId"`
	Label            string `json:"label"` // "Vestígio 2.1"
	Evidence         string `json:"evidence,omitempty"`
	EvidenceSHA256   string `json:"evidenceSha256,omitempty"`
	Suitability      string `json:"suitability,omitempty"`
	SuitabilityNotes string `json:"suitabilityNotes,omitempty"`
	ImageSHA256      string `json:"imageSha256,omitempty"`
	// Source is the video the evidence frame was taken from: its name, hash, time and frame.
	Source *cases.VideoSource `json:"source,omitempty"`
}

// ReferenceInfo is reference material a report compared with: a reference trace sent in, or an
// enrolled record.
type ReferenceInfo struct {
	FeatureID      string `json:"featureId"`
	Kind           string `json:"kind"` // SENT_IN | ENROLLED
	Label          string `json:"label"`
	Name           string `json:"name,omitempty"`
	Document       string `json:"document,omitempty"`
	Register       string `json:"register,omitempty"`
	EnrolledOn     string `json:"enrolledOn,omitempty"`
	Evidence       string `json:"evidence,omitempty"`
	EvidenceSHA256 string `json:"evidenceSha256,omitempty"`
	ImageSHA256    string `json:"imageSha256,omitempty"`
}

// Method is how the pairs were found and examined.
type Method struct {
	Systems []SystemInfo `json:"systems"` // the matcher(s) behind a review's candidates
}

// SystemInfo is one matcher and the cutoff its candidates were classified against.
type SystemInfo struct {
	Source    string   `json:"source"`
	Threshold *float64 `json:"threshold,omitempty"`
}

// StepDecision is one examiner's decision on a pair.
type StepDecision struct {
	Step       analyses.Step      `json:"step"`
	Examiner   string             `json:"examiner"`
	Decision   string             `json:"decision"`
	Conclusion cluster.Conclusion `json:"conclusion,omitempty"`
	Notes      string             `json:"notes,omitempty"`
}

// PairInfo is one examined pair with its decisions and outcome.
type PairInfo struct {
	Questioned    string             `json:"questioned"`  // label of the case's face
	Counterpart   string             `json:"counterpart"` // label of what it was compared with
	QuestionedID  string             `json:"questionedId"`
	CounterpartID string             `json:"counterpartId"`
	Score         *float64           `json:"score,omitempty"`
	Steps         []StepDecision     `json:"steps"`
	Disputed      bool               `json:"disputed"`
	Decision      string             `json:"decision,omitempty"`   // the settled decision, empty if unsettled
	Conclusion    cluster.Conclusion `json:"conclusion,omitempty"` // a comparison's settled level
	Result        string             `json:"result"`               // IDENTIFIED | SUPPORTED | INCONCLUSIVE | EXCLUDED | LEAD | NO_LEAD | UNSETTLED
	Lead          string             `json:"lead,omitempty"`       // a comparison: what an earlier review concluded on the pair
	Effect        string             `json:"effect,omitempty"`     // CONFIRMS | CHANGES | LEAVES_STANDING | NEW
}

// FaceResult is a review's result for one questioned face: leads to enrolled persons, or none.
type FaceResult struct {
	Face        string   `json:"face"`
	Suitability string   `json:"suitability,omitempty"`
	Leads       []string `json:"leads"`
	Result      string   `json:"result"` // LEAD | NO_LEAD | UNSUITABLE
}

// Person names an examiner as a report signs them.
type Person struct {
	Username    string `json:"username"`
	Name        string `json:"name"`
	StaffNumber string `json:"staffNumber,omitempty"`
	JobTitle    string `json:"jobTitle,omitempty"`
}

// Examiners are who worked each step.
type Examiners struct {
	Examiner      *Person `json:"examiner,omitempty"`
	Verifier      *Person `json:"verifier,omitempty"`
	Inconsistence *Person `json:"inconsistence,omitempty"`
	Issuer        *Person `json:"issuer,omitempty"`
}

// PriorReport is another issued report of the case.
type PriorReport struct {
	Type     string    `json:"type"`
	Number   string    `json:"number"`
	IssuedAt time.Time `json:"issuedAt"`
}

// Result values.
const (
	ResultIdentified   = "IDENTIFIED"
	ResultSupported    = "SUPPORTED"
	ResultInconclusive = "INCONCLUSIVE"
	ResultExcluded     = "EXCLUDED"
	ResultLead         = "LEAD"
	ResultNoLead       = "NO_LEAD"
	ResultUnsuitable   = "UNSUITABLE"
	ResultUnsettled    = "UNSETTLED"
)

// Image is one image a report shows.
type Image struct {
	Data        []byte
	ContentType string
}

// Images loads the image of a feature (examination.FeatureImage in production).
type Images func(ctx context.Context, featureID string) (Image, error)

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Build reads a report's data: the analysis, its case and request, the material, the pairs and
// their decisions, the examiners and the case's other reports. images, when set, loads each face
// shown so its hash is recorded and returns them by feature id; nil skips images.
func Build(ctx context.Context, q *db.Queries, analysisID int64, images Images, now time.Time) (Data, map[string]Image, error) {
	t, err := analyses.GetTarget(ctx, q, analysisID)
	if err != nil {
		return Data{}, nil, err
	}
	report, err := q.GetAnalysisReport(ctx, analysisID)
	if err != nil {
		return Data{}, nil, err
	}
	c, err := q.GetCaseForReport(ctx, t.BiometricCaseID)
	if err != nil {
		return Data{}, nil, err
	}
	level, err := cluster.IdentificationLevel(ctx, q)
	if err != nil {
		return Data{}, nil, err
	}
	d := Data{AnalysisID: t.ID, AnalysisType: t.Type, ReportType: report.ReportType, Identification: level, GeneratedAt: now,
		Case:       CaseInfo{CaseID: c.CaseID, CaseType: c.CaseType, Description: c.Description, OccurrenceTypes: c.OccurrenceTypes},
		Questioned: []TraceInfo{}, References: []ReferenceInfo{}, Pairs: []PairInfo{}, Previous: []PriorReport{}}
	if c.OccurrenceID.Valid {
		d.Case.Occurrence = occurrenceLabel(c.OccurrenceYear, c.OccurrenceSeq, c.Agency, c.OccurrenceNumber)
	}
	if report.ComplementsReportID.Valid {
		n, err := q.GetReportNumber(ctx, report.ComplementsReportID.Int64)
		if err != nil {
			return Data{}, nil, err
		}
		if n.ReportNumber.Valid {
			d.Complements = fmt.Sprintf("%04d.%04d", n.ReportYear.Int32, n.ReportNumber.Int32)
		}
	}

	// The request, with its questions and received files.
	analysisRows, err := q.ListCaseAnalyses(ctx, t.CaseID)
	if err != nil {
		return Data{}, nil, err
	}
	var requestID int64
	for _, a := range analysisRows {
		if a.ID == t.ID {
			requestID = a.CaseRequestID.Int64
		}
	}
	if requestID != 0 {
		list, err := requests.ListWith(ctx, q, t.CaseID)
		if err != nil {
			return Data{}, nil, err
		}
		for _, r := range list {
			if r.ID != requestID {
				continue
			}
			info := &RequestInfo{Sequence: r.Sequence, Authority: r.Authority, DocumentReference: r.DocumentReference,
				DocumentDate: r.DocumentDate, ReceivedOn: r.ReceivedOn, DueOn: r.DueOn, CitedReference: r.CitedReference,
				Questions: r.Questions, Files: []FileInfo{}}
			for _, f := range r.Files {
				info.Files = append(info.Files, FileInfo{Filename: f.Filename, SHA256: f.SHA256})
			}
			d.Request = info
		}
	}

	// Material: every face of the case, and the evidence files they are on.
	material, err := cases.ListMaterialWith(ctx, q, t.CaseID)
	if err != nil {
		return Data{}, nil, err
	}
	files, err := q.ListRequestFiles(ctx, t.CaseID)
	if err != nil {
		return Data{}, nil, err
	}
	fileByID := map[int64]db.ListRequestFilesRow{}
	for _, f := range files {
		fileByID[f.ID] = f
	}
	traceByFeature := map[string]cases.MaterialTrace{}
	for _, m := range material {
		traceByFeature[m.FeatureID] = m
	}

	records, own, err := examination.Records(ctx, q, t)
	if err != nil {
		return Data{}, nil, err
	}

	// Who decided what, by username; the examiners by step.
	usernames := map[string]bool{}
	for _, r := range records {
		for _, dec := range r.Decisions {
			usernames[dec.Username] = true
		}
	}
	people, err := loadPeople(ctx, q, t, usernames)
	if err != nil {
		return Data{}, nil, err
	}
	person := func(username string) *Person {
		if p, ok := people.byUsername[username]; ok {
			return &p
		}
		if username == "" {
			return nil
		}
		return &Person{Username: username, Name: username}
	}
	d.Examiners = Examiners{Examiner: people.byStep[analyses.StepVerificator], Verifier: people.byStep[analyses.StepReviewer],
		Inconsistence: people.byStep[analyses.StepInconsistence]}

	imgs := map[string]Image{}
	loadImage := func(featureID string) string {
		if images == nil {
			return ""
		}
		if img, ok := imgs[featureID]; ok {
			return sha(img.Data)
		}
		img, err := images(ctx, featureID)
		if err != nil || len(img.Data) == 0 {
			return ""
		}
		imgs[featureID] = img
		return sha(img.Data)
	}
	traceLabel := func(m cases.MaterialTrace) string {
		if m.Role == cases.RoleReference {
			return "Padrão " + m.Label
		}
		return "Vestígio " + m.Label
	}
	evidence := func(m cases.MaterialTrace) (string, string) {
		f, ok := fileByID[m.EvidenceFileID]
		if !ok {
			return "", ""
		}
		return f.Filename.String, f.HashID.String
	}

	// The faces this report is about: a comparison's questioned sides, or every questioned face of
	// the case for a review.
	inReport := map[string]bool{}
	for _, r := range records {
		for _, id := range []string{r.FeatureA, r.FeatureB} {
			if own[id] && traceByFeature[id].Role == cases.RoleQuestioned {
				inReport[id] = true
			}
		}
	}
	if t.Type == analyses.FacialReview {
		for _, m := range material {
			if m.Role == cases.RoleQuestioned {
				inReport[m.FeatureID] = true
			}
		}
	}
	for _, m := range material {
		if !inReport[m.FeatureID] {
			continue
		}
		name, hash := evidence(m)
		d.Questioned = append(d.Questioned, TraceInfo{FeatureID: m.FeatureID, Label: traceLabel(m), Evidence: name, EvidenceSHA256: hash,
			Suitability: m.Suitability, SuitabilityNotes: m.SuitabilityNotes, ImageSHA256: loadImage(m.FeatureID), Source: m.Source})
	}

	// Reference material: the counterpart of every pair that is not a questioned face of the case.
	var enrolledIDs []int64
	counterparts := []string{}
	seen := map[string]bool{}
	for _, r := range records {
		for _, id := range []string{r.FeatureA, r.FeatureB} {
			if inReport[id] || seen[id] {
				continue
			}
			seen[id] = true
			counterparts = append(counterparts, id)
			if !own[id] {
				if n, err := strconv.ParseInt(id, 10, 64); err == nil {
					enrolledIDs = append(enrolledIDs, n)
				}
			}
		}
	}
	enrolled := map[string]db.ListEnrolledReferencesRow{}
	if len(enrolledIDs) > 0 {
		rows, err := q.ListEnrolledReferences(ctx, enrolledIDs)
		if err != nil {
			return Data{}, nil, err
		}
		for _, e := range rows {
			enrolled[graph.KnownFeatureID(e.IdentityFileID)] = e
		}
	}
	labels := map[string]string{}
	for _, q := range d.Questioned {
		labels[q.FeatureID] = q.Label
	}
	for _, id := range counterparts {
		if m, ok := traceByFeature[id]; ok {
			name, hash := evidence(m)
			ref := ReferenceInfo{FeatureID: id, Kind: "SENT_IN", Label: traceLabel(m), Evidence: name, EvidenceSHA256: hash}
			if t.Type.Comparison() {
				ref.ImageSHA256 = loadImage(id)
			}
			labels[id] = ref.Label
			d.References = append(d.References, ref)
			continue
		}
		e, ok := enrolled[id]
		if !ok {
			continue
		}
		ref := ReferenceInfo{FeatureID: id, Kind: "ENROLLED", Label: "Registro " + e.RegisterNumber, Name: e.Name,
			Document: e.DocumentType + " " + e.DocumentNumber, Register: e.RegisterNumber, EnrolledOn: e.EnrolledAt.Format("2006-01-02")}
		if t.Type.Comparison() {
			ref.ImageSHA256 = loadImage(id)
		}
		labels[id] = ref.Label + " (" + e.Name + ")"
		d.References = append(d.References, ref)
	}

	// Method: the matcher behind a review's candidates.
	if t.Type == analyses.FacialReview {
		ids := make([]string, 0, len(own))
		for id := range own {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		if len(ids) > 0 {
			rows, err := q.ListSystemSourcesForFeatures(ctx, ids)
			if err != nil {
				return Data{}, nil, err
			}
			for _, r := range rows {
				s := SystemInfo{Source: r.SystemSource.String}
				if r.Threshold.Valid {
					v := r.Threshold.Float64
					s.Threshold = &v
				}
				d.Method.Systems = append(d.Method.Systems, s)
			}
		}
	}

	// Earlier leads on this comparison's pairs.
	var leads map[examination.Pair]string
	if t.Type.Comparison() {
		if leads, err = reviewLeads(ctx, q, records); err != nil {
			return Data{}, nil, err
		}
	}

	// Pairs and their outcome.
	for _, r := range records {
		questioned, counterpart := r.FeatureA, r.FeatureB
		if !inReport[questioned] {
			questioned, counterpart = counterpart, questioned
		}
		p := PairInfo{Questioned: labels[questioned], Counterpart: labels[counterpart], QuestionedID: questioned, CounterpartID: counterpart,
			Score: r.Score, Disputed: r.Disputed, Steps: []StepDecision{}}
		if p.Counterpart == "" {
			p.Counterpart = counterpart
		}
		for _, step := range analyses.Steps {
			if dec, ok := r.Decisions[step]; ok {
				name := dec.Username
				if pp := person(dec.Username); pp != nil {
					name = pp.Name
				}
				p.Steps = append(p.Steps, StepDecision{Step: step, Examiner: name, Decision: dec.Decision, Conclusion: dec.Conclusion, Notes: dec.Notes})
			}
		}
		p.Decision, p.Conclusion = settled(r.Decisions)
		p.Result = result(t.Type, p.Decision, p.Conclusion, level)
		if leads != nil {
			p.Lead = leads[r.Pair]
			p.Effect = effect(p.Lead, p.Decision)
		}
		d.Pairs = append(d.Pairs, p)
	}

	if t.Type == analyses.FacialReview {
		for _, qt := range d.Questioned {
			f := FaceResult{Face: qt.Label, Suitability: qt.Suitability, Leads: []string{}, Result: ResultNoLead}
			for _, p := range d.Pairs {
				if p.QuestionedID == qt.FeatureID && p.Result == ResultLead {
					f.Leads = append(f.Leads, p.Counterpart)
					f.Result = ResultLead
				}
			}
			if f.Result == ResultNoLead && qt.Suitability == "UNSUITABLE" {
				f.Result = ResultUnsuitable
			}
			d.Faces = append(d.Faces, f)
		}
	}

	prior, err := q.ListCaseReports(ctx, t.BiometricCaseID)
	if err != nil {
		return Data{}, nil, err
	}
	for _, r := range prior {
		if r.CaseAnalysisID == t.ID {
			continue
		}
		d.Previous = append(d.Previous, PriorReport{Type: r.ReportType, Number: fmt.Sprintf("%04d.%04d", r.ReportYear.Int32, r.ReportNumber.Int32),
			IssuedAt: r.IssuedAt.Time})
	}
	return d, imgs, nil
}

func occurrenceLabel(year, seq sql.NullInt32, agency, number sql.NullString) string {
	s := ""
	if year.Valid && seq.Valid {
		s = fmt.Sprintf("%04d.%04d", year.Int32, seq.Int32)
	}
	if number.Valid && number.String != "" {
		police := number.String
		if agency.Valid && agency.String != "" {
			police = agency.String + " " + police
		}
		if s != "" {
			s += " (" + police + ")"
		} else {
			s = police
		}
	}
	return s
}

// settled is a pair's outcome in its analysis: the INCONSISTENCE decision, or the agreeing
// VERIFICATOR and REVIEWER (same decision and, in a comparison, same level); empty otherwise.
func settled(d map[analyses.Step]examination.Decision) (string, cluster.Conclusion) {
	if i, ok := d[analyses.StepInconsistence]; ok {
		return i.Decision, i.Conclusion
	}
	v, okV := d[analyses.StepVerificator]
	r, okR := d[analyses.StepReviewer]
	if okV && okR && v.Decision == r.Decision && v.Conclusion == r.Conclusion {
		return r.Decision, r.Conclusion
	}
	return "", ""
}

func result(t analyses.Type, decision string, level, identification cluster.Conclusion) string {
	if decision == "" {
		return ResultUnsettled
	}
	if t == analyses.FacialReview {
		if decision == examination.Positive {
			return ResultLead
		}
		return ResultNoLead
	}
	switch {
	case decision == examination.Negative:
		return ResultExcluded
	case decision == examination.Inconclusive:
		return ResultInconclusive
	case level.AtLeast(identification):
		return ResultIdentified
	}
	return ResultSupported
}

// effect is what a comparison's outcome does to an earlier lead on the same pair.
func effect(lead, decision string) string {
	switch {
	case lead == "":
		return "NEW"
	case decision == "" || decision == examination.Inconclusive:
		return "LEAVES_STANDING"
	case lead == decision:
		return "CONFIRMS"
	}
	return "CHANGES"
}

// reviewLeads is, per pair, what the latest review that settled it concluded (POSITIVE or
// NEGATIVE), from the decision chains.
func reviewLeads(ctx context.Context, q *db.Queries, records []examination.Record) (map[examination.Pair]string, error) {
	ids := []string{}
	for _, r := range records {
		ids = append(ids, r.FeatureA)
	}
	if len(ids) == 0 {
		return map[examination.Pair]string{}, nil
	}
	rows, err := q.ListBiometricDecisionsForFeatures(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := map[examination.Pair]string{}
	for _, r := range records {
		var chain []cluster.ChainEntry
		for _, row := range rows {
			if row.FeatureID == r.FeatureA && row.CounterpartID == r.FeatureB && row.Role != string(cluster.RoleSystem) &&
				row.AnalysisType.String == string(analyses.FacialReview) {
				chain = append(chain, cluster.ChainEntry{Role: cluster.Role(row.Role), Decision: cluster.Decision(row.Decision),
					Analysis: row.CaseAnalysisID.Int64, AnalysisType: row.AnalysisType.String})
			}
		}
		if _, status, ok := cluster.DecidingRound(chain); ok {
			switch status {
			case cluster.EdgeStatusConfirmed:
				out[r.Pair] = examination.Positive
			case cluster.EdgeStatusRejected:
				out[r.Pair] = examination.Negative
			}
		}
	}
	return out, nil
}

type peopleIndex struct {
	byUsername map[string]Person
	byStep     map[analyses.Step]*Person
}

func toPerson(username, first, last, staff, title string) Person {
	name := first
	if last != "" {
		name += " " + last
	}
	if name == "" {
		name = username
	}
	return Person{Username: username, Name: name, StaffNumber: staff, JobTitle: title}
}

// loadPeople resolves the examiners holding the analysis's steps and every username that decided.
func loadPeople(ctx context.Context, q *db.Queries, t analyses.Target, usernames map[string]bool) (peopleIndex, error) {
	out := peopleIndex{byUsername: map[string]Person{}, byStep: map[analyses.Step]*Person{}}
	var ids []int64
	for _, id := range t.Examiners {
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		rows, err := q.GetUserNames(ctx, ids)
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			p := toPerson(r.Username, r.FirstName, r.LastName, r.StaffNumber, r.JobTitle)
			out.byUsername[r.Username] = p
			for step, id := range t.Examiners {
				if id == r.ID {
					pp := p
					out.byStep[step] = &pp
				}
			}
		}
	}
	var names []string
	for u := range usernames {
		if _, ok := out.byUsername[u]; !ok && u != "" {
			names = append(names, u)
		}
	}
	if len(names) > 0 {
		rows, err := q.GetUsersByUsername(ctx, names)
		if err != nil {
			return out, err
		}
		for _, r := range rows {
			out.byUsername[r.Username] = toPerson(r.Username, r.FirstName, r.LastName, r.StaffNumber, r.JobTitle)
		}
	}
	return out, nil
}
