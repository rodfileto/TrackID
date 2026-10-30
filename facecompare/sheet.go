package facecompare

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/rodfileto/trackid/imageproc"
)

// The comparison sheet: the two processed images side by side, one with the markups and one
// without (FISWG's Best Practices for Markups & Annotations, §1.2: a non-annotated image goes with
// the annotated one). The server draws it so the image that goes into a report never comes from a
// browser canvas.

// SheetImage is one rendered image and the transform from its original pixels to it.
type SheetImage struct {
	Image     image.Image
	Transform imageproc.Affine
}

// Sheet is the drawn sheet.
type Sheet struct {
	Annotated       []byte
	AnnotatedSHA256 string
	Clean           []byte
	CleanSHA256     string
	// KnownResampled is true when the known image was carried into the questioned image's frame
	// by the alignment (resampled bilinearly).
	KnownResampled bool
}

var (
	markupColour = color.NRGBA{255, 214, 0, 255}
	haloColour   = color.NRGBA{0, 0, 0, 255}
	inkColour    = color.NRGBA{40, 40, 40, 255}
)

const (
	sheetMargin = 24
	sheetHeader = 36
	sheetGap    = 24
)

// RenderSheet draws q and k side by side. With an alignment (k's render onto q's), k is drawn in
// q's frame, at the same scale and roll; otherwise each is drawn as rendered.
func RenderSheet(q, k SheetImage, align *Similarity, markups []Markup) (Sheet, error) {
	var out Sheet
	kImg, kT := k.Image, k.Transform
	if align != nil {
		kImg = warpInto(k.Image, align.Transform, q.Image.Bounds().Dx(), q.Image.Bounds().Dy())
		kT = Compose(k.Transform, align.Transform)
		out.KnownResampled = true
	}
	qb, kb := q.Image.Bounds(), kImg.Bounds()
	w := sheetMargin*2 + qb.Dx() + sheetGap + kb.Dx()
	h := sheetMargin*2 + sheetHeader + max(qb.Dy(), kb.Dy())
	qAt := image.Pt(sheetMargin, sheetMargin+sheetHeader)
	kAt := image.Pt(sheetMargin+qb.Dx()+sheetGap, sheetMargin+sheetHeader)

	base := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(base, base.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(base, image.Rectangle{qAt, qAt.Add(qb.Size())}, q.Image, qb.Min, draw.Src)
	draw.Draw(base, image.Rectangle{kAt, kAt.Add(kb.Size())}, kImg, kb.Min, draw.Src)
	face := sheetFace(18)
	drawText(base, face, "Questionada (Q)", float64(qAt.X), float64(sheetMargin+24), inkColour, false)
	drawText(base, face, "Padrão (K)", float64(kAt.X), float64(sheetMargin+24), inkColour, false)

	var err error
	if out.Clean, err = encodePNG(base); err != nil {
		return Sheet{}, err
	}
	annotated := image.NewNRGBA(base.Bounds())
	copy(annotated.Pix, base.Pix)
	stroke := math.Max(2, math.Min(float64(min(qb.Dx(), qb.Dy())), float64(min(kb.Dx(), kb.Dy())))/250)
	label := sheetFace(math.Max(14, stroke*7))
	for _, m := range markups {
		t, at, bounds := q.Transform, qAt, image.Rectangle{qAt, qAt.Add(qb.Size())}
		if m.Image == "K" {
			t, at, bounds = kT, kAt, image.Rectangle{kAt, kAt.Add(kb.Size())}
		}
		drawMarkup(annotated, label, m, t, at, bounds, stroke)
	}
	if out.Annotated, err = encodePNG(annotated); err != nil {
		return Sheet{}, err
	}
	out.CleanSHA256, out.AnnotatedSHA256 = sha(out.Clean), sha(out.Annotated)
	return out, nil
}

// warpInto draws src through t (src pixels to the result's) on a w x h canvas of neutral grey,
// sampling bilinearly.
func warpInto(src image.Image, t imageproc.Affine, w, h int) image.Image {
	inv := t.Invert()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	at := func(x, y int) [3]float64 {
		x = min(max(x, 0), b.Dx()-1)
		y = min(max(y, 0), b.Dy()-1)
		r, g, bl, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return [3]float64{float64(r) / 257, float64(g) / 257, float64(bl) / 257}
	}
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			sx, sy := inv.Apply(float64(i)+0.5, float64(j)+0.5)
			sx, sy = sx-0.5, sy-0.5
			o := 4 * (j*w + i)
			if sx < -0.5 || sy < -0.5 || sx > float64(b.Dx())-0.5 || sy > float64(b.Dy())-0.5 {
				out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = 128, 128, 128, 255
				continue
			}
			fx, fy := math.Floor(sx), math.Floor(sy)
			tx, ty := sx-fx, sy-fy
			ix, iy := int(fx), int(fy)
			p00, p10, p01, p11 := at(ix, iy), at(ix+1, iy), at(ix, iy+1), at(ix+1, iy+1)
			for c := 0; c < 3; c++ {
				v := (p00[c]*(1-tx)+p10[c]*tx)*(1-ty) + (p01[c]*(1-tx)+p11[c]*tx)*ty
				out.Pix[o+c] = uint8(math.Round(math.Max(0, math.Min(255, v))))
			}
			out.Pix[o+3] = 255
		}
	}
	return out
}

