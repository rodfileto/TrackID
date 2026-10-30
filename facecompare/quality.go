package facecompare

import (
	"fmt"
	"image"
	"math"

	"github.com/rodfileto/trackid/imageproc"
)

// The numbers here describe the image, never the face: FISWG rules out photo-anthropometry
// (methodology guidelines v2.1, §6.4), so nothing measures distances or proportions between facial
// features for comparison. The interpupillary distance in pixels says how much detail the image
// can hold (Image Factors v1.0, §13.1); the pose estimates warn when two images are not from the
// same viewpoint, which superimposition requires (§6.3.2).

// LowIPD is the interpupillary distance, in pixels, below which the lab warns that fine features
// may not be resolved. It is TrackID's working value, not a FISWG figure.
const LowIPD = 40

// Pose is what the five landmarks say about the image.
type Pose struct {
	IPD  float64 `json:"ipd"`  // pixels between the eye centres
	Roll float64 `json:"roll"` // degrees the eye line is turned, clockwise on screen
	// Yaw is a rough turn of the head, in degrees, from where the nose tip falls between the eyes,
	// assuming it stands half an IPD in front of them; positive when it falls to the image's right.
	Yaw float64 `json:"yaw"`
	// PitchIndex is the nose tip's distance below the eye line over the mouth's: it moves with the
	// head's tilt up or down, but has no unit. Only its difference between images is used.
	PitchIndex float64 `json:"pitchIndex"`
}

// PoseOf reads the pose from landmarks.
func PoseOf(l Landmarks) Pose {
	e0, e1, nose := l.Points[0], l.Points[1], l.Points[2]
	mouth := [2]float64{(l.Points[3][0] + l.Points[4][0]) / 2, (l.Points[3][1] + l.Points[4][1]) / 2}
	dx, dy := e1[0]-e0[0], e1[1]-e0[1]
	ipd := math.Hypot(dx, dy)
	p := Pose{IPD: ipd, Roll: math.Atan2(dy, dx) * 180 / math.Pi}
	if ipd < 1 {
		return p
	}
	ux, uy := dx/ipd, dy/ipd
	vx, vy := -uy, ux // perpendicular, pointing down the face
	mx, my := (e0[0]+e1[0])/2, (e0[1]+e1[1])/2
	along := (nose[0]-mx)*ux + (nose[1]-my)*uy
	p.Yaw = math.Asin(math.Max(-1, math.Min(1, along/(ipd/2)))) * 180 / math.Pi
	down := (nose[0]-mx)*vx + (nose[1]-my)*vy
	mouthDown := (mouth[0]-mx)*vx + (mouth[1]-my)*vy
	if math.Abs(mouthDown) > 1 {
		p.PitchIndex = down / mouthDown
	}
	return p
}

// Metrics are an image's measurements for the analysis step.
type Metrics struct {
	Pose *Pose `json:"pose,omitempty"`
	// Region measured: the face box, or the whole image.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Sharpness is the variance of the Laplacian of the luminance: higher is sharper, but it
	// depends on content and size, so it compares images of the same face region only.
	Sharpness float64 `json:"sharpness"`
	// Fractions of pixels at the ends of the tonal range: lost shadow and highlight detail.
	ClippedDark   float64 `json:"clippedDark"`
	ClippedBright float64 `json:"clippedBright"`
	MeanLuminance float64 `json:"meanLuminance"`
}

// Measure measures img inside box (the whole image when nil), with the pose when landmarks are
// given. img and box are in original pixels.
func Measure(img image.Image, box *imageproc.Rect, l *Landmarks) Metrics {
	b := img.Bounds()
	region := b
	if box != nil {
		region = image.Rect(b.Min.X+int(box.X1), b.Min.Y+int(box.Y1), b.Min.X+int(math.Ceil(box.X2)), b.Min.Y+int(math.Ceil(box.Y2))).Intersect(b)
	}
	w, h := region.Dx(), region.Dy()
	m := Metrics{Width: w, Height: h}
	if l != nil {
		p := PoseOf(*l)
		m.Pose = &p
	}
	if w < 3 || h < 3 {
		return m
	}
	lum := make([]float64, w*h)
	var sum float64
	dark, bright := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(region.Min.X+x, region.Min.Y+y).RGBA()
			v := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 257
			lum[y*w+x] = v
			sum += v
			if v <= 2 {
				dark++
			} else if v >= 253 {
				bright++
			}
		}
	}
	n := float64(w * h)
	m.MeanLuminance = sum / n
	m.ClippedDark, m.ClippedBright = float64(dark)/n, float64(bright)/n
	var ls, ls2 float64
	count := 0
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			lap := lum[i-1] + lum[i+1] + lum[i-w] + lum[i+w] - 4*lum[i]
			ls += lap
			ls2 += lap * lap
			count++
		}
	}
	mean := ls / float64(count)
	m.Sharpness = ls2/float64(count) - mean*mean
	return m
}

// PoseWarnings compares the two images' poses for superimposition (methodology guidelines v2.1,
// §6.3.2: same viewpoint) and warns on low resolution. Roll is not compared: alignment removes it,
// and it does not change how features look (Image Factors v1.0, §11.1).
func PoseWarnings(q, k *Landmarks) []Warning {
	var out []Warning
	poses := map[string]*Pose{}
	for _, side := range []struct {
		img string
		l   *Landmarks
	}{{"Q", q}, {"K", k}} {
		img, l := side.img, side.l
		if l == nil {
			continue
		}
		p := PoseOf(*l)
		poses[img] = &p
		if p.IPD < LowIPD {
			out = append(out, Warning{"LOW_IPD", fmt.Sprintf("Imagem %s: distância interpupilar de %.0f pixels; detalhes finos podem não estar resolvidos.", imageName(img), p.IPD), img})
		}
	}
	if q, k := poses["Q"], poses["K"]; q != nil && k != nil {
		if d := math.Abs(q.Yaw - k.Yaw); d > 10 {
			out = append(out, Warning{"POSE_YAW", fmt.Sprintf("As faces estão giradas de forma diferente (cerca de %.0f° de diferença, estimativa grosseira): a superposição exige o mesmo ponto de vista.", d), ""})
		}
		if d := math.Abs(q.PitchIndex - k.PitchIndex); q.PitchIndex != 0 && k.PitchIndex != 0 && d > 0.1 {
			out = append(out, Warning{"POSE_PITCH", "As faces parecem inclinadas de forma diferente (para cima ou para baixo): a superposição exige o mesmo ponto de vista.", ""})
		}
	}
	return out
}
