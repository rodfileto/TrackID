package reports_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cases"
	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/reports"
)

func face(c color.Color) reports.Image {
	img := image.NewRGBA(image.Rect(0, 0, 120, 150))
	for x := 0; x < 120; x++ {
		for y := 0; y < 150; y++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return reports.Image{Data: b.Bytes(), ContentType: "image/png"}
}

// sampleLaudo is a comparison with two pairs, one disputed, and two questions.
func sampleLaudo() (reports.Data, map[string]reports.Image) {
	examiner := &reports.Person{Username: "ana", Name: "Ana Souza", StaffNumber: "0001", JobTitle: "Perita"}
	verifier := &reports.Person{Username: "caio", Name: "Caio Lima", StaffNumber: "0002", JobTitle: "Perito"}
	third := &reports.Person{Username: "bia", Name: "Bia Reis", StaffNumber: "0003", JobTitle: "Perita"}
	d := reports.Data{AnalysisID: 7, AnalysisType: analyses.FacialComparison, ReportType: "FACIAL_COMPARISON_REPORT",
		Identification: cluster.ConclusionStrongSupport, GeneratedAt: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
		Case: reports.CaseInfo{CaseID: "2026.0007", CaseType: "CRIMINAL", Description: "Roubo a agência bancária", Occurrence: "2026.0003 (37ª DP 1234/2026)", OccurrenceTypes: "ROUBO"},
		Request: &reports.RequestInfo{Sequence: 2, Authority: "37ª Delegacia de Polícia", DocumentReference: "Ofício nº 456/2026", ReceivedOn: "2026-10-20",
			DueOn: "2026-11-20", Questions: []string{"As imagens retratam a mesma pessoa?", "Há elementos que indiquem disfarce?"},
			Files: []reports.FileInfo{{Filename: "cftv_caixa.png", SHA256: "ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12cd34ef56ab12"}}},
		Questioned: []reports.TraceInfo{{FeatureID: "TRACE:1#feature", Label: "Vestígio 1.1", Evidence: "cftv_caixa.png", EvidenceSHA256: "ab12", Suitability: "SUITABLE"}},
		References: []reports.ReferenceInfo{{FeatureID: "TRACE:2#feature", Kind: "SENT_IN", Label: "Padrão 2.1", Evidence: "suspeito.png"},
			{FeatureID: "101", Kind: "ENROLLED", Label: "Registro SIM-RG-1", Name: "João da Silva", Document: "RG SIM-RG-1", Register: "SIM-RG-1", EnrolledOn: "2019-05-02"}},
		Pairs: []reports.PairInfo{
			{Questioned: "Vestígio 1.1", Counterpart: "Padrão 2.1", QuestionedID: "TRACE:1#feature", CounterpartID: "TRACE:2#feature",
				Steps: []reports.StepDecision{{Step: analyses.StepVerificator, Examiner: "Ana Souza", Decision: "POSITIVE", Conclusion: cluster.ConclusionStrongSupport},
					{Step: analyses.StepReviewer, Examiner: "Caio Lima", Decision: "POSITIVE", Conclusion: cluster.ConclusionStrongSupport}},
				Decision: "POSITIVE", Conclusion: cluster.ConclusionStrongSupport, Result: reports.ResultIdentified, Lead: "POSITIVE", Effect: "CONFIRMS"},
			{Questioned: "Vestígio 1.1", Counterpart: "Registro SIM-RG-1 (João da Silva)", QuestionedID: "TRACE:1#feature", CounterpartID: "101", Disputed: true,
				Steps: []reports.StepDecision{{Step: analyses.StepVerificator, Examiner: "Ana Souza", Decision: "POSITIVE", Conclusion: cluster.ConclusionModerateSupport},
					{Step: analyses.StepReviewer, Examiner: "Caio Lima", Decision: "INCONCLUSIVE", Conclusion: cluster.ConclusionInconclusive},
					{Step: analyses.StepInconsistence, Examiner: "Bia Reis", Decision: "INCONCLUSIVE", Conclusion: cluster.ConclusionInconclusive, Notes: "Resolução insuficiente na região dos olhos."}},
				Decision: "INCONCLUSIVE", Conclusion: cluster.ConclusionInconclusive, Result: reports.ResultInconclusive, Effect: "NEW"},
		},
		Examiners: reports.Examiners{Examiner: examiner, Verifier: verifier, Inconsistence: third},
		Previous:  []reports.PriorReport{{Type: "FACIAL_REVIEW_REPORT", Number: "2026.0004", IssuedAt: time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)}},
	}
	imgs := map[string]reports.Image{"TRACE:1#feature": face(color.RGBA{200, 150, 120, 255}), "TRACE:2#feature": face(color.RGBA{180, 140, 110, 255}),
		"101": face(color.RGBA{190, 145, 115, 255})}
	return d, imgs
}

