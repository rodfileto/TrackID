package reports

import (
	"errors"
	"sync"
)

// A deployment's own layout for a report type (docs/scene-examination-plan.md, decision 7). Core
// lays out the facial reports and the fingerprint comparison; an institute's scene laudo follows
// its own format, so core has none: a scene examination's report exists only where a layout is
// registered for it, from the deployment's main before the server starts.

// Layout writes a report into doc from its data. imgs are the images Build loaded, by feature id
// (the lifts and references of the pairs, the questioned traces). It must:
//   - write Number() and IssueDate() where the number and date go: issuing replaces them;
//   - add, in this order, a region per pair (ExamRegion) and one per question (AnswerRegion): a
//     laudo is not issued with them empty. Other regions are free.
type Layout func(doc *Document, d Data, imgs map[string]Image) error

// ErrNoLayout is a report type core lays no layout out for, and no deployment registered.
var ErrNoLayout = errors.New("no layout is registered for this report type")

var (
	layoutsMu sync.RWMutex
	layouts   = map[string]Layout{}
)

// RegisterLayout makes layout the one for reportType (analysis_reports.report_type, e.g.
// "CRIME_SCENE_EXAMINATION_REPORT"), replacing core's or an earlier one.
func RegisterLayout(reportType string, layout Layout) {
	layoutsMu.Lock()
	defer layoutsMu.Unlock()
	layouts[reportType] = layout
}

func registeredLayout(reportType string) Layout {
	layoutsMu.RLock()
	defer layoutsMu.RUnlock()
	return layouts[reportType]
}

// coreLayouts are the report types core lays out itself.
var coreLayouts = map[string]bool{
	"FACIAL_REVIEW_REPORT": true, "FACIAL_COMPARISON_REPORT": true, "FINGERPRINT_COMPARISON_REPORT": true,
}

// HasLayout reports whether a report of reportType can be drafted here.
func HasLayout(reportType string) bool {
	return coreLayouts[reportType] || registeredLayout(reportType) != nil
}

// Document is the .docx a Layout writes: paragraphs, headings, tables and images, a header and a
// footer, and the regions the examiner edits. Everything outside a region is locked.
type Document struct{ d *doc }

// Header sets the header's lines; the first is bold.
func (w *Document) Header(lines ...string) { w.d.header = lines }

// Footer sets the footer's text; the page number follows it.
func (w *Document) Footer(text string) { w.d.footer = text }

func (w *Document) Title(text string)              { w.d.title(text) }
func (w *Document) Heading(level int, text string) { w.d.heading(level, text) }
func (w *Document) Para(text string)               { w.d.para(text) }
func (w *Document) Hint(text string)               { w.d.italic(text) }
func (w *Document) Labelled(label, value string)   { w.d.labelled(label, value) }
func (w *Document) Notice(text string)             { w.d.notice(text) }
func (w *Document) PageBreak()                     { w.d.pageBreak() }
func (w *Document) Region(key, defaultText string) { w.d.region(key, defaultText) }
func (w *Document) Table(widths []int, header []string, rows [][]string) {
	w.d.table(widths, header, rows)
}

// ImagePair shows two images side by side with their captions; either may be nil.
func (w *Document) ImagePair(left, right *Image, leftCaption, rightCaption string) {
	w.d.imagePair(left, right, leftCaption, rightCaption)
}

// Figure shows one image, at most maxW × maxH twips, with its caption below.
func (w *Document) Figure(img Image, maxW, maxH int, caption string) {
	w.d.body.WriteString(paragraph("Figure", "center", w.d.picture(img, maxW, maxH)))
	if caption != "" {
		w.d.body.WriteString(paragraph("Caption", "center", run(caption, false, true)))
	}
}

// TextWidth is the page's text width in twips (A4, 3 cm and 2 cm margins), for table columns.
const TextWidth = 9071

// ExamRegion and AnswerRegion are the keys of a laudo's required regions: the i-th pair's
// examination (from 0) and the answer to the i-th question.
func ExamRegion(i int) string   { return examRegion(i) }
func AnswerRegion(i int) string { return answerRegion(i) }

// Number is the report's number, or the placeholder issuing replaces.
func (d Data) NumberText() string { return d.number() }

// IssueDate is the report's date (dd/mm/yyyy), or the placeholder issuing replaces.
func (d Data) IssueDateText() string { return d.issueDate() }

// ResultText is a pair's result in the words core's laudos use.
func (d Data) ResultText(result string) string { return resultLabel(d.AnalysisType, result) }
