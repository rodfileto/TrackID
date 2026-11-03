package articles

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A document as TipTap sends it: editor defaults on the link mark, a mention with TipTap's own
// extra attribute, and an ordered list starting at 1.
const tiptapDoc = `{
  "type": "doc",
  "content": [
    {"type": "heading", "attrs": {"level": 2, "textAlign": null}, "content": [{"type": "text", "text": "Background"}]},
    {"type": "paragraph", "content": [
      {"type": "text", "text": "Identified in "},
      {"type": "mention", "attrs": {"kind": "CASE", "ref": "2026.0042", "label": "2026.0042", "mentionSuggestionChar": "@"}},
      {"type": "text", "text": " and in "},
      {"type": "mention", "attrs": {"kind": "OCCURRENCE", "ref": "0017", "label": "DP-01 123/2026"}},
      {"type": "text", "marks": [{"type": "link", "attrs": {"href": "https://example.org/x", "target": "_blank", "rel": "noopener noreferrer nofollow", "class": null}}], "text": "source"},
      {"type": "hardBreak"},
      {"type": "text", "marks": [{"type": "italic"}, {"type": "bold"}], "text": "again "},
      {"type": "mention", "attrs": {"kind": "CASE", "ref": "2026.0042", "label": "2026.0042"}}
    ]},
    {"type": "orderedList", "attrs": {"start": 1, "type": null}, "content": [
      {"type": "listItem", "content": [
        {"type": "paragraph", "content": [{"type": "text", "text": "one"}]},
        {"type": "bulletList", "content": [{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "inner"}]}]}]}
      ]},
      {"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "two"}]}]}
    ]},
    {"type": "blockquote", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "quoted"}]}]},
    {"type": "paragraph"}
  ]
}`

func TestParseBodyCanonical(t *testing.T) {
	body, err := ParseBody([]byte(tiptapDoc))
	if err != nil {
		t.Fatal(err)
	}
	canonical := string(body.Canonical)
	for _, dropped := range []string{"textAlign", "mentionSuggestionChar", "_blank", "nofollow", `"class"`, `"start"`} {
		if strings.Contains(canonical, dropped) {
			t.Errorf("canonical body keeps %s: %s", dropped, canonical)
		}
	}
	if !strings.Contains(canonical, `"ref":"17"`) {
		t.Errorf("occurrence ref not normalized to 17: %s", canonical)
	}

	wantText := "Background\n\n" +
		"Identified in @2026.0042 and in @DP-01 123/2026source\nagain @2026.0042\n\n" +
		"1. one\n  - inner\n2. two\n\n" +
		"> quoted\n\n"
	if body.Text != wantText {
		t.Errorf("plain text:\n%q\nwant\n%q", body.Text, wantText)
	}
	if body.Chars != len([]rune(wantText)) {
		t.Errorf("chars = %d", body.Chars)
	}

	want := []MentionCount{
		{MentionKey{KindCase, "2026.0042"}, 2},
		{MentionKey{KindOccurrence, "17"}, 1},
	}
	if len(body.Mentions) != len(want) {
		t.Fatalf("mentions = %+v", body.Mentions)
	}
	for i := range want {
		if body.Mentions[i] != want[i] {
			t.Errorf("mention %d = %+v, want %+v", i, body.Mentions[i], want[i])
		}
	}

	// Parsing the canonical form again changes nothing: same bytes, same hash.
	again, err := ParseBody(body.Canonical)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Canonical) != canonical || again.SHA256 != body.SHA256 {
		t.Errorf("canonical form not stable:\n%s\n%s", canonical, again.Canonical)
	}

	// Mark order does not change the hash.
	swapped := strings.Replace(tiptapDoc, `[{"type": "italic"}, {"type": "bold"}]`, `[{"type": "bold"}, {"type": "italic"}]`, 1)
	other, err := ParseBody([]byte(swapped))
	if err != nil {
		t.Fatal(err)
	}
	if other.SHA256 != body.SHA256 {
		t.Error("mark order changes the hash")
	}
}

