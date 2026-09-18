package mosaic

import "math"

// DownsampleHeight resizes a 2D height field (area average of finite samples).
func DownsampleHeight(z []float32, h, w, maxEdge int) ([]float32, int, int) {
	if maxEdge <= 0 {
		maxEdge = ThumbEdge
	}
	m := h
	if w > m {
		m = w
	}
	if m < 1 {
		m = 1
	}
	scale := float64(maxEdge) / float64(m)
	nh := int(math.Round(float64(h) * scale))
	nw := int(math.Round(float64(w) * scale))
	if nh < 1 {
		nh = 1
	}
	if nw < 1 {
		nw = 1
	}
	if nh == h && nw == w {
		return append([]float32(nil), z...), h, w
	}
	out := make([]float32, nh*nw)
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			y0 := y * h / nh
			y1 := (y + 1) * h / nh
			x0 := x * w / nw
			x1 := (x + 1) * w / nw
			if y1 <= y0 {
				y1 = y0 + 1
			}
			if x1 <= x0 {
				x1 = x0 + 1
			}
			sum := 0.0
			n := 0
			for yy := y0; yy < y1 && yy < h; yy++ {
				for xx := x0; xx < x1 && xx < w; xx++ {
					v := z[yy*w+xx]
					if math.IsNaN(float64(v)) {
						continue
					}
					sum += float64(v)
					n++
				}
			}
			if n == 0 {
				out[y*nw+x] = float32(math.NaN())
			} else {
				out[y*nw+x] = float32(sum / float64(n))
			}
		}
	}
	return out, nh, nw
}

func TileThumbnail(path string, nodata float64, maxEdge int) ([]float32, int, int, error) {
	t, err := LoadTile(path, nodata)
	if err != nil {
		return nil, 0, 0, err
	}
	out, nh, nw := DownsampleHeight(t.Array, t.H, t.W, maxEdge)
	return out, nh, nw, nil
}

func FloatThumbsToUnit(thumbs map[string][]float32) map[string][]float32 {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, t := range thumbs {
		for _, v := range t {
			if math.IsNaN(float64(v)) {
				continue
			}
			fv := float64(v)
			if fv < lo {
				lo = fv
			}
			if fv > hi {
				hi = fv
			}
		}
	}
	out := make(map[string][]float32, len(thumbs))
	if math.IsInf(lo, 1) {
		for k, t := range thumbs {
			u := make([]float32, len(t))
			for i := range u {
				u[i] = float32(math.NaN())
			}
			out[k] = u
		}
		return out
	}
	span := math.Max(hi-lo, 1e-6)
	for k, t := range thumbs {
		u := make([]float32, len(t))
		for i, v := range t {
			if math.IsNaN(float64(v)) {
				u[i] = float32(math.NaN())
			} else {
				u[i] = float32((float64(v) - lo) / span)
			}
		}
		out[k] = u
	}
	return out
}

// DivergingRGB maps unit [0,1] to blue–white–red; non-finite → black.
func DivergingRGB(unit []float32, h, w int) []uint8 {
	rgb := make([]uint8, h*w*3)
	for i, t := range unit {
		if math.IsNaN(float64(t)) || math.IsInf(float64(t), 0) {
			continue
		}
		x := float64(t)
		if x < 0 {
			x = 0
		}
		if x > 1 {
			x = 1
		}
		var r, g, b float64
		if x <= 0.5 {
			s := x * 2
			r, g, b = s, s, 1
		} else {
			s := (x - 0.5) * 2
			r, g, b = 1, 1-s, 1-s
		}
		rgb[i*3+0] = uint8(math.Min(255, math.Round(r*255)))
		rgb[i*3+1] = uint8(math.Min(255, math.Round(g*255)))
		rgb[i*3+2] = uint8(math.Min(255, math.Round(b*255)))
	}
	return rgb
}

func ComposePreviewRGB(rows, cols int, unitThumbs map[[2]int][]float32, th, tw int) []uint8 {
	if rows < 1 || cols < 1 || th < 1 || tw < 1 {
		return []uint8{0, 0, 0}
	}
	rgb := make([]uint8, rows*th*cols*tw*3)
	for key, unit := range unitThumbs {
		r, c := key[0], key[1]
		if r < 0 || c < 0 || r >= rows || c >= cols {
			continue
		}
		cell := DivergingRGB(unit, th, tw)
		for y := 0; y < th; y++ {
			for x := 0; x < tw; x++ {
				di := ((r*th+y)*cols*tw + (c*tw + x)) * 3
				si := (y*tw + x) * 3
				rgb[di], rgb[di+1], rgb[di+2] = cell[si], cell[si+1], cell[si+2]
			}
		}
	}
	return rgb
}
