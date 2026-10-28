package imageproc

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// Record is what Describe writes out: the original, the recipe, the render, who and when.
type Record struct {
	Filename   string
	Original   Original
	Recipe     Recipe
	Result     Result
	Examiner   string
	RenderedAt time.Time
}

// Software names this renderer and, when the binary carries it, its source revision.
func Software() string {
	s := fmt.Sprintf("TrackID, módulo imageproc, receita versão %d", RecipeVersion)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 12 {
				s += ", revisão " + setting.Value[:12]
			}
		}
	}
	return s
}

// Describe writes the processing record in Portuguese, for the laudo and the case file. It
// follows what FISWG's Image Processing Techniques for Facial Image Comparison (v1.0, sections 4
// and 6) and ASTM E2825 ask of forensic image processing: the original preserved and processing
// done on a working copy; every step, in order, with its settings; the software and its version;
// reproducibility; a lossless result; and the risk of artifacts.
func Describe(rec Record) string {
	r, o, res := rec.Recipe, rec.Original, rec.Result
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("REGISTRO DE PROCESSAMENTO DE IMAGEM")
	line("")
	line("1. Imagem original")
	name := rec.Filename
	if name == "" {
		name = "(sem nome)"
	}
	line("Arquivo %s, formato %s, %d × %d pixels.", name, strings.ToUpper(o.Format), o.Width, o.Height)
	line("SHA-256: %s.", o.SHA256)
	if o.Orientation >= 2 && o.Orientation <= 8 {
		line("A orientação registrada nos metadados EXIF (%s) foi aplicada na leitura, como na exibição da foto.", orientationText(o.Orientation))
	}
	line("A imagem original foi preservada sem alteração; todo o processamento foi feito sobre cópia de trabalho.")
	line("")

	line("2. Resolução de origem")
	src := r.Source
	switch src.Method {
	case PPICalibrated:
		c := src.Calibration
		line("Calibrada: segmento de %s pixels traçado sobre escala presente na imagem corresponde a %s mm, o que resulta em %s ppi.",
			num(c.LengthPX(), 1), num(c.MM, 2), num(src.PPI, 0))
	case PPIFile:
		line("Informada nos metadados do arquivo: %s ppi, conferida pelo perito.", num(src.PPI, 0))
	case PPIEstimated:
		line("Estimada pelo período médio das cristas na região analisada: %s pixels, considerando distância média entre cristas de %s mm, o que resulta em %s ppi.",
			num(src.RidgePeriodPX, 1), num(src.RidgeDistanceMM, 2), num(src.PPI, 0))
		line("Trata-se de estimativa: a distância entre cristas varia entre pessoas, entre dedos e entre regiões de um mesmo dedo (aproximadamente 0,4 a 0,5 mm em adultos), e o erro da escala resultante é da mesma ordem.")
	default:
		line("Não determinada: assumida igual à de saída, %d ppi, sem calibração. As medidas na imagem resultante não têm escala verificada.", OutputPPI)
	}
	line("")

	line("3. Operações aplicadas, nesta ordem")
	n := 0
	step := func(format string, args ...any) {
		n++
		line("3.%d %s", n, fmt.Sprintf(format, args...))
	}
	step("Conversão para tons de cinza: %s.", channelText(r.Channel))
	geometryFirst := n + 1
	if r.Crop != nil {
		c := r.Crop
		step("Recorte da região x = %s a %s, y = %s a %s da imagem original (%s × %s pixels).",
			num(c.X1, 0), num(c.X2, 0), num(c.Y1, 0), num(c.Y2, 0), num(c.X2-c.X1, 0), num(c.Y2-c.Y1, 0))
	}
	if r.Rotate != 0 {
		dir := "horário"
		if r.Rotate < 0 {
			dir = "anti-horário"
		}
		step("Rotação de %s° no sentido %s.", num(abs(r.Rotate), 1), dir)
	}
	if r.Mirror {
		step("Espelhamento horizontal (inversão lateral), para que a imagem tenha a orientação de uma impressão por contato.")
	}
	resample := fmt.Sprintf("Reamostragem de %s ppi para %d ppi (fator %s) por %s", num(src.PPI, 0), OutputPPI, num(res.Scale, 3), interpolationText(r.Interpolation))
	if res.AntialiasSigma > 0 {
		resample += fmt.Sprintf(", precedida de filtro gaussiano antisserrilhamento (σ = %s pixels da original)", num(res.AntialiasSigma, 2))
	}
	step("%s.", resample)
	if n > geometryFirst {
		line("As operações 3.%d a 3.%d foram executadas numa única transformação geométrica, com uma única interpolação.", geometryFirst, n)
	}
	if r.Rotate != 0 {
		line("Os cantos sem dados após a rotação foram preenchidos com o tom médio da região (%s).", num(res.Fill, 0))
	}
	if r.Flatten != nil {
		step("Correção de iluminação: divisão da imagem por cópia suavizada por filtro gaussiano (σ = %s mm, %s pixels), reescalada ao tom médio.",
			num(r.Flatten.SigmaMM, 1), num(res.FlattenSigmaPX, 1))
	}
	if l := r.Levels; l != nil {
		step("Níveis: entrada %s a %s, gama %s; saída %s a %s.",
			num(l.InBlack, 0), num(l.InWhite, 0), num(l.Gamma, 2), num(l.OutBlack, 0), num(l.OutWhite, 0))
	}
	if c := r.CLAHE; c != nil {
		step("Equalização adaptativa de histograma com limitação de contraste (CLAHE): grade de %d × %d blocos, limite de corte %s.",
			c.Tiles, c.Tiles, num(c.ClipLimit, 1))
	}
	if g := r.Gabor; g != nil {
		period := fmt.Sprintf("período de cristas de %s pixels (%s mm), medido na imagem", num(res.GaborPeriodPX, 1), num(res.GaborPeriodPX/OutputPPI*25.4, 2))
		if !res.GaborEstimated {
			period = fmt.Sprintf("período de cristas padrão de %s pixels, por não se ter medido um período claro na imagem", num(res.GaborPeriodPX, 1))
		}
		step("Realce de cristas por filtragem de Gabor orientada pelo fluxo local das cristas (Hong, Wan e Jain, 1998): %s; %d orientações. Regiões sem padrão de cristas foram preenchidas com o fundo.", period, g.Orientations)
	}
	if r.Invert {
		step("Inversão de tons (negativo).")
	}
	line("")

	line("4. Imagem resultante")
	line("PNG (compressão sem perdas), tons de cinza de 8 bits, %d × %d pixels, %d ppi (registrado no arquivo).", res.Width, res.Height, OutputPPI)
	line("SHA-256: %s.", res.SHA256)
	line("")

	line("5. Software e reprodutibilidade")
	line("%s.", Software())
	line("Os parâmetros completos estão na receita anexa; aplicada à imagem original identificada acima, ela reproduz a imagem resultante.")
	line("")

	line("6. Observações")
	line("- Conversão de canal, níveis, inversão e a transformação geométrica atuam igualmente sobre todos os pixels.")
	var adaptive []string
	if r.Flatten != nil {
		adaptive = append(adaptive, "a correção de iluminação")
	}
	if r.CLAHE != nil {
		adaptive = append(adaptive, "o CLAHE")
	}
	if r.Gabor != nil {
		adaptive = append(adaptive, "a filtragem de Gabor")
	}
	if len(adaptive) > 0 {
		line("- %s %s adaptativ%s: o ajuste varia com o conteúdo local, calculado automaticamente sobre a imagem inteira. Nenhuma área foi retocada manualmente.",
			capitalize(joinPT(adaptive)), plural(len(adaptive), "é", "são"), plural(len(adaptive), "a", "as"))
	} else {
		line("- Nenhuma área foi retocada manualmente.")
	}
	if r.Gabor != nil {
		line("- A filtragem de Gabor pode dar aparência de continuidade a cristas onde a imagem original é ambígua e criar padrões nas bordas e em regiões de baixa qualidade. Características observadas na imagem realçada devem ser confirmadas na imagem original.")
	}
	if src.Method == PPIEstimated || src.Method == PPIAssumed {
		line("- Sem calibração por escala, a resolução da imagem resultante é aproximada.")
	}
	line("- Referências: FISWG, Image Processing Techniques for Facial Image Comparison, v1.0, seções 4 e 6; ASTM E2825, Standard Guide for Forensic Digital Image Processing.")
	line("")
	who := rec.Examiner
	if who == "" {
		who = "(não identificado)"
	}
	line("Processado por %s em %s.", who, rec.RenderedAt.Format("02/01/2006 15:04:05 MST"))
	return b.String()
}

func channelText(c string) string {
	switch c {
	case ChannelRed:
		return "canal vermelho"
	case ChannelGreen:
		return "canal verde"
	case ChannelBlue:
		return "canal azul"
	}
	return "luminância (pesos ITU-R BT.601: 0,299 R + 0,587 G + 0,114 B)"
}

func interpolationText(i string) string {
	switch i {
	case InterpolationNearest:
		return "interpolação pelo vizinho mais próximo"
	case InterpolationBilinear:
		return "interpolação bilinear"
	}
	return "interpolação bicúbica (Catmull-Rom)"
}

func orientationText(o int) string {
	return map[int]string{
		2: "espelhamento horizontal", 3: "rotação de 180°", 4: "espelhamento vertical",
		5: "transposição", 6: "rotação de 90° no sentido horário", 7: "transposição inversa",
		8: "rotação de 90° no sentido anti-horário",
	}[o]
}

// num formats with a decimal comma.
func num(v float64, decimals int) string {
	return strings.Replace(fmt.Sprintf("%.*f", decimals, v), ".", ",", 1)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func joinPT(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " e " + items[len(items)-1]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
