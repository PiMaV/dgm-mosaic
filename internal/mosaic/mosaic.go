// Package mosaic mosaics LGL DGM GeoTIFF tiles for BLITZ (no GDAL).
package mosaic

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PiMaV/dgm-mosaic/internal/npy"
	"github.com/PiMaV/dgm-mosaic/internal/tiff"
)

const (
	NodataDefault = -9999.0
	pixelEps      = 1e-3
	rotEps        = 1e-9
	ThumbEdge     = 192
)

// Z0Unset means auto floor(zmin) for u16cm.
var Z0Unset = math.NaN()


var reDGM = regexp.MustCompile(`(?i)^dgm(?P<res>\d+)_(?P<zone>\d+)_(?P<e>\d+)_(?P<n>\d+)_`)

type Mode string

const (
	ModeU16cm      Mode = "u16cm"
	ModeU8stretch  Mode = "u8stretch"
	ModeU8step     Mode = "u8step"
	ModeF32        Mode = "f32"
)

type RefMode string

const (
	RefMin  RefMode = "min"
	RefMean RefMode = "mean"
)

type Options struct {
	Mode    Mode
	StepM   float64
	Ref     RefMode
	Nodata  float64
	Z0      float64 // NaN = auto
	TileBox *[4]int // r0,c0,r1,c1; nil = all
}

type DgmName struct {
	ResToken string
	Zone     int
	EKm      int
	NKm      int
	PixelM   float64
}

type WorldFile struct {
	PixelX, RotY, RotX, PixelY float64
	XULCenter, YULCenter       float64
}

func (w WorldFile) PixelM() float64 { return w.PixelX }
func (w WorldFile) West() float64   { return w.XULCenter - 0.5*w.PixelX }
func (w WorldFile) North() float64  { return w.YULCenter - 0.5*w.PixelY }

type Tile struct {
	Path   string
	Array  []float32 // H*W
	H, W   int
	West   float64
	North  float64
	PixelM float64
	Name   *DgmName
}

type TileStub struct {
	Path           string
	Height, Width  int
	West, North    float64
	PixelM         float64
	Name           *DgmName
}

func (s TileStub) East() float64  { return s.West + float64(s.Width)*s.PixelM }
func (s TileStub) South() float64 { return s.North - float64(s.Height)*s.PixelM }
func (s TileStub) LabelKm() string {
	if s.Name != nil {
		return fmt.Sprintf("%d_%d", s.Name.EKm, s.Name.NKm)
	}
	return fmt.Sprintf("E%.0f N%.0f", s.West, s.North)
}

type MosaicLayout struct {
	Stubs              []TileStub
	PixelM             float64
	West, North        float64
	East, South        float64
	Height, Width      int
	TileRows, TileCols int
	Cells              map[[2]int]TileStub
	Holes              int
	Regular            bool
	CRS                string
}

func (l *MosaicLayout) NBytes(mode Mode) int {
	return EstimateNPYBytes(l.Height, l.Width, mode)
}

func (l *MosaicLayout) NBytesBox(mode Mode, box [4]int) int {
	if len(l.Cells) == 0 {
		return 0
	}
	var sample TileStub
	for _, s := range l.Cells {
		sample = s
		break
	}
	r0, c0, r1, c1 := NormTileBox(box, l.TileRows, l.TileCols)
	h := (r1 - r0 + 1) * sample.Height
	w := (c1 - c0 + 1) * sample.Width
	return EstimateNPYBytes(h, w, mode)
}

func EstimateNPYBytes(h, w int, mode Mode) int {
	bpp := 4
	switch mode {
	case ModeU16cm:
		bpp = 2
	case ModeU8stretch, ModeU8step:
		bpp = 1
	}
	return h * w * bpp
}

