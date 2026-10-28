package imageproc

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"math"
)

// withPHYs inserts a pHYs chunk stating ppi into an encoded PNG, right after IHDR, so the file
// carries its resolution for any tool that opens it.
func withPHYs(encoded []byte, ppi float64) []byte {
	const ihdrEnd = 8 + 8 + 13 + 4 // signature, IHDR length+type, IHDR data, CRC
	if len(encoded) < ihdrEnd || !bytes.Equal(encoded[12:16], []byte("IHDR")) {
		return encoded
	}
	ppm := uint32(math.Round(ppi / 0.0254))
	chunk := make([]byte, 0, 21)
	chunk = binary.BigEndian.AppendUint32(chunk, 9)
	chunk = append(chunk, "pHYs"...)
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = binary.BigEndian.AppendUint32(chunk, ppm)
	chunk = append(chunk, 1) // unit: metre
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := make([]byte, 0, len(encoded)+len(chunk))
	out = append(out, encoded[:ihdrEnd]...)
	out = append(out, chunk...)
	return append(out, encoded[ihdrEnd:]...)
}
