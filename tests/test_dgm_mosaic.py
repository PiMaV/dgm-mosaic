"""Unit tests for dgm_mosaic.mosaic (in-memory tiles)."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

import numpy as np

from dgm_mosaic.mosaic import (
    Tile,
    TileStub,
    convert,
    diverging_rgb,
    estimate_npy_bytes,
    float_thumbs_to_unit,
    layout_from_stubs,
    mosaic_for_tile_box,
    mosaic_tiles,
    parse_dgm_name,
    parse_tfw,
    quantize,
)


def _tile(name: str, arr: np.ndarray, west: float, north: float, pixel_m: float = 0.25) -> Tile:
    parsed = parse_dgm_name(Path(name).stem)
    return Tile(
        path=Path(name),
        array=np.asarray(arr, dtype=np.float32),
        west=west,
        north=north,
        pixel_m=pixel_m,
        name=parsed,
    )


class ParseTests(unittest.TestCase):
    def test_dgm025_name(self) -> None:
        n = parse_dgm_name("dgm025_32_505_5364_1_bw_2020")
        assert n is not None
        self.assertEqual(n.zone, 32)
        self.assertEqual(n.e_km, 505)
        self.assertEqual(n.n_km, 5364)
        self.assertAlmostEqual(n.pixel_m, 0.25)

    def test_dgm1_name(self) -> None:
        n = parse_dgm_name("dgm1_32_500_5400_1")
        assert n is not None
        self.assertAlmostEqual(n.pixel_m, 1.0)

    def test_tfw_ul_corner(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "a.tfw"
            p.write_text(
                "0.2500000000\n0.0000000000\n0.0000000000\n-0.2500000000\n"
                "505000.1250000000\n5364999.8750000000\n",
                encoding="utf-8",
            )
            w = parse_tfw(p)
            self.assertAlmostEqual(w.west, 505000.0)
            self.assertAlmostEqual(w.north, 5365000.0)
            self.assertAlmostEqual(w.pixel_m, 0.25)

    def test_tfw_rejects_rotation(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "a.tfw"
            p.write_text("0.25\n0.1\n0\n-0.25\n0\n0\n", encoding="utf-8")
            with self.assertRaises(ValueError):
                parse_tfw(p)


class MosaicTests(unittest.TestCase):
    def test_2x2_abutting(self) -> None:
        a = np.array([[1.0, 2.0], [3.0, 4.0]], dtype=np.float32)
        b = np.array([[5.0, 6.0], [7.0, 8.0]], dtype=np.float32)
        c = np.array([[9.0, 10.0], [11.0, 12.0]], dtype=np.float32)
        d = np.array([[13.0, 14.0], [15.0, 16.0]], dtype=np.float32)
        tiles = [
            _tile("sw.tif", a, west=0.0, north=0.5),
            _tile("se.tif", b, west=0.5, north=0.5),
            _tile("nw.tif", c, west=0.0, north=1.0),
            _tile("ne.tif", d, west=0.5, north=1.0),
        ]
        mosaic, info = mosaic_tiles(tiles)
        self.assertEqual(mosaic.shape, (4, 4))
        np.testing.assert_array_equal(mosaic[0:2, 0:2], c)
        np.testing.assert_array_equal(mosaic[0:2, 2:4], d)
        np.testing.assert_array_equal(mosaic[2:4, 0:2], a)
        np.testing.assert_array_equal(mosaic[2:4, 2:4], b)
        self.assertEqual(info["overlap_pixels"], 0)
        self.assertAlmostEqual(info["west"], 0.0)
        self.assertAlmostEqual(info["north"], 1.0)

    def test_name_km_corners_match_lgl(self) -> None:
        n = parse_dgm_name("dgm025_32_505_5364_1_bw_2020")
        assert n is not None
        self.assertEqual(n.e_km * 1000, 505000)
        self.assertEqual((n.n_km + 1) * 1000, 5365000)

    def test_off_grid_refused(self) -> None:
        a = np.ones((2, 2), dtype=np.float32)
        tiles = [
            _tile("a.tif", a, west=0.0, north=1.0),
            _tile("b.tif", a, west=0.01, north=1.0),
        ]
        with self.assertRaises(ValueError):
            mosaic_tiles(tiles)


class LayoutTests(unittest.TestCase):
    def test_2x2_grid_and_sizes(self) -> None:
        n_sw = parse_dgm_name("dgm025_32_505_5364_1")
        n_se = parse_dgm_name("dgm025_32_506_5364_1")
        n_nw = parse_dgm_name("dgm025_32_505_5365_1")
        n_ne = parse_dgm_name("dgm025_32_506_5365_1")
        stubs = [
            TileStub(Path("sw.tif"), 4000, 4000, 505000, 5365000, 0.25, n_sw),
            TileStub(Path("se.tif"), 4000, 4000, 506000, 5365000, 0.25, n_se),
            TileStub(Path("nw.tif"), 4000, 4000, 505000, 5366000, 0.25, n_nw),
            TileStub(Path("ne.tif"), 4000, 4000, 506000, 5366000, 0.25, n_ne),
        ]
        layout = layout_from_stubs(stubs)
        self.assertEqual((layout.tile_rows, layout.tile_cols), (2, 2))
        self.assertEqual((layout.height, layout.width), (8000, 8000))
        self.assertEqual(layout.holes, 0)
        self.assertEqual(layout.cells[(0, 0)].label_km, "505_5365")
        self.assertEqual(layout.cells[(1, 1)].label_km, "506_5364")
        self.assertEqual(layout.nbytes("u16dm"), 8000 * 8000 * 2)
        self.assertEqual(estimate_npy_bytes(8000, 8000, "u8step"), 64_000_000)
        self.assertEqual(layout.crs, "EPSG:25832")

    def test_hole_counted(self) -> None:
        n_sw = parse_dgm_name("dgm025_32_505_5364_1")
        n_ne = parse_dgm_name("dgm025_32_506_5365_1")
        stubs = [
            TileStub(Path("sw.tif"), 4000, 4000, 505000, 5365000, 0.25, n_sw),
            TileStub(Path("ne.tif"), 4000, 4000, 506000, 5366000, 0.25, n_ne),
        ]
        layout = layout_from_stubs(stubs)
        self.assertEqual(layout.holes, 2)
        self.assertIsNone(layout.cells.get((0, 0)))
        self.assertIsNone(layout.cells.get((1, 1)))


class BoxMosaicTests(unittest.TestCase):
    def test_missing_cell_is_zero(self) -> None:
        a = np.array([[1.0, 2.0], [3.0, 4.0]], dtype=np.float32)
        c = np.array([[9.0, 10.0], [11.0, 12.0]], dtype=np.float32)
        d = np.array([[13.0, 14.0], [15.0, 16.0]], dtype=np.float32)
        stubs = [
            TileStub(Path("sw.tif"), 2, 2, 0.0, 0.5, 0.25),
            TileStub(Path("nw.tif"), 2, 2, 0.0, 1.0, 0.25),
            TileStub(Path("ne.tif"), 2, 2, 0.5, 1.0, 0.25),
        ]
        layout = layout_from_stubs(stubs)
        self.assertEqual(layout.holes, 1)
        tiles = [
            _tile("sw.tif", a, west=0.0, north=0.5),
            _tile("nw.tif", c, west=0.0, north=1.0),
            _tile("ne.tif", d, west=0.5, north=1.0),
        ]
        mosaic, info = mosaic_for_tile_box(tiles, layout, (0, 0, 1, 1))
        self.assertEqual(mosaic.shape, (4, 4))
        np.testing.assert_array_equal(mosaic[0:2, 0:2], c)
        np.testing.assert_array_equal(mosaic[0:2, 2:4], d)
        np.testing.assert_array_equal(mosaic[2:4, 0:2], a)
        np.testing.assert_array_equal(mosaic[2:4, 2:4], np.zeros((2, 2)))
        self.assertEqual(info["zero_padded"], 1)

    def test_single_tile_box(self) -> None:
        c = np.array([[9.0, 10.0], [11.0, 12.0]], dtype=np.float32)
        d = np.array([[13.0, 14.0], [15.0, 16.0]], dtype=np.float32)
        stubs = [
            TileStub(Path("nw.tif"), 2, 2, 0.0, 1.0, 0.25),
            TileStub(Path("ne.tif"), 2, 2, 0.5, 1.0, 0.25),
        ]
        layout = layout_from_stubs(stubs)
        tiles = [
            _tile("nw.tif", c, west=0.0, north=1.0),
            _tile("ne.tif", d, west=0.5, north=1.0),
        ]
        mosaic, _info = mosaic_for_tile_box(tiles, layout, (0, 0, 0, 0))
        np.testing.assert_array_equal(mosaic, c)
        self.assertEqual(layout.nbytes_box("u16dm", (0, 0, 0, 0)), 2 * 2 * 2)


class QuantizeTests(unittest.TestCase):
    def test_u16dm_roundtrip(self) -> None:
        z = np.array([[398.7, 447.0], [np.nan, 410.0]], dtype=np.float32)
        q = quantize(z, "u16dm")
        self.assertEqual(q.array.dtype, np.uint16)
        self.assertEqual(int(q.array[1, 0]), 0)
        z0 = float(q.meta["z0_m"])
        scale = float(q.meta["scale_m"])
        self.assertEqual(scale, 0.1)
        recon = z0 + q.array.astype(np.float64) * scale
        self.assertAlmostEqual(recon[0, 0], 398.7, places=1)
        self.assertAlmostEqual(recon[0, 1], 447.0, places=1)
        self.assertGreater(int(q.array[0, 0]), 0)

    def test_u16dm_751_3_is_7513(self) -> None:
        z = np.array([[751.3]], dtype=np.float32)
        q = quantize(z, "u16dm", z0=0.0)
        self.assertEqual(int(q.array[0, 0]), 7513)
        self.assertEqual(float(q.meta["scale_m"]), 0.1)
        self.assertEqual(q.meta["unit"], "dm")
        recon = 0.0 + 7513 * 0.1
        self.assertAlmostEqual(recon, 751.3, places=1)

    def test_u16dm_no_auto_scale(self) -> None:
        # Relief larger than 6553.5 m from z0=0
        z = np.array([[0.0, 7000.0]], dtype=np.float32)
        with self.assertRaises(ValueError) as ctx:
            quantize(z, "u16dm", z0=0.0)
        self.assertIn("f32", str(ctx.exception))
        self.assertIn("not rescale", str(ctx.exception))

    def test_u8step_no_silent_clip(self) -> None:
        z = np.linspace(0.0, 100.0, 20, dtype=np.float32).reshape(4, 5)
        with self.assertRaises(ValueError) as ctx:
            quantize(z, "u8step", step_m=0.25, ref="min")
        self.assertIn("f32", str(ctx.exception))
        q = quantize(z, "u8step", step_m=0.5, ref="min")
        self.assertEqual(q.array.min(), 1)
        self.assertLessEqual(int(q.array.max()), 255)

    def test_f32_keeps_nan(self) -> None:
        z = np.array([[1.5, np.nan]], dtype=np.float32)
        q = quantize(z, "f32")
        self.assertTrue(np.isnan(q.array[0, 1]))
        self.assertAlmostEqual(float(q.array[0, 0]), 1.5)


class ThumbTests(unittest.TestCase):
    def test_minmax_unit_and_diverging(self) -> None:
        a = np.array([[10.0, 20.0], [10.0, 20.0]], dtype=np.float32)
        b = np.array([[10.0, 20.0], [np.nan, 15.0]], dtype=np.float32)
        out = float_thumbs_to_unit({"a": a, "b": b}, per_tile=False)
        self.assertAlmostEqual(float(out["a"][0, 0]), 0.0)
        self.assertAlmostEqual(float(out["a"][0, 1]), 1.0)
        self.assertTrue(np.isnan(out["b"][1, 0]))
        blue = diverging_rgb(np.array([[0.0]], dtype=np.float32))[0, 0]
        white = diverging_rgb(np.array([[0.5]], dtype=np.float32))[0, 0]
        red = diverging_rgb(np.array([[1.0]], dtype=np.float32))[0, 0]
        np.testing.assert_array_equal(blue, [0, 0, 255])
        np.testing.assert_array_equal(white, [255, 255, 255])
        np.testing.assert_array_equal(red, [255, 0, 0])
        hole = diverging_rgb(np.array([[np.nan]], dtype=np.float32))[0, 0]
        np.testing.assert_array_equal(hole, [0, 0, 0])

    def test_per_tile_normalize(self) -> None:
        low = np.array([[0.0, 1.0]], dtype=np.float32)
        high = np.array([[100.0, 200.0]], dtype=np.float32)
        out = float_thumbs_to_unit({"low": low, "high": high}, per_tile=True)
        self.assertAlmostEqual(float(out["low"][0, 0]), 0.0)
        self.assertAlmostEqual(float(out["low"][0, 1]), 1.0)
        self.assertAlmostEqual(float(out["high"][0, 0]), 0.0)
        self.assertAlmostEqual(float(out["high"][0, 1]), 1.0)
        global_out = float_thumbs_to_unit({"low": low, "high": high}, per_tile=False)
        self.assertAlmostEqual(float(global_out["high"][0, 0]), 0.5)


class TiffIoTests(unittest.TestCase):
    def test_read_sample_dgm_tile(self) -> None:
        sample = Path(
            "/home/pm/Cursor/WETTER-Suite/datasets/Geo_Karte/Pfaffenweiler"
            "/dgm025_32_456_5320_1_bw_2022.tif"
        )
        if not sample.is_file():
            self.skipTest("sample DGM TIFF not on disk")
        from dgm_mosaic.tiffio import peek_hw, read_float32

        h, w = peek_hw(sample)
        arr = read_float32(sample)
        self.assertEqual(arr.shape, (h, w))
        self.assertEqual(arr.dtype, np.float32)
        self.assertTrue(np.isfinite(arr).any())


class ConvertStubTests(unittest.TestCase):
    def test_missing_folder(self) -> None:
        with self.assertRaises(ValueError):
            convert(Path("/no/such/dgm/folder"))


if __name__ == "__main__":
    unittest.main()
