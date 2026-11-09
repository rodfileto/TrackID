package facecompare

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

// Markup kinds: only those FISWG's Best Practices for Markups & Annotations (v1.0, §4.2) allows.
// There is no freehand pen and no text on the image (§4.3): a markup carries a number, and what it
// shows is written in the findings, outside the image.
const (
	MarkupEllipse = "ELLIPSE" // around a mark or scar
	MarkupBox     = "BOX"     // around a mark or scar
	MarkupBracket = "BRACKET" // a relative distance or a grouping, with hash marks at its ends
	MarkupArrow   = "ARROW"   // points at a feature: from its tail to its head
	MarkupPoint   = "POINT"   // a numbered dot on a feature
)

// MarkupKinds lists the kinds with their labels.
var MarkupKinds = []Label{
	{MarkupEllipse, "Ellipse", "Elipse"},
	{MarkupBox, "Box", "Retângulo"},
	{MarkupBracket, "Bracket", "Colchete"},
	{MarkupArrow, "Arrow", "Seta"},
	{MarkupPoint, "Numbered dot", "Ponto numerado"},
}

// Markup is one markup on one image, in its original pixels. Ellipses and boxes take two opposite
// corners; brackets their two ends; arrows tail then head; points one point. The same number on
// both images marks corresponding features.
type Markup struct {
	Image  string       `json:"image"` // Q or K
	Number int          `json:"number"`
	Kind   string       `json:"kind"`
	Points [][2]float64 `json:"points"`
}

func (m *Markup) validate() error {
	if m.Image != "Q" && m.Image != "K" {
		return errors.New("a markup is on the questioned (Q) or the known (K) image")
	}
	if m.Number < 1 || m.Number > 99 {
		return errors.New("markups are numbered 1 to 99")
	}
	want := 2
	switch m.Kind {
	case MarkupEllipse, MarkupBox, MarkupBracket, MarkupArrow:
	case MarkupPoint:
		want = 1
	default:
		return fmt.Errorf("%q is not a markup FISWG allows", m.Kind)
	}
	if len(m.Points) != want {
		return fmt.Errorf("a %s takes %d point(s)", m.Kind, want)
	}
	for _, p := range m.Points {
		if math.IsNaN(p[0]) || math.IsNaN(p[1]) {
			return errors.New("a point is not a number")
		}
	}
	if want == 2 && math.Hypot(m.Points[1][0]-m.Points[0][0], m.Points[1][1]-m.Points[0][1]) < 2 {
		return errors.New("the markup is too small")
	}
	return nil
}

// bounds is the markup's bounding box, with pad pixels around lines and dots.
func (m Markup) bounds(pad float64) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range m.Points {
		b[0], b[1] = math.Min(b[0], p[0]), math.Min(b[1], p[1])
		b[2], b[3] = math.Max(b[2], p[0]), math.Max(b[3], p[1])
	}
	if m.Kind != MarkupBox && m.Kind != MarkupEllipse {
		b[0], b[1], b[2], b[3] = b[0]-pad, b[1]-pad, b[2]+pad, b[3]+pad
	}
	return b
}

// Warning is something the examiner should look at; it never blocks.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"` // in Portuguese, as the record words it
	Where   string `json:"where,omitempty"`
}

// MarkupWarnings looks for what FISWG's markup guide warns against or the comparison needs:
// overlapping markups (§4.3.4), a number marked on one image only, and findings citing a number
// no markup has.
func MarkupWarnings(s Session) []Warning {
	var out []Warning
	for _, img := range []string{"Q", "K"} {
		var ms []Markup
		for _, m := range s.Markups {
			if m.Image == img {
				ms = append(ms, m)
			}
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].Number < ms[j].Number })
		for i := range ms {
			for j := i + 1; j < len(ms); j++ {
				if ms[i].Number == ms[j].Number {
					out = append(out, Warning{"MARKUP_DUPLICATE", fmt.Sprintf("O número %d aparece mais de uma vez na imagem %s.", ms[i].Number, imageName(img)), img})
					continue
				}
				if overlap(ms[i], ms[j]) {
					out = append(out, Warning{"MARKUP_OVERLAP", fmt.Sprintf("As marcações %d e %d se sobrepõem na imagem %s; marcações sobrepostas poluem a imagem e podem ocultar características.",
						ms[i].Number, ms[j].Number, imageName(img)), img})
				}
			}
		}
	}
	on := map[string]map[int]bool{"Q": {}, "K": {}}
	for _, m := range s.Markups {
		on[m.Image][m.Number] = true
	}
	var numbers []int
	for n := range on["Q"] {
		numbers = append(numbers, n)
	}
	for n := range on["K"] {
		if !on["Q"][n] {
			numbers = append(numbers, n)
		}
	}
	sort.Ints(numbers)
	for _, n := range numbers {
		switch {
		case !on["K"][n]:
			out = append(out, Warning{"MARKUP_ONE_SIDE", fmt.Sprintf("A marcação %d existe só na imagem questionada.", n), "Q"})
		case !on["Q"][n]:
			out = append(out, Warning{"MARKUP_ONE_SIDE", fmt.Sprintf("A marcação %d existe só na imagem padrão.", n), "K"})
		}
	}
	for _, f := range s.Findings {
		for _, n := range f.Markups {
			if !on["Q"][n] && !on["K"][n] {
				out = append(out, Warning{"FINDING_MARKUP_MISSING", fmt.Sprintf("Um achado cita a marcação %d, que não existe.", n), f.Characteristic})
			}
		}
	}
	return out
}

// overlap reports whether two markups' bounding boxes intersect (lines and dots padded a little):
// crossing outlines, or one markup drawn inside another, are what clutters the image.
func overlap(a, b Markup) bool {
	const pad = 6
	ba, bb := a.bounds(pad), b.bounds(pad)
	return ba[2] >= bb[0] && bb[2] >= ba[0] && ba[3] >= bb[1] && bb[3] >= ba[1]
}

func imageName(img string) string {
	if img == "Q" {
		return "questionada"
	}
	return "padrão"
}
