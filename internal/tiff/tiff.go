// Package tiff reads single-band classic TIFF float/int rasters (no BigTIFF, no GDAL).
package tiff

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	tagImageWidth      = 256
	tagImageLength     = 257
	tagBitsPerSample   = 258
	tagCompression     = 259
	tagPhotometric     = 262
	tagStripOffsets    = 273
	tagSamplesPerPixel = 277
	tagRowsPerStrip    = 278
	tagStripByteCounts = 279
	tagSampleFormat    = 339

	compNone = 1
	sfUint   = 1
	sfInt    = 2
	sfFloat  = 3
)

// PeekHW returns (height, width) from the first IFD without decoding pixels.
func PeekHW(path string) (h, w int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	bo, ifd, err := readHeader(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", base(path), err)
	}
	entries, err := readIFD(f, bo, ifd)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %w", base(path), err)
	}
	width, okW := entryUint(entries, tagImageWidth, bo)
	height, okH := entryUint(entries, tagImageLength, bo)
	if !okW || !okH {
		return 0, 0, fmt.Errorf("%s: TIFF missing ImageWidth/Length", base(path))
	}
	return int(height), int(width), nil
}

// ReadFloat32 loads a single-band uncompressed TIFF as float32 (H, W).
func ReadFloat32(path string) (h, w int, data []float32, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, nil, err
	}
	defer f.Close()
	bo, ifdOff, err := readHeader(f)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%s: %w", base(path), err)
	}
	entries, err := readIFD(f, bo, ifdOff)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%s: %w", base(path), err)
	}
	width, okW := entryUint(entries, tagImageWidth, bo)
	height, okH := entryUint(entries, tagImageLength, bo)
	if !okW || !okH {
		return 0, 0, nil, fmt.Errorf("%s: TIFF missing ImageWidth/Length", base(path))
	}
	comp, _ := entryUint(entries, tagCompression, bo)
	if comp == 0 {
		comp = compNone
	}
	if comp != compNone {
		return 0, 0, nil, fmt.Errorf("%s: compressed TIFF unsupported (compression=%d)", base(path), comp)
	}
	spp, _ := entryUint(entries, tagSamplesPerPixel, bo)
	if spp == 0 {
		spp = 1
	}
	if spp != 1 {
		return 0, 0, nil, fmt.Errorf("%s: expected 1 sample/pixel, got %d", base(path), spp)
	}
	bps, err := entryUintOrFirst(entries, tagBitsPerSample, bo)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%s: %w", base(path), err)
	}
	sf, _ := entryUint(entries, tagSampleFormat, bo)
	if sf == 0 {
		sf = sfUint
	}
	offsets, err := entryUintSlice(f, entries, tagStripOffsets, bo)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%s: strip offsets: %w", base(path), err)
	}
	counts, err := entryUintSlice(f, entries, tagStripByteCounts, bo)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%s: strip byte counts: %w", base(path), err)
	}
	if len(offsets) != len(counts) || len(offsets) == 0 {
		return 0, 0, nil, fmt.Errorf("%s: bad strip tags", base(path))
	}
	rowsPerStrip, _ := entryUint(entries, tagRowsPerStrip, bo)
	if rowsPerStrip == 0 {
		rowsPerStrip = height
	}

	elem := int(bps / 8)
	if bps%8 != 0 || (elem != 1 && elem != 2 && elem != 4) {
		return 0, 0, nil, fmt.Errorf("%s: unsupported BitsPerSample %d", base(path), bps)
	}
	out := make([]float32, int(height*width))
	row := 0
	for i := range offsets {
		n := int(counts[i])
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, int64(offsets[i])); err != nil && err != io.EOF {
			return 0, 0, nil, fmt.Errorf("%s: read strip: %w", base(path), err)
		}
		rows := int(rowsPerStrip)
		if row+rows > int(height) {
			rows = int(height) - row
		}
		need := rows * int(width) * elem
		if need > len(buf) {
			need = len(buf) - len(buf)%elem
		}
		decodeStrip(buf[:need], out[row*int(width):], int(width), rows, elem, sf, bo)
		row += rows
	}
	return int(height), int(width), out, nil
}

