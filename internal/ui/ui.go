// Package ui serves the embedded DGM mosaic web stage.
package ui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	_ "embed"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/PiMaV/dgm-mosaic/internal/hub"
	"github.com/PiMaV/dgm-mosaic/internal/mosaic"
	"github.com/PiMaV/dgm-mosaic/internal/npy"
)

//go:embed web/index.html
var indexHTML []byte

type Options struct {
	DefaultDtype string
	StepM        float64
	Ref          string
	Nodata       float64
	Z0           float64
}

type Stage struct {
	pub  *hub.Publisher
	opts Options

	mu     sync.Mutex
	folder string
}

func New(pub *hub.Publisher, opts Options) *Stage {
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
	return &Stage{pub: pub, opts: opts}
}

func (s *Stage) PrefillFolder(path string) {
	s.mu.Lock()
	s.folder = path
	s.mu.Unlock()
}

func (s *Stage) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/browse", s.handleBrowse)
	mux.HandleFunc("/api/open", s.handleOpen)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/save", s.handleSave)
	return mux
}

func (s *Stage) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	folder := s.folder
	s.mu.Unlock()
	writeJSON(w, map[string]any{
		"folder":  folder,
		"connect": s.pub.ConnectHint(),
		"token":   s.pub.Token,
		"ready":   s.pub.HasStack(),
		"dtype":   s.opts.DefaultDtype,
		"step_m":  s.opts.StepM,
		"ref":     s.opts.Ref,
	})
}

func (s *Stage) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	dir, err := pickFolder()
	if err != nil {
		writeErr(w, err)
		return
	}
	if dir == "" {
		writeJSON(w, map[string]any{"cancelled": true})
		return
	}
	s.mu.Lock()
	s.folder = dir
	s.mu.Unlock()
	writeJSON(w, map[string]any{"folder": dir})
}

func (s *Stage) handleOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if body.Folder == "" {
		writeErr(w, fmt.Errorf("folder required"))
		return
	}
	layout, err := mosaic.InspectFolder(body.Folder)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.mu.Lock()
	s.folder = body.Folder
	s.mu.Unlock()
	labels := map[string]string{}
	for key, stub := range layout.Cells {
		labels[fmt.Sprintf("%d,%d", key[0], key[1])] = stub.LabelKm()
	}
	writeJSON(w, map[string]any{
		"folder":    body.Folder,
		"tile_rows": layout.TileRows,
		"tile_cols": layout.TileCols,
		"height":    layout.Height,
		"width":     layout.Width,
		"pixel_m":   layout.PixelM,
		"holes":     layout.Holes,
		"crs":       layout.CRS,
		"tiles":     len(layout.Stubs),
		"labels":    labels,
		"sizes": map[string]string{
			"u16cm":     mosaic.FmtMB(layout.NBytes(mosaic.ModeU16cm)),
			"u8stretch": mosaic.FmtMB(layout.NBytes(mosaic.ModeU8stretch)),
			"u8step":    mosaic.FmtMB(layout.NBytes(mosaic.ModeU8step)),
			"f32":       mosaic.FmtMB(layout.NBytes(mosaic.ModeF32)),
		},
	})
}

