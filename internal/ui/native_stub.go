//go:build !fyne

// Package ui stub when no native GUI is linked.
// Product download: PyInstaller binary from GitHub Releases (DnD stage).
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

// Run explains that the Go binary is CLI/hub; the product GUI is the release binary.
func Run(pub *hub.Publisher, opts Options) {
	_ = pub
	fmt.Fprintln(os.Stderr, "dgm-mosaic CLI: hub is up; this build has no window.")
	fmt.Fprintln(os.Stderr, "Product UI (drag-drop + tile preview): download DGM from GitHub Releases,")
	fmt.Fprintln(os.Stderr, "or: uv run dgm-mosaic")
	if opts.Prefill != "" {
		fmt.Fprintf(os.Stderr, "Prefill path was: %s\n", opts.Prefill)
	}
	os.Exit(2)
}
