package mosaic

import (
	"fmt"
	"math"

	"github.com/PiMaV/dgm-mosaic/internal/npy"
)

func Quantize(z []float32, h, w int, opts Options) (npy.Array, map[string]any, error) {
	mode := opts.Mode
	if mode == "" {
		mode = ModeF32
	}
	switch mode {
	case ModeU16dm:
		return quantizeU16dm(z, h, w, opts.Z0)
	case ModeU8stretch:
		return npy.Array{}, nil, fmt.Errorf("u8stretch removed: it remapped heights — use f32 or u8step")
	case ModeU8step:
		step := opts.StepM
		if step <= 0 {
			step = 0.25
		}
		ref := opts.Ref
		if ref == "" {
			ref = RefMin
		}
		return quantizeU8step(z, h, w, step, ref)
	case ModeF32:
		return quantizeF32(z, h, w)
	default:
		return npy.Array{}, nil, fmt.Errorf("unknown mode %s", mode)
	}
}

func validMask(z []float32) []bool {
	m := make([]bool, len(z))
	any := false
	for i, v := range z {
		ok := !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
		m[i] = ok
		if ok {
			any = true
		}
	}
	_ = any
	return m
}

func hasValid(m []bool) bool {
	for _, v := range m {
		if v {
			return true
		}
	}
	return false
}

