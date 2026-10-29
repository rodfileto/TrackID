package reports

import (
	"fmt"
	"strings"
	"time"

	"github.com/rodfileto/trackid/analyses"
	"github.com/rodfileto/trackid/cluster"
)

// The reports are written in Portuguese (docs/facial-reports-plan.md, decision 15); the code
// around them stays in English.

// Institution is the issuing body named in every report's header.
var Institution = "Instituto de Identificação — Perícia em Biometria Facial"

// Placeholders for what a report gets only when issued. They are locked text in the draft and are
// replaced, in the saved document, by IssueReport.
const (
	numberPlaceholder = "(número atribuído na emissão)"
	datePlaceholder   = "(data da emissão)"
)

var conclusionText = map[cluster.Conclusion]string{
	cluster.ConclusionIdentification:   "Identificação",
	cluster.ConclusionStrongSupport:    "Suporte forte à mesma origem",
	cluster.ConclusionModerateSupport:  "Suporte moderado à mesma origem",
	cluster.ConclusionLimitedSupport:   "Suporte limitado à mesma origem",
	cluster.ConclusionInconclusive:     "Inconclusivo",
	cluster.ConclusionLimitedExclusion: "Suporte limitado a origens distintas",
	cluster.ConclusionExclusion:        "Exclusão",
}

var conclusionMeaning = map[cluster.Conclusion]string{
	cluster.ConclusionIdentification:   "as características observadas são concordantes e suficientes para afirmar que as imagens retratam a mesma pessoa.",
	cluster.ConclusionStrongSupport:    "as observações são muito mais prováveis se as imagens retratarem a mesma pessoa do que pessoas distintas.",
	cluster.ConclusionModerateSupport:  "as observações são mais prováveis se as imagens retratarem a mesma pessoa do que pessoas distintas.",
	cluster.ConclusionLimitedSupport:   "as observações favorecem, de forma limitada, a hipótese de mesma pessoa.",
	cluster.ConclusionInconclusive:     "as observações não permitem favorecer nenhuma das hipóteses, em geral por limitação de qualidade das imagens.",
	cluster.ConclusionLimitedExclusion: "as observações favorecem, de forma limitada, a hipótese de pessoas distintas.",
	cluster.ConclusionExclusion:        "as diferenças observadas são incompatíveis com a hipótese de mesma pessoa.",
}

var resultText = map[string]string{
	ResultIdentified:   "Identificado",
	ResultSupported:    "Suporte (abaixo do nível de identificação)",
	ResultInconclusive: "Inconclusivo",
	ResultExcluded:     "Excluído",
	ResultLead:         "Indicação investigativa",
	ResultNoLead:       "Sem indicação",
	ResultUnsuitable:   "Imagem sem qualidade",
	ResultUnsettled:    "Não concluído",
}

var suitabilityText = map[string]string{
	"SUITABLE":   "Apta para comparação",
	"LEAD_ONLY":  "Apta apenas para indicação investigativa",
	"UNSUITABLE": "Sem qualidade",
	"":           "Não avaliada",
}

var decisionText = map[string]string{"POSITIVE": "Positivo", "NEGATIVE": "Negativo", "INCONCLUSIVE": "Inconclusivo"}

var stepText = map[analyses.Step]string{
	analyses.StepVerificator:   "Perito examinador",
	analyses.StepReviewer:      "Perito verificador (cego)",
	analyses.StepInconsistence: "Terceiro perito (divergência)",
}

var effectText = map[string]string{
	"CONFIRMS":        "confirma a indicação anterior",
	"CHANGES":         "altera a indicação anterior",
	"LEAVES_STANDING": "não altera a indicação anterior, que permanece",
	"NEW":             "sem indicação anterior",
}

var reportTypeText = map[string]string{
	"FACIAL_REVIEW_REPORT":          "Relatório de Indicação Investigativa",
	"FACIAL_COMPARISON_REPORT":      "Laudo de Comparação Facial",
	"FINGERPRINT_COMPARISON_REPORT": "Laudo de Confronto Papiloscópico",
}

