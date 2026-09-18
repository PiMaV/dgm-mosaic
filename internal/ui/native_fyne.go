//go:build fyne

// Package ui is the native DGM mosaic stage (Fyne): drag-drop folders, tile preview, export box.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"github.com/PiMaV/dgm-mosaic/internal/hub"
	"github.com/PiMaV/dgm-mosaic/internal/mosaic"
	"github.com/PiMaV/dgm-mosaic/internal/npy"
)

type Options struct {
	DefaultDtype string
	StepM        float64
	Ref          string
	Nodata       float64
	Z0           float64
	Prefill      string
}

// Run opens the native window (blocking). Hub must already be listening in the background.
func Run(pub *hub.Publisher, opts Options) {
	if opts.DefaultDtype == "" {
		opts.DefaultDtype = "u16cm"
	}
	if opts.StepM == 0 {
		opts.StepM = 0.25
	}
	if opts.Ref == "" {
		opts.Ref = "min"
	}
	if opts.Nodata == 0 {
		opts.Nodata = mosaic.NodataDefault
	}

	a := app.NewWithID("engineering.mess.dgm-mosaic")
	w := a.NewWindow("DGM mosaic → BLITZ")
	w.Resize(fyne.NewSize(820, 900))
	w.SetFixedSize(false)

	stage := &stage{
		pub:  pub,
		opts: opts,
		win:  w,
	}
	stage.build()
	w.SetContent(stage.root)
	w.SetOnDropped(stage.onDropped)

	if opts.Prefill != "" {
		go stage.loadFolder(opts.Prefill)
	}

	w.ShowAndRun()
}

type stage struct {
	pub  *hub.Publisher
	opts Options
	win  fyne.Window
	root fyne.CanvasObject

	folderEntry *widget.Entry
	metaLabel   *widget.Label
	selLabel    *widget.Label
	sizeLabel   *widget.Label
	connectLbl  *widget.Label
	status      *widget.Label
	sendBtn     *widget.Button
	saveBtn     *widget.Button
	mode        *widget.RadioGroup
	stepEntry   *widget.Entry
	refSelect   *widget.Select
	mosaicView  *mosaicView

	mu     sync.Mutex
	folder string
	layout *mosaic.MosaicLayout
	busy   bool
}

func (s *stage) build() {
	s.folderEntry = widget.NewEntry()
	s.folderEntry.SetPlaceHolder("Drop a tile folder here, or Browse…")
	s.folderEntry.Disable()

	browse := widget.NewButton("Browse…", func() {
		dialog.ShowFolderOpen(func(lu fyne.ListableURI, err error) {
			if err != nil || lu == nil {
				return
			}
			s.loadFolder(lu.Path())
		}, s.win)
	})

	s.metaLabel = widget.NewLabel("No folder loaded.")
	s.metaLabel.Wrapping = fyne.TextWrapWord
	s.selLabel = widget.NewLabel("Drag a rectangle of tiles to export.")
	s.selLabel.Wrapping = fyne.TextWrapWord
	s.sizeLabel = widget.NewLabel("")
	s.connectLbl = widget.NewLabel("BLITZ Stream → Connect  " + s.pub.ConnectHint() +
		"\nNot WOLKE — same contract as the Event reader (one mosaic push).")
	s.connectLbl.Wrapping = fyne.TextWrapWord
	s.status = widget.NewLabel("Drop a folder of DGM TIFFs.")
	s.status.Wrapping = fyne.TextWrapWord

	s.mosaicView = newMosaicView(func(box [4]int) {
		s.refreshSelection()
	})

	allBtn := widget.NewButton("All tiles", func() {
		s.mosaicView.selectAll()
		s.refreshSelection()
	})

	s.mode = widget.NewRadioGroup([]string{
		"u16cm — uint16 centimetres (default)",
		"u8stretch — uint8 min…max → 1…255",
		"u8step — uint8 fixed step",
		"f32 — float32 metres",
	}, func(string) { s.refreshSelection() })
	s.mode.SetSelected("u16cm — uint16 centimetres (default)")

	s.stepEntry = widget.NewEntry()
	s.stepEntry.SetText(fmt.Sprintf("%g", s.opts.StepM))
	s.refSelect = widget.NewSelect([]string{"min", "mean"}, func(string) { s.refreshSelection() })
	s.refSelect.SetSelected(s.opts.Ref)

	s.sendBtn = widget.NewButton("Send to BLITZ", s.onSend)
	s.sendBtn.Disable()
	s.saveBtn = widget.NewButton("Save .npy…", s.onSave)
	s.saveBtn.Disable()

	top := container.NewBorder(nil, nil, nil, browse, s.folderEntry)
	north := widget.NewLabelWithStyle("N ↑", fyne.TextAlignCenter, fyne.TextStyle{})
	east := widget.NewLabelWithStyle("west  ←  tiles  →  east", fyne.TextAlignCenter, fyne.TextStyle{})
	selRow := container.NewBorder(nil, nil, nil, allBtn, s.selLabel)

	fmtBox := container.NewVBox(
		widget.NewLabel("Format (size = selected tiles)"),
		s.mode,
		container.NewHBox(widget.NewLabel("u8step:"), s.stepEntry, widget.NewLabel("ref"), s.refSelect),
		s.sizeLabel,
	)

	s.root = container.NewBorder(
		container.NewVBox(top, s.metaLabel, north),
		container.NewVBox(east, selRow, fmtBox, s.connectLbl, s.sendBtn, s.saveBtn, s.status),
		nil, nil,
		s.mosaicView,
	)
}

