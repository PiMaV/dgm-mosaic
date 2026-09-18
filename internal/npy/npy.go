// Package npy writes NumPy .npy files (v1.0, C-order) for the WETTER Viewer Contract.
package npy

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// Array is a contiguous numeric cube with C-order layout.
type Array struct {
	Shape []int
	// DType is a NumPy descr: "<u1", "<u2", "<i2", "<f4", …
	DType string
	Data  []byte
}

func (a Array) NBytes() int { return len(a.Data) }

// Write encodes a as a .npy v1.0 stream.
func Write(w io.Writer, a Array) error {
	if len(a.Shape) == 0 {
		return fmt.Errorf("npy: empty shape")
	}
	if a.DType == "" {
		return fmt.Errorf("npy: missing dtype")
	}
	elem := elemSize(a.DType)
	need := elem
	for _, d := range a.Shape {
		if d < 0 {
			return fmt.Errorf("npy: negative dim")
		}
		need *= d
	}
	if len(a.Data) != need {
		return fmt.Errorf("npy: data length %d != product %d", len(a.Data), need)
	}

	shapeParts := make([]string, len(a.Shape))
	for i, d := range a.Shape {
		shapeParts[i] = fmt.Sprintf("%d", d)
	}
	shapeStr := "(" + strings.Join(shapeParts, ", ")
	if len(a.Shape) == 1 {
		shapeStr += ","
	}
	shapeStr += ")"

	header := fmt.Sprintf(
		"{'descr': '%s', 'fortran_order': False, 'shape': %s, }",
		a.DType, shapeStr,
	)
	// magic(6) + ver(2) + hdrlen(2) + header + '\n' must be multiple of 64
	const prefix = 10
	pad := 64 - ((prefix + len(header) + 1) % 64)
	if pad == 64 {
		pad = 0
	}
	header += strings.Repeat(" ", pad) + "\n"
	if len(header) > 65535 {
		return fmt.Errorf("npy: header too long")
	}

	if _, err := w.Write([]byte{0x93, 'N', 'U', 'M', 'P', 'Y', 1, 0}); err != nil {
		return err
	}
	var hl [2]byte
	binary.LittleEndian.PutUint16(hl[:], uint16(len(header)))
	if _, err := w.Write(hl[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(a.Data)
	return err
}

func elemSize(dtype string) int {
	switch dtype {
	case "|u1", "<u1", ">u1", "|i1", "<i1", ">i1", "|b1":
		return 1
	case "<u2", ">u2", "<i2", ">i2":
		return 2
	case "<u4", ">u4", "<i4", ">i4", "<f4", ">f4":
		return 4
	case "<u8", ">u8", "<i8", ">i8", "<f8", ">f8":
		return 8
	default:
		return 0
	}
}

// FromUint8 builds an Array from a C-order uint8 buffer.
func FromUint8(shape []int, data []uint8) Array {
	b := make([]byte, len(data))
	copy(b, data)
	return Array{Shape: append([]int(nil), shape...), DType: "|u1", Data: b}
}

// FromUint16LE builds an Array from uint16 values (host → little-endian wire).
func FromUint16LE(shape []int, data []uint16) Array {
	b := make([]byte, len(data)*2)
	for i, v := range data {
		binary.LittleEndian.PutUint16(b[i*2:], v)
	}
	return Array{Shape: append([]int(nil), shape...), DType: "<u2", Data: b}
}

// FromFloat32LE builds an Array from float32 values.
func FromFloat32LE(shape []int, data []float32) Array {
	b := make([]byte, len(data)*4)
	for i, v := range data {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return Array{Shape: append([]int(nil), shape...), DType: "<f4", Data: b}
}
