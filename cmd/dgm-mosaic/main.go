package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PiMaV/dgm-mosaic/internal/hub"
	"github.com/PiMaV/dgm-mosaic/internal/mosaic"
	"github.com/PiMaV/dgm-mosaic/internal/npy"
	"github.com/PiMaV/dgm-mosaic/internal/ui"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("dgm-mosaic: ")

	host := flag.String("host", "127.0.0.1", "bind host")
	port := flag.Int("port", 5056, "bind port")
	token := flag.String("token", "dgm", "HTTP token for .npy GET")
	dtype := flag.String("dtype", "u16cm", "u16cm|u8stretch|u8step|f32")
	stepM := flag.Float64("step-m", 0.25, "u8step metres")
	ref := flag.String("ref", "min", "u8step ref: min|mean")
	nodata := flag.Float64("nodata", mosaic.NodataDefault, "nodata value in source TIFF")
	z0Flag := flag.String("z0", "", "u16cm base metres (default: floor(zmin))")
	out := flag.String("o", "", "output .npy path (CLI mode)")
	gui := flag.Bool("gui", false, "open native GUI (needs -tags fyne build; else points to converters)")
	dummy := flag.Bool("dummy", false, "seed a dummy cube then open GUI (hub handshake)")
	flag.Parse()

	z0 := math.NaN()
	if *z0Flag != "" {
		var err error
		z0, err = strconv.ParseFloat(*z0Flag, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: bad --z0: %v\n", err)
			os.Exit(2)
		}
	}

	input := flag.Arg(0)
	cliMode := input != "" && !*gui && !*dummy
	if cliMode {
		if err := runCLI(input, *out, *dtype, *stepM, *ref, *nodata, z0); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	pub := hub.New(*host, *port, *token, "mosaic.npy")
	if *dummy {
		pub.SetStack(hub.DummyStack(), false)
	}
	go func() {
		log.Printf("hub %s", pub.ConnectHint())
		if err := pub.ListenAndServe(nil); err != nil {
			log.Printf("hub stopped: %v", err)
		}
	}()

	fmt.Printf("BLITZ Stream: %s\n", pub.ConnectHint())
	ui.Run(pub, ui.Options{
		DefaultDtype: *dtype,
		StepM:        *stepM,
		Ref:          *ref,
		Nodata:       *nodata,
		Z0:           z0,
		Prefill:      input,
	})
}

func runCLI(input, out, dtype string, stepM float64, ref string, nodata, z0 float64) error {
	opts := mosaic.Options{
		Mode:   mosaic.Mode(dtype),
		StepM:  stepM,
		Ref:    mosaic.RefMode(ref),
		Nodata: nodata,
		Z0:     z0,
	}
	arr, meta, err := mosaic.Build(input, opts)
	if err != nil {
		return err
	}
	path := out
	if path == "" {
		path = mosaic.DefaultOutputPath(input)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := npy.Write(f, arr); err != nil {
		return err
	}
	side := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
	if err := mosaic.WriteMeta(side, meta, filepath.Base(path)); err != nil {
		return err
	}
	h, w := arr.Shape[0], arr.Shape[1]
	fmt.Printf("Mosaic %d×%d px  (%v m)  %s\n", w, h, meta["pixel_m"], path)
	fmt.Printf("  dtype %s: %s  %.1f MB\n", meta["mode"], arr.DType, float64(arr.NBytes())/(1024*1024))
	return nil
}