func decodeStrip(buf []byte, dest []float32, width, rows, elem int, sf uint32, bo binary.ByteOrder) {
	nPix := rows * width
	if nPix > len(dest) {
		nPix = len(dest)
	}
	for i := 0; i < nPix; i++ {
		off := i * elem
		if off+elem > len(buf) {
			break
		}
		switch {
		case elem == 4 && sf == sfFloat:
			bits := bo.Uint32(buf[off:])
			dest[i] = math.Float32frombits(bits)
		case elem == 4 && sf == sfUint:
			dest[i] = float32(bo.Uint32(buf[off:]))
		case elem == 4 && sf == sfInt:
			dest[i] = float32(int32(bo.Uint32(buf[off:])))
		case elem == 2 && sf == sfUint:
			dest[i] = float32(bo.Uint16(buf[off:]))
		case elem == 2 && sf == sfInt:
			dest[i] = float32(int16(bo.Uint16(buf[off:])))
		case elem == 1 && sf == sfUint:
			dest[i] = float32(buf[off])
		case elem == 1 && sf == sfInt:
			dest[i] = float32(int8(buf[off]))
		default:
			dest[i] = float32(math.NaN())
		}
	}
}

type ifdEntry struct {
	Tag         uint16
	Type        uint16
	Count       uint32
	ValueOffset uint32
	Raw         [4]byte
}

func readHeader(f *os.File) (binary.ByteOrder, uint32, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, 0, fmt.Errorf("truncated TIFF header")
	}
	var bo binary.ByteOrder
	switch string(hdr[0:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, 0, fmt.Errorf("not a TIFF")
	}
	magic := bo.Uint16(hdr[2:4])
	if magic == 43 {
		return nil, 0, fmt.Errorf("BigTIFF is unsupported")
	}
	if magic != 42 {
		return nil, 0, fmt.Errorf("not a TIFF")
	}
	return bo, bo.Uint32(hdr[4:8]), nil
}

func readIFD(f *os.File, bo binary.ByteOrder, off uint32) ([]ifdEntry, error) {
	if _, err := f.Seek(int64(off), io.SeekStart); err != nil {
		return nil, err
	}
	var nbuf [2]byte
	if _, err := io.ReadFull(f, nbuf[:]); err != nil {
		return nil, fmt.Errorf("truncated IFD")
	}
	n := int(bo.Uint16(nbuf[:]))
	entries := make([]ifdEntry, 0, n)
	for i := 0; i < n; i++ {
		var raw [12]byte
		if _, err := io.ReadFull(f, raw[:]); err != nil {
			return nil, err
		}
		e := ifdEntry{
			Tag:         bo.Uint16(raw[0:2]),
			Type:        bo.Uint16(raw[2:4]),
			Count:       bo.Uint32(raw[4:8]),
			ValueOffset: bo.Uint32(raw[8:12]),
		}
		copy(e.Raw[:], raw[8:12])
		entries = append(entries, e)
	}
	return entries, nil
}

func typeSize(t uint16) int {
	switch t {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11:
		return 4
	case 5, 10, 12:
		return 8
	default:
		return 1
	}
}

func entryUint(entries []ifdEntry, tag uint16, bo binary.ByteOrder) (uint32, bool) {
	for _, e := range entries {
		if e.Tag != tag || e.Count != 1 {
			continue
		}
		switch e.Type {
		case 3: // SHORT
			return uint32(bo.Uint16(e.Raw[:])), true
		case 4: // LONG
			return bo.Uint32(e.Raw[:]), true
		case 1: // BYTE
			return uint32(e.Raw[0]), true
		}
	}
	return 0, false
}

func entryUintOrFirst(entries []ifdEntry, tag uint16, bo binary.ByteOrder) (uint32, error) {
	for _, e := range entries {
		if e.Tag != tag {
			continue
		}
		if e.Count == 1 {
			if v, ok := entryUint(entries, tag, bo); ok {
				return v, nil
			}
		}
		// BitsPerSample may be inline for count=1
		if e.Type == 3 {
			return uint32(bo.Uint16(e.Raw[:])), nil
		}
	}
	return 0, fmt.Errorf("missing tag %d", tag)
}

func entryUintSlice(f *os.File, entries []ifdEntry, tag uint16, bo binary.ByteOrder) ([]uint32, error) {
	for _, e := range entries {
		if e.Tag != tag {
			continue
		}
		out := make([]uint32, e.Count)
		nbytes := int(e.Count) * typeSize(e.Type)
		var raw []byte
		if nbytes <= 4 {
			raw = e.Raw[:nbytes]
		} else {
			raw = make([]byte, nbytes)
			if _, err := f.ReadAt(raw, int64(e.ValueOffset)); err != nil {
				return nil, err
			}
		}
		for i := 0; i < int(e.Count); i++ {
			switch e.Type {
			case 3:
				out[i] = uint32(bo.Uint16(raw[i*2:]))
			case 4:
				out[i] = bo.Uint32(raw[i*4:])
			default:
				return nil, fmt.Errorf("unsupported type %d for tag %d", e.Type, tag)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("missing tag %d", tag)
}

func base(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