func ParseDgmName(stem string) *DgmName {
	m := reDGM.FindStringSubmatch(stem)
	if m == nil {
		return nil
	}
	idx := map[string]int{}
	for i, name := range reDGM.SubexpNames() {
		if name != "" {
			idx[name] = i
		}
	}
	res := m[idx["res"]]
	var pixelM float64
	if strings.HasPrefix(res, "0") {
		frac := strings.TrimLeft(res, "0")
		if frac == "" {
			return nil
		}
		pixelM, _ = strconv.ParseFloat("0."+frac, 64)
	} else {
		pixelM, _ = strconv.ParseFloat(res, 64)
	}
	if pixelM <= 0 {
		return nil
	}
	zone, _ := strconv.Atoi(m[idx["zone"]])
	e, _ := strconv.Atoi(m[idx["e"]])
	n, _ := strconv.Atoi(m[idx["n"]])
	return &DgmName{ResToken: res, Zone: zone, EKm: e, NKm: n, PixelM: pixelM}
}

func ParseTFW(path string) (WorldFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return WorldFile{}, err
	}
	var vals []float64
	for _, ln := range strings.Split(string(raw), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		ln = strings.ReplaceAll(ln, ",", ".")
		v, err := strconv.ParseFloat(ln, 64)
		if err != nil {
			return WorldFile{}, fmt.Errorf("%s: bad number %q", filepath.Base(path), ln)
		}
		vals = append(vals, v)
		if len(vals) == 6 {
			break
		}
	}
	if len(vals) < 6 {
		return WorldFile{}, fmt.Errorf("%s: world file needs 6 numbers, got %d", filepath.Base(path), len(vals))
	}
	w := WorldFile{vals[0], vals[1], vals[2], vals[3], vals[4], vals[5]}
	if math.Abs(w.RotX) > rotEps || math.Abs(w.RotY) > rotEps {
		return WorldFile{}, fmt.Errorf("%s: rotation is not zero (unsupported)", filepath.Base(path))
	}
	if w.PixelX <= 0 {
		return WorldFile{}, fmt.Errorf("%s: x pixel size must be > 0", filepath.Base(path))
	}
	if w.PixelY >= 0 {
		return WorldFile{}, fmt.Errorf("%s: y pixel size must be negative (north-up)", filepath.Base(path))
	}
	if math.Abs(w.PixelX-math.Abs(w.PixelY)) > 1e-6 {
		return WorldFile{}, fmt.Errorf("%s: non-square pixels are unsupported", filepath.Base(path))
	}
	return w, nil
}

