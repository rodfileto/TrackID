package imageproc

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// FaceRecord is what DescribeFace writes out.
type FaceRecord struct {
	Filename   string
	Original   Original
	Recipe     FaceRecipe
	Result     FaceResult
	Examiner   string
	RenderedAt time.Time
}

// FaceSoftware names the face renderer and, when the binary carries it, its source revision.
func FaceSoftware() string {
	s := fmt.Sprintf("TrackID, módulo imageproc, receita facial versão %d", FaceRecipeVersion)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 12 {
				s += ", revisão " + setting.Value[:12]
			}
		}
	}
	return s
}

// DescribeFace writes a face image's processing record in Portuguese, after FISWG's Image
// Processing Techniques for Facial Image Comparison (v1.0, §4 and §6) and ASTM E2825, as Describe
// does for fingerprints. A recipe that changes nothing is recorded as no processing.
func DescribeFace(rec FaceRecord) string {
	r, o, res := rec.Recipe, rec.Original, rec.Result
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	name := rec.Filename
	if name == "" {
		name = "(sem nome)"
	}
	line("Imagem original: arquivo %s, formato %s, %d × %d pixels, SHA-256 %s.", name, strings.ToUpper(o.Format), o.Width, o.Height, o.SHA256)
	if o.Orientation >= 2 && o.Orientation <= 8 {
		line("A orientação registrada nos metadados EXIF (%s) foi aplicada na leitura, como na exibição da foto.", orientationText(o.Orientation))
	}
	line("A imagem original foi preservada sem alteração; o processamento foi feito sobre cópia de trabalho.")

	var steps []string
	step := func(format string, args ...any) { steps = append(steps, fmt.Sprintf(format, args...)) }
	if r.Crop != nil {
		c := r.Crop
		step("recorte da região x = %s a %s, y = %s a %s (%s × %s pixels)",
			num(c.X1, 0), num(c.X2, 0), num(c.Y1, 0), num(c.Y2, 0), num(c.X2-c.X1, 0), num(c.Y2-c.Y1, 0))
	}
	if r.Rotate != 0 {
		dir := "horário"
		if r.Rotate < 0 {
			dir = "anti-horário"
		}
		interp := ""
		if r.Scale == 1 {
			interp = ", por " + interpolationText(r.Interpolation)
		}
		step("rotação de %s° no sentido %s%s, com os cantos sem dados preenchidos em cinza neutro (%d)", num(abs(r.Rotate), 1), dir, interp, faceFill)
	}
	if r.Mirror {
		step("espelhamento horizontal (inversão lateral)")
	}
	geometric := len(steps)
	if r.Scale != 1 {
		s := fmt.Sprintf("redimensionamento uniforme pelo fator %s, por %s", num(r.Scale, 3), interpolationText(r.Interpolation))
		if res.AntialiasSigma > 0 {
			s += fmt.Sprintf(", precedido de filtro gaussiano antisserrilhamento (σ = %s pixels da original)", num(res.AntialiasSigma, 2))
		}
		if r.Scale > 1 {
			s += "; a ampliação não acrescenta detalhe à imagem"
		}
		step("%s", s)
		geometric++
	}
	if cb := r.ColorBalance; cb != nil {
		step("equilíbrio de cor por ganho em cada canal: vermelho × %s, verde × %s, azul × %s", num(cb.Red, 3), num(cb.Green, 3), num(cb.Blue, 3))
	}
	if r.Grey != "" {
		step("conversão para tons de cinza: %s", channelText(r.Grey))
	}
	if bc := r.BrightnessContrast; bc != nil {
		step("brilho %s e contraste %s (escala de -100 a 100; tom' = (tom − 128) × (100 + contraste) / (100 − contraste) + 128 + 1,275 × brilho)",
			signed(bc.Brightness), signed(bc.Contrast))
	}
	if l := r.Levels; l != nil {
		step("níveis: entrada %s a %s, gama %s; saída %s a %s",
			num(l.InBlack, 0), num(l.InWhite, 0), num(l.Gamma, 2), num(l.OutBlack, 0), num(l.OutWhite, 0))
	}
	if cv := r.Curves; cv != nil {
		var pts []string
		for _, p := range cv.Points {
			pts = append(pts, fmt.Sprintf("%s→%s", num(p[0], 0), num(p[1], 0)))
		}
		step("curvas: interpolação cúbica monotônica pelos pontos %s (entrada→saída)", strings.Join(pts, ", "))
	}

	if len(steps) == 0 {
		line("Nenhum processamento: a imagem foi examinada como recebida.")
		return b.String()
	}
	line("Operações aplicadas, nesta ordem:")
	for i, s := range steps {
		line("%d. %s.", i+1, capitalize(s))
	}
	if geometric > 1 {
		which := fmt.Sprintf("1 a %d", geometric)
		if geometric == 2 {
			which = "1 e 2"
		}
		line("As operações %s foram executadas numa única transformação geométrica, com uma única interpolação, sem alterar as proporções da face.", which)
	}
	kind := "em cores RGB de 8 bits"
	if res.Grey {
		kind = "em tons de cinza de 8 bits"
	}
	line("Imagem resultante: PNG (compressão sem perdas), %s, %d × %d pixels, SHA-256 %s.", kind, res.Width, res.Height, res.SHA256)
	line("Todas as operações atuam igualmente sobre a imagem inteira; nenhuma área foi retocada localmente.")
	line("Software: %s. Os parâmetros completos estão na receita; aplicada à imagem original identificada acima, ela reproduz a imagem resultante.", FaceSoftware())
	who := rec.Examiner
	if who == "" {
		who = "(não identificado)"
	}
	line("Processado por %s em %s.", who, rec.RenderedAt.Format("02/01/2006 15:04:05 MST"))
	return b.String()
}

// Identity reports whether the recipe changes nothing: the render is the original's pixels.
func (r FaceRecipe) Identity() bool {
	return r.Crop == nil && r.Rotate == 0 && !r.Mirror && (r.Scale == 1 || r.Scale == 0) && r.ColorBalance == nil &&
		r.Grey == "" && r.BrightnessContrast == nil && r.Levels == nil && r.Curves == nil
}

func signed(v float64) string {
	if v > 0 {
		return "+" + num(v, 0)
	}
	return num(v, 0)
}
