package reports

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // pair images may be JPEG
	_ "image/png"
	"strings"
)

// The template's fonts are ones WordCraft bundles in both its web and its server build, so the
// page the examiner edits breaks where the issued PDF does.
const (
	bodyFont    = "Source Serif 4"
	headingFont = "Source Sans 3"
)

// doc builds a .docx: a body of paragraphs, tables and images, a header and a footer, and editable
// regions. The document is protected read-only (w:documentProtection) and each region is a
// w:permStart/w:permEnd range open to everyone, so in WordCraft only the regions can be edited.
type doc struct {
	body    strings.Builder
	header  []string // paragraphs of the header
	footer  string
	media   []mediaFile
	regions []string // region keys, in order; a region's id is its index + 1
	fill    []string // texts for the regions, in order, replacing their defaults (a refresh)
}

type mediaFile struct {
	name        string
	contentType string
	data        []byte
	rid         string
}

func esc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			if r < 0x20 && r != '\t' && r != '\n' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// run is one run of text; bold and italic as asked.
func run(text string, bold, italic bool) string {
	props := ""
	if bold {
		props += "<w:b/>"
	}
	if italic {
		props += "<w:i/>"
	}
	if props != "" {
		props = "<w:rPr>" + props + "</w:rPr>"
	}
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteString("<w:r>" + props + "<w:br/></w:r>")
		}
		b.WriteString(`<w:r>` + props + `<w:t xml:space="preserve">` + esc(line) + `</w:t></w:r>`)
	}
	return b.String()
}

func paragraph(style, align, runs string) string {
	props := ""
	if style != "" {
		props += `<w:pStyle w:val="` + style + `"/>`
	}
	if align != "" {
		props += `<w:jc w:val="` + align + `"/>`
	}
	if props != "" {
		props = "<w:pPr>" + props + "</w:pPr>"
	}
	return "<w:p>" + props + runs + "</w:p>"
}

func (d *doc) heading(level int, text string) {
	d.body.WriteString(paragraph(fmt.Sprintf("Heading%d", level), "", run(text, false, false)))
}

func (d *doc) title(text string) {
	d.body.WriteString(paragraph("Title", "center", run(text, false, false)))
}

func (d *doc) para(text string) {
	d.body.WriteString(paragraph("", "both", run(text, false, false)))
}

func (d *doc) italic(text string) {
	d.body.WriteString(paragraph("Hint", "", run(text, false, true)))
}

// labelled is "Label: value" with the label in bold.
func (d *doc) labelled(label, value string) {
	d.body.WriteString(paragraph("", "", run(label+": ", true, false)+run(value, false, false)))
}

func (d *doc) notice(text string) {
	d.body.WriteString(paragraph("Notice", "center", run(text, true, false)))
}

// region adds an editable region, key naming it, holding one empty paragraph (or text, a default
// the examiner may change). A region's content is the examiner's; everything else is locked.
func (d *doc) region(key, text string) {
	d.regions = append(d.regions, key)
	id := len(d.regions)
	if id <= len(d.fill) {
		text = d.fill[id-1]
	}
	d.body.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Editable"/><w:jc w:val="both"/></w:pPr><w:permStart w:id="%d" w:edGrp="everyone"/>%s<w:permEnd w:id="%d"/></w:p>`,
		id, run(text, false, false), id))
}

// table adds a table: header row in bold, then rows. widths are in twentieths of a point and sum
// to the text width.
func (d *doc) table(widths []int, header []string, rows [][]string) {
	var b strings.Builder
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="0" w:type="auto"/><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid>`)
	for _, w := range widths {
		b.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, w))
	}
	b.WriteString(`</w:tblGrid>`)
	row := func(cells []string, bold bool) {
		b.WriteString(`<w:tr>`)
		if bold {
			b.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for i, c := range cells {
			b.WriteString(fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`, widths[i], paragraph("TableText", "", run(c, bold, false))))
		}
		b.WriteString(`</w:tr>`)
	}
	if header != nil {
		row(header, true)
	}
	for _, r := range rows {
		row(r, false)
	}
	b.WriteString(`</w:tbl>`)
	d.body.WriteString(b.String())
	d.body.WriteString(paragraph("", "", ""))
}