func tfwForTif(tif string) string {
	base := strings.TrimSuffix(tif, filepath.Ext(tif))
	for _, suf := range []string{".tfw", ".tifw"} {
		cand := base + suf
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	// also try with .tif replaced
	ext := filepath.Ext(tif)
	cand := strings.TrimSuffix(tif, ext) + ".tfw"
	if st, err := os.Stat(cand); err == nil && !st.IsDir() {
		return cand
	}
	return ""
}

func pixelMClose(a, b float64) bool {
	return math.Abs(a-b) <= math.Max(pixelEps*math.Max(math.Abs(a), math.Max(math.Abs(b), 1e-9)), 1e-9)
}

func ListTIFFs(root string) ([]string, error) {
	st, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("%s does not exist", root)
	}
	if !st.IsDir() {
		ext := strings.ToLower(filepath.Ext(root))
		if ext == ".tif" || ext == ".tiff" {
			return []string{root}, nil
		}
		return nil, fmt.Errorf("%s is not a TIFF", root)
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".tif" || ext == ".tiff" {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no TIFF files in %s", root)
	}
	// sort by lower name
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if strings.ToLower(filepath.Base(out[j])) < strings.ToLower(filepath.Base(out[i])) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func originForTif(path string) (west, north, pixelM float64, name *DgmName, err error) {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = ParseDgmName(stem)
	if tfw := tfwForTif(path); tfw != "" {
		w, e := ParseTFW(tfw)
		if e != nil {
			return 0, 0, 0, nil, e
		}
		return w.West(), w.North(), w.PixelM(), name, nil
	}
	if name == nil {
		return 0, 0, 0, nil, fmt.Errorf("%s: need a .tfw world file or an LGL name (dgm025_32_{e}_{n}_…)", filepath.Base(path))
	}
	return float64(name.EKm * 1000), float64((name.NKm + 1) * 1000), name.PixelM, name, nil
}

func StubTile(path string) (TileStub, error) {
	west, north, pixelM, name, err := originForTif(path)
	if err != nil {
		return TileStub{}, err
	}
	h, w, err := tiff.PeekHW(path)
	if err != nil {
		return TileStub{}, err
	}
	return TileStub{Path: path, Height: h, Width: w, West: west, North: north, PixelM: pixelM, Name: name}, nil
}

func InspectFolder(inputPath string) (*MosaicLayout, error) {
	tiffs, err := ListTIFFs(inputPath)
	if err != nil {
		return nil, err
	}
	stubs := make([]TileStub, 0, len(tiffs))
	for _, p := range tiffs {
		s, err := StubTile(p)
		if err != nil {
			return nil, err
		}
		stubs = append(stubs, s)
	}
	return LayoutFromStubs(stubs)
}

func LayoutFromStubs(stubs []TileStub) (*MosaicLayout, error) {
	if len(stubs) == 0 {
		return nil, fmt.Errorf("no tiles to mosaic")
	}
	pixelM := stubs[0].PixelM
	for _, s := range stubs[1:] {
		if !pixelMClose(s.PixelM, pixelM) {
			return nil, fmt.Errorf("mixed pixel sizes: %s=%g m, %s=%g m",
				filepath.Base(stubs[0].Path), pixelM, filepath.Base(s.Path), s.PixelM)
		}
	}
	west := stubs[0].West
	north := stubs[0].North
	east := stubs[0].East()
	south := stubs[0].South()
	for _, s := range stubs[1:] {
		if s.West < west {
			west = s.West
		}
		if s.North > north {
			north = s.North
		}
		if s.East() > east {
			east = s.East()
		}
		if s.South() < south {
			south = s.South()
		}
	}
	width, err := nearInt((east - west) / pixelM)
	if err != nil {
		return nil, err
	}
	height, err := nearInt((north - south) / pixelM)
	if err != nil {
		return nil, err
	}
	shapes := map[[2]int]struct{}{}
	for _, s := range stubs {
		shapes[[2]int{s.Height, s.Width}] = struct{}{}
	}
	regular := len(shapes) == 1
	cells := map[[2]int]TileStub{}
	var tileRows, tileCols int
	if regular {
		var th, tw int
		for k := range shapes {
			th, tw = k[0], k[1]
		}
		tileCols, err = nearInt((east - west) / (float64(tw) * pixelM))
		if err != nil {
			return nil, err
		}
		tileRows, err = nearInt((north - south) / (float64(th) * pixelM))
		if err != nil {
			return nil, err
		}
		for _, s := range stubs {
			col, err := nearInt((s.West - west) / (float64(tw) * pixelM))
			if err != nil {
				return nil, err
			}
			row, err := nearInt((north - s.North) / (float64(th) * pixelM))
			if err != nil {
				return nil, err
			}
			cells[[2]int{row, col}] = s
		}
	} else {
		tileRows, tileCols = 1, len(stubs)
		// sort by -north, west
		ordered := append([]TileStub(nil), stubs...)
		for i := 0; i < len(ordered); i++ {
			for j := i + 1; j < len(ordered); j++ {
				a, b := ordered[i], ordered[j]
				if b.North > a.North || (b.North == a.North && b.West < a.West) {
					ordered[i], ordered[j] = ordered[j], ordered[i]
				}
			}
		}
		for i, s := range ordered {
			cells[[2]int{0, i}] = s
		}
	}
	holes := 0
	if regular {
		holes = tileRows*tileCols - len(cells)
	}
	crs := ""
	for _, s := range stubs {
		if s.Name != nil && (s.Name.Zone == 32 || s.Name.Zone == 33) {
			crs = fmt.Sprintf("EPSG:258%d", s.Name.Zone)
			break
		}
	}
	return &MosaicLayout{
		Stubs: stubs, PixelM: pixelM, West: west, North: north, East: east, South: south,
		Height: height, Width: width, TileRows: tileRows, TileCols: tileCols,
		Cells: cells, Holes: holes, Regular: regular, CRS: crs,
	}, nil
}

func nearInt(value float64) (int, error) {
	rounded := math.Round(value)
	if math.Abs(value-rounded) > pixelEps {
		return 0, fmt.Errorf("tile origin is not on the mosaic pixel grid (%.6f px) — refusing to resample", value)
	}
	return int(rounded), nil
}

func NormTileBox(box [4]int, rows, cols int) (r0, c0, r1, c1 int) {
	r0, c0, r1, c1 = box[0], box[1], box[2], box[3]
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	r0 = clamp(r0, 0, rows-1)
	r1 = clamp(r1, 0, rows-1)
	c0 = clamp(c0, 0, cols-1)
	c1 = clamp(c1, 0, cols-1)
	return
}

func LoadTile(path string, nodata float64) (Tile, error) {
	h, w, pix, err := tiff.ReadFloat32(path)
	if err != nil {
		return Tile{}, err
	}
	for i, v := range pix {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || float64(v) == nodata {
			pix[i] = float32(math.NaN())
		}
	}
	west, north, pixelM, name, err := originForTif(path)
	if err != nil {
		return Tile{}, err
	}
	return Tile{Path: path, Array: pix, H: h, W: w, West: west, North: north, PixelM: pixelM, Name: name}, nil
}

func MosaicForTileBox(tiles []Tile, layout *MosaicLayout, box [4]int) ([]float32, int, int, map[string]any, error) {
	if len(layout.Cells) == 0 {
		return nil, 0, 0, nil, fmt.Errorf("layout has no tiles")
	}
	r0, c0, r1, c1 := NormTileBox(box, layout.TileRows, layout.TileCols)
	var sample TileStub
	for _, s := range layout.Cells {
		sample = s
		break
	}
	th, tw := sample.Height, sample.Width
	pixelM := layout.PixelM
	west := layout.West + float64(c0)*float64(tw)*pixelM
	north := layout.North - float64(r0)*float64(th)*pixelM
	nrows, ncols := r1-r0+1, c1-c0+1
	canvas := make([]float32, nrows*th*ncols*tw) // zeros
	used := []string{}
	for _, t := range tiles {
		col, err := nearInt((t.West - layout.West) / (float64(tw) * pixelM))
		if err != nil {
			return nil, 0, 0, nil, err
		}
		row, err := nearInt((layout.North - t.North) / (float64(th) * pixelM))
		if err != nil {
			return nil, 0, 0, nil, err
		}
		if row < r0 || row > r1 || col < c0 || col > c1 {
			continue
		}
		rr, cc := row-r0, col-c0
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				src := t.Array[y*t.W+x]
				v := float32(0)
				if !math.IsNaN(float64(src)) {
					v = src
				}
				canvas[(rr*th+y)*ncols*tw+(cc*tw+x)] = v
			}
		}
		used = append(used, filepath.Base(t.Path))
	}
	east := west + float64(ncols)*float64(tw)*pixelM
	south := north - float64(nrows)*float64(th)*pixelM
	zone := 0
	for _, t := range tiles {
		if t.Name != nil {
			zone = t.Name.Zone
			break
		}
	}
	crs := any(nil)
	if zone == 32 || zone == 33 {
		crs = fmt.Sprintf("EPSG:258%d", zone)
	}
	info := map[string]any{
		"pixel_m":        pixelM,
		"west":           west,
		"north":          north,
		"east":           east,
		"south":          south,
		"shape_hw":       []int{nrows * th, ncols * tw},
		"overlap_pixels": 0,
		"crs":            crs,
		"tiles":          used,
		"tile_box":       []int{r0, c0, r1, c1},
		"zero_padded":    nrows*ncols - len(used),
	}
	return canvas, nrows * th, ncols * tw, info, nil
}