// drawMarkup draws m, given in original pixels, through t onto the panel placed at at.
func drawMarkup(dst *image.NRGBA, label font.Face, m Markup, t imageproc.Affine, at image.Point, panel image.Rectangle, w float64) {
	pt := func(x, y float64) [2]float64 {
		px, py := t.Apply(x, y)
		return [2]float64{px + float64(at.X), py + float64(at.Y)}
	}
	var paths [][][2]float64
	var dots [][2]float64
	var anchor [2]float64 // where the number goes
	switch m.Kind {
	case MarkupBox:
		x1, y1, x2, y2 := m.Points[0][0], m.Points[0][1], m.Points[1][0], m.Points[1][1]
		corners := [][2]float64{pt(x1, y1), pt(x2, y1), pt(x2, y2), pt(x1, y2), pt(x1, y1)}
		paths = append(paths, corners)
		anchor = outside(corners, w)
	case MarkupEllipse:
		cx, cy := (m.Points[0][0]+m.Points[1][0])/2, (m.Points[0][1]+m.Points[1][1])/2
		rx, ry := math.Abs(m.Points[1][0]-m.Points[0][0])/2, math.Abs(m.Points[1][1]-m.Points[0][1])/2
		var ring [][2]float64
		for i := 0; i <= 72; i++ {
			a := float64(i) / 72 * 2 * math.Pi
			ring = append(ring, pt(cx+rx*math.Cos(a), cy+ry*math.Sin(a)))
		}
		paths = append(paths, ring)
		anchor = outside(ring, w)
	case MarkupBracket:
		a, b := pt(m.Points[0][0], m.Points[0][1]), pt(m.Points[1][0], m.Points[1][1])
		nx, ny := perpendicular(a, b)
		tick := 4 * w
		paths = append(paths, [][2]float64{a, b},
			[][2]float64{{a[0] - nx*tick, a[1] - ny*tick}, {a[0] + nx*tick, a[1] + ny*tick}},
			[][2]float64{{b[0] - nx*tick, b[1] - ny*tick}, {b[0] + nx*tick, b[1] + ny*tick}})
		anchor = [2]float64{(a[0]+b[0])/2 + nx*(tick+3*w), (a[1]+b[1])/2 + ny*(tick+3*w)}
	case MarkupArrow:
		tail, head := pt(m.Points[0][0], m.Points[0][1]), pt(m.Points[1][0], m.Points[1][1])
		dx, dy := head[0]-tail[0], head[1]-tail[1]
		l := math.Hypot(dx, dy)
		ux, uy := dx/l, dy/l
		barb := math.Min(5*w, l/2)
		paths = append(paths, [][2]float64{tail, head},
			[][2]float64{{head[0] - barb*(ux*0.87-uy*0.5), head[1] - barb*(uy*0.87+ux*0.5)}, head,
				{head[0] - barb*(ux*0.87+uy*0.5), head[1] - barb*(uy*0.87-ux*0.5)}})
		anchor = [2]float64{tail[0] - ux*3*w, tail[1] - uy*3*w}
	case MarkupPoint:
		p := pt(m.Points[0][0], m.Points[0][1])
		dots = append(dots, p)
		anchor = [2]float64{p[0] + 4*w, p[1] - 2*w}
	}
	for _, p := range paths {
		strokePath(dst, p, w+2, haloColour)
	}
	for _, d := range dots {
		fillCircle(dst, d, 2.5*w+1, haloColour)
	}
	for _, p := range paths {
		strokePath(dst, p, w, markupColour)
	}
	for _, d := range dots {
		fillCircle(dst, d, 2.5*w, markupColour)
	}
	text := strconv.Itoa(m.Number)
	adv := font.MeasureString(label, text).Ceil()
	asc := label.Metrics().Ascent.Ceil()
	x := math.Max(float64(panel.Min.X), math.Min(anchor[0]-float64(adv)/2, float64(panel.Max.X-adv)))
	y := math.Max(float64(panel.Min.Y+asc), math.Min(anchor[1]+float64(asc)/2, float64(panel.Max.Y)))
	drawText(dst, label, text, x, y, markupColour, true)
}