// emuPerTwip converts twentieths of a point to the EMUs drawings are sized in.
const emuPerTwip = 635

// imagePair shows two images side by side with their captions, each at most maxWidth twips wide
// and maxHeight high, keeping its aspect ratio. A missing image leaves its cell with the caption.
func (d *doc) imagePair(left, right *Image, leftCaption, rightCaption string) {
	cell := func(img *Image, caption string) string {
		pic := ""
		if img != nil {
			pic = d.picture(*img, 4200, 4200)
		}
		return `<w:tc><w:tcPr><w:tcW w:w="4535" w:type="dxa"/></w:tcPr>` + paragraph("Figure", "center", pic) + paragraph("Caption", "center", run(caption, false, true)) + `</w:tc>`
	}
	d.body.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid><w:gridCol w:w="4535"/><w:gridCol w:w="4535"/></w:tblGrid><w:tr>` +
		cell(left, leftCaption) + cell(right, rightCaption) + `</w:tr></w:tbl>`)
	d.body.WriteString(paragraph("", "", ""))
}

// picture embeds img as an inline drawing scaled to fit maxW × maxH twips. Its docPr description
// is the image's SHA-256, which the locked check reads back.
func (d *doc) picture(img Image, maxW, maxH int) string {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(img.Data))
	if err != nil || cfg.Width == 0 || cfg.Height == 0 {
		return ""
	}
	w, h := maxW, maxW*cfg.Height/cfg.Width
	if h > maxH {
		w, h = maxH*cfg.Width/cfg.Height, maxH
	}
	ext, ct := "png", "image/png"
	if strings.Contains(img.ContentType, "jpeg") || strings.Contains(img.ContentType, "jpg") {
		ext, ct = "jpeg", "image/jpeg"
	}
	n := len(d.media) + 1
	m := mediaFile{name: fmt.Sprintf("image%d.%s", n, ext), contentType: ct, data: img.Data, rid: fmt.Sprintf("rIdImg%d", n)}
	d.media = append(d.media, m)
	cx, cy := w*emuPerTwip, h*emuPerTwip
	return fmt.Sprintf(`<w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="Imagem %d" descr="sha256:%s"/>`+
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">`+
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="%d" name="%s"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic>`+
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`, cx, cy, n, n, sha(img.Data), n, m.name, m.rid, cx, cy)
}