func MosaicTiles(tiles []Tile) ([]float32, int, int, map[string]any, error) {
	if len(tiles) == 0 {
		return nil, 0, 0, nil, fmt.Errorf("no tiles to mosaic")
	}
	pixelM := tiles[0].PixelM
	for _, t := range tiles[1:] {
		if !pixelMClose(t.PixelM, pixelM) {
			return nil, 0, 0, nil, fmt.Errorf("mixed pixel sizes")
		}
	}
	west, north := tiles[0].West, tiles[0].North
	east := tiles[0].West + float64(tiles[0].W)*tiles[0].PixelM
	south := tiles[0].North - float64(tiles[0].H)*tiles[0].PixelM
	for _, t := range tiles[1:] {
		if t.West < west {
			west = t.West
		}
		if t.North > north {
			north = t.North
		}
		e := t.West + float64(t.W)*t.PixelM
		s := t.North - float64(t.H)*t.PixelM
		if e > east {
			east = e
		}
		if s < south {
			south = s
		}
	}
	width, err := nearInt((east - west) / pixelM)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	height, err := nearInt((north - south) / pixelM)
	if err != nil {
		return nil, 0, 0, nil, err
	}
	canvas := make([]float32, height*width)
	for i := range canvas {
		canvas[i] = float32(math.NaN())
	}
	overlap := 0
	for _, t := range tiles {
		c0, err := nearInt((t.West - west) / pixelM)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		r0, err := nearInt((north - t.North) / pixelM)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		th, tw := t.H, t.W
		if r0 < 0 || c0 < 0 || r0+th > height || c0+tw > width {
			return nil, 0, 0, nil, fmt.Errorf("%s: paste outside", filepath.Base(t.Path))
		}
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				di := (r0+y)*width + (c0 + x)
				src := t.Array[y*t.W+x]
				if math.IsNaN(float64(canvas[di])) == false && math.IsNaN(float64(src)) == false {
					overlap++
				}
				if !math.IsNaN(float64(src)) {
					canvas[di] = src
				}
			}
		}
	}
	zone := 0
	names := make([]string, 0, len(tiles))
	for _, t := range tiles {
		names = append(names, filepath.Base(t.Path))
		if t.Name != nil && zone == 0 {
			zone = t.Name.Zone
		}
	}
	crs := any(nil)
	if zone == 32 || zone == 33 {
		crs = fmt.Sprintf("EPSG:258%d", zone)
	}
	info := map[string]any{
		"pixel_m": pixelM, "west": west, "north": north, "east": east, "south": south,
		"shape_hw": []int{height, width}, "overlap_pixels": overlap, "crs": crs, "tiles": names,
	}
	return canvas, height, width, info, nil
}