func nanMinMax(z []float32, m []bool) (float64, float64) {
	minV, maxV := math.Inf(1), math.Inf(-1)
	for i, ok := range m {
		if !ok {
			continue
		}
		v := float64(z[i])
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	return minV, maxV
}

func quantizeU16dm(z []float32, h, w int, z0 float64) (npy.Array, map[string]any, error) {
	m := validMask(z)
	if !hasValid(m) {
		return npy.Array{}, nil, fmt.Errorf("mosaic has no valid pixels")
	}
	zMin, zMax := nanMinMax(z, m)
	z0m := z0
	if math.IsNaN(z0m) {
		z0m = math.Floor(zMin)
	}
	out := make([]uint16, len(z))
	dm := make([]float64, len(z))
	for i, ok := range m {
		if !ok {
			continue
		}
		dm[i] = math.Round((float64(z[i]) - z0m) * 10.0)
	}
	needBump := false
	for i, ok := range m {
		if ok && dm[i] < 1 {
			needBump = true
			break
		}
	}
	if needBump {
		z0m -= 0.1
		for i, ok := range m {
			if !ok {
				continue
			}
			dm[i] = math.Round((float64(z[i]) - z0m) * 10.0)
		}
	}
	for i, ok := range m {
		if ok && (dm[i] < 1 || dm[i] > 65535) {
			relief := zMax - z0m
			return npy.Array{}, nil, fmt.Errorf(
				"u16dm: relief %.1f m from z0=%g does not fit fixed 1 dm/DN (max 6553.5 m). Use f32 — do not rescale heights to fit uint16",
				relief, z0m,
			)
		}
	}
	for i, ok := range m {
		if !ok {
			continue
		}
		out[i] = uint16(dm[i])
	}
	meta := map[string]any{
		"mode": "u16dm", "dtype": "uint16", "nodata": 0,
		"z0_m": z0m, "scale_m": 0.1, "z_min_m": zMin, "z_max_m": zMax,
		"clipped_pixels": 0, "unit": "dm",
		"reconstruct": "z_m = z0_m + pixel * scale_m  (pixel 0 = nodata)",
	}
	return npy.FromUint16LE([]int{h, w}, out), meta, nil
}

func quantizeU8stretch(z []float32, h, w int) (npy.Array, map[string]any, error) {
	m := validMask(z)
	if !hasValid(m) {
		return npy.Array{}, nil, fmt.Errorf("mosaic has no valid pixels")
	}
	zMin, zMax := nanMinMax(z, m)
	span := zMax - zMin
	out := make([]uint8, len(z))
	scale := 0.0
	if span <= 0 {
		for i, ok := range m {
			if ok {
				out[i] = 1
			}
		}
	} else {
		scale = span / 254.0
		for i, ok := range m {
			if !ok {
				continue
			}
			scaled := 1.0 + math.Round((float64(z[i])-zMin)/span*254.0)
			if scaled < 1 {
				scaled = 1
			}
			if scaled > 255 {
				scaled = 255
			}
			out[i] = uint8(scaled)
		}
	}
	meta := map[string]any{
		"mode": "u8stretch", "dtype": "uint8", "nodata": 0,
		"z0_m": zMin, "scale_m": scale, "z_min_m": zMin, "z_max_m": zMax,
		"clipped_pixels": 0,
		"reconstruct":    "z_m ≈ z0_m + (pixel - 1) * scale_m  (pixel 0 = nodata)",
	}
	return npy.FromUint8([]int{h, w}, out), meta, nil
}

func quantizeU8step(z []float32, h, w int, stepM float64, ref RefMode) (npy.Array, map[string]any, error) {
	if stepM <= 0 {
		return npy.Array{}, nil, fmt.Errorf("--step-m must be > 0")
	}
	m := validMask(z)
	if !hasValid(m) {
		return npy.Array{}, nil, fmt.Errorf("mosaic has no valid pixels")
	}
	zMin, zMax := nanMinMax(z, m)
	zRef := zMin
	if ref == RefMean {
		sum := 0.0
		n := 0
		for i, ok := range m {
			if ok {
				sum += float64(z[i])
				n++
			}
		}
		zRef = sum / float64(n)
	}
	out := make([]uint8, len(z))
	nClip := 0
	var reconstruct string
	z0m := zRef
	if ref == RefMin {
		reconstruct = "z_m = z0_m + (pixel - 1) * scale_m  (pixel 0 = nodata)"
		for i, ok := range m {
			if !ok {
				continue
			}
			raw := math.Round((float64(z[i])-zRef)/stepM) + 1.0
			if raw < 1 || raw > 255 {
				nClip++
			}
			out[i] = uint8(raw) // may be wrong if clip — checked below
			if raw >= 1 && raw <= 255 {
				out[i] = uint8(raw)
			}
		}
	} else {
		reconstruct = "z_m = z0_m + (pixel - 128) * scale_m  (pixel 0 = nodata)"
		for i, ok := range m {
			if !ok {
				continue
			}
			raw := math.Round((float64(z[i])-zRef)/stepM) + 128.0
			if raw < 1 || raw > 255 {
				nClip++
			}
			if raw >= 1 && raw <= 255 {
				out[i] = uint8(raw)
			}
		}
	}
	relief := zMax - zMin
	suggested := stepM
	if relief > 0 {
		suggested = relief / 254.0
	}
	if nClip > 0 {
		return npy.Array{}, nil, fmt.Errorf(
			"u8step: %d pixel(s) outside 1…255 at step %g m (relief %.1f m). Increase step (e.g. ≥ %.3f m) or use f32 — heights are not rescaled to fit",
			nClip, stepM, relief, suggested,
		)
	}
	meta := map[string]any{
		"mode": "u8step", "dtype": "uint8", "nodata": 0, "ref": string(ref),
		"z0_m": z0m, "scale_m": stepM, "z_min_m": zMin, "z_max_m": zMax,
		"clipped_pixels": 0, "reconstruct": reconstruct,
	}
	return npy.FromUint8([]int{h, w}, out), meta, nil
}

func quantizeF32(z []float32, h, w int) (npy.Array, map[string]any, error) {
	m := validMask(z)
	zMin, zMax := math.NaN(), math.NaN()
	if hasValid(m) {
		zMin, zMax = nanMinMax(z, m)
	}
	cp := append([]float32(nil), z...)
	meta := map[string]any{
		"mode": "f32", "dtype": "float32", "nodata": nil,
		"z0_m": 0.0, "scale_m": 1.0, "z_min_m": zMin, "z_max_m": zMax,
		"clipped_pixels": 0, "reconstruct": "pixel is metres; NaN = nodata",
	}
	return npy.FromFloat32LE([]int{h, w}, cp), meta, nil
}
