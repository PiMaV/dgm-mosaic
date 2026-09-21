package mosaic_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/PiMaV/dgm-mosaic/internal/mosaic"
)

func TestParseDgm025(t *testing.T) {
	n := mosaic.ParseDgmName("dgm025_32_505_5364_1_bw_2020")
	if n == nil {
		t.Fatal("expected parse")
	}
	if n.Zone != 32 || n.EKm != 505 || n.NKm != 5364 {
		t.Fatalf("got %+v", n)
	}
	if math.Abs(n.PixelM-0.25) > 1e-9 {
		t.Fatalf("pixel %g", n.PixelM)
	}
}

func TestParseDgm1(t *testing.T) {
	n := mosaic.ParseDgmName("dgm1_32_500_5400_1")
	if n == nil || math.Abs(n.PixelM-1.0) > 1e-9 {
		t.Fatalf("got %+v", n)
	}
}

func TestTFW(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.tfw")
	if err := writeFile(p, "0.2500000000\n0.0000000000\n0.0000000000\n-0.2500000000\n505000.1250000000\n5364999.8750000000\n"); err != nil {
		t.Fatal(err)
	}
	w, err := mosaic.ParseTFW(p)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(w.West()-505000.0) > 1e-6 || math.Abs(w.North()-5365000.0) > 1e-6 {
		t.Fatalf("west=%g north=%g", w.West(), w.North())
	}
}

func TestTFWRejectsRotation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.tfw")
	_ = writeFile(p, "0.25\n0.1\n0\n-0.25\n0\n0\n")
	if _, err := mosaic.ParseTFW(p); err == nil {
		t.Fatal("expected error")
	}
}

func TestMosaic2x2(t *testing.T) {
	a := []float32{1, 2, 3, 4}
	b := []float32{5, 6, 7, 8}
	c := []float32{9, 10, 11, 12}
	d := []float32{13, 14, 15, 16}
	tiles := []mosaic.Tile{
		{Path: "sw.tif", Array: a, H: 2, W: 2, West: 0, North: 0.5, PixelM: 0.25},
		{Path: "se.tif", Array: b, H: 2, W: 2, West: 0.5, North: 0.5, PixelM: 0.25},
		{Path: "nw.tif", Array: c, H: 2, W: 2, West: 0, North: 1.0, PixelM: 0.25},
		{Path: "ne.tif", Array: d, H: 2, W: 2, West: 0.5, North: 1.0, PixelM: 0.25},
	}
	m, h, w, info, err := mosaic.MosaicTiles(tiles)
	if err != nil {
		t.Fatal(err)
	}
	if h != 4 || w != 4 {
		t.Fatalf("shape %dx%d", h, w)
	}
	expect := []float32{
		9, 10, 13, 14,
		11, 12, 15, 16,
		1, 2, 5, 6,
		3, 4, 7, 8,
	}
	for i := range expect {
		if m[i] != expect[i] {
			t.Fatalf("at %d got %v want %v", i, m[i], expect[i])
		}
	}
	if info["overlap_pixels"].(int) != 0 {
		t.Fatal(info["overlap_pixels"])
	}
}

func TestOffGridRefused(t *testing.T) {
	a := []float32{1, 1, 1, 1}
	tiles := []mosaic.Tile{
		{Path: "a.tif", Array: a, H: 2, W: 2, West: 0, North: 1, PixelM: 0.25},
		{Path: "b.tif", Array: a, H: 2, W: 2, West: 0.01, North: 1, PixelM: 0.25},
	}
	if _, _, _, _, err := mosaic.MosaicTiles(tiles); err == nil {
		t.Fatal("expected error")
	}
}

func TestLayout2x2(t *testing.T) {
	nSW := mosaic.ParseDgmName("dgm025_32_505_5364_1")
	nSE := mosaic.ParseDgmName("dgm025_32_506_5364_1")
	nNW := mosaic.ParseDgmName("dgm025_32_505_5365_1")
	nNE := mosaic.ParseDgmName("dgm025_32_506_5365_1")
	stubs := []mosaic.TileStub{
		{Path: "sw.tif", Height: 4000, Width: 4000, West: 505000, North: 5365000, PixelM: 0.25, Name: nSW},
		{Path: "se.tif", Height: 4000, Width: 4000, West: 506000, North: 5365000, PixelM: 0.25, Name: nSE},
		{Path: "nw.tif", Height: 4000, Width: 4000, West: 505000, North: 5366000, PixelM: 0.25, Name: nNW},
		{Path: "ne.tif", Height: 4000, Width: 4000, West: 506000, North: 5366000, PixelM: 0.25, Name: nNE},
	}
	layout, err := mosaic.LayoutFromStubs(stubs)
	if err != nil {
		t.Fatal(err)
	}
	if layout.TileRows != 2 || layout.TileCols != 2 || layout.Height != 8000 || layout.Width != 8000 {
		t.Fatalf("%+v", layout)
	}
	if layout.Cells[[2]int{0, 0}].LabelKm() != "505_5365" {
		t.Fatal(layout.Cells[[2]int{0, 0}].LabelKm())
	}
	if layout.NBytes(mosaic.ModeU16dm) != 8000*8000*2 {
		t.Fatal(layout.NBytes(mosaic.ModeU16dm))
	}
	if layout.CRS != "EPSG:25832" {
		t.Fatal(layout.CRS)
	}
}

func TestQuantizeU16dm(t *testing.T) {
	z := []float32{751.3}
	arr, meta, err := mosaic.Quantize(z, 1, 1, mosaic.Options{Mode: mosaic.ModeU16dm, Z0: 0})
	if err != nil {
		t.Fatal(err)
	}
	if arr.DType != "<u2" {
		t.Fatal(arr.DType)
	}
	v0 := uint16(arr.Data[0]) | uint16(arr.Data[1])<<8
	if v0 != 7513 {
		t.Fatalf("want 7513 dm, got %d", v0)
	}
	if meta["scale_m"].(float64) != 0.1 {
		t.Fatal(meta["scale_m"])
	}
	recon := float64(v0) * 0.1
	if math.Abs(recon-751.3) > 0.05 {
		t.Fatalf("recon %g", recon)
	}
}

func TestDiverging(t *testing.T) {
	blue := mosaic.DivergingRGB([]float32{0}, 1, 1)
	white := mosaic.DivergingRGB([]float32{0.5}, 1, 1)
	red := mosaic.DivergingRGB([]float32{1}, 1, 1)
	if blue[0] != 0 || blue[1] != 0 || blue[2] != 255 {
		t.Fatal(blue)
	}
	if white[0] != 255 || white[1] != 255 || white[2] != 255 {
		t.Fatal(white)
	}
	if red[0] != 255 || red[1] != 0 || red[2] != 0 {
		t.Fatal(red)
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
