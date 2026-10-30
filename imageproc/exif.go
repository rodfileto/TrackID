package imageproc

import (
	"encoding/binary"
	"strings"
)

// Exif is what a JPEG's EXIF says about its capture, for the analysis of a face image (FISWG's
// ACE-V guideline for one-to-one examinations, §5.1.2: capture timeframe, alterations, metadata).
// Fields the file does not carry are empty. Metadata can be edited, so it is reported, not trusted.
type Exif struct {
	DateTimeOriginal string `json:"dateTimeOriginal,omitempty"` // when the photo was taken
	DateTime         string `json:"dateTime,omitempty"`         // when the file was last changed
	Make             string `json:"make,omitempty"`
	Model            string `json:"model,omitempty"`
	Software         string `json:"software,omitempty"` // an editor's name here suggests post-processing
}

// ReadExif reads IFD0 and the EXIF sub-IFD of a JPEG's APP1 segment; zero for other files.
func ReadExif(data []byte) Exif {
	var x Exif
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
		exifIFD := 0
		readIFD(tiff, order, int(order.Uint32(tiff[4:])), func(tag uint16, value func() string, offset uint32) {
			switch tag {
			case 0x010F:
				x.Make = value()
			case 0x0110:
				x.Model = value()
			case 0x0131:
				x.Software = value()
			case 0x0132:
				x.DateTime = value()
			case 0x8769:
				exifIFD = int(offset)
			}
		})
		if exifIFD > 0 {
			readIFD(tiff, order, exifIFD, func(tag uint16, value func() string, _ uint32) {
				if tag == 0x9003 {
					x.DateTimeOriginal = value()
				}
			})
		}
		return false
	})
	return x
}

// readIFD calls fn with each entry of the IFD at offset: its tag, its value read as ASCII, and its
// value/offset field as a number.
func readIFD(tiff []byte, order binary.ByteOrder, offset int, fn func(tag uint16, value func() string, offset uint32)) {
	if offset <= 0 || offset+2 > len(tiff) {
		return
	}
	entries := int(order.Uint16(tiff[offset:]))
	for e := 0; e < entries; e++ {
		at := offset + 2 + 12*e
		if at+12 > len(tiff) {
			return
		}
		tag := order.Uint16(tiff[at:])
		kind := order.Uint16(tiff[at+2:])
		count := int(order.Uint32(tiff[at+4:]))
		field := order.Uint32(tiff[at+8:])
		value := func() string {
			if kind != 2 || count <= 0 || count > 256 {
				return ""
			}
			var raw []byte
			if count <= 4 {
				raw = tiff[at+8 : at+8+count]
			} else if int(field)+count <= len(tiff) {
				raw = tiff[field : int(field)+count]
			}
			return strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
		}
		fn(tag, value, field)
	}
}
