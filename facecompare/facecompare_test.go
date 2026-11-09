package facecompare

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/rodfileto/trackid/cluster"
	"github.com/rodfileto/trackid/imageproc"
)

func TestFeatureList(t *testing.T) {
	if len(Components) != 19 {
		t.Errorf("%d components, want FISWG's 19", len(Components))
	}
	seen := map[string]bool{}
	for _, comp := range Components {
		for _, ch := range comp.Characteristics {
			if seen[ch.Code] {
				t.Errorf("%s twice", ch.Code)
			}
			seen[ch.Code] = true
			if len(ch.Descriptors) == 0 {
				t.Errorf("%s has no descriptor", ch.Code)
			}
			for _, d := range ch.Descriptors {
				if _, ok := Descriptors[d]; !ok {
					t.Errorf("%s: descriptor %s is not named", ch.Code, d)
				}
			}
			if r := ch.Stability; r != nil {
				for _, f := range StabilityFactors {
					if s := r.Under(f.Code); s != High && s != Medium && s != Low {
						t.Errorf("%s has no rating under %s", ch.Code, f.Code)
					}
				}
				if r.Page < 5 || r.Page > 14 {
					t.Errorf("%s cites page %d", ch.Code, r.Page)
				}
			}
		}
	}
	_, ic, _ := Find("INTERCANTHAL")
	if Weakest(ic, []string{FactorExpression, FactorLongTerm}) != High {
		t.Error("intercanthal distance is stable under every factor")
	}
	_, iris, _ := Find("IRIS")
	if Weakest(iris, []string{FactorShortTerm, FactorExpression}) != Low || Weakest(iris, nil) != "" {
		t.Error("Weakest does not take the lowest rating of the factors given")
	}
}

func TestFitRecoversASimilarity(t *testing.T) {
	theta := 17 * math.Pi / 180
	s, tx, ty := 1.6, 30.0, -12.0
	k := [][2]float64{{10, 20}, {80, 25}, {45, 70}, {30, 95}}
	var q [][2]float64
	for _, p := range k {
		q = append(q, [2]float64{s*(math.Cos(theta)*p[0]-math.Sin(theta)*p[1]) + tx, s*(math.Sin(theta)*p[0]+math.Cos(theta)*p[1]) + ty})
	}
	sim, err := Fit(q, k)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sim.Scale-s) > 1e-9 || math.Abs(sim.Rotation-17) > 1e-9 || sim.RMS > 1e-9 {
		t.Errorf("got %+v", sim)
	}
	if _, err := Fit(q[:1], k[:1]); err != ErrTooFewPairs {
		t.Error("one pair should not align")
	}
	// Align composes each side's processing: a known image mirrored by its recipe aligns back.
	kt := imageproc.Affine{A: -1, C: 100, E: 1}
	pairs := []PointPair{{Q: q[0], K: k[0]}, {Q: q[1], K: k[1]}}
	for i := range pairs {
		pairs[i].K[0] = 100 - pairs[i].K[0]
	}
	if sim, err := Align(pairs, imageproc.Affine{A: 1, E: 1}, kt); err != nil || sim.RMS > 1e-9 {
		t.Errorf("align through the transforms: %+v, %v", sim, err)
	}
}

func frontal(dx float64) *Landmarks {
	return &Landmarks{Source: "DETECTOR", Points: [5][2]float64{{100, 100}, {160, 100}, {130 + dx, 135}, {108, 165}, {152, 165}}}
}

func TestPose(t *testing.T) {
	p := PoseOf(*frontal(0))
	if p.IPD != 60 || p.Roll != 0 || p.Yaw != 0 || math.Abs(p.PitchIndex-35.0/65) > 1e-9 {
		t.Errorf("frontal pose %+v", p)
	}
	if PoseOf(*frontal(12)).Yaw <= 0 {
		t.Error("a nose to the image's right should give a positive yaw")
	}
	w := PoseWarnings(frontal(0), frontal(20))
	if len(w) != 1 || w[0].Code != "POSE_YAW" {
		t.Errorf("warnings %+v", w)
	}
	small := &Landmarks{Source: "EXAMINER", Points: [5][2]float64{{10, 10}, {30, 10}, {20, 20}, {13, 28}, {27, 28}}}
	if w := PoseWarnings(small, nil); len(w) != 1 || w[0].Code != "LOW_IPD" {
		t.Errorf("low IPD: %+v", w)
	}
}