// leadNotice is the fixed notice every lead report carries.
const leadNotice = "Este relatório constitui indicação investigativa e NÃO constitui identificação. " +
	"A indicação deve ser confirmada por exame de comparação facial (laudo) ou por outros meios de prova."

func dateBR(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02/01/2006")
	}
	return iso
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func personLine(p *Person) string {
	if p == nil {
		return "—"
	}
	s := p.Name
	if p.JobTitle != "" {
		s += ", " + p.JobTitle
	}
	if p.StaffNumber != "" {
		s += ", matrícula " + p.StaffNumber
	}
	return s
}

func (d Data) number() string {
	if d.Number == "" {
		return numberPlaceholder
	}
	return d.Number
}

func (d Data) issueDate() string {
	if d.IssuedAt == nil {
		return datePlaceholder
	}
	return d.IssuedAt.In(saoPaulo).Format("02/01/2006")
}

var saoPaulo = func() *time.Location {
	if l, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return l
	}
	return time.UTC
}()

// Region keys: the editable sections, in document order. A comparison has one exam region per
// pair and one answer per question.
const (
	regionObjective   = "objective"
	regionMaterial    = "material"
	regionMethodNotes = "method"
	regionDiscussion  = "discussion"
	regionObservation = "observations"
)

func examRegion(i int) string   { return fmt.Sprintf("exam.%d", i+1) }
func answerRegion(i int) string { return fmt.Sprintf("answer.%d", i+1) }

// requiredRegions are the regions a laudo cannot be issued with empty.
func (d Data) requiredRegions() []string {
	if !d.AnalysisType.Comparison() {
		return nil
	}
	var out []string
	for i := range d.Pairs {
		out = append(out, examRegion(i))
	}
	if d.Request != nil {
		for i := range d.Request.Questions {
			out = append(out, answerRegion(i))
		}
	}
	return out
}

// render lays a report out: the executive summary, then the sections of its type. imgs are the
// faces by feature id (Build's).
func render(d Data, imgs map[string]Image) (*doc, error) {
	out := &doc{}
	layout(out, d, imgs)
	return out, nil
}

// renderFilled is render with the editable regions holding texts, in order (a refresh).
func renderFilled(d Data, imgs map[string]Image, texts []string) (*doc, error) {
	out := &doc{fill: texts}
	layout(out, d, imgs)
	return out, nil
}

func layout(out *doc, d Data, imgs map[string]Image) {
	title := reportTypeText[d.ReportType]
	out.header = []string{Institution, title + " nº " + d.number() + " — Caso " + d.Case.CaseID}
	out.footer = title + " nº " + d.number()
	out.title(strings.ToUpper(title))
	out.notice("Nº " + d.number() + " · Caso " + d.Case.CaseID + " · Emitido em " + d.issueDate())
	if d.Complements != "" {
		out.para("Este documento complementa o " + strings.ToLower(title) + " nº " + d.Complements + ".")
	}
	summary(out, d)
	out.pageBreak()
	if d.AnalysisType.Comparison() {
		laudo(out, d, imgs)
	} else {
		leadReport(out, d, imgs)
	}
}