func (s *stage) onDropped(_ fyne.Position, uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}
	path := uris[0].Path()
	if dir, err := isDir(path); err == nil && !dir {
		path = filepath.Dir(path)
	}
	s.loadFolder(path)
}

func (s *stage) loadFolder(path string) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.mu.Unlock()

	s.status.SetText("Inspecting tiles…")
	s.sendBtn.Disable()
	s.saveBtn.Disable()

	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()
		layout, err := mosaic.InspectFolder(path)
		if err != nil {
			s.status.SetText("Error: " + err.Error())
			return
		}
		rgb, rows, cols, labels, err := buildPreview(layout, s.opts.Nodata)
		if err != nil {
			s.status.SetText("Error: " + err.Error())
			return
		}
		s.mu.Lock()
		s.folder = path
		s.layout = layout
		s.mu.Unlock()
		s.folderEntry.SetText(path)
		s.metaLabel.SetText(fmt.Sprintf(
			"%d tiles · %d×%d px · %g m%s%s",
			len(layout.Stubs), layout.Width, layout.Height, layout.PixelM,
			crsSuffix(layout.CRS), holesSuffix(layout.Holes),
		))
		s.mosaicView.setMosaic(rgb, rows, cols, labels)
		s.sendBtn.Enable()
		s.saveBtn.Enable()
		s.refreshSelection()
		s.status.SetText("Ready — drag a tile rectangle, then Send.")
	}()
}

func crsSuffix(crs string) string {
	if crs == "" {
		return ""
	}
	return " · " + crs
}
func holesSuffix(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" · %d holes", n)
}