func TestMeasure(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetGray(x, y, color.Gray{uint8(255 * ((x / 4) % 2))})
		}
	}
	m := Measure(img, &imageproc.Rect{X1: 0, Y1: 0, X2: 20, Y2: 20}, nil)
	if m.Width != 20 || m.Sharpness <= 0 || math.Abs(m.ClippedDark-0.5) > 0.11 || math.Abs(m.ClippedBright-0.5) > 0.11 {
		t.Errorf("metrics %+v", m)
	}
}

func TestWording(t *testing.T) {
	w := WordingWarnings([]Text{
		{"NOSE", "Ponta nasal com forma idêntica"},
		{"opinion", "As imagens permitem a identificação"},
		{"EYES", "Fenda palpebral estreita, inclinada"},
	})
	if len(w) != 2 || w[0].Code != "WORDING_FEATURE" || w[1].Code != "WORDING_OPINION" {
		t.Errorf("warnings %+v", w)
	}
}

func session() Session {
	return Session{
		Questioned: Side{SourceID: "q", Landmarks: frontal(0), Analysis: Analysis{Suitability: "SUITABLE", Done: true,
			Factors: []ImageFactor{{Code: "RESOLUTION", Effect: "MINOR", Notes: "compressão JPEG visível"}}}},
		Known:           Side{SourceID: "k", Landmarks: frontal(2), Analysis: Analysis{Suitability: "SUITABLE", Done: true}},
		QuestionedFirst: true,
		Factors:         []string{FactorLongTerm},
		Markups: []Markup{
			{Image: "Q", Number: 1, Kind: MarkupEllipse, Points: [][2]float64{{20, 20}, {40, 34}}},
			{Image: "K", Number: 1, Kind: MarkupEllipse, Points: [][2]float64{{22, 21}, {41, 35}}},
			{Image: "Q", Number: 2, Kind: MarkupArrow, Points: [][2]float64{{70, 70}, {60, 60}}},
		},
		Findings: []Finding{
			{Characteristic: "SKIN_MARK", Questioned: "Mancha escura acima da sobrancelha", Known: "Mancha na mesma posição", Result: Similar, Markups: []int{1}},
			{Characteristic: "EYEBROW", Side: "RIGHT", Descriptors: []string{"SHAPE"}, Result: Dissimilar, Explanations: []string{"EXPRESSION", "IMAGING"}},
			{Characteristic: "NASAL_TIP", Result: Similar},
			{Characteristic: "TEETH", Result: NotComparable},
		},
		Opinion: &Opinion{Conclusion: cluster.ConclusionSupportCommon, Reasoning: "Semelhanças em características de estabilidade alta."},
	}
}

