package facecompare

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rodfileto/trackid/imageproc"
)

// SideRecord is what the record says about one image beyond the session.
type SideRecord struct {
	Filename   string
	Original   imageproc.Original
	Exif       imageproc.Exif
	Processing string // imageproc.DescribeFace
	Metrics    Metrics
}

// RecordInput is everything Describe writes out.
type RecordInput struct {
	Session    Session
	Questioned SideRecord
	Known      SideRecord
	Alignment  *Similarity
	Sheet      *Sheet
	Warnings   []Warning
	Examiner   string
	At         time.Time
}

// Software names the comparison module and, when the binary carries it, its source revision.
func Software() string {
	s := fmt.Sprintf("TrackID, módulo facecompare, sessão versão %d", SessionVersion)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 12 {
				s += ", revisão " + setting.Value[:12]
			}
		}
	}
	return s
}

// Describe writes the comparison record in Portuguese. Its sections follow FISWG's Minimum
// Guidelines for Facial Image Comparison Documentation (v2.0, §5–9): the images, the analysis of
// each, the method, the features compared, the evaluation, and the references.
func Describe(in RecordInput) string {
	s := in.Session
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }

	line("REGISTRO DE COMPARAÇÃO FACIAL (UM PARA UM)")
	line("")
	line("1. Imagens")
	for _, side := range []struct {
		code, name string
		rec        SideRecord
	}{{"Q", "Questionada", in.Questioned}, {"K", "Padrão", in.Known}} {
		o := side.rec.Original
		line("%s (%s): arquivo %s, formato %s, %d × %d pixels, SHA-256 %s.", side.name, side.code, orDash(side.rec.Filename),
			strings.ToUpper(o.Format), o.Width, o.Height, o.SHA256)
	}
	line("")

	line("2. Análise")
	if s.QuestionedFirst {
		line("A imagem questionada foi analisada antes da padrão: a padrão permaneceu oculta até o término dessa análise.")
	} else {
		line("A imagem padrão esteve visível durante a análise da questionada.")
	}
	for i, side := range []struct {
		name string
		side Side
		rec  SideRecord
	}{{"questionada", s.Questioned, in.Questioned}, {"padrão", s.Known, in.Known}} {
		a := side.side.Analysis
		line("2.%d Imagem %s", i+1, side.name)
		x := side.rec.Exif
		var meta []string
		if x.DateTimeOriginal != "" {
			meta = append(meta, "data de captura "+x.DateTimeOriginal)
		}
		if x.Make != "" || x.Model != "" {
			meta = append(meta, "câmera "+strings.TrimSpace(x.Make+" "+x.Model))
		}
		if x.Software != "" {
			meta = append(meta, "software "+x.Software)
		}
		if x.DateTime != "" && x.DateTime != x.DateTimeOriginal {
			meta = append(meta, "última alteração do arquivo "+x.DateTime)
		}
		if len(meta) > 0 {
			line("Metadados EXIF (informados pelo arquivo, não verificados): %s.", strings.Join(meta, "; "))
		} else {
			line("O arquivo não traz metadados EXIF de captura.")
		}
		if a.CaptureDate != "" {
			line("Data de captura informada: %s.", a.CaptureDate)
		}
		m := side.rec.Metrics
		if m.Pose != nil {
			line("Distância interpupilar: %.0f pixels na imagem original. Rotação no plano: %s°. Giro estimado: %s° (estimativa grosseira a partir dos pontos de referência).",
				m.Pose.IPD, num(m.Pose.Roll, 1), num(m.Pose.Yaw, 0))
		}
		if m.Width > 0 {
			line("Região da face: %d × %d pixels; pixels saturados: %s%% em sombras e %s%% em altas luzes.",
				m.Width, m.Height, num(100*m.ClippedDark, 1), num(100*m.ClippedBright, 1))
		}
		if l := side.side.Landmarks; l != nil {
			src := "pelo detector automático e conferidos pelo perito"
			if l.Source == "EXAMINER" {
				src = "pelo perito"
			}
			line("Pontos de referência (centros dos olhos, ponta do nariz e cantos da boca) marcados %s; servem ao alinhamento e à estimativa de pose, não à comparação.", src)
		}
		if len(a.Factors) > 0 {
			line("Fatores de imagem:")
			for _, f := range a.Factors {
				factor, _ := labelOf(ImageFactors, f.Code)
				effect, _ := labelOf(Effects, f.Effect)
				note := ""
				if f.Notes != "" {
					note = " — " + f.Notes
				}
				line("- %s: %s%s.", factor.PT, strings.ToLower(effect.PT), strings.TrimSuffix(note, "."))
			}
		}
		if a.Mirrored {
			line("A imagem apresenta indícios de espelhamento (inversão lateral).")
		}
		if a.ManipulationSuspected {
			line("Há suspeita de manipulação da imagem%s. Recomenda-se encaminhá-la a perito em imagem digital ou requisitar o arquivo original.", withNote(a.ManipulationNotes))
		}
		if a.Notes != "" {
			line("Observações: %s", sentence(a.Notes))
		}
		if suit, ok := labelOf(Suitabilities, a.Suitability); ok {
			line("Aptidão: %s%s.", strings.ToLower(suit.PT), withNote(a.SuitabilityReason))
		} else {
			line("Aptidão: não avaliada.")
		}
		line("Processamento: %s", strings.TrimSpace(strings.ReplaceAll(side.rec.Processing, "\n", " ")))
	}
	line("")

	line("3. Método")
	line("Análise morfológica: comparação visual, lado a lado, dos componentes faciais e de suas características, segundo a lista de características do FISWG (ASTM E3149), considerando a estabilidade de cada característica.")
	if in.Alignment != nil {
		line("Alinhamento por transformação de similaridade (rotação, escala uniforme e translação, sem alterar proporções) ajustada a %d par(es) de pontos correspondentes: escala %s, rotação %s°, erro médio quadrático de %s pixels.",
			in.Alignment.Pairs, num(in.Alignment.Scale, 3), num(in.Alignment.Rotation, 1), num(in.Alignment.RMS, 1))
	}
	if s.Superimposition {
		line("Superposição (sobreposição com transparência, cortina ou alternância) foi usada como recurso auxiliar, em conjunto com a análise morfológica e nunca isoladamente.")
	}
	line("Não foi utilizada fotoantropometria (medição de distâncias, proporções ou ângulos entre pontos da face), conforme o FISWG.")
	if len(s.Markups) > 0 {
		line("As marcações (elipses, retângulos, colchetes, setas e pontos numerados) seguem as boas práticas do FISWG: nenhum traçado sobre as características e nenhum texto sobre a face; cada número remete a um achado abaixo.")
	}
	if len(s.Factors) > 0 {
		var names []string
		for _, f := range s.Factors {
			l, _ := labelOf(StabilityFactors, f)
			names = append(names, strings.ToLower(l.PT))
		}
		line("Fatores de variação considerados entre as imagens: %s.", joinPT(names))
	} else {
		line("Nenhum fator de variação entre as imagens foi indicado; a estabilidade das características não foi ponderada.")
	}
	line("")

	line("4. Características comparadas")
	if len(s.Findings) == 0 {
		line("Nenhuma característica registrada.")
	}
	n := 0
	for _, comp := range Components {
		var rows []Finding
		for _, f := range s.Findings {
			if c, _, ok := Find(f.Characteristic); ok && c.Code == comp.Code {
				rows = append(rows, f)
			}
		}
		if len(rows) == 0 {
			continue
		}
		n++
		line("4.%d %s", n, comp.PT)
		for _, f := range rows {
			_, ch, _ := Find(f.Characteristic)
			name := ch.PT
			if f.Side == "RIGHT" {
				name += " (lado direito)"
			} else if f.Side == "LEFT" {
				name += " (lado esquerdo)"
			}
			res, _ := labelOf(Results, f.Result)
			text := fmt.Sprintf("- %s: %s", name, strings.ToLower(res.PT))
			if len(f.Descriptors) > 0 {
				var ds []string
				for _, d := range f.Descriptors {
					ds = append(ds, strings.ToLower(Descriptors[d].PT))
				}
				text += " (" + strings.Join(ds, ", ") + ")"
			}
			if contains(f.Explanations, Unexplained) {
				text += "; diferença sem explicação"
			} else if len(f.Explanations) > 0 {
				var es []string
				for _, code := range f.Explanations {
					e, _ := labelOf(Explanations, code)
					es = append(es, strings.ToLower(e.PT))
				}
				text += "; diferença atribuível a: " + joinPT(es)
			}
			if st := Weakest(ch, s.Factors); st != "" {
				text += "; estabilidade " + stabilityText(st) + " sob os fatores considerados"
			}
			if len(f.Markups) > 0 {
				var ns []string
				for _, m := range f.Markups {
					ns = append(ns, fmt.Sprint(m))
				}
				text += "; marcação " + strings.Join(ns, ", ")
			}
			line("%s.", text)
			if f.Questioned != "" {
				line("  Q: %s", sentence(f.Questioned))
			}
			if f.Known != "" {
				line("  K: %s", sentence(f.Known))
			}
			if f.Notes != "" {
				line("  %s", sentence(f.Notes))
			}
		}
	}
	var missing []string
	for _, comp := range Components {
		found := false
		for _, f := range s.Findings {
			if c, _, ok := Find(f.Characteristic); ok && c.Code == comp.Code {
				found = true
			}
		}
		if !found && comp.Code != "OTHER" && comp.Code != "ALTERATIONS" && comp.Code != "SCARS" && comp.Code != "MARKS" {
			missing = append(missing, strings.ToLower(comp.PT))
		}
	}
	if len(s.Findings) > 0 && len(missing) > 0 {
		line("Componentes sem registro: %s.", joinPT(missing))
	}
	line("")

	line("5. Avaliação")
	sum := Summarize(s)
	line("Semelhanças: %d. Diferenças: %d, das quais %d atribuídas a variação do sujeito ou às condições de captura. Não comparáveis: %d. Componentes comparados: %d de %d.",
		sum.Similar, sum.Dissimilar, sum.Explained, sum.NotComparable, sum.ComponentsCompared, len(Components))
	if o := s.Opinion; o != nil {
		l, _ := Level(o.Conclusion)
		line("Opinião: %s — %s", l.PT, l.MeaningPT)
		if o.Reasoning != "" {
			line("Fundamentação: %s", sentence(o.Reasoning))
		}
		line("A opinião exprime a força com que as observações sustentam uma das hipóteses (mesma pessoa ou pessoas distintas), na escala da OSAC 2022-S-0001 referida pelo FISWG; não é uma certeza absoluta.")
	} else {
		line("Opinião: não registrada.")
	}
	line("Esta comparação foi feita no laboratório facial do TrackID, fora de um caso: não passou por verificação independente.")
	line("")

	if len(in.Warnings) > 0 {
		line("6. Avisos")
		for _, w := range in.Warnings {
			line("- %s", w.Message)
		}
		line("")
	}

	if sh := in.Sheet; sh != nil {
		line("Folha de comparação: com marcações, SHA-256 %s; sem marcações, SHA-256 %s.", sh.AnnotatedSHA256, sh.CleanSHA256)
		if sh.KnownResampled {
			line("Na folha, a imagem padrão foi levada ao enquadramento da questionada pelo alinhamento acima, com interpolação bilinear; a ampliação não acrescenta detalhe.")
		}
		line("")
	}

	line("Referências")
	for _, r := range References {
		line("- %s, versão %s, %s: %s.", r.Title, r.Version, r.Sections, r.Use)
	}
	line("Os nomes das características e as classificações de estabilidade foram tomados desses documentos, que devem ser consultados para suas definições.")
	line("")
	line("Software: %s; %s.", Software(), imageproc.FaceSoftware())
	who := in.Examiner
	if who == "" {
		who = "(não identificado)"
	}
	line("Registro gerado para %s em %s.", who, in.At.Format("02/01/2006 15:04:05 MST"))
	return b.String()
}

func stabilityText(s Stability) string {
	switch s {
	case High:
		return "alta"
	case Medium:
		return "média"
	}
	return "baixa"
}

func withNote(note string) string {
	note = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(note), "."))
	if note == "" {
		return ""
	}
	return " (" + note + ")"
}

func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "?") && !strings.HasSuffix(s, "!") {
		s += "."
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "(sem nome)"
	}
	return s
}

func num(v float64, decimals int) string {
	return strings.Replace(fmt.Sprintf("%.*f", decimals, v), ".", ",", 1)
}

func joinPT(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " e " + items[len(items)-1]
}
