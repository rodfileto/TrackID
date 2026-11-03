package imageproc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg" // decoders for image.Decode
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

// Original is a decoded original with what the record needs about it.
type Original struct {
	Image  image.Image
	Format string // "jpeg", "png", "bmp", "tiff"
	SHA256 string // of the file as received
	Width  int    // after Orientation is applied
	Height int
	// Orientation is the EXIF orientation (1-8) applied on decoding, as browsers apply it when
	// they show the photo; 1 or 0 when there was none to apply.
	Orientation int
	// FilePPI is the resolution the file's metadata states (JFIF, PNG pHYs, BMP header); 0
	// when it states none.
	FilePPI float64
}

// Decode reads an original. Pixel coordinates in a recipe (crop, calibration) refer to the
// image after its EXIF orientation is applied.
func Decode(data []byte) (Original, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Original{}, fmt.Errorf("imageproc: not a readable image: %w", err)
	}
	sum := sha256.Sum256(data)
	o := Original{Format: format, SHA256: hex.EncodeToString(sum[:])}
	if format == "jpeg" {
		o.Orientation = jpegOrientation(data)
		o.FilePPI = jfifPPI(data)
	}
	if format == "png" {
		o.FilePPI = pngPPI(data)
	}
	if format == "bmp" {
		o.FilePPI = bmpPPI(data)
	}
	o.Image = orient(img, o.Orientation)
	b := o.Image.Bounds()
	o.Width, o.Height = b.Dx(), b.Dy()
	if o.Width == 0 || o.Height == 0 {
		return Original{}, errEmpty
	}
	return o, nil
}

// orient applies an EXIF orientation: 2 mirror, 3 rotate 180, 4 flip, 5 transpose,
// 6 rotate 90 clockwise, 7 transverse, 8 rotate 90 counter-clockwise.
func orient(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	src := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	ow, oh := w, h
	if o >= 5 {
		ow, oh = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			si, di := src.PixOffset(x, y), dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// jpegSegments calls fn with each marker and its payload until the image data starts.
func jpegSegments(data []byte, fn func(marker byte, payload []byte) bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan, end of image
			return
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return
		}
		if !fn(marker, data[i+4:i+2+size]) {
			return
		}
		i += 2 + size
	}
}

// jpegOrientation is the EXIF Orientation tag (0x0112) of IFD0, or 0.
func jpegOrientation(data []byte) int {
	orientation := 0
	jpegSegments(data, func(marker byte, p []byte) bool {
		if marker != 0xE1 || len(p) < 14 || string(p[:6]) != "Exif\x00\x00" {
			return true
		}
		tiff := p[6:]
		var order binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			order = binary.LittleEndian
		case "MM":
			order = binary.BigEndian
		default:
			return false
		}
		ifd := int(order.Uint32(tiff[4:]))
		if ifd+2 > len(tiff) {
			return false
		}
		entries := int(order.Uint16(tiff[ifd:]))
		for e := 0; e < entries; e++ {
			at := ifd + 2 + 12*e
			if at+12 > len(tiff) {
				break
			}
			if order.Uint16(tiff[at:]) == 0x0112 {
				orientation = int(order.Uint16(tiff[at+8:]))
				break
			}
		}
		return false
	})
	return orientation
}

// jfifPPI is the JFIF APP0 density in pixels per inch (units 1) or per cm (units 2); 0 when the
// file gives only an aspect ratio (units 0) or none.
func jfifPPI(data []byte) float64 {
	ppi := 0.0
	jpegSegments(data, func(marker byte, p []byte) bool {
		if marker != 0xE0 || len(p) < 12 || string(p[:5]) != "JFIF\x00" {
			return true
		}
		x := float64(binary.BigEndian.Uint16(p[8:]))
		switch p[7] {
		case 1:
			ppi = x
		case 2:
			ppi = x * 2.54
		}
		return false
	})
	return ppi
}

// pngPPI reads the pHYs chunk (pixels per metre, unit 1).
func pngPPI(data []byte) float64 {
	for i := 8; i+12 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[i:]))
		kind := string(data[i+4 : i+8])
		if kind == "pHYs" && size >= 9 && i+8+9 <= len(data) && data[i+16] == 1 {
			return float64(binary.BigEndian.Uint32(data[i+8:])) * 0.0254
		}
		if kind == "IDAT" || size < 0 {
			return 0
		}
		i += 12 + size
	}
	return 0
}

// bmpPPI reads the header's horizontal pixels per metre.
func bmpPPI(data []byte) float64 {
	if len(data) < 42 {
		return 0
	}
	return float64(int32(binary.LittleEndian.Uint32(data[38:]))) * 0.0254
}