func TestSessionValidateAndSummary(t *testing.T) {
	s := session()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	sum := Summarize(s)
	if sum.Similar != 2 || sum.Dissimilar != 1 || sum.Explained != 1 || sum.NotComparable != 1 || sum.ComponentsCompared != 3 {
		t.Errorf("summary %+v", sum)
	}
	for name, mutate := range map[string]func(*Session){
		"freehand":          func(s *Session) { s.Markups[0].Kind = "FREEHAND" },
		"text on face":      func(s *Session) { s.Markups[0].Kind = "TEXT" },
		"no side":           func(s *Session) { s.Findings[1].Side = "" },
		"side on nose":      func(s *Session) { s.Findings[2].Side = "LEFT" },
		"descriptor":        func(s *Session) { s.Findings[2].Descriptors = []string{"LOBULE"} },
		"explained alike":   func(s *Session) { s.Findings[2].Explanations = []string{"POSE"} },
		"explained and not": func(s *Session) { s.Findings[1].Explanations = []string{"AGE", Unexplained} },
		"given twice":       func(s *Session) { s.Findings[1].Explanations = []string{"AGE", "AGE"} },
		"old scale":         func(s *Session) { s.Opinion.Conclusion = "IDENTIFICATION" },
	} {
		bad := session()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	w := MarkupWarnings(s)
	if len(w) != 1 || w[0].Code != "MARKUP_ONE_SIDE" {
		t.Errorf("markup warnings %+v", w)
	}
	s.Markups = append(s.Markups, Markup{Image: "Q", Number: 3, Kind: MarkupBox, Points: [][2]float64{{25, 25}, {50, 40}}})
	found := false
	for _, w := range MarkupWarnings(s) {
		found = found || w.Code == "MARKUP_OVERLAP"
	}
	if !found {
		t.Error("overlapping markups are not warned about")
	}
}

func TestSheetAndRecord(t *testing.T) {
	q := image.NewNRGBA(image.Rect(0, 0, 120, 100))
	k := image.NewNRGBA(image.Rect(0, 0, 60, 50))
	for i := range q.Pix {
		q.Pix[i] = 90
	}
	for i := range k.Pix {
		k.Pix[i] = 150
	}
	id := imageproc.Affine{A: 1, E: 1}
	align := Similarity{Transform: imageproc.Affine{A: 2, E: 2}, Scale: 2, Pairs: 2}
	s := session()
	sheet, err := RenderSheet(SheetImage{q, id}, SheetImage{k, id}, &align, s.Markups)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := png.Decode(bytes.NewReader(sheet.Clean))
	if err != nil {
		t.Fatal(err)
	}
	annotated, _ := png.Decode(bytes.NewReader(sheet.Annotated))
	if clean.Bounds() != annotated.Bounds() || sheet.CleanSHA256 == sheet.AnnotatedSHA256 || !sheet.KnownResampled {
		t.Fatal("the annotated sheet should differ from the clean one only by its markups")
	}
	// The known image is drawn in the questioned frame: 120 x 100 after the scale of 2.
	if clean.Bounds().Dx() != sheetMargin*2+120+sheetGap+120 {
		t.Errorf("sheet width %d", clean.Bounds().Dx())
	}
	// The known ellipse's outline crosses x = 22 (original), at its vertical centre, in the panel.
	kx := sheetMargin + 120 + sheetGap + 2*22
	ky := sheetMargin + sheetHeader + 2*28
	r, g, b, _ := annotated.At(kx, ky).RGBA()
	if r>>8 < 200 || g>>8 < 150 || b>>8 > 80 {
		t.Errorf("no markup colour at (%d, %d): %d %d %d", kx, ky, r>>8, g>>8, b>>8)
	}

	rec := Describe(RecordInput{Session: s, Questioned: SideRecord{Filename: "q.jpg", Metrics: Measure(q, nil, s.Questioned.Landmarks)},
		Known: SideRecord{Filename: "k.jpg"}, Alignment: &align, Sheet: &sheet, Warnings: MarkupWarnings(s), Examiner: "Ana", At: time.Now()})
	for _, want := range []string{"questionada foi analisada antes", "Não foi utilizada fotoantropometria", "Marca na pele: semelhante",
		"Sobrancelha (lado direito): diferente (forma); diferença atribuível a: expressão e condições de captura da imagem", "Suporte à origem comum",
		"OSAC 2022-S-0001", "Physical Stability of Facial Features of Adults", sheet.AnnotatedSHA256, "existe só na imagem questionada",
		"não passou por verificação independente"} {
		if !strings.Contains(rec, want) {
			t.Errorf("the record lacks %q", want)
		}
	}
}

// A session saved with the single explanation of before still loads.
func TestOldExplanationLoads(t *testing.T) {
	s := session()
	s.Findings[1].Explanations, s.Findings[1].Explanation = nil, "AGE"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if f := s.Findings[1]; f.Explanation != "" || len(f.Explanations) != 1 || f.Explanations[0] != "AGE" {
		t.Errorf("finding %+v", f)
	}
}
