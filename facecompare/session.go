package facecompare

import (
	"fmt"
	"strings"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/imageproc"
)

// SessionVersion is the version of the session format the face lab saves and loads.
const SessionVersion = 1

// Session is one comparison as the face lab holds it: the two images with their processing,
// landmarks and analysis, the alignment, the markups, the findings and the opinion. All
// coordinates are in each image's original pixels.
type Session struct {
	Version    int  `json:"version"`
	Questioned Side `json:"questioned"`
	Known      Side `json:"known"`
	// QuestionedFirst is true when the known image stayed hidden until the questioned image's
	// analysis was done (FISWG ACE-V one-to-one guideline, §5.1.1).
	QuestionedFirst bool `json:"questionedFirst"`
	// Pairs are corresponding points the alignment is fitted from.
	Pairs []PointPair `json:"pairs,omitempty"`
	// Superimposition is true when an overlay, wipe or flicker view was used.
	Superimposition bool `json:"superimposition"`
	// Factors are the stability factors (Factor*) that apply between the two images, such as an
	// expression change or years apart.
	Factors  []string  `json:"factors,omitempty"`
	Markups  []Markup  `json:"markups,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	Opinion  *Opinion  `json:"opinion,omitempty"`
}

// Side is one of the two images.
type Side struct {
	SourceID  string               `json:"sourceId"`
	Recipe    imageproc.FaceRecipe `json:"recipe"`
	Face      *imageproc.Rect      `json:"face,omitempty"`
	Landmarks *Landmarks           `json:"landmarks,omitempty"`
	Analysis  Analysis             `json:"analysis"`
}

// Landmarks are the five points the detector gives (the eye centres, the nose tip and the mouth
// corners, in image order: the one further left in the image first), or that the examiner placed.
// They serve alignment and pose, never comparison.
type Landmarks struct {
	Points [5][2]float64 `json:"points"`
	Source string        `json:"source"` // DETECTOR or EXAMINER
}

// Analysis is the ACE-V analysis of one image.
type Analysis struct {
	Factors               []ImageFactor `json:"factors,omitempty"`
	Mirrored              bool          `json:"mirrored"`
	ManipulationSuspected bool          `json:"manipulationSuspected"`
	ManipulationNotes     string        `json:"manipulationNotes,omitempty"`
	CaptureDate           string        `json:"captureDate,omitempty"`
	Suitability           string        `json:"suitability,omitempty"` // SUITABLE, LEAD_ONLY, UNSUITABLE
	SuitabilityReason     string        `json:"suitabilityReason,omitempty"`
	Notes                 string        `json:"notes,omitempty"`
	Done                  bool          `json:"done"`
}

// ImageFactor is one of FISWG's image factors, as observed in an image.
type ImageFactor struct {
	Code   string `json:"code"`
	Effect string `json:"effect"` // NONE, MINOR, SIGNIFICANT
	Notes  string `json:"notes,omitempty"`
}

// ImageFactors are FISWG's Image Factors to Consider in Facial Image Comparison (v1.0), §5–13.
var ImageFactors = []Label{
	{"ARTIFACTS", "Artifacts (blooming, banding, ringing, blocking, aliasing)", "Artefatos (saturação, posterização, halos, blocos de compressão, serrilhamento)"},
	{"DISTORTION", "Distortion (aspect ratio, lens, perspective, rolling shutter, interlacing)", "Distorção (proporção, lente, perspectiva, obturador, entrelaçamento)"},
	{"LIGHTING", "Lighting (exposure, direction, contrast, colour)", "Iluminação (exposição, direção, contraste, cor)"},
	{"NEAR_INFRARED", "Near infrared", "Infravermelho próximo"},
	{"NOISE", "Noise", "Ruído"},
	{"OBSTRUCTIONS", "Obstructions (objects, flare, vignetting)", "Obstruções (objetos, reflexos, vinheta)"},
	{"POSE", "Pose (pitch, roll, yaw)", "Pose (inclinação, rotação, giro)"},
	{"SHARPNESS", "Sharpness", "Nitidez"},
	{"RESOLUTION", "Resolution", "Resolução"},
}

// Effects an image factor has on the comparison.
var Effects = []Label{
	{"NONE", "None observed", "Não observado"},
	{"MINOR", "Present, minor effect", "Presente, efeito pequeno"},
	{"SIGNIFICANT", "Present, limits the comparison", "Presente, limita a comparação"},
}

// Suitabilities are the analysis outcomes, the same values a questioned trace takes in cases.
var Suitabilities = []Label{
	{"SUITABLE", "Suitable for comparison", "Apta para comparação"},
	{"LEAD_ONLY", "Lead only", "Apta apenas para indicação investigativa"},
	{"UNSUITABLE", "Unsuitable", "Sem qualidade para comparação"},
}

// PointPair is a point in the questioned image and the corresponding one in the known image.
type PointPair struct {
	Q [2]float64 `json:"q"`
	K [2]float64 `json:"k"`
}

// Finding results.
const (
	Similar       = "SIMILAR"
	Dissimilar    = "DISSIMILAR"
	NotComparable = "NOT_COMPARABLE"
)

// Results lists the finding results.
var Results = []Label{
	{Similar, "Similar", "Semelhante"},
	{Dissimilar, "Dissimilar", "Diferente"},
	{NotComparable, "Not comparable", "Não comparável"},
}

// Unexplained is the explanation that says none was found; it excludes the others.
const Unexplained = "UNEXPLAINED"

// Explanations of a dissimilarity: subject variance, imaging conditions, or none found (ACE-V
// one-to-one guideline, §5.2.1).
var Explanations = []Label{
	{"EXPRESSION", "Expression", "Expressão"},
	{"POSE", "Pose", "Pose"},
	{"WEIGHT", "Weight change", "Variação de peso"},
	{"AGE", "Age", "Idade"},
	{"HEALTH", "Health", "Saúde"},
	{"ALTERATION", "Intentional alteration", "Alteração intencional"},
	{"IMAGING", "Imaging conditions", "Condições de captura da imagem"},
	{"UNEXPLAINED", "Not explained", "Sem explicação"},
}

// Finding is the comparison of one characteristic in the two images.
type Finding struct {
	Characteristic string   `json:"characteristic"`
	Side           string   `json:"side,omitempty"` // RIGHT or LEFT (the subject's) for sided ones
	Descriptors    []string `json:"descriptors,omitempty"`
	Questioned     string   `json:"questioned,omitempty"` // what is observed in Q
	Known          string   `json:"known,omitempty"`      // what is observed in K
	Result         string   `json:"result"`
	// Explanations are what accounts for a DISSIMILAR result: one or more causes, or UNEXPLAINED
	// alone.
	Explanations []string `json:"explanations,omitempty"`
	// Explanation is the single explanation of sessions saved before 2026-10-30; Validate moves it
	// into Explanations.
	Explanation string `json:"explanation,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Markups     []int  `json:"markups,omitempty"` // the numbers illustrating it
}

