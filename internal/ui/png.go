package ui

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

func writePNG(w io.Writer, rgb []uint8, width, height int) error {
	if width < 1 || height < 1 {
		return fmt.Errorf("bad png size")
	}
	if len(rgb) < width*height*3 {
		return fmt.Errorf("rgb buffer short")
	}
	if _, err := w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10}); err != nil {
		return err
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8  // bit depth
	ihdr[9] = 2  // RGB
	ihdr[10] = 0 // compression
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // interlace
	if err := writeChunk(w, "IHDR", ihdr); err != nil {
		return err
	}
	// raw scanlines with filter 0, then zlib store (no compress) for simplicity
	raw := make([]byte, 0, height*(1+width*3))
	for y := 0; y < height; y++ {
		raw = append(raw, 0)
		off := y * width * 3
		raw = append(raw, rgb[off:off+width*3]...)
	}
	z := zlibStore(raw)
	if err := writeChunk(w, "IDAT", z); err != nil {
		return err
	}
	return writeChunk(w, "IEND", nil)
}

func writeChunk(w io.Writer, typ string, data []byte) error {
	var lenbuf [4]byte
	binary.BigEndian.PutUint32(lenbuf[:], uint32(len(data)))
	if _, err := w.Write(lenbuf[:]); err != nil {
		return err
	}
	if _, err := io.WriteString(w, typ); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	crc := crc32.NewIEEE()
	_, _ = io.WriteString(crc, typ)
	_, _ = crc.Write(data)
	var cbuf [4]byte
	binary.BigEndian.PutUint32(cbuf[:], crc.Sum32())
	_, err := w.Write(cbuf[:])
	return err
}

// zlibStore wraps raw deflate stored blocks (no compression).
func zlibStore(data []byte) []byte {
	const maxBlock = 65535
	out := []byte{0x78, 0x01} // zlib header, no dict, fastest
	adler := adler32(data)
	for len(data) > 0 {
		n := len(data)
		final := byte(0)
		if n <= maxBlock {
			final = 1
		} else {
			n = maxBlock
		}
		block := data[:n]
		data = data[n:]
		out = append(out, final) // BFINAL|BTYPE=00
		var lenbuf [2]byte
		binary.LittleEndian.PutUint16(lenbuf[:], uint16(n))
		out = append(out, lenbuf[0], lenbuf[1], ^lenbuf[0], ^lenbuf[1])
		out = append(out, block...)
	}
	var abuf [4]byte
	binary.BigEndian.PutUint32(abuf[:], adler)
	out = append(out, abuf[:]...)
	return out
}

func adler32(data []byte) uint32 {
	const mod = 65521
	var a, b uint32 = 1, 0
	for _, v := range data {
		a = (a + uint32(v)) % mod
		b = (b + a) % mod
	}
	return (b << 16) | a
}
