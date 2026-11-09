package articles

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Limits on one article body.
const (
	// MaxBodyBytes is the largest body accepted, as sent.
	MaxBodyBytes = 1 << 20
	maxDepth     = 16
	maxNodes     = 50_000
	maxRefLen    = 200
	maxLabelLen  = 300
	maxHrefLen   = 2000
)

// Node is one node of a ProseMirror document, as the editor (TipTap) sends it.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is a ProseMirror mark on a text node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// BodyError is an article body the schema does not allow.
type BodyError struct{ Reason string }

func (e *BodyError) Error() string { return "articles: invalid body: " + e.Reason }

func bodyErr(format string, args ...any) error {
	return &BodyError{Reason: fmt.Sprintf(format, args...)}
}

// Body is a checked article body: its canonical form, the bytes stored and hashed, its plain
// text and its mentions.
type Body struct {
	Doc       Node
	Canonical []byte
	SHA256    [32]byte
	Text      string
	Chars     int
	Mentions  []MentionCount
}

// MentionCount is one target an article mentions and how many times.
type MentionCount struct {
	MentionKey
	Count int
}

// Block and inline node types the article schema allows (docs/person-article-plan.md §2): what
// each one may contain. Anything else is refused.
var (
	inlineTypes   = map[string]bool{"text": true, "hardBreak": true, "mention": true}
	topBlocks     = map[string]bool{"paragraph": true, "heading": true, "bulletList": true, "orderedList": true, "blockquote": true}
	listItemTypes = map[string]bool{"paragraph": true, "bulletList": true, "orderedList": true, "blockquote": true}
	quoteTypes    = map[string]bool{"paragraph": true, "bulletList": true, "orderedList": true}
)