func buildPreview(layout *mosaic.MosaicLayout, nodata float64) (*image.RGBA, int, int, map[[2]int]string, error) {
	raw := map[string][]float32{}
	var th, tw int
	labels := map[[2]int]string{}
	for key, stub := range layout.Cells {
		pix, h, w, err := mosaic.TileThumbnail(stub.Path, nodata, mosaic.ThumbEdge)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		th, tw = h, w
		raw[fmt.Sprintf("%d,%d", key[0], key[1])] = pix
		labels[key] = stub.LabelKm()
	}
	unitNamed := mosaic.FloatThumbsToUnit(raw)
	unitKeyed := map[[2]int][]float32{}
	for k, v := range unitNamed {
		var r, c int
		_, _ = fmt.Sscanf(k, "%d,%d", &r, &c)
		unitKeyed[[2]int{r, c}] = v
	}
	rgbBytes := mosaic.ComposePreviewRGB(layout.TileRows, layout.TileCols, unitKeyed, th, tw)
	img := image.NewRGBA(image.Rect(0, 0, layout.TileCols*tw, layout.TileRows*th))
	for y := 0; y < layout.TileRows*th; y++ {
		for x := 0; x < layout.TileCols*tw; x++ {
			i := (y*layout.TileCols*tw + x) * 3
			img.SetRGBA(x, y, color.RGBA{R: rgbBytes[i], G: rgbBytes[i+1], B: rgbBytes[i+2], A: 255})
		}
	}
	return img, layout.TileRows, layout.TileCols, labels, nil
}

func (s *stage) selectedMode() mosaic.Mode {
	sel := s.mode.Selected
	switch {
	case len(sel) >= 8 && sel[:8] == "u8stretc":
		return mosaic.ModeU8stretch
	case len(sel) >= 6 && sel[:6] == "u8step":
		return mosaic.ModeU8step
	case len(sel) >= 3 && sel[:3] == "f32":
		return mosaic.ModeF32
	default:
		return mosaic.ModeU16cm
	}
}

func (s *stage) refreshSelection() {
	s.mu.Lock()
	layout := s.layout
	s.mu.Unlock()
	if layout == nil {
		return
	}
	box := s.mosaicView.box
	r0, c0, r1, c1 := mosaic.NormTileBox(box, layout.TileRows, layout.TileCols)
	s.selLabel.SetText(fmt.Sprintf("Selected tiles r%d…%d × c%d…%d", r0, r1, c0, c1))
	nbytes := layout.NBytesBox(s.selectedMode(), [4]int{r0, c0, r1, c1})
	s.sizeLabel.SetText("Selected size ≈ " + mosaic.FmtMB(nbytes))
}

func (s *stage) buildOpts() (string, mosaic.Options, error) {
	s.mu.Lock()
	folder := s.folder
	s.mu.Unlock()
	if folder == "" {
		return "", mosaic.Options{}, fmt.Errorf("no folder loaded")
	}
	step := s.opts.StepM
	if _, err := fmt.Sscanf(s.stepEntry.Text, "%f", &step); err != nil || step <= 0 {
		step = 0.25
	}
	box := s.mosaicView.box
	opts := mosaic.Options{
		Mode:    s.selectedMode(),
		StepM:   step,
		Ref:     mosaic.RefMode(s.refSelect.Selected),
		Nodata:  s.opts.Nodata,
		Z0:      s.opts.Z0,
		TileBox: &box,
	}
	return folder, opts, nil
}

func (s *stage) onSend() {
	folder, opts, err := s.buildOpts()
	if err != nil {
		s.status.SetText(err.Error())
		return
	}
	s.status.SetText("Building mosaic and sending…")
	s.sendBtn.Disable()
	go func() {
		arr, meta, err := mosaic.Build(folder, opts)
		s.sendBtn.Enable()
		if err != nil {
			s.status.SetText("Error: " + err.Error())
			dialog.ShowError(err, s.win)
			return
		}
		if len(arr.Shape) == 2 {
			arr.Shape = []int{1, arr.Shape[0], arr.Shape[1]}
		}
		s.pub.SetStack(arr, true)
		s.status.SetText(fmt.Sprintf(
			"Pushed %v %v (%.1f MB) to BLITZ.\nStream should already be connected to %s.",
			meta["mode"], arr.Shape, float64(arr.NBytes())/(1024*1024), s.pub.ConnectHint(),
		))
	}()
}

