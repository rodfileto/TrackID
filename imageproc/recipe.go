// Package imageproc renders a fingerprint image from an original and a recipe: the ordered
// processing steps and their settings (docs/fingerprint-image-processing-plan.md). The original is
// never changed; the same recipe on the same original always gives the same image, at OutputPPI,
// and Describe writes the steps out in Portuguese for the record.
package imageproc

import (
	"errors"
	"fmt"
	"math"
)

// RecipeVersion is the version of the recipe format and of the rendering it names. A change to
// what any step does to the pixels is a new version.
const RecipeVersion = 1

// OutputPPI is the resolution every render is resampled to: the resolution of ten-print cards,
// of trackid-sim's prints and of SourceAFIS's default.
const OutputPPI = 500

// DefaultRidgeDistanceMM is the mean distance between adjacent ridge centres assumed when the
// source resolution is estimated from the ridges. It varies between people (sex, age), fingers
// and regions of a finger, roughly 0.4 to 0.5 mm, so an estimated resolution carries that error.
const DefaultRidgeDistanceMM = 0.45

// How the source resolution was obtained.
const (
	PPICalibrated = "CALIBRATED" // a line drawn along a scale in the image, of known length
	PPIFile       = "FILE"       // the file's metadata (a scanner's resolution), confirmed
	PPIEstimated  = "ESTIMATED"  // from the ridge period and DefaultRidgeDistanceMM
	PPIAssumed    = "ASSUMED"    // OutputPPI, with nothing to support it
)

// Channels: how colour becomes grey.
const (
	ChannelLuminance = "luminance" // ITU-R BT.601 weights
	ChannelRed       = "red"
	ChannelGreen     = "green"
	ChannelBlue      = "blue"
)

// Interpolations for the geometric transform.
const (
	InterpolationNearest  = "nearest"
	InterpolationBilinear = "bilinear"
	InterpolationBicubic  = "bicubic" // Catmull-Rom
)

// Recipe is what a render does, in a fixed order (see Render). Steps left nil are off.
type Recipe struct {
	Version       int      `json:"version"`
	Source        Source   `json:"source"`
	Channel       string   `json:"channel"`
	Crop          *Rect    `json:"crop,omitempty"`
	Rotate        float64  `json:"rotate"` // degrees, clockwise
	Mirror        bool     `json:"mirror"`
	Interpolation string   `json:"interpolation"`
	Flatten       *Flatten `json:"flatten,omitempty"`
	Levels        *Levels  `json:"levels,omitempty"`
	CLAHE         *CLAHE   `json:"clahe,omitempty"`
	Gabor         *Gabor   `json:"gabor,omitempty"`
	Invert        bool     `json:"invert"`
}

// Source is the original's resolution and where it came from.
type Source struct {
	PPI    float64 `json:"ppi"`
	Method string  `json:"method"`
	// Calibration is the line drawn along the scale, in original pixels, and its length in mm
	// (PPICalibrated). The resolution is computed from it, not taken from PPI.
	Calibration *Calibration `json:"calibration,omitempty"`
	// RidgePeriodPX and RidgeDistanceMM are the estimate's inputs (PPIEstimated).
	RidgePeriodPX   float64 `json:"ridgePeriodPx,omitempty"`
	RidgeDistanceMM float64 `json:"ridgeDistanceMm,omitempty"`
}

// Calibration is a segment of known length on a scale in the image.
type Calibration struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
	MM float64 `json:"mm"`
}

// LengthPX is the segment's length in pixels.
func (c Calibration) LengthPX() float64 { return math.Hypot(c.X2-c.X1, c.Y2-c.Y1) }

// Rect is a region of the original, in its pixels (x2, y2 exclusive).
type Rect struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

// Flatten corrects uneven lighting: the image divided by a Gaussian-blurred copy of itself.
type Flatten struct {
	SigmaMM float64 `json:"sigmaMm"`
}

// Levels maps input black..white onto output black..white through a gamma, as Photoshop's.
type Levels struct {
	InBlack  float64 `json:"inBlack"`
	InWhite  float64 `json:"inWhite"`
	Gamma    float64 `json:"gamma"`
	OutBlack float64 `json:"outBlack"`
	OutWhite float64 `json:"outWhite"`
}

// CLAHE is contrast-limited adaptive histogram equalisation over a grid of tiles.
type CLAHE struct {
	Tiles     int     `json:"tiles"`
	ClipLimit float64 `json:"clipLimit"`
}

// Gabor is ridge enhancement by Gabor filters tuned to the local ridge orientation and the
// image's ridge frequency (Hong, Wan & Jain, 1998).
type Gabor struct {
	Orientations int `json:"orientations"`
}