func DefaultOutputPath(inputPath string) string {
	st, err := os.Stat(inputPath)
	if err == nil && st.IsDir() {
		return filepath.Join(filepath.Dir(inputPath), filepath.Base(inputPath)+"_mosaic.npy")
	}
	dir := filepath.Dir(inputPath)
	stem := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	return filepath.Join(dir, stem+"_mosaic.npy")
}

func WriteMeta(path string, meta map[string]any, npyName string) error {
	m := map[string]any{}
	for k, v := range meta {
		m[k] = v
	}
	m["npy"] = npyName
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Build inspects, mosaics the box (or all), and quantizes.
func Build(inputPath string, opts Options) (npy.Array, map[string]any, error) {
	layout, err := InspectFolder(inputPath)
	if err != nil {
		return npy.Array{}, nil, err
	}
	box := [4]int{0, 0, layout.TileRows - 1, layout.TileCols - 1}
	if opts.TileBox != nil {
		box = *opts.TileBox
	}
	r0, c0, r1, c1 := NormTileBox(box, layout.TileRows, layout.TileCols)
	var paths []string
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			if s, ok := layout.Cells[[2]int{r, c}]; ok {
				paths = append(paths, s.Path)
			}
		}
	}
	nodata := opts.Nodata
	if nodata == 0 || math.IsNaN(nodata) {
		nodata = NodataDefault
	}
	tiles := make([]Tile, 0, len(paths))
	for _, p := range paths {
		t, err := LoadTile(p, nodata)
		if err != nil {
			return npy.Array{}, nil, err
		}
		tiles = append(tiles, t)
	}
	mosaic, h, w, geo, err := MosaicForTileBox(tiles, layout, [4]int{r0, c0, r1, c1})
	if err != nil {
		return npy.Array{}, nil, err
	}
	arr, qmeta, err := Quantize(mosaic, h, w, opts)
	if err != nil {
		return npy.Array{}, nil, err
	}
	meta := map[string]any{
		"format": "wetter.dgm_mosaic.v1",
		"layout": "row0_north_col0_west",
		"source": inputPath,
	}
	for k, v := range geo {
		meta[k] = v
	}
	for k, v := range qmeta {
		meta[k] = v
	}
	meta["nbytes"] = arr.NBytes()
	return arr, meta, nil
}