func (s *stage) onSave() {
	folder, opts, err := s.buildOpts()
	if err != nil {
		s.status.SetText(err.Error())
		return
	}
	dialog.ShowFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil || uc == nil {
			return
		}
		path := uc.URI().Path()
		_ = uc.Close()
		s.status.SetText("Saving…")
		go func() {
			arr, meta, err := mosaic.Build(folder, opts)
			if err != nil {
				s.status.SetText("Error: " + err.Error())
				dialog.ShowError(err, s.win)
				return
			}
			f, err := createFile(path)
			if err != nil {
				dialog.ShowError(err, s.win)
				return
			}
			err = npy.Write(f, arr)
			_ = f.Close()
			if err != nil {
				dialog.ShowError(err, s.win)
				return
			}
			side := path[:len(path)-len(filepath.Ext(path))] + ".json"
			_ = mosaic.WriteMeta(side, meta, filepath.Base(path))
			s.status.SetText("Wrote " + path)
		}()
	}, s.win)
}

// --- mosaic view (image + tile rectangle) ---

type mosaicView struct {
	widget.BaseWidget
	img         *image.RGBA
	raster      *canvas.Raster
	rows, cols  int
	labels      map[[2]int]string
	box         [4]int
	dragOrigin  *[2]int
	onChange    func([4]int)
	imgW, imgH  int
}

func newMosaicView(onChange func([4]int)) *mosaicView {
	m := &mosaicView{
		rows: 1, cols: 1, box: [4]int{0, 0, 0, 0},
		labels: map[[2]int]string{}, onChange: onChange,
	}
	m.raster = canvas.NewRaster(m.draw)
	m.ExtendBaseWidget(m)
	return m
}

func (m *mosaicView) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.raster)
}

func (m *mosaicView) MinSize() fyne.Size { return fyne.NewSize(320, 320) }

func (m *mosaicView) setMosaic(img *image.RGBA, rows, cols int, labels map[[2]int]string) {
	m.img = img
	m.rows, m.cols = rows, cols
	m.labels = labels
	m.box = [4]int{0, 0, rows - 1, cols - 1}
	if img != nil {
		m.imgW, m.imgH = img.Bounds().Dx(), img.Bounds().Dy()
	}
	m.raster.Refresh()
	m.Refresh()
}

func (m *mosaicView) selectAll() {
	m.box = [4]int{0, 0, m.rows - 1, m.cols - 1}
	m.raster.Refresh()
	if m.onChange != nil {
		m.onChange(m.box)
	}
}

func (m *mosaicView) draw(w, h int) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	// dark background
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetRGBA(x, y, color.RGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	if m.img == nil || m.imgW < 1 || m.imgH < 1 {
		return out
	}
	scale := float32(w) / float32(m.imgW)
	if float32(h)/float32(m.imgH) < scale {
		scale = float32(h) / float32(m.imgH)
	}
	nw := int(float32(m.imgW) * scale)
	nh := int(float32(m.imgH) * scale)
	ox := (w - nw) / 2
	oy := (h - nh) / 2
	// nearest-neighbour blit
	for y := 0; y < nh; y++ {
		sy := y * m.imgH / nh
		for x := 0; x < nw; x++ {
			sx := x * m.imgW / nw
			out.SetRGBA(ox+x, oy+y, m.img.RGBAAt(sx, sy))
		}
	}
	// selection overlay
	r0, c0, r1, c1 := mosaic.NormTileBox(m.box, m.rows, m.cols)
	cellW := float32(nw) / float32(m.cols)
	cellH := float32(nh) / float32(m.rows)
	x0 := ox + int(float32(c0)*cellW)
	y0 := oy + int(float32(r0)*cellH)
	x1 := ox + int(float32(c1+1)*cellW)
	y1 := oy + int(float32(r1+1)*cellH)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			c := out.RGBAAt(x, y)
			c.R = uint8(min255(int(c.R) + 40))
			c.G = uint8(min255(int(c.G) + 40))
			c.B = uint8(min255(int(c.B) + 20))
			out.SetRGBA(x, y, c)
		}
	}
	drawRect(out, x0, y0, x1-1, y1-1)
	return out
}

func min255(v int) int {
	if v > 255 {
		return 255
	}
	return v
}