func (d *doc) pageBreak() {
	d.body.WriteString(`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
}

const nsDoc = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`

const stylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="` + bodyFont + `" w:hAnsi="` + bodyFont + `" w:cs="` + bodyFont + `"/><w:sz w:val="22"/><w:lang w:val="pt-BR"/></w:rPr></w:rPrDefault>
<w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="300" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:before="120" w:after="240"/></w:pPr><w:rPr><w:rFonts w:ascii="` + headingFont + `" w:hAnsi="` + headingFont + `"/><w:b/><w:sz w:val="32"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="280" w:after="120"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:rFonts w:ascii="` + headingFont + `" w:hAnsi="` + headingFont + `"/><w:b/><w:sz w:val="26"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:spacing w:before="200" w:after="80"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:rFonts w:ascii="` + headingFont + `" w:hAnsi="` + headingFont + `"/><w:b/><w:sz w:val="23"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Hint"><w:name w:val="Hint"/><w:basedOn w:val="Normal"/><w:rPr><w:i/><w:color w:val="595959"/><w:sz w:val="20"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Notice"><w:name w:val="Notice"/><w:basedOn w:val="Normal"/><w:pPr><w:pBdr><w:top w:val="single" w:sz="6" w:space="4" w:color="000000"/><w:bottom w:val="single" w:sz="6" w:space="4" w:color="000000"/></w:pBdr><w:jc w:val="center"/></w:pPr><w:rPr><w:b/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Editable"><w:name w:val="Editable"/><w:basedOn w:val="Normal"/></w:style>
<w:style w:type="paragraph" w:styleId="TableText"><w:name w:val="Table Text"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="19"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Figure"><w:name w:val="Figure"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/><w:spacing w:after="0"/></w:pPr></w:style>
<w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/><w:basedOn w:val="Normal"/><w:pPr><w:jc w:val="center"/></w:pPr><w:rPr><w:i/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Header"><w:name w:val="header"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/><w:jc w:val="center"/></w:pPr><w:rPr><w:rFonts w:ascii="` + headingFont + `" w:hAnsi="` + headingFont + `"/><w:sz w:val="18"/></w:rPr></w:style>
<w:style w:type="paragraph" w:styleId="Footer"><w:name w:val="footer"/><w:basedOn w:val="Normal"/><w:pPr><w:spacing w:after="0"/><w:jc w:val="center"/></w:pPr><w:rPr><w:rFonts w:ascii="` + headingFont + `" w:hAnsi="` + headingFont + `"/><w:sz w:val="16"/></w:rPr></w:style>
<w:style w:type="table" w:styleId="TableGrid"><w:name w:val="Table Grid"/><w:tblPr><w:tblBorders><w:top w:val="single" w:sz="4" w:space="0" w:color="808080"/><w:left w:val="single" w:sz="4" w:space="0" w:color="808080"/><w:bottom w:val="single" w:sz="4" w:space="0" w:color="808080"/><w:right w:val="single" w:sz="4" w:space="0" w:color="808080"/><w:insideH w:val="single" w:sz="4" w:space="0" w:color="808080"/><w:insideV w:val="single" w:sz="4" w:space="0" w:color="808080"/></w:tblBorders><w:tblCellMar><w:left w:w="80" w:type="dxa"/><w:right w:w="80" w:type="dxa"/></w:tblCellMar></w:tblPr></w:style>
</w:styles>`

// The document is protected read-only; only the permStart/permEnd regions stay editable.
const settingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:documentProtection w:edit="readOnly" w:enforcement="1"/><w:defaultTabStop w:val="709"/><w:compat/></w:settings>`

// bytes assembles the .docx package.
func (d *doc) bytes() ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name, content string) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	types := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Default Extension="jpeg" ContentType="image/jpeg"/>` +
		`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
		`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
		`<Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>` +
		`<Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>` +
		`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/></Types>`
	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`
	var docRels strings.Builder
	docRels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/><Relationship Id="rIdHeader" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/><Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`)
	for _, m := range d.media {
		docRels.WriteString(`<Relationship Id="` + m.rid + `" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/` + m.name + `"/>`)
	}
	docRels.WriteString(`</Relationships>`)
	// A4, margins 3 cm top and left, 2 cm bottom and right; text width 9071 twips.
	document := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document ` + nsDoc + `><w:body>` + d.body.String() +
		`<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader"/><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1701" w:right="1134" w:bottom="1134" w:left="1701" w:header="709" w:footer="567" w:gutter="0"/></w:sectPr></w:body></w:document>`
	var header strings.Builder
	header.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr ` + nsDoc + `>`)
	for i, h := range d.header {
		header.WriteString(paragraph("Header", "", run(h, i == 0, false)))
	}
	header.WriteString(`</w:hdr>`)
	field := func(instr string) string {
		return `<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> ` + instr + ` </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`
	}
	footer := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr ` + nsDoc + `>` + paragraph("Footer", "", run(d.footer+" — Página ", false, false)+field("PAGE")+run(" de ", false, false)+field("NUMPAGES")) + `</w:ftr>`
	for _, f := range []struct{ name, content string }{
		{"[Content_Types].xml", types}, {"_rels/.rels", rels}, {"word/_rels/document.xml.rels", docRels.String()},
		{"word/document.xml", document}, {"word/styles.xml", stylesXML}, {"word/settings.xml", settingsXML},
		{"word/header1.xml", header.String()}, {"word/footer1.xml", footer},
	} {
		if err := add(f.name, f.content); err != nil {
			return nil, err
		}
	}
	for _, m := range d.media {
		w, err := z.Create("word/media/" + m.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(m.data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