// summary writes the executive summary (sumário executivo): data only, one page.
func summary(out *doc, d Data) {
	s := Summarize(d)
	out.heading(1, "Sumário executivo")
	rows := [][]string{
		{"Documento", reportTypeText[d.ReportType] + " nº " + d.number() + ", emitido em " + d.issueDate()},
		{"Caso", d.Case.CaseID + " (" + map[string]string{"CRIMINAL": "criminal", "CIVIL": "cível"}[d.Case.CaseType] + ")"},
		{"Ocorrência", orDash(d.Case.Occurrence) + tail(d.Case.OccurrenceTypes)},
	}
	if d.Request != nil {
		rows = append(rows, []string{"Requisição", fmt.Sprintf("%s — %s, recebida em %s%s; %d quesito(s)", d.Request.Authority, d.Request.DocumentReference,
			dateBR(d.Request.ReceivedOn), dueTail(d.Request.DueOn), len(d.Request.Questions))})
	} else {
		rows = append(rows, []string{"Requisição", "—"})
	}
	rows = append(rows,
		[]string{"Material questionado", s.MaterialLine},
		[]string{"Material de referência", s.ReferenceLine},
		[]string{"Verificação", s.VerificationLine},
	)
	if len(s.PreviousLines) > 0 {
		rows = append(rows, []string{"Documentos anteriores", strings.Join(s.PreviousLines, "; ")})
	}
	rows = append(rows, []string{"Peritos", s.ExaminersLine})
	out.table([]int{2400, 6671}, nil, rows)

	out.heading(2, "Resultados")
	if d.AnalysisType.Comparison() {
		var r [][]string
		for _, p := range s.Results {
			r = append(r, []string{p.Questioned, p.Counterpart, p.Level, p.Result, p.Lead})
		}
		if len(r) == 0 {
			r = [][]string{{"—", "—", "—", "—", "—"}}
		}
		out.table([]int{1500, 2500, 1900, 1600, 1571}, []string{"Questionado", "Referência", "Conclusão", "Resultado", "Indicação anterior"}, r)
	} else {
		var r [][]string
		for _, f := range s.Faces {
			r = append(r, []string{f.Face, f.Suitability, f.Result, f.Leads})
		}
		if len(r) == 0 {
			r = [][]string{{"—", "—", "—", "—"}}
		}
		out.table([]int{1700, 2300, 1900, 3171}, []string{"Face", "Qualidade", "Resultado", "Indicado(s)"}, r)
		out.notice(leadNotice)
	}
}

func tail(s string) string {
	if s == "" {
		return ""
	}
	return " — " + s
}

func dueTail(due string) string {
	if due == "" {
		return ""
	}
	return ", prazo " + dateBR(due)
}

func preamble(out *doc, d Data, n int) {
	out.heading(1, fmt.Sprintf("%d. Preâmbulo", n))
	who := personLine(d.Examiners.Examiner)
	req := "sem requisição registrada"
	if d.Request != nil {
		req = fmt.Sprintf("em atendimento a %s, de %s", d.Request.DocumentReference, d.Request.Authority)
		if d.Request.DocumentDate != "" {
			req += ", datado de " + dateBR(d.Request.DocumentDate)
		}
		req += ", recebido em " + dateBR(d.Request.ReceivedOn)
	}
	out.para(fmt.Sprintf("Em %s, %s, %s, procedeu ao exame descrito neste documento, referente ao caso %s.", d.issueDate(), who, req, d.Case.CaseID))
}

func history(out *doc, d Data, n int) {
	out.heading(1, fmt.Sprintf("%d. Histórico", n))
	if d.Case.Description != "" {
		out.labelled("Descrição do caso", d.Case.Description)
	}
	out.labelled("Ocorrência", orDash(d.Case.Occurrence)+tail(d.Case.OccurrenceTypes))
	if d.Request != nil && d.Request.CitedReference != "" {
		out.labelled("Referência citada na requisição", d.Request.CitedReference)
	}
	if len(d.Previous) == 0 {
		out.labelled("Documentos anteriores do caso", "nenhum")
	} else {
		var lines []string
		for _, p := range d.Previous {
			lines = append(lines, fmt.Sprintf("%s nº %s (%s)", reportTypeText[p.Type], p.Number, p.IssuedAt.In(saoPaulo).Format("02/01/2006")))
		}
		out.labelled("Documentos anteriores do caso", strings.Join(lines, "; "))
	}
}