// ValidationError is a recipe the renderer refuses.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// Normalize fills defaults and checks the recipe against an original of width x height. It
// computes a calibrated resolution from its line.
func (r *Recipe) Normalize(width, height int) error {
	if r.Version == 0 {
		r.Version = RecipeVersion
	}
	if r.Version != RecipeVersion {
		return ValidationError(fmt.Sprintf("recipe version %d is not supported (this is %d)", r.Version, RecipeVersion))
	}
	if r.Channel == "" {
		r.Channel = ChannelLuminance
	}
	switch r.Channel {
	case ChannelLuminance, ChannelRed, ChannelGreen, ChannelBlue:
	default:
		return ValidationError("unknown channel " + r.Channel)
	}
	if r.Interpolation == "" {
		r.Interpolation = InterpolationBicubic
	}
	switch r.Interpolation {
	case InterpolationNearest, InterpolationBilinear, InterpolationBicubic:
	default:
		return ValidationError("unknown interpolation " + r.Interpolation)
	}
	if err := r.Source.normalize(); err != nil {
		return err
	}
	if r.Crop != nil {
		c := r.Crop
		c.X1, c.X2 = math.Max(0, math.Min(c.X1, c.X2)), math.Min(float64(width), math.Max(c.X1, c.X2))
		c.Y1, c.Y2 = math.Max(0, math.Min(c.Y1, c.Y2)), math.Min(float64(height), math.Max(c.Y1, c.Y2))
		if c.X2-c.X1 < 8 || c.Y2-c.Y1 < 8 {
			return ValidationError("the crop is smaller than 8 pixels")
		}
	}
	if math.IsNaN(r.Rotate) || math.Abs(r.Rotate) > 360 {
		return ValidationError("rotation must be between -360 and 360 degrees")
	}
	if f := r.Flatten; f != nil && (f.SigmaMM < 0.2 || f.SigmaMM > 20) {
		return ValidationError("the lighting correction's blur must be between 0.2 and 20 mm")
	}
	if l := r.Levels; l != nil {
		if l.InBlack < 0 || l.InWhite > 255 || l.InWhite-l.InBlack < 1 {
			return ValidationError("levels: input black must be below input white, within 0-255")
		}
		if l.Gamma < 0.1 || l.Gamma > 9.99 {
			return ValidationError("levels: gamma must be between 0.10 and 9.99")
		}
		if l.OutBlack < 0 || l.OutWhite > 255 || l.OutBlack > l.OutWhite {
			return ValidationError("levels: output black must not exceed output white, within 0-255")
		}
	}
	if c := r.CLAHE; c != nil {
		if c.Tiles == 0 {
			c.Tiles = 8
		}
		if c.ClipLimit == 0 {
			c.ClipLimit = 2
		}
		if c.Tiles < 2 || c.Tiles > 32 || c.ClipLimit < 1 || c.ClipLimit > 40 {
			return ValidationError("CLAHE: 2-32 tiles and a clip limit of 1-40")
		}
	}
	if g := r.Gabor; g != nil {
		if g.Orientations == 0 {
			g.Orientations = 16
		}
		if g.Orientations < 4 || g.Orientations > 36 {
			return ValidationError("Gabor: 4-36 orientations")
		}
	}
	return nil
}

func (s *Source) normalize() error {
	switch s.Method {
	case PPICalibrated:
		c := s.Calibration
		if c == nil || c.MM <= 0 || c.LengthPX() < 10 {
			return ValidationError("calibration needs a line of at least 10 pixels and its length in mm")
		}
		s.PPI = c.LengthPX() / c.MM * 25.4
	case PPIEstimated:
		if s.RidgePeriodPX <= 0 {
			return ValidationError("an estimated resolution needs the ridge period")
		}
		if s.RidgeDistanceMM == 0 {
			s.RidgeDistanceMM = DefaultRidgeDistanceMM
		}
		if s.RidgeDistanceMM < 0.2 || s.RidgeDistanceMM > 1 {
			return ValidationError("the assumed ridge distance must be between 0.2 and 1 mm")
		}
		s.PPI = s.RidgePeriodPX / s.RidgeDistanceMM * 25.4
	case PPIFile:
		if s.PPI <= 0 {
			return ValidationError("the file gives no resolution")
		}
	case PPIAssumed, "":
		s.Method, s.PPI = PPIAssumed, OutputPPI
	default:
		return ValidationError("unknown resolution method " + s.Method)
	}
	if s.PPI < 50 || s.PPI > 20000 {
		return ValidationError(fmt.Sprintf("a source resolution of %.0f ppi is not plausible", s.PPI))
	}
	return nil
}

// FingerPhoto is the preset for a photo of a finger: green channel (ridges contrast best in it on
// skin), lighting correction, CLAHE, Gabor enhancement, inversion so ridges are dark, and the
// mirror that turns the finger as the camera saw it into the orientation of a contact print.
func FingerPhoto(source Source, crop *Rect) Recipe {
	return Recipe{
		Version:       RecipeVersion,
		Source:        source,
		Channel:       ChannelGreen,
		Crop:          crop,
		Mirror:        true,
		Interpolation: InterpolationBicubic,
		Flatten:       &Flatten{SigmaMM: 2},
		CLAHE:         &CLAHE{Tiles: 8, ClipLimit: 2},
		Gabor:         &Gabor{Orientations: 16},
		Invert:        true,
	}
}

var errEmpty = errors.New("imageproc: empty image")