// TestRenderAndParse renders a laudo and reads it back: the executive summary and data are
// locked, the examiner's sections are regions in order, and the issue placeholders are there for
// the number and date.
func TestRenderAndParse(t *testing.T) {
	d, imgs := sampleLaudo()
	b, err := reports.RenderForTest(d, imgs)
	if err != nil {
		t.Fatal(err)
	}
	locked, regions, err := reports.ParseForTest(b)
	if err != nil {
		t.Fatal(err)
	}
	// objective, material, method, two exams, discussion, two answers
	if len(regions) != 8 || !strings.HasPrefix(regions[0], "Realizar exame de comparação facial") || regions[3] != "" {
		t.Fatalf("regions = %q", regions)
	}
	for _, want := range []string{"Sumário executivo", "Suporte forte à mesma origem", "Ofício nº 456/2026", "(número atribuído na emissão)",
		"Resolução insuficiente na região dos olhos.", "As imagens retratam a mesma pessoa?", "[img:"} {
		if !strings.Contains(locked, want) {
			t.Errorf("the locked text lacks %q", want)
		}
	}
	if strings.Contains(locked, "Realizar exame de comparação facial entre") {
		t.Error("a region's text counted as locked")
	}
}

// TestLeadReport renders a review's report: the investigative-lead notice, a result per face and
// a frame taken from a video named with its time, and only the optional observations editable.
func TestLeadReport(t *testing.T) {
	frame := int64(175)
	d := reports.Data{AnalysisType: analyses.FacialReview, ReportType: "FACIAL_REVIEW_REPORT", Identification: cluster.ConclusionStrongSupport,
		Case: reports.CaseInfo{CaseID: "2026.0009", CaseType: "CRIMINAL"},
		Questioned: []reports.TraceInfo{{FeatureID: "TRACE:5#feature", Label: "Vestígio 3.1", Evidence: "cftv @ 00-00-07.000.png", Suitability: "LEAD_ONLY",
			Source: &cases.VideoSource{Video: "cftv.mp4", TimestampMs: 7000, Frame: &frame}}},
		Method: reports.Method{Systems: []reports.SystemInfo{{Source: "auraface:v1"}}},
		Pairs: []reports.PairInfo{{Questioned: "Vestígio 3.1", Counterpart: "Registro SIM-RG-9 (Maria)", QuestionedID: "TRACE:5#feature", CounterpartID: "9",
			Decision: "POSITIVE", Result: reports.ResultLead}},
		Faces: []reports.FaceResult{{Face: "Vestígio 3.1", Suitability: "LEAD_ONLY", Leads: []string{"Registro SIM-RG-9 (Maria)"}, Result: reports.ResultLead}},
	}
	b, err := reports.RenderForTest(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	locked, regions, err := reports.ParseForTest(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 {
		t.Fatalf("a lead report has one editable section (observations), got %d", len(regions))
	}
	for _, want := range []string{"NÃO constitui identificação", "Indicação investigativa: Registro SIM-RG-9 (Maria)", "quadro de cftv.mp4 em 00:00:07.000 (quadro nº 175)",
		"auraface:v1"} {
		if !strings.Contains(locked, want) {
			t.Errorf("the lead report lacks %q", want)
		}
	}
	if s := reports.Summarize(d); s.Notice == "" || len(s.Faces) != 1 || s.Faces[0].ResultCode != reports.ResultLead || len(s.Results) != 0 {
		t.Errorf("summary = %+v", s)
	}
}

// TestFingerprintLaudo renders a fingerprint comparison's report (confronto papiloscópico): its
// own title, method and conclusions (positive, negative, inconclusive, with no graded scale), and a
// decision instead of a level in each pair's result.
func TestFingerprintLaudo(t *testing.T) {
	d, imgs := sampleLaudo()
	d.AnalysisType, d.ReportType = analyses.FingerprintComparison, "FINGERPRINT_COMPARISON_REPORT"
	for i := range d.Pairs {
		d.Pairs[i].Conclusion = ""
		for j := range d.Pairs[i].Steps {
			d.Pairs[i].Steps[j].Conclusion = ""
		}
	}
	b, err := reports.RenderForTest(d, imgs)
	if err != nil {
		t.Fatal(err)
	}
	locked, regions, err := reports.ParseForTest(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 8 || !strings.HasPrefix(regions[0], "Realizar confronto papiloscópico entre") {
		t.Fatalf("regions = %q", regions)
	}
	for _, want := range []string{"LAUDO DE CONFRONTO PAPILOSCÓPICO", "cristas papilares", "Conclusões possíveis", "procedem do mesmo dedo",
		"fragmento(s) papilar(es) questionado(s)", "Positivo", "Inconclusivo"} {
		if !strings.Contains(locked, want) {
			t.Errorf("the fingerprint laudo lacks %q", want)
		}
	}
	for _, unwanted := range []string{"Escala de conclusões", "Nível mínimo para identificação", "comparação facial", "FISWG"} {
		if strings.Contains(locked, unwanted) {
			t.Errorf("the fingerprint laudo carries the facial %q", unwanted)
		}
	}
}
