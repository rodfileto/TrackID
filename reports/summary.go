package reports

import (
	"fmt"
	"github.com/rodfileto/trackid/auth"
	"strings"
)

// Summary is the executive summary (sumário executivo): a report's essentials, from its data
// alone. The report's first page prints it, and the analysis and case pages show it, marked
// preliminary, before the report is issued.
type Summary struct {
	Preliminary      bool         `json:"preliminary"`
	ReportType       string       `json:"reportType"`
	Title            string       `json:"title"`
	Number           string       `json:"number,omitempty"`
	CaseID           string       `json:"caseId"`
	Occurrence       string       `json:"occurrence,omitempty"`
	Request          string       `json:"request,omitempty"`
	Questions        int          `json:"questions"`
	MaterialLine     string       `json:"material"`
	ReferenceLine    string       `json:"references"`
	VerificationLine string       `json:"verification"`
	PreviousLines    []string     `json:"previous"`
	ExaminersLine    string       `json:"examiners"`
	Results          []ResultLine `json:"results,omitempty"` // a laudo's, per pair
	Faces            []FaceLine   `json:"faces,omitempty"`   // a lead report's, per face
	Notice           string       `json:"notice,omitempty"`
}

// ResultLine is one pair of a laudo in the summary.
type ResultLine struct {
	Questioned  string `json:"questioned"`
	Counterpart string `json:"counterpart"`
	Level       string `json:"level"`
	Result      string `json:"result"`
	ResultCode  string `json:"resultCode"`
	Lead        string `json:"lead"`
}

// FaceLine is one face of a lead report in the summary.
type FaceLine struct {
	Face        string `json:"face"`
	Suitability string `json:"suitability"`
	Result      string `json:"result"`
	ResultCode  string `json:"resultCode"`
	Leads       string `json:"leads"`
}

// Summarize builds the executive summary of d.
func Summarize(d Data) Summary {
	s := Summary{Preliminary: d.Number == "", ReportType: d.ReportType, Title: reportTypeText[d.ReportType], Number: d.Number,
		CaseID: d.Case.CaseID, Occurrence: d.Case.Occurrence, PreviousLines: []string{}}
	if d.Request != nil {
		s.Request = d.Request.Authority + " — " + d.Request.DocumentReference
		s.Questions = len(d.Request.Questions)
	}

	counts := map[string]int{}
	for _, q := range d.Questioned {
		counts[q.Suitability]++
	}
	var parts []string
	for _, k := range []string{"SUITABLE", "LEAD_ONLY", "UNSUITABLE", ""} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], strings.ToLower(suitabilityText[k])))
		}
	}
	unit := "face(s) questionada(s)"
	if d.AnalysisType.Modality() == auth.ModalityFingerprint {
		unit = "fragmento(s) papilar(es) questionado(s)"
	}
	s.MaterialLine = fmt.Sprintf("%d %s", len(d.Questioned), unit)
	if len(parts) > 0 {
		s.MaterialLine += ": " + strings.Join(parts, ", ")
	}
	if d.Request != nil && len(d.Request.Files) > 0 {
		s.MaterialLine += fmt.Sprintf("; %d arquivo(s) recebido(s)", len(d.Request.Files))
	}
	sent, enrolled := 0, 0
	for _, r := range d.References {
		if r.Kind == "ENROLLED" {
			enrolled++
		} else {
			sent++
		}
	}
	s.ReferenceLine = fmt.Sprintf("%d encaminhada(s) com a requisição, %d do cadastro de identificação", sent, enrolled)

	disputed, unsettled := 0, 0
	for _, p := range d.Pairs {
		if p.Disputed {
			disputed++
		}
		if p.Result == ResultUnsettled {
			unsettled++
		}
	}
	switch {
	case len(d.Pairs) == 0:
		s.VerificationLine = "nenhum par examinado"
	case disputed == 0:
		s.VerificationLine = fmt.Sprintf("examinador e verificador concordaram nos %d par(es)", len(d.Pairs))
	default:
		s.VerificationLine = fmt.Sprintf("%d de %d par(es) dirimido(s) por terceiro perito", disputed, len(d.Pairs))
	}
	if unsettled > 0 {
		s.VerificationLine += fmt.Sprintf("; %d par(es) ainda não concluído(s)", unsettled)
	}
	for _, p := range d.Previous {
		s.PreviousLines = append(s.PreviousLines, reportTypeText[p.Type]+" nº "+p.Number)
	}
	var names []string
	for _, p := range []*Person{d.Examiners.Examiner, d.Examiners.Verifier, d.Examiners.Inconsistence} {
		if p != nil {
			names = append(names, p.Name)
		}
	}
	s.ExaminersLine = orDash(strings.Join(names, ", "))

	for _, p := range d.Pairs {
		lead := "—"
		if p.Effect != "" {
			lead = effectText[p.Effect]
		}
		s.Results = append(s.Results, ResultLine{Questioned: p.Questioned, Counterpart: p.Counterpart, Level: levelOrDecision(p.Conclusion, p.Decision),
			Result: resultLabel(d.AnalysisType, p.Result), ResultCode: p.Result, Lead: lead})
	}
	for _, f := range d.Faces {
		s.Faces = append(s.Faces, FaceLine{Face: f.Face, Suitability: suitabilityText[f.Suitability], Result: resultText[f.Result], ResultCode: f.Result,
			Leads: orDash(strings.Join(f.Leads, "; "))})
	}
	if d.ReportType == "FACIAL_REVIEW_REPORT" {
		s.Notice = leadNotice
		s.Results = nil
	}
	return s
}