// ParseBody checks a body sent by the editor against the article schema and returns its
// canonical form: only the allowed node and mark types, each with only the attributes the
// schema knows (others, such as the editor's defaults, are dropped), so what is stored and later
// rendered is exactly what the schema allows. Unknown node or mark types, bad attribute values
// and links other than http(s) are refused with a *BodyError.
func ParseBody(raw []byte) (Body, error) {
	if len(raw) > MaxBodyBytes {
		return Body{}, bodyErr("larger than %d bytes", MaxBodyBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var doc Node
	if err := decoder.Decode(&doc); err != nil {
		return Body{}, bodyErr("not a document: %v", err)
	}
	if decoder.More() {
		return Body{}, bodyErr("trailing data after the document")
	}
	if doc.Type != "doc" {
		return Body{}, bodyErr("top node is %q, want doc", doc.Type)
	}
	n := &normalizer{}
	clean, err := n.blockContainer(doc, topBlocks, 0, false)
	if err != nil {
		return Body{}, err
	}
	clean.Type = "doc"

	canonical, err := json.Marshal(clean)
	if err != nil {
		return Body{}, err
	}
	text := PlainText(clean)
	return Body{
		Doc:       clean,
		Canonical: canonical,
		SHA256:    sha256.Sum256(canonical),
		Text:      text,
		Chars:     utf8.RuneCountInString(text),
		Mentions:  countMentions(clean),
	}, nil
}

type normalizer struct{ nodes int }

func (n *normalizer) count(depth int) error {
	n.nodes++
	if n.nodes > maxNodes {
		return bodyErr("more than %d nodes", maxNodes)
	}
	if depth > maxDepth {
		return bodyErr("nested deeper than %d", maxDepth)
	}
	return nil
}

func noText(node Node) error {
	if node.Text != "" || len(node.Marks) > 0 {
		return bodyErr("%s carries text or marks", node.Type)
	}
	return nil
}

// blockContainer cleans a node whose children are blocks of the given types; nonEmpty requires
// at least one.
func (n *normalizer) blockContainer(node Node, allowed map[string]bool, depth int, nonEmpty bool) (Node, error) {
	if err := n.count(depth); err != nil {
		return Node{}, err
	}
	if err := noText(node); err != nil {
		return Node{}, err
	}
	if nonEmpty && len(node.Content) == 0 {
		return Node{}, bodyErr("empty %s", node.Type)
	}
	out := Node{Type: node.Type}
	for _, child := range node.Content {
		if !allowed[child.Type] {
			return Node{}, bodyErr("%s not allowed in %s", child.Type, node.Type)
		}
		clean, err := n.block(child, depth+1)
		if err != nil {
			return Node{}, err
		}
		out.Content = append(out.Content, clean)
	}
	return out, nil
}

func (n *normalizer) block(node Node, depth int) (Node, error) {
	switch node.Type {
	case "paragraph":
		return n.inlineContainer(node, nil, depth)
	case "heading":
		level, ok := intAttr(node.Attrs, "level")
		if !ok || level < 2 || level > 3 {
			return Node{}, bodyErr("heading level must be 2 or 3")
		}
		return n.inlineContainer(node, map[string]any{"level": level}, depth)
	case "bulletList", "orderedList":
		if err := n.count(depth); err != nil {
			return Node{}, err
		}
		if err := noText(node); err != nil {
			return Node{}, err
		}
		if len(node.Content) == 0 {
			return Node{}, bodyErr("empty %s", node.Type)
		}
		out := Node{Type: node.Type}
		if node.Type == "orderedList" {
			if start, ok := intAttr(node.Attrs, "start"); ok && start != 1 {
				if start < 0 || start > 1_000_000 {
					return Node{}, bodyErr("orderedList start out of range")
				}
				out.Attrs = map[string]any{"start": start}
			}
		}
		for _, item := range node.Content {
			if item.Type != "listItem" {
				return Node{}, bodyErr("%s not allowed in %s", item.Type, node.Type)
			}
			clean, err := n.blockContainer(item, listItemTypes, depth+1, true)
			if err != nil {
				return Node{}, err
			}
			out.Content = append(out.Content, clean)
		}
		return out, nil
	case "blockquote":
		return n.blockContainer(node, quoteTypes, depth, true)
	}
	return Node{}, bodyErr("unknown block %q", node.Type)
}

// inlineContainer cleans a paragraph or heading: text, hard breaks and mentions.
func (n *normalizer) inlineContainer(node Node, attrs map[string]any, depth int) (Node, error) {
	if err := n.count(depth); err != nil {
		return Node{}, err
	}
	if err := noText(node); err != nil {
		return Node{}, err
	}
	out := Node{Type: node.Type, Attrs: attrs}
	for _, child := range node.Content {
		if !inlineTypes[child.Type] {
			return Node{}, bodyErr("%s not allowed in %s", child.Type, node.Type)
		}
		if err := n.count(depth + 1); err != nil {
			return Node{}, err
		}
		if len(child.Content) > 0 {
			return Node{}, bodyErr("%s has content", child.Type)
		}
		switch child.Type {
		case "text":
			clean, err := cleanText(child)
			if err != nil {
				return Node{}, err
			}
			out.Content = append(out.Content, clean)
		case "hardBreak":
			if err := noText(child); err != nil {
				return Node{}, err
			}
			out.Content = append(out.Content, Node{Type: "hardBreak"})
		case "mention":
			if err := noText(child); err != nil {
				return Node{}, err
			}
			m, err := mentionAttrs(child.Attrs)
			if err != nil {
				return Node{}, err
			}
			out.Content = append(out.Content, Node{Type: "mention", Attrs: map[string]any{
				"kind": string(m.Kind), "ref": m.Ref, "label": m.Label,
			}})
		}
	}
	return out, nil
}

func cleanText(node Node) (Node, error) {
	if node.Text == "" {
		return Node{}, bodyErr("empty text node")
	}
	if !utf8.ValidString(node.Text) {
		return Node{}, bodyErr("text is not UTF-8")
	}
	if strings.ContainsRune(node.Text, 0) {
		return Node{}, bodyErr("text holds a NUL character")
	}
	out := Node{Type: "text", Text: node.Text}
	seen := map[string]bool{}
	for _, mark := range node.Marks {
		if seen[mark.Type] {
			return Node{}, bodyErr("mark %s repeated", mark.Type)
		}
		seen[mark.Type] = true
		switch mark.Type {
		case "bold", "italic":
			out.Marks = append(out.Marks, Mark{Type: mark.Type})
		case "link":
			href, _ := mark.Attrs["href"].(string)
			if err := checkHref(href); err != nil {
				return Node{}, err
			}
			out.Marks = append(out.Marks, Mark{Type: "link", Attrs: map[string]any{"href": href}})
		default:
			return Node{}, bodyErr("unknown mark %q", mark.Type)
		}
	}
	// One order for the same marks, so equal texts hash equal.
	sort.Slice(out.Marks, func(i, j int) bool { return out.Marks[i].Type < out.Marks[j].Type })
	return out, nil
}

func checkHref(href string) error {
	if href == "" || len(href) > maxHrefLen {
		return bodyErr("link without a valid href")
	}
	u, err := url.Parse(href)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return bodyErr("links must be http(s) addresses: %q", href)
	}
	return nil
}

// intAttr reads a whole-number attribute (sent as a JSON number).
func intAttr(attrs map[string]any, key string) (int, bool) {
	switch v := attrs[key].(type) {
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	case float64: // a Node built in Go rather than decoded
		if v == float64(int(v)) {
			return int(v), true
		}
	case int:
		return v, true
	}
	return 0, false
}

// Mention is a mention node's attributes: the target and the label shown when it was inserted.
type Mention struct {
	MentionKey
	Label string `json:"label"`
}

func mentionAttrs(attrs map[string]any) (Mention, error) {
	kind, _ := attrs["kind"].(string)
	ref, _ := attrs["ref"].(string)
	label, _ := attrs["label"].(string)
	key, err := ParseKey(kind, ref)
	if err != nil {
		return Mention{}, &BodyError{Reason: err.Error()}
	}
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > maxLabelLen || !utf8.ValidString(label) || strings.ContainsRune(label, 0) {
		return Mention{}, bodyErr("mention label invalid")
	}
	if label == "" {
		label = key.Ref
	}
	return Mention{MentionKey: key, Label: label}, nil
}

// MentionsIn lists every mention node in a document, in order, repeats included.
func MentionsIn(doc Node) []Mention {
	var out []Mention
	var walk func(Node)
	walk = func(node Node) {
		if node.Type == "mention" {
			if m, err := mentionAttrs(node.Attrs); err == nil {
				out = append(out, m)
			}
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(doc)
	return out
}

func countMentions(doc Node) []MentionCount {
	counts := map[MentionKey]int{}
	for _, m := range MentionsIn(doc) {
		counts[m.MentionKey]++
	}
	out := make([]MentionCount, 0, len(counts))
	for key, count := range counts {
		out = append(out, MentionCount{MentionKey: key, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MentionKey.less(out[j].MentionKey) })
	return out
}

// PlainText renders a document as plain text, for search and diffs: blocks separated by a blank
// line, list items on their own lines with "- " or "n. ", nested lists indented, quotes with
// "> ", and each mention as "@label".
func PlainText(doc Node) string {
	var blocks []string
	for _, child := range doc.Content {
		blocks = append(blocks, blockText(child, "")...)
	}
	return strings.Join(blocks, "\n\n")
}

func blockText(node Node, indent string) []string {
	switch node.Type {
	case "paragraph", "heading":
		return []string{indent + inlineText(node, indent)}
	case "blockquote":
		var lines []string
		for _, child := range node.Content {
			for _, block := range blockText(child, "") {
				lines = append(lines, quoteLines(block))
			}
		}
		return lines
	case "bulletList", "orderedList":
		start := 1
		if s, ok := intAttr(node.Attrs, "start"); ok {
			start = s
		}
		var items []string
		for i, item := range node.Content {
			marker := "- "
			if node.Type == "orderedList" {
				marker = strconv.Itoa(start+i) + ". "
			}
			var parts []string
			for j, child := range item.Content {
				if j == 0 && child.Type == "paragraph" {
					parts = append(parts, indent+marker+inlineText(child, indent))
					continue
				}
				parts = append(parts, blockText(child, indent+"  ")...)
			}
			items = append(items, strings.Join(parts, "\n"))
		}
		return []string{strings.Join(items, "\n")}
	}
	return nil
}

func quoteLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

func inlineText(node Node, indent string) string {
	var b strings.Builder
	for _, child := range node.Content {
		switch child.Type {
		case "text":
			b.WriteString(child.Text)
		case "hardBreak":
			b.WriteString("\n" + indent)
		case "mention":
			label, _ := child.Attrs["label"].(string)
			if label == "" {
				label, _ = child.Attrs["ref"].(string)
			}
			b.WriteString("@" + label)
		}
	}
	return b.String()
}