// Opinion is the evaluation's outcome on the OSAC scale, with its reasoning.
type Opinion struct {
	Conclusion cluster.Conclusion `json:"conclusion"`
	Reasoning  string             `json:"reasoning"`
}

func labelOf(list []Label, code string) (Label, bool) {
	for _, l := range list {
		if l.Code == code {
			return l, true
		}
	}
	return Label{}, false
}

// Validate checks the session's codes and shapes; problems that only call for the examiner's
// attention are Check's warnings, not errors here.
func (s *Session) Validate() error {
	if s.Version == 0 {
		s.Version = SessionVersion
	}
	if s.Version != SessionVersion {
		return fmt.Errorf("session version %d is not supported (this is %d)", s.Version, SessionVersion)
	}
	for _, side := range []*Side{&s.Questioned, &s.Known} {
		a := side.Analysis
		for _, f := range a.Factors {
			if _, ok := labelOf(ImageFactors, f.Code); !ok {
				return fmt.Errorf("unknown image factor %q", f.Code)
			}
			if _, ok := labelOf(Effects, f.Effect); !ok {
				return fmt.Errorf("unknown effect %q", f.Effect)
			}
		}
		if _, ok := labelOf(Suitabilities, a.Suitability); a.Suitability != "" && !ok {
			return fmt.Errorf("unknown suitability %q", a.Suitability)
		}
		if l := side.Landmarks; l != nil && l.Source != "DETECTOR" && l.Source != "EXAMINER" {
			return fmt.Errorf("landmarks come from the DETECTOR or the EXAMINER")
		}
	}
	for _, f := range s.Factors {
		if _, ok := labelOf(StabilityFactors, f); !ok {
			return fmt.Errorf("unknown stability factor %q", f)
		}
	}
	for i := range s.Markups {
		if err := s.Markups[i].validate(); err != nil {
			return fmt.Errorf("markup %d: %w", s.Markups[i].Number, err)
		}
	}
	seen := map[string]bool{}
	for i := range s.Findings {
		f := &s.Findings[i]
		if f.Explanation != "" {
			if !contains(f.Explanations, f.Explanation) {
				f.Explanations = append(f.Explanations, f.Explanation)
			}
			f.Explanation = ""
		}
		_, ch, ok := Find(f.Characteristic)
		if !ok {
			return fmt.Errorf("unknown characteristic %q", f.Characteristic)
		}
		switch {
		case ch.Sided && f.Side != "RIGHT" && f.Side != "LEFT":
			return fmt.Errorf("%s is recorded for the right or the left side", ch.EN)
		case !ch.Sided && f.Side != "":
			return fmt.Errorf("%s has no side", ch.EN)
		}
		key := f.Characteristic + "/" + f.Side
		if seen[key] && f.Characteristic != "SCAR" && f.Characteristic != "SKIN_MARK" && !strings.HasPrefix(f.Characteristic, "OTHER") {
			return fmt.Errorf("%s is recorded twice", ch.EN)
		}
		seen[key] = true
		for _, d := range f.Descriptors {
			if !contains(ch.Descriptors, d) {
				return fmt.Errorf("%s has no descriptor %q", ch.EN, d)
			}
		}
		if _, ok := labelOf(Results, f.Result); !ok {
			return fmt.Errorf("%s: unknown result %q", ch.EN, f.Result)
		}
		if len(f.Explanations) > 0 && f.Result != Dissimilar {
			return fmt.Errorf("%s: only a dissimilarity takes an explanation", ch.EN)
		}
		for j, e := range f.Explanations {
			if _, ok := labelOf(Explanations, e); !ok {
				return fmt.Errorf("%s: unknown explanation %q", ch.EN, e)
			}
			if contains(f.Explanations[:j], e) {
				return fmt.Errorf("%s: explanation %q given twice", ch.EN, e)
			}
		}
		if contains(f.Explanations, Unexplained) && len(f.Explanations) > 1 {
			return fmt.Errorf("%s: a difference is either explained or not", ch.EN)
		}
	}
	if o := s.Opinion; o != nil && !o.Conclusion.Valid() {
		return fmt.Errorf("unknown conclusion %q", o.Conclusion)
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Summary counts the findings, for the evaluation step. It is not the opinion.
type Summary struct {
	Similar       int `json:"similar"`
	Dissimilar    int `json:"dissimilar"`
	Explained     int `json:"explained"` // dissimilarities explained by subject variance or imaging
	NotComparable int `json:"notComparable"`
	// By the characteristic's weakest stability under the pair's factors ("" when unrated or no
	// factor applies): similarities and dissimilarities.
	SimilarByStability    map[Stability]int `json:"similarByStability"`
	DissimilarByStability map[Stability]int `json:"dissimilarByStability"`
	ComponentsCompared    int               `json:"componentsCompared"`
}

// Summarize counts s's findings.
func Summarize(s Session) Summary {
	sum := Summary{SimilarByStability: map[Stability]int{}, DissimilarByStability: map[Stability]int{}}
	components := map[string]bool{}
	for _, f := range s.Findings {
		comp, ch, ok := Find(f.Characteristic)
		if !ok {
			continue
		}
		st := Weakest(ch, s.Factors)
		switch f.Result {
		case Similar:
			sum.Similar++
			sum.SimilarByStability[st]++
			components[comp.Code] = true
		case Dissimilar:
			sum.Dissimilar++
			sum.DissimilarByStability[st]++
			if len(f.Explanations) > 0 && !contains(f.Explanations, Unexplained) {
				sum.Explained++
			}
			components[comp.Code] = true
		case NotComparable:
			sum.NotComparable++
		}
	}
	sum.ComponentsCompared = len(components)
	return sum
}