// outside is a point just above and left of a closed shape, for its number.
func outside(points [][2]float64, w float64) [2]float64 {
	minX, minY := math.Inf(1), math.Inf(1)
	for _, p := range points {
		minX, minY = math.Min(minX, p[0]), math.Min(minY, p[1])
	}
	return [2]float64{minX - 4*w, minY - 4*w}
}

func perpendicular(a, b [2]float64) (float64, float64) {
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := math.Hypot(dx, dy)
	if l == 0 {
		return 0, -1
	}
	return -dy / l, dx / l
}

// strokePath draws the polyline p, width w, as one quad per segment and a disc at each joint.
func strokePath(dst *image.NRGBA, p [][2]float64, w float64, c color.Color) {
	for i := 0; i+1 < len(p); i++ {
		a, b := p[i], p[i+1]
		nx, ny := perpendicular(a, b)
		h := w / 2
		fillPolygon(dst, [][2]float64{
			{a[0] + nx*h, a[1] + ny*h}, {b[0] + nx*h, b[1] + ny*h},
			{b[0] - nx*h, b[1] - ny*h}, {a[0] - nx*h, a[1] - ny*h},
		}, c)
	}
	for _, q := range p {
		fillCircle(dst, q, w/2, c)
	}
}

func fillCircle(dst *image.NRGBA, centre [2]float64, r float64, c color.Color) {
	var ring [][2]float64
	n := max(12, int(r*2))
	for i := 0; i < n; i++ {
		a := float64(i) / float64(n) * 2 * math.Pi
		ring = append(ring, [2]float64{centre[0] + r*math.Cos(a), centre[1] + r*math.Sin(a)})
	}
	fillPolygon(dst, ring, c)
}

// fillPolygon fills an anti-aliased polygon, rasterised only over its bounding box.
func fillPolygon(dst *image.NRGBA, p [][2]float64, c color.Color) {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, q := range p {
		minX, minY = math.Min(minX, q[0]), math.Min(minY, q[1])
		maxX, maxY = math.Max(maxX, q[0]), math.Max(maxY, q[1])
	}
	box := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).Intersect(dst.Bounds())
	if box.Empty() {
		return
	}
	z := vector.NewRasterizer(box.Dx(), box.Dy())
	ox, oy := float32(box.Min.X), float32(box.Min.Y)
	z.MoveTo(float32(p[0][0])-ox, float32(p[0][1])-oy)
	for _, q := range p[1:] {
		z.LineTo(float32(q[0])-ox, float32(q[1])-oy)
	}
	z.ClosePath()
	z.Draw(dst, box, image.NewUniform(c), image.Point{})
}

var (
	fontOnce sync.Once
	boldFont *opentype.Font
)

func sheetFace(size float64) font.Face {
	fontOnce.Do(func() {
		f, err := opentype.Parse(gobold.TTF)
		if err != nil {
			panic(err) // the embedded Go font always parses
		}
		boldFont = f
	})
	face, err := opentype.NewFace(boldFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return face
}

// drawText draws s with its baseline at (x, y); halo outlines it in black so it reads on any image.
func drawText(dst *image.NRGBA, face font.Face, s string, x, y float64, c color.Color, halo bool) {
	d := font.Drawer{Dst: dst, Face: face}
	if halo {
		d.Src = image.NewUniform(haloColour)
		for _, o := range [][2]float64{{-2, 0}, {2, 0}, {0, -2}, {0, 2}, {-1.5, -1.5}, {1.5, -1.5}, {-1.5, 1.5}, {1.5, 1.5}} {
			d.Dot = fixed.P(int(x+o[0]), int(y+o[1]))
			d.DrawString(s)
		}
	}
	d.Src = image.NewUniform(c)
	d.Dot = fixed.P(int(x), int(y))
	d.DrawString(s)
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
