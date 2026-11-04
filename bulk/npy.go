package bulk

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The files exchanged with the GPU matcher (tools/bulk-match) and the vector shards are NumPy
// .npy files (format version 1.0), little-endian, C order, so the matcher memory-maps them with
// numpy.load and Go needs no Python. Only the three dtypes used here are handled.
const (
	dtypeInt64   = "<i8"
	dtypeFloat32 = "<f4"
	dtypeFloat16 = "<f2"
)

var npyMagic = []byte("\x93NUMPY")

// npyHeader is an .npy file's dtype and shape.
type npyHeader struct {
	dtype string
	shape []int
}

func (h npyHeader) rows() int {
	if len(h.shape) == 0 {
		return 1
	}
	return h.shape[0]
}

func (h npyHeader) cols() int {
	if len(h.shape) < 2 {
		return 1
	}
	return h.shape[1]
}

func writeNpyHeader(w io.Writer, dtype string, shape []int) error {
	dims := make([]string, len(shape))
	for i, d := range shape {
		dims[i] = strconv.Itoa(d)
	}
	shapeText := strings.Join(dims, ", ")
	if len(shape) == 1 {
		shapeText += ","
	}
	dict := fmt.Sprintf("{'descr': '%s', 'fortran_order': False, 'shape': (%s), }", dtype, shapeText)
	// magic(6) + version(2) + header length(2) + dict + padding + "\n", a multiple of 64.
	total := 10 + len(dict) + 1
	pad := (64 - total%64) % 64
	header := dict + strings.Repeat(" ", pad) + "\n"
	if _, err := w.Write(npyMagic); err != nil {
		return err
	}
	if _, err := w.Write([]byte{1, 0}); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(len(header))); err != nil {
		return err
	}
	_, err := io.WriteString(w, header)
	return err
}

var (
	descrPattern = regexp.MustCompile(`'descr':\s*'([^']+)'`)
	orderPattern = regexp.MustCompile(`'fortran_order':\s*(True|False)`)
	shapePattern = regexp.MustCompile(`'shape':\s*\(([^)]*)\)`)
)

func readNpyHeader(r io.Reader) (npyHeader, error) {
	prefix := make([]byte, 8)
	if _, err := io.ReadFull(r, prefix); err != nil {
		return npyHeader{}, err
	}
	if string(prefix[:6]) != string(npyMagic) {
		return npyHeader{}, fmt.Errorf("not an .npy file")
	}
	var length int
	switch prefix[6] {
	case 1:
		var n uint16
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return npyHeader{}, err
		}
		length = int(n)
	case 2, 3:
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return npyHeader{}, err
		}
		length = int(n)
	default:
		return npyHeader{}, fmt.Errorf(".npy version %d not supported", prefix[6])
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(r, raw); err != nil {
		return npyHeader{}, err
	}
	text := string(raw)
	descr := descrPattern.FindStringSubmatch(text)
	order := orderPattern.FindStringSubmatch(text)
	shape := shapePattern.FindStringSubmatch(text)
	if descr == nil || order == nil || shape == nil {
		return npyHeader{}, fmt.Errorf(".npy header %q not understood", text)
	}
	if order[1] == "True" {
		return npyHeader{}, fmt.Errorf(".npy in Fortran order not supported")
	}
	h := npyHeader{dtype: descr[1]}
	if h.dtype == "|i8" {
		h.dtype = dtypeInt64
	}
	for _, part := range strings.Split(shape[1], ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return npyHeader{}, fmt.Errorf(".npy shape %q: %w", shape[1], err)
		}
		h.shape = append(h.shape, n)
	}
	return h, nil
}