func (s *Stage) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Folder string `json:"folder"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	if body.Folder == "" {
		body.Folder = s.folder
	}
	s.mu.Unlock()
	layout, err := mosaic.InspectFolder(body.Folder)
	if err != nil {
		writeErr(w, err)
		return
	}
	raw := map[string][]float32{}
	var th, tw int
	for key, stub := range layout.Cells {
		pix, h, wdim, err := mosaic.TileThumbnail(stub.Path, s.opts.Nodata, mosaic.ThumbEdge)
		if err != nil {
			writeErr(w, err)
			return
		}
		th, tw = h, wdim
		raw[fmt.Sprintf("%d,%d", key[0], key[1])] = pix
	}
	unitNamed := mosaic.FloatThumbsToUnit(raw)
	unitKeyed := map[[2]int][]float32{}
	for k, v := range unitNamed {
		var r, c int
		_, _ = fmt.Sscanf(k, "%d,%d", &r, &c)
		unitKeyed[[2]int{r, c}] = v
	}
	rgb := mosaic.ComposePreviewRGB(layout.TileRows, layout.TileCols, unitKeyed, th, tw)
	var png bytes.Buffer
	if err := writePNG(&png, rgb, layout.TileCols*tw, layout.TileRows*th); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, map[string]any{
		"width":  layout.TileCols * tw,
		"height": layout.TileRows * th,
		"rows":   layout.TileRows,
		"cols":   layout.TileCols,
		"png":    "data:image/png;base64," + base64.StdEncoding.EncodeToString(png.Bytes()),
	})
}

type buildBody struct {
	Folder  string `json:"folder"`
	Dtype   string `json:"dtype"`
	StepM   float64 `json:"step_m"`
	Ref     string `json:"ref"`
	Box     []int  `json:"box"` // r0,c0,r1,c1
	OutPath string `json:"out_path"`
}

func (s *Stage) parseOpts(body buildBody) (string, mosaic.Options, error) {
	folder := body.Folder
	s.mu.Lock()
	if folder == "" {
		folder = s.folder
	}
	s.mu.Unlock()
	if folder == "" {
		return "", mosaic.Options{}, fmt.Errorf("no folder loaded")
	}
	opts := mosaic.Options{
		Mode:   mosaic.Mode(body.Dtype),
		StepM:  body.StepM,
		Ref:    mosaic.RefMode(body.Ref),
		Nodata: s.opts.Nodata,
		Z0:     s.opts.Z0,
	}
	if opts.Mode == "" {
		opts.Mode = mosaic.Mode(s.opts.DefaultDtype)
	}
	if opts.StepM == 0 {
		opts.StepM = s.opts.StepM
	}
	if opts.Ref == "" {
		opts.Ref = mosaic.RefMode(s.opts.Ref)
	}
	if len(body.Box) == 4 {
		box := [4]int{body.Box[0], body.Box[1], body.Box[2], body.Box[3]}
		opts.TileBox = &box
	}
	return folder, opts, nil
}

func (s *Stage) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body buildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	folder, opts, err := s.parseOpts(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	arr, meta, err := mosaic.Build(folder, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Ensure T×H×W for viewers that expect a stack: promote 2D → 1×H×W
	if len(arr.Shape) == 2 {
		arr.Shape = []int{1, arr.Shape[0], arr.Shape[1]}
	}
	s.pub.SetStack(arr, true)
	writeJSON(w, map[string]any{
		"ok":      true,
		"shape":   arr.Shape,
		"dtype":   arr.DType,
		"mb":      float64(arr.NBytes()) / (1024 * 1024),
		"mode":    meta["mode"],
		"connect": s.pub.ConnectHint(),
	})
}

func (s *Stage) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var body buildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	folder, opts, err := s.parseOpts(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	arr, meta, err := mosaic.Build(folder, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	path := body.OutPath
	if path == "" {
		path = mosaic.DefaultOutputPath(folder)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		writeErr(w, err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := npy.Write(f, arr); err != nil {
		_ = f.Close()
		writeErr(w, err)
		return
	}
	_ = f.Close()
	side := path[:len(path)-len(filepath.Ext(path))] + ".json"
	_ = mosaic.WriteMeta(side, meta, filepath.Base(path))
	writeJSON(w, map[string]any{"ok": true, "path": path, "meta": side})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func pickFolder() (string, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("zenity"); err == nil {
			out, err := exec.Command("zenity", "--file-selection", "--directory", "--title=DGM tile folder").Output()
			if err != nil {
				// cancel exits non-zero
				if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
					return "", nil
				}
				return "", err
			}
			return trimNL(string(out)), nil
		}
		if _, err := exec.LookPath("kdialog"); err == nil {
			out, err := exec.Command("kdialog", "--getexistingdirectory", ".").Output()
			if err != nil {
				return "", nil
			}
			return trimNL(string(out)), nil
		}
		return "", fmt.Errorf("install zenity or kdialog for the folder dialog")
	case "darwin":
		out, err := exec.Command("osascript", "-e", `return POSIX path of (choose folder with prompt "DGM tile folder")`).Output()
		if err != nil {
			return "", nil
		}
		return trimNL(string(out)), nil
	case "windows":
		// PowerShell FolderBrowserDialog
		ps := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='DGM tile folder'; if($d.ShowDialog() -eq 'OK'){$d.SelectedPath}`
		out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
		if err != nil {
			return "", err
		}
		return trimNL(string(out)), nil
	default:
		return "", fmt.Errorf("folder dialog unsupported on %s", runtime.GOOS)
	}
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