func drawRect(img *image.RGBA, x0, y0, x1, y1 int) {
	b := img.Bounds()
	for x := x0; x <= x1; x++ {
		if x >= b.Min.X && x < b.Max.X {
			if y0 >= b.Min.Y && y0 < b.Max.Y {
				img.SetRGBA(x, y0, color.RGBA{R: 255, G: 224, B: 102, A: 255})
			}
			if y1 >= b.Min.Y && y1 < b.Max.Y {
				img.SetRGBA(x, y1, color.RGBA{R: 255, G: 224, B: 102, A: 255})
			}
		}
	}
	for y := y0; y <= y1; y++ {
		if y >= b.Min.Y && y < b.Max.Y {
			if x0 >= b.Min.X && x0 < b.Max.X {
				img.SetRGBA(x0, y, color.RGBA{R: 255, G: 224, B: 102, A: 255})
			}
			if x1 >= b.Min.X && x1 < b.Max.X {
				img.SetRGBA(x1, y, color.RGBA{R: 255, G: 224, B: 102, A: 255})
			}
		}
	}
}

func (m *mosaicView) tileAt(p fyne.Position, clamp bool) *[2]int {
	sz := m.Size()
	w, h := int(sz.Width), int(sz.Height)
	if m.imgW < 1 || m.imgH < 1 || m.rows < 1 || m.cols < 1 {
		return nil
	}
	scale := float32(w) / float32(m.imgW)
	if float32(h)/float32(m.imgH) < scale {
		scale = float32(h) / float32(m.imgH)
	}
	nw := float32(m.imgW) * scale
	nh := float32(m.imgH) * scale
	ox := (float32(w) - nw) / 2
	oy := (float32(h) - nh) / 2
	x, y := p.X, p.Y
	if x < ox || y < oy || x > ox+nw || y > oy+nh {
		if !clamp {
			return nil
		}
		if x < ox {
			x = ox
		}
		if y < oy {
			y = oy
		}
		if x > ox+nw {
			x = ox + nw
		}
		if y > oy+nh {
			y = oy + nh
		}
	}
	fx := (x - ox) / nw
	fy := (y - oy) / nh
	c := int(fx * float32(m.cols))
	r := int(fy * float32(m.rows))
	if c >= m.cols {
		c = m.cols - 1
	}
	if r >= m.rows {
		r = m.rows - 1
	}
	if c < 0 {
		c = 0
	}
	if r < 0 {
		r = 0
	}
	return &[2]int{r, c}
}

func (m *mosaicView) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button != desktop.MouseButtonPrimary {
		return
	}
	t := m.tileAt(ev.Position, false)
	if t == nil {
		return
	}
	m.dragOrigin = t
	m.box = [4]int{t[0], t[1], t[0], t[1]}
	m.raster.Refresh()
}

func (m *mosaicView) MouseUp(*desktop.MouseEvent) {
	if m.dragOrigin != nil {
		m.dragOrigin = nil
		if m.onChange != nil {
			m.onChange(m.box)
		}
	}
}

func (m *mosaicView) MouseMoved(ev *desktop.MouseEvent) {
	if m.dragOrigin == nil {
		return
	}
	t := m.tileAt(ev.Position, true)
	if t == nil {
		return
	}
	r0, r1 := m.dragOrigin[0], t[0]
	c0, c1 := m.dragOrigin[1], t[1]
	if r0 > r1 {
		r0, r1 = r1, r0
	}
	if c0 > c1 {
		c0, c1 = c1, c0
	}
	m.box = [4]int{r0, c0, r1, c1}
	m.raster.Refresh()
}

func (m *mosaicView) MouseIn(*desktop.MouseEvent)  {}
func (m *mosaicView) MouseOut()                    {}

// Cursor implements desktop.Hoverable/Cursorable if needed — default arrow.
var (
	_ desktop.Mouseable = (*mosaicView)(nil)
	_ desktop.Hoverable = (*mosaicView)(nil)
)