// writeFileAtomic writes path through a temporary file renamed into place, so a reader (or a
// rerun after a crash) never sees half a file.
func writeFileAtomic(path string, write func(w io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	bw := bufio.NewWriterSize(tmp, 1<<20)
	if err := write(bw); err != nil {
		tmp.Close()
		return err
	}
	if err := bw.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// WriteInt64s writes a 1-D int64 .npy file.
func WriteInt64s(path string, values []int64) error {
	return writeFileAtomic(path, func(w io.Writer) error {
		if err := writeNpyHeader(w, dtypeInt64, []int{len(values)}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, values)
	})
}

// WriteFloat32s writes a 1-D float32 .npy file.
func WriteFloat32s(path string, values []float32) error {
	return writeFileAtomic(path, func(w io.Writer) error {
		if err := writeNpyHeader(w, dtypeFloat32, []int{len(values)}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, values)
	})
}

// WriteMatrix writes rows of equal length as a 2-D .npy file of dtype float32, or float16 when
// half is set (the vector shards).
func WriteMatrix(path string, rows [][]float32, dim int, half bool) error {
	dtype := dtypeFloat32
	if half {
		dtype = dtypeFloat16
	}
	return writeFileAtomic(path, func(w io.Writer) error {
		if err := writeNpyHeader(w, dtype, []int{len(rows), dim}); err != nil {
			return err
		}
		size := 4
		if half {
			size = 2
		}
		buf := make([]byte, dim*size)
		for i, row := range rows {
			if len(row) != dim {
				return fmt.Errorf("row %d has %d values, want %d", i, len(row), dim)
			}
			for j, v := range row {
				if half {
					binary.LittleEndian.PutUint16(buf[j*2:], float32ToHalf(v))
				} else {
					binary.LittleEndian.PutUint32(buf[j*4:], math.Float32bits(v))
				}
			}
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReadInt64s reads a 1-D int64 .npy file.
func ReadInt64s(path string) ([]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	h, err := readNpyHeader(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if h.dtype != dtypeInt64 || len(h.shape) != 1 {
		return nil, fmt.Errorf("%s: want a 1-D %s array, got %s %v", path, dtypeInt64, h.dtype, h.shape)
	}
	out := make([]int64, h.rows())
	if err := binary.Read(r, binary.LittleEndian, out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// ReadFloat32s reads a 1-D float32 .npy file.
func ReadFloat32s(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	h, err := readNpyHeader(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if h.dtype != dtypeFloat32 || len(h.shape) != 1 {
		return nil, fmt.Errorf("%s: want a 1-D %s array, got %s %v", path, dtypeFloat32, h.dtype, h.shape)
	}
	out := make([]float32, h.rows())
	if err := binary.Read(r, binary.LittleEndian, out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// ReadMatrix reads a 2-D float16 or float32 .npy file as float32 rows.
func ReadMatrix(path string) ([][]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	h, err := readNpyHeader(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(h.shape) != 2 || (h.dtype != dtypeFloat16 && h.dtype != dtypeFloat32) {
		return nil, fmt.Errorf("%s: want a 2-D float array, got %s %v", path, h.dtype, h.shape)
	}
	size := 4
	if h.dtype == dtypeFloat16 {
		size = 2
	}
	buf := make([]byte, h.cols()*size)
	out := make([][]float32, h.rows())
	for i := range out {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("%s: row %d: %w", path, i, err)
		}
		row := make([]float32, h.cols())
		for j := range row {
			if size == 2 {
				row[j] = halfToFloat32(binary.LittleEndian.Uint16(buf[j*2:]))
			} else {
				row[j] = math.Float32frombits(binary.LittleEndian.Uint32(buf[j*4:]))
			}
		}
		out[i] = row
	}
	return out, nil
}

// float32ToHalf converts to IEEE 754 binary16, rounding to nearest even. Embeddings are unit
// vectors, so their components never overflow; subnormals are kept.
func float32ToHalf(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := uint16(bits>>16) & 0x8000
	exp := int((bits>>23)&0xff) - 127 + 15
	mant := bits & 0x7fffff
	switch {
	case (bits>>23)&0xff == 0xff: // Inf or NaN
		if mant != 0 {
			return sign | 0x7e00
		}
		return sign | 0x7c00
	case exp >= 0x1f:
		return sign | 0x7c00
	case exp <= 0:
		if exp < -10 {
			return sign
		}
		mant |= 0x800000
		shift := uint32(14 - exp)
		half := mant >> shift
		rem := mant & (1<<shift - 1)
		mid := uint32(1) << (shift - 1)
		if rem > mid || (rem == mid && half&1 == 1) {
			half++
		}
		return sign | uint16(half)
	}
	half := uint32(exp)<<10 | mant>>13
	rem := mant & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++ // may carry into the exponent, which is the correct rounding
	}
	return sign | uint16(half)
}

func halfToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1f
	mant := uint32(h & 0x3ff)
	switch {
	case exp == 0:
		if mant == 0 {
			return math.Float32frombits(sign)
		}
		// subnormal: normalize
		e := uint32(127 - 15 + 1)
		for mant&0x400 == 0 {
			mant <<= 1
			e--
		}
		mant &= 0x3ff
		return math.Float32frombits(sign | e<<23 | mant<<13)
	case exp == 0x1f:
		return math.Float32frombits(sign | 0xff<<23 | mant<<13)
	}
	return math.Float32frombits(sign | (exp-15+127)<<23 | mant<<13)
}