func questionedTable(out *doc, d Data) {
	var rows [][]string
	for _, q := range d.Questioned {
		file := orDash(q.Evidence)
		if q.Source != nil {
			file = fmt.Sprintf("quadro de %s em %s", q.Source.Video, clock(q.Source.TimestampMs))
			if q.Source.Frame != nil {
				file += fmt.Sprintf(" (quadro nº %d)", *q.Source.Frame)
			}
		}
		rows = append(rows, []string{q.Label, file, shortHash(q.EvidenceSHA256), suitabilityText[q.Suitability] + notesTail(q.SuitabilityNotes)})
	}
	if len(rows) == 0 {
		rows = [][]string{{"—", "—", "—", "—"}}
	}
	out.table([]int{1500, 2700, 1900, 2971}, []string{"Vestígio", "Arquivo", "SHA-256 (início)", "Qualidade"}, rows)
}

// clock is a time in a video as HH:MM:SS.mmm.
func clock(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func notesTail(s string) string {
	if s == "" {
		return ""
	}
	return " — " + s
}

func shortHash(h string) string {
	if len(h) > 16 {
		return h[:16] + "…"
	}
	return orDash(h)
}

func receivedFiles(out *doc, d Data) {
	if d.Request == nil || len(d.Request.Files) == 0 {
		return
	}
	var rows [][]string
	for _, f := range d.Request.Files {
		rows = append(rows, []string{orDash(f.Filename), f.SHA256})
	}
	out.para("Arquivos recebidos com a requisição (SHA-256 integral):")
	out.table([]int{3000, 6071}, []string{"Arquivo", "SHA-256"}, rows)
}

// laudo writes the sections of a comparison's report: a FACIAL_COMPARISON_REPORT, on the graded
// conclusion scale, or a FINGERPRINT_COMPARISON_REPORT (confronto papiloscópico), concluding
// positive, negative or inconclusive.
func laudo(out *doc, d Data, imgs map[string]Image) {
	fingerprint := d.AnalysisType == analyses.FingerprintComparison
	exam := "exame de comparação facial"
	if fingerprint {
		exam = "confronto papiloscópico"
	}
	preamble(out, d, 1)
	history(out, d, 2)

	out.heading(1, "3. Objetivo")
	var pairs []string
	for _, p := range d.Pairs {
		pairs = append(pairs, p.Questioned+" × "+p.Counterpart)
	}
	out.region(regionObjective, "Realizar "+exam+" entre "+strings.Join(pairs, "; ")+", em resposta aos quesitos formulados.")

	out.heading(1, "4. Material")
	out.heading(2, "4.1 Material questionado")
	questionedTable(out, d)
	receivedFiles(out, d)
	out.heading(2, "4.2 Material de referência")
	var rows [][]string
	for _, r := range d.References {
		origin := "Encaminhado com a requisição"
		detail := orDash(r.Evidence)
		if r.Kind == "ENROLLED" {
			origin = "Cadastro de identificação"
			detail = fmt.Sprintf("%s; %s; registro %s de %s", r.Name, r.Document, r.Register, dateBR(r.EnrolledOn))
		}
		rows = append(rows, []string{r.Label, origin, detail, shortHash(r.ImageSHA256)})
	}
	if len(rows) == 0 {
		rows = [][]string{{"—", "—", "—", "—"}}
	}
	out.table([]int{1700, 2000, 3571, 1800}, []string{"Padrão", "Origem", "Identificação", "Imagem (SHA-256)"}, rows)
	out.italic("Descrição do material recebido (acondicionamento, formato, condições):")
	out.region(regionMaterial, "")

	out.heading(1, "5. Método")
	if fingerprint {
		out.para("O exame seguiu a metodologia ACE-V (Análise, Comparação, Avaliação e Verificação), com análise das " +
			"cristas papilares em seus três níveis de detalhe: o desenho geral, os pontos característicos (minúcias) e os " +
			"detalhes das cristas. Cada par foi examinado pelo perito examinador e, de forma independente e cega, por um " +
			"perito verificador; divergências foram dirimidas por um terceiro perito. Sistemas automatizados de identificação " +
			"papiloscópica servem apenas para localizar candidatos e não fundamentam as conclusões deste laudo.")
		out.para("Conclusões possíveis:")
		out.table([]int{2600, 6471}, []string{"Conclusão", "Significado"}, [][]string{
			{decisionText["POSITIVE"], "Os fragmentos procedem do mesmo dedo (identificação)."},
			{decisionText["NEGATIVE"], "Os fragmentos não procedem do mesmo dedo (exclusão)."},
			{decisionText["INCONCLUSIVE"], "Os elementos disponíveis não permitem concluir pela identificação nem pela exclusão."},
		})
	} else {
		out.para("O exame seguiu a metodologia ACE-V (Análise, Comparação, Avaliação e Verificação), com análise morfológica " +
			"das características faciais conforme as diretrizes do FISWG (Facial Identification Scientific Working Group). " +
			"Cada par foi examinado pelo perito examinador e, de forma independente e cega, por um perito verificador; " +
			"divergências foram dirimidas por um terceiro perito. Sistemas de reconhecimento facial automatizado servem apenas " +
			"para localizar candidatos e não fundamentam as conclusões deste laudo.")
		out.para("Escala de conclusões:")
		var scale [][]string
		for _, c := range cluster.Conclusions {
			scale = append(scale, []string{conclusionText[c], conclusionMeaning[c]})
		}
		out.table([]int{2600, 6471}, []string{"Conclusão", "Significado"}, scale)
		out.para("Nível mínimo para identificação: " + conclusionText[d.Identification] + ".")
	}
	out.italic("Observações sobre o método (opcional):")
	out.region(regionMethodNotes, "")

	out.heading(1, "6. Exames")
	for i, p := range d.Pairs {
		out.heading(2, fmt.Sprintf("6.%d %s × %s", i+1, p.Questioned, p.Counterpart))
		l, r := faceImage(imgs, p.QuestionedID), faceImage(imgs, p.CounterpartID)
		out.imagePair(l, r, p.Questioned, p.Counterpart)
		if fingerprint {
			out.italic("Análise do par: qualidade dos fragmentos, desenho, pontos característicos confrontados, concordâncias, discordâncias e sua explicação.")
		} else {
			out.italic("Análise do par: qualidade e limitações das imagens, características comparadas, concordâncias, discordâncias e sua explicação.")
		}
		out.region(examRegion(i), "")
	}

	out.heading(1, "7. Processo de verificação")
	out.labelled("Perito examinador", personLine(d.Examiners.Examiner))
	out.labelled("Perito verificador (exame independente e cego)", personLine(d.Examiners.Verifier))
	if d.Examiners.Inconsistence != nil {
		out.labelled("Terceiro perito", personLine(d.Examiners.Inconsistence))
	}
	var steps [][]string
	for _, p := range d.Pairs {
		for _, st := range p.Steps {
			steps = append(steps, []string{p.Questioned + " × " + p.Counterpart, stepText[st.Step], st.Examiner, levelOrDecision(st.Conclusion, st.Decision), orDash(st.Notes)})
		}
	}
	if len(steps) > 0 {
		out.table([]int{1900, 1700, 1700, 1700, 2071}, []string{"Par", "Etapa", "Perito", "Conclusão", "Observações"}, steps)
	}
	disputed := 0
	for _, p := range d.Pairs {
		if p.Disputed {
			disputed++
		}
	}
	if disputed == 0 {
		out.para("O perito examinador e o perito verificador chegaram à mesma conclusão em todos os pares.")
	} else {
		out.para(fmt.Sprintf("Houve divergência entre o perito examinador e o perito verificador em %d par(es); a conclusão final desses pares é a do terceiro perito, registrada acima com sua fundamentação.", disputed))
	}

	out.heading(1, "8. Conclusão")
	var concl [][]string
	for _, p := range d.Pairs {
		lead := "—"
		if p.Lead != "" || p.Effect != "" {
			lead = effectText[p.Effect]
		}
		concl = append(concl, []string{p.Questioned + " × " + p.Counterpart, levelOrDecision(p.Conclusion, p.Decision), resultText[p.Result], lead})
	}
	if len(concl) == 0 {
		concl = [][]string{{"—", "—", "—", "—"}}
	}
	out.table([]int{2400, 2300, 2000, 2371}, []string{"Par", "Conclusão", "Resultado", "Em relação à indicação"}, concl)
	out.italic("Discussão (opcional):")
	out.region(regionDiscussion, "")

	out.heading(1, "9. Resposta aos quesitos")
	if d.Request == nil || len(d.Request.Questions) == 0 {
		out.para("A requisição não formulou quesitos.")
	} else {
		for i, q := range d.Request.Questions {
			out.labelled(fmt.Sprintf("Quesito %d", i+1), q)
			out.region(answerRegion(i), "")
		}
	}

	closing(out, d, 10)
}

func levelOrDecision(c cluster.Conclusion, decision string) string {
	if c != "" {
		return conclusionText[c]
	}
	if decision == "" {
		return "—"
	}
	return decisionText[decision]
}

func faceImage(imgs map[string]Image, id string) *Image {
	if img, ok := imgs[id]; ok {
		return &img
	}
	return nil
}

// leadReport writes the sections of a FACIAL_REVIEW_REPORT.
func leadReport(out *doc, d Data, imgs map[string]Image) {
	out.notice(leadNotice)
	preamble(out, d, 1)
	history(out, d, 2)

	out.heading(1, "3. Material questionado")
	questionedTable(out, d)
	receivedFiles(out, d)

	out.heading(1, "4. Método")
	var systems []string
	for _, s := range d.Method.Systems {
		line := s.Source
		if s.Threshold != nil {
			line += fmt.Sprintf(" (limiar de similaridade %.2f)", *s.Threshold)
		}
		systems = append(systems, line)
	}
	out.para("As faces questionadas foram pesquisadas no cadastro de identificação por sistema de reconhecimento facial automatizado (" +
		orDash(strings.Join(systems, "; ")) + "). Os candidatos apresentados pelo sistema foram examinados pelo perito examinador e, " +
		"de forma independente e cega, por um perito verificador; divergências foram dirimidas por um terceiro perito.")

	out.heading(1, "5. Resultado")
	for _, f := range d.Faces {
		res := resultText[f.Result]
		if len(f.Leads) > 0 {
			res += ": " + strings.Join(f.Leads, "; ")
		}
		out.labelled(f.Face, res)
	}
	if len(d.Faces) == 0 {
		out.para("Nenhuma face questionada foi examinada.")
	}
	var rows [][]string
	for _, p := range d.Pairs {
		score := "—"
		if p.Score != nil {
			score = fmt.Sprintf("%.3f", *p.Score)
		}
		rows = append(rows, []string{p.Questioned, p.Counterpart, score, resultText[p.Result]})
	}
	if len(rows) > 0 {
		out.para("Candidatos examinados:")
		out.table([]int{1800, 3800, 1200, 2271}, []string{"Face", "Candidato", "Escore", "Resultado"}, rows)
	}
	out.italic("Observações (opcional):")
	out.region(regionObservation, "")
	closing(out, d, 6)
}

func closing(out *doc, d Data, n int) {
	out.heading(1, fmt.Sprintf("%d. Encerramento", n))
	out.para(fmt.Sprintf("Nada mais havendo a consignar, encerra-se o presente %s nº %s, emitido em %s.", strings.ToLower(reportTypeText[d.ReportType]),
		d.number(), d.issueDate()))
	out.para("\n\n______________________________________\n" + personLine(d.Examiners.Examiner) + "\nPerito examinador")
	if d.AnalysisType.Comparison() && d.Examiners.Verifier != nil {
		out.para("\n______________________________________\n" + personLine(d.Examiners.Verifier) + "\nPerito verificador")
	}
	out.italic("A integridade deste documento é verificável pelo SHA-256 do arquivo PDF registrado no TrackID.")
}