func TestParseBodyRejects(t *testing.T) {
	para := func(inline string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[` + inline + `]}]}`
	}
	for name, doc := range map[string]string{
		"not json":            `{`,
		"not a doc":           `{"type":"paragraph"}`,
		"unknown field":       `{"type":"doc","html":"<b>x</b>"}`,
		"unknown block":       `{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"x"}]}]}`,
		"image":               `{"type":"doc","content":[{"type":"image","attrs":{"src":"https://x/y.png"}}]}`,
		"h1":                  `{"type":"doc","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"x"}]}]}`,
		"heading no level":    `{"type":"doc","content":[{"type":"heading","content":[{"type":"text","text":"x"}]}]}`,
		"unknown mark":        para(`{"type":"text","text":"x","marks":[{"type":"code"}]}`),
		"javascript link":     para(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}`),
		"data link":           para(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"data:text/html,x"}}]}`),
		"relative link":       para(`{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/cases/1"}}]}`),
		"repeated mark":       para(`{"type":"text","text":"x","marks":[{"type":"bold"},{"type":"bold"}]}`),
		"empty text":          para(`{"type":"text","text":""}`),
		"NUL in text":         para(`{"type":"text","text":"a\u0000b"}`),
		"mention bad kind":    para(`{"type":"mention","attrs":{"kind":"VEHICLE","ref":"1"}}`),
		"mention no ref":      para(`{"type":"mention","attrs":{"kind":"CASE","ref":" "}}`),
		"mention bad id":      para(`{"type":"mention","attrs":{"kind":"CLUSTER","ref":"abc"}}`),
		"mention negative id": para(`{"type":"mention","attrs":{"kind":"REPORT","ref":"-3"}}`),
		"block in paragraph":  para(`{"type":"paragraph"}`),
		"text in doc":         `{"type":"doc","content":[{"type":"text","text":"x"}]}`,
		"empty list":          `{"type":"doc","content":[{"type":"bulletList"}]}`,
		"paragraph in list":   `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"paragraph"}]}]}`,
		"heading in list":     `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"heading","attrs":{"level":2}}]}]}]}`,
		"text on a block":     `{"type":"doc","content":[{"type":"paragraph","text":"x"}]}`,
		"trailing data":       `{"type":"doc"} {"type":"doc"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBody([]byte(doc))
			var bodyErr *BodyError
			if !errors.As(err, &bodyErr) {
				t.Errorf("ParseBody(%s) = %v, want a *BodyError", doc, err)
			}
		})
	}
}

func TestParseBodyLimits(t *testing.T) {
	// Too deep: lists nested past the limit.
	doc := `{"type":"paragraph","content":[{"type":"text","text":"x"}]}`
	for i := 0; i < maxDepth; i++ {
		doc = `{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph"},` + doc + `]}]}`
	}
	if _, err := ParseBody([]byte(`{"type":"doc","content":[` + doc + `]}`)); err == nil {
		t.Error("nesting past the limit accepted")
	}

	// Too large.
	big := strings.Repeat("x", MaxBodyBytes)
	if _, err := ParseBody([]byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + big + `"}]}]}`)); err == nil {
		t.Error("body over MaxBodyBytes accepted")
	}

	// An empty document is a valid (blank) article.
	body, err := ParseBody([]byte(`{"type":"doc","content":[{"type":"paragraph"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if body.Text != "" || body.Chars != 0 || len(body.Mentions) != 0 {
		t.Errorf("empty document: %+v", body)
	}
}

func TestMentionLabelFallsBackToRef(t *testing.T) {
	body, err := ParseBody([]byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"mention","attrs":{"kind":"INQUIRY","ref":"4512/2026"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Content []struct {
			Content []struct {
				Attrs map[string]string `json:"attrs"`
			} `json:"content"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body.Canonical, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Content[0].Content[0].Attrs["label"]; got != "4512/2026" {
		t.Errorf("label = %q, want the ref", got)
	}
	if body.Text != "@4512/2026" {
		t.Errorf("text = %q", body.Text)
	}
}
