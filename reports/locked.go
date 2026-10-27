package reports

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// ErrNotDocx is returned for a file that is not a readable .docx.
var ErrNotDocx = errors.New("reports: not a .docx document")

// parsed is a .docx read back: its locked content -- the text of everything outside the editable
// regions, one line per paragraph, with an [img:<sha256>] mark per picture -- and each region's text,
// in document order. Formatting is not compared: an editor may re-split runs or rewrite styles
// without changing what the document says.
type parsed struct {
	locked  string
	regions []string
}

func readZip(data []byte) (map[string][]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrNotDocx
	}
	out := map[string][]byte{}
	for _, f := range z.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[f.Name] = b
	}
	if _, ok := out["word/document.xml"]; !ok {
		return nil, ErrNotDocx
	}
	return out, nil
}

// parseDocx reads a .docx's locked content and regions.
func parseDocx(data []byte) (parsed, error) {
	files, err := readZip(data)
	if err != nil {
		return parsed{}, err
	}
	media := map[string]string{} // relationship id -> sha256 of the image it targets
	if rels, ok := files["word/_rels/document.xml.rels"]; ok {
		var r struct {
			Rels []struct {
				ID     string `xml:"Id,attr"`
				Target string `xml:"Target,attr"`
			} `xml:"Relationship"`
		}
		if err := xml.Unmarshal(rels, &r); err != nil {
			return parsed{}, fmt.Errorf("%w: %v", ErrNotDocx, err)
		}
		for _, rel := range r.Rels {
			target := strings.TrimPrefix(rel.Target, "/")
			if !strings.HasPrefix(target, "word/") {
				target = path.Join("word", target)
			}
			if b, ok := files[target]; ok {
				media[rel.ID] = sha(b)
			}
		}
	}

	dec := xml.NewDecoder(bytes.NewReader(files["word/document.xml"]))
	var locked strings.Builder
	var regions []*strings.Builder
	open := map[string]int{} // permStart id -> region index, while open
	inText := false
	current := func() *strings.Builder {
		// The innermost open region takes the text; outside every region it is locked.
		best := -1
		for _, i := range open {
			if i > best {
				best = i
			}
		}
		if best < 0 {
			return &locked
		}
		return regions[best]
	}
	attr := func(se xml.StartElement, name string) string {
		for _, a := range se.Attr {
			if a.Name.Local == name {
				return a.Value
			}
		}
		return ""
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return parsed{}, fmt.Errorf("%w: %v", ErrNotDocx, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "permStart":
				regions = append(regions, &strings.Builder{})
				open[attr(t, "id")] = len(regions) - 1
			case "permEnd":
				delete(open, attr(t, "id"))
			case "t":
				inText = true
			case "tab":
				current().WriteString("\t")
			case "br", "cr":
				if attr(t, "type") != "page" {
					current().WriteString("\n")
				}
			case "blip":
				current().WriteString("[img:" + media[attr(t, "embed")] + "]")
			case "instrText", "delText":
				_ = dec.Skip() // field codes and deleted text are not what the page says
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				current().WriteString("\n")
			}
		case xml.CharData:
			if inText {
				current().Write(t)
			}
		}
	}
	out := parsed{locked: normalize(locked.String())}
	for _, r := range regions {
		out.regions = append(out.regions, strings.TrimSpace(normalize(r.String())))
	}
	return out, nil
}

// normalize trims each line and drops empty ones, so an editor's spacing choices do not count.
func normalize(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// replaceInDocx replaces text in the document, header and footer parts of a .docx (the issue
// placeholders) and reports how many replacements each old string got.
func replaceInDocx(data []byte, pairs map[string]string) ([]byte, map[string]int, error) {
	files, err := readZip(data)
	if err != nil {
		return nil, nil, err
	}
	src, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, ErrNotDocx
	}
	counts := map[string]int{}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, f := range src.File {
		content := files[f.Name]
		if f.Name == "word/document.xml" || strings.HasPrefix(f.Name, "word/header") || strings.HasPrefix(f.Name, "word/footer") {
			s := string(content)
			for old, repl := range pairs {
				counts[old] += strings.Count(s, esc(old))
				s = strings.ReplaceAll(s, esc(old), esc(repl))
			}
			content = []byte(s)
		}
		fw, err := w.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate})
		if err != nil {
			return nil, nil, err
		}
		if _, err := fw.Write(content); err != nil {
			return nil, nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), counts, nil
}
