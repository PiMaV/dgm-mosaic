//go:build !fyne

// Package ui defaults to a stub when Fyne is not built in.
// Product DnD GUI: sibling converters repo — `uv run dgm-mosaic`.
// Optional native attempt: `go build -tags fyne` (needs OpenGL / X11 / Fyne deps).
package ui

import (
	"fmt"
	"os"

	"github.com/PiMaV/dgm-mosaic/internal/hub"
)

type Options struct {
	DefaultDtype string
	StepM        float64
	Ref          string
	Nodata       float64
	Z0           float64
	Prefill      string
}

// Run prints where the real stage lives. Hub may already be running in the background.
func Run(pub *hub.Publisher, opts Options) {
	_ = pub
	fmt.Fprintln(os.Stderr, "DGM Go binary: CLI / hub only in this build (no embedded browser UI).")
	fmt.Fprintln(os.Stderr, "Product UI — tile preview, rectangle select, drag-drop folder:")
	fmt.Fprintln(os.Stderr, "  cd ../converters && uv run dgm-mosaic")
	fmt.Fprintln(os.Stderr, "Optional Fyne (experimental): go build -tags fyne ./cmd/dgm-mosaic")
	if opts.Prefill != "" {
		fmt.Fprintf(os.Stderr, "Prefill path was: %s\n", opts.Prefill)
	}
	os.Exit(2)
}
