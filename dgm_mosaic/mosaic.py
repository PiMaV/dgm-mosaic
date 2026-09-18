"""
DGM GeoTIFF tiles → one mosaic for BLITZ.

No GDAL: OpenCV reads Float32 TIFF; placement from .tfw or LGL
``dgm025_32_{e_km}_{n_km}_…`` names. Axis-aligned square pixels only.
"""
from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Literal

import numpy as np

os.environ.setdefault("OPENCV_LOG_LEVEL", "SILENT")

DtypeMode = Literal["u16cm", "u8stretch", "u8step", "f32"]
RefMode = Literal["min", "mean"]

TIFF_SUFFIXES = {".tif", ".tiff"}
NODATA_DEFAULT = -9999.0
PIXEL_EPS = 1e-3
ROT_EPS = 1e-9
_RE_DGM = re.compile(
    r"^dgm(?P<res>\d+)_(?P<zone>\d+)_(?P<e>\d+)_(?P<n>\d+)_",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class DgmName:
    res_token: str
    zone: int
    e_km: int
    n_km: int
    pixel_m: float


@dataclass(frozen=True)
class WorldFile:
    pixel_x: float
    rot_y: float
    rot_x: float
    pixel_y: float
    x_ul_center: float
    y_ul_center: float

    @property
    def pixel_m(self) -> float:
        return float(self.pixel_x)

    @property
    def west(self) -> float:
        return float(self.x_ul_center - 0.5 * self.pixel_x)

    @property
    def north(self) -> float:
        return float(self.y_ul_center - 0.5 * self.pixel_y)


@dataclass
class Tile:
    path: Path
    array: np.ndarray
    west: float
    north: float
    pixel_m: float
    name: DgmName | None = None


@dataclass
class QuantizeResult:
    array: np.ndarray
    meta: dict[str, Any]


def parse_dgm_name(stem: str) -> DgmName | None:
    """Parse LGL-style ``dgm025_32_505_5364_…`` stem."""
    m = _RE_DGM.match(stem)
    if not m:
        return None
    res = m.group("res")
    if res.startswith("0"):
        frac = res.lstrip("0")
        if not frac:
            return None
        pixel_m = float(f"0.{frac}")
    else:
        pixel_m = float(res)
    if pixel_m <= 0:
        return None
    return DgmName(
        res_token=res,
        zone=int(m.group("zone")),
        e_km=int(m.group("e")),
        n_km=int(m.group("n")),
        pixel_m=pixel_m,
    )


def parse_tfw(path: Path) -> WorldFile:
    lines = [
        ln.strip()
        for ln in path.read_text(encoding="utf-8", errors="replace").splitlines()
        if ln.strip()
    ]
    if len(lines) < 6:
        raise ValueError(f"{path.name}: world file needs 6 numbers, got {len(lines)}")
    vals = [float(x.replace(",", ".")) for x in lines[:6]]
    world = WorldFile(*vals)
    if abs(world.rot_x) > ROT_EPS or abs(world.rot_y) > ROT_EPS:
        raise ValueError(f"{path.name}: rotation is not zero (unsupported)")
    if world.pixel_x <= 0:
        raise ValueError(f"{path.name}: x pixel size must be > 0")
    if world.pixel_y >= 0:
        raise ValueError(f"{path.name}: y pixel size must be negative (north-up)")
    if abs(world.pixel_x - abs(world.pixel_y)) > 1e-6:
        raise ValueError(f"{path.name}: non-square pixels are unsupported")
    return world


def tfw_for_tif(tif: Path) -> Path | None:
    for suf in (".tfw", ".tifw"):
        cand = tif.with_suffix(suf)
        if cand.is_file():
            return cand
    return None


def pixel_m_close(a: float, b: float) -> bool:
    return abs(a - b) <= max(PIXEL_EPS * max(abs(a), abs(b), 1e-9), 1e-9)


def _import_cv2():
    try:
        import cv2
    except ImportError as e:
        raise RuntimeError(
            "OpenCV is required to read DGM GeoTIFFs. "
            "From converters/: uv sync && uv run dgm-mosaic"
        ) from e
    return cv2


def read_float_tif(path: Path) -> np.ndarray:
    """Read a single-band TIFF as float32 (H, W). GeoTIFF tags are ignored."""
    cv2 = _import_cv2()
    img = cv2.imread(str(path), cv2.IMREAD_UNCHANGED)
    if img is None:
        raise ValueError(f"{path.name}: OpenCV could not read TIFF")
    if img.ndim == 3:
        img = img[..., 0]
    if img.ndim != 2:
        raise ValueError(f"{path.name}: expected 2D raster, got shape {img.shape}")
    return np.asarray(img, dtype=np.float32)


THUMB_EDGE = 192


def downsample_height(z: np.ndarray, max_edge: int = THUMB_EDGE) -> np.ndarray:
    """Resize a 2D height field for GUI tiles. Keeps NaNs via INTER_AREA on finite data."""
    cv2 = _import_cv2()
    arr = np.asarray(z, dtype=np.float32)
    if arr.ndim != 2:
        raise ValueError(f"expected 2D, got {arr.shape}")
    h, w = arr.shape
    scale = max_edge / max(h, w, 1)
    nh, nw = max(1, int(round(h * scale))), max(1, int(round(w * scale)))
    if (nh, nw) == (h, w):
        return arr
    return cv2.resize(arr, (nw, nh), interpolation=cv2.INTER_AREA)


def tile_thumbnail(path: Path, max_edge: int = THUMB_EDGE) -> np.ndarray:
    """Float32 preview of one TIFF, downsampled. NoData stays non-finite."""
    z = read_float_tif(path)
    z = z.astype(np.float32, copy=True)
    z[~np.isfinite(z) | (z == NODATA_DEFAULT)] = np.nan
    return downsample_height(z, max_edge=max_edge)


def float_thumbs_to_unit(thumbs: dict) -> dict:
    """Min–max of the whole set → [0, 1]. NaN stays NaN (holes stay empty)."""
    finite = [
        t[np.isfinite(t)].ravel()
        for t in thumbs.values()
        if np.asarray(t).size and np.isfinite(t).any()
    ]
    if not finite:
        return {
            k: np.full(np.asarray(v).shape, np.nan, dtype=np.float32)
            for k, v in thumbs.items()
        }
    cat = np.concatenate(finite)
    lo, hi = float(np.min(cat)), float(np.max(cat))
    span = max(hi - lo, 1e-6)
    out = {}
    for k, t in thumbs.items():
        arr = np.asarray(t, dtype=np.float32)
        u = np.full(arr.shape, np.nan, dtype=np.float32)
        m = np.isfinite(arr)
        u[m] = (arr[m] - lo) / span
        out[k] = u
    return out


def diverging_rgb(unit: np.ndarray) -> np.ndarray:
    """Map [0, 1] to blue–white–red. Non-finite → black."""
    t = np.asarray(unit, dtype=np.float32)
    rgb = np.zeros(t.shape + (3,), dtype=np.float32)
    m = np.isfinite(t)
    if not np.any(m):
        return np.zeros(t.shape + (3,), dtype=np.uint8)
    x = np.clip(t, 0.0, 1.0)
    lo = m & (x <= 0.5)
    hi = m & (x > 0.5)
    s = x * 2.0
    rgb[lo, 0] = s[lo]
    rgb[lo, 1] = s[lo]
    rgb[lo, 2] = 1.0
    s = (x - 0.5) * 2.0
    rgb[hi, 0] = 1.0
    rgb[hi, 1] = 1.0 - s[hi]
    rgb[hi, 2] = 1.0 - s[hi]
    return np.clip(np.rint(rgb * 255.0), 0, 255).astype(np.uint8)


def compose_preview_rgb(
    rows: int,
    cols: int,
    unit_thumbs: dict[tuple[int, int], np.ndarray],
) -> np.ndarray:
    """Edge-to-edge RGB mosaic; missing cells stay black."""
    if not unit_thumbs:
        return np.zeros((1, 1, 3), dtype=np.uint8)
    th, tw = next(iter(unit_thumbs.values())).shape[:2]
    rgb = np.zeros((rows * th, cols * tw, 3), dtype=np.uint8)
    for (r, c), unit in unit_thumbs.items():
        if r < 0 or c < 0 or r >= rows or c >= cols:
            continue
        rgb[r * th : (r + 1) * th, c * tw : (c + 1) * tw] = diverging_rgb(unit)
    return rgb


def legend_rgb(width: int = 256, height: int = 12) -> np.ndarray:
    ramp = np.linspace(0.0, 1.0, max(width, 1), dtype=np.float32)
    strip = np.repeat(ramp[None, :], max(height, 1), axis=0)
    return diverging_rgb(strip)


def norm_tile_box(
    box: tuple[int, int, int, int],
    rows: int,
    cols: int,
) -> tuple[int, int, int, int]:
    r0, c0, r1, c1 = (int(x) for x in box)
    r0, r1 = sorted((r0, r1))
    c0, c1 = sorted((c0, c1))
    r0 = max(0, min(rows - 1, r0))
    r1 = max(0, min(rows - 1, r1))
    c0 = max(0, min(cols - 1, c0))
    c1 = max(0, min(cols - 1, c1))
    return r0, c0, r1, c1


def list_tiffs(root: Path) -> list[Path]:
    if root.is_file():
        if root.suffix.lower() in TIFF_SUFFIXES:
            return [root]
        raise ValueError(f"{root} is not a TIFF")
    if not root.is_dir():
        raise ValueError(f"{root} does not exist")
    out = [
        p
        for p in root.iterdir()
        if p.is_file()
        and not p.name.startswith(".")
        and p.suffix.lower() in TIFF_SUFFIXES
    ]
    return sorted(out, key=lambda p: p.name.lower())


def peek_tif_hw(path: Path) -> tuple[int, int]:
    """Return (height, width) from a classic TIFF IFD — no pixel decode."""
    import struct

    with path.open("rb") as f:
        hdr = f.read(8)
        if len(hdr) < 8:
            raise ValueError(f"{path.name}: truncated TIFF header")
        if hdr[:2] == b"II":
            endian = "<"
        elif hdr[:2] == b"MM":
            endian = ">"
        else:
            raise ValueError(f"{path.name}: not a TIFF")
        magic = struct.unpack(endian + "H", hdr[2:4])[0]
        if magic == 43:
            raise ValueError(f"{path.name}: BigTIFF is unsupported")
        if magic != 42:
            raise ValueError(f"{path.name}: not a TIFF")
        ifd = struct.unpack(endian + "I", hdr[4:8])[0]
        f.seek(ifd)
        n_raw = f.read(2)
        if len(n_raw) < 2:
            raise ValueError(f"{path.name}: truncated IFD")
        n = struct.unpack(endian + "H", n_raw)[0]
        width = height = None
        for _ in range(n):
            entry = f.read(12)
            if len(entry) < 12:
                break
            tag, typ, count, val = struct.unpack(endian + "HHII", entry)
            if count != 1:
                continue
            parsed = val
            if typ == 3:  # SHORT, stored in low 16 bits of val
                parsed = val & 0xFFFF
            if tag == 256:
                width = int(parsed)
            elif tag == 257:
                height = int(parsed)
        if width is None or height is None:
            raise ValueError(f"{path.name}: TIFF missing ImageWidth/Length")
        return height, width


def _origin_for_tif(path: Path) -> tuple[float, float, float, DgmName | None]:
    """west, north, pixel_m, parsed LGL name (if any)."""
    name = parse_dgm_name(path.stem)
    tfw_path = tfw_for_tif(path)
    if tfw_path is not None:
        world = parse_tfw(tfw_path)
        return world.west, world.north, world.pixel_m, name
    if name is None:
        raise ValueError(
            f"{path.name}: need a .tfw world file or an LGL name "
            "(dgm025_32_{e}_{n}_…)"
        )
    west = float(name.e_km * 1000)
    north = float((name.n_km + 1) * 1000)
    return west, north, name.pixel_m, name


def load_tile(path: Path, *, nodata: float = NODATA_DEFAULT) -> Tile:
    arr = read_float_tif(path)
    valid = np.isfinite(arr) & (arr != nodata)
    arr = arr.astype(np.float32, copy=True)
    arr[~valid] = np.nan
    west, north, pixel_m, name = _origin_for_tif(path)
    return Tile(
        path=path,
        array=arr,
        west=west,
        north=north,
        pixel_m=pixel_m,
        name=name,
    )


BYTES_PER_PIXEL: dict[str, int] = {
    "f32": 4,
    "u16cm": 2,
    "u8stretch": 1,
    "u8step": 1,
}


def estimate_npy_bytes(height: int, width: int, mode: DtypeMode) -> int:
    """Uncompressed .npy payload estimate (header ignored)."""
    bpp = BYTES_PER_PIXEL[mode]
    return int(height) * int(width) * bpp


def fmt_mb(nbytes: int) -> str:
    return f"{nbytes / (1024 * 1024):.1f} MB"


@dataclass(frozen=True)
class TileStub:
    """Geometry only — used by the GUI before reading pixel data."""

    path: Path
    height: int
    width: int
    west: float
    north: float
    pixel_m: float
    name: DgmName | None = None
    error: str | None = None

    @property
    def east(self) -> float:
        return self.west + self.width * self.pixel_m

    @property
    def south(self) -> float:
        return self.north - self.height * self.pixel_m

    @property
    def label_km(self) -> str:
        if self.name is not None:
            return f"{self.name.e_km}_{self.name.n_km}"
        return f"E{self.west:.0f} N{self.north:.0f}"


@dataclass
class MosaicLayout:
    stubs: list[TileStub]
    pixel_m: float
    west: float
    north: float
    east: float
    south: float
    height: int
    width: int
    tile_rows: int
    tile_cols: int
    cells: dict[tuple[int, int], TileStub]
    holes: int
    regular: bool
    crs: str | None

    def nbytes(self, mode: DtypeMode) -> int:
        return estimate_npy_bytes(self.height, self.width, mode)

    def nbytes_box(self, mode: DtypeMode, box: tuple[int, int, int, int]) -> int:
        if not self.cells:
            return 0
        sample = next(iter(self.cells.values()))
        r0, c0, r1, c1 = norm_tile_box(box, self.tile_rows, self.tile_cols)
        h = (r1 - r0 + 1) * sample.height
        w = (c1 - c0 + 1) * sample.width
        return estimate_npy_bytes(h, w, mode)


def stub_tile(path: Path) -> TileStub:
    west, north, pixel_m, name = _origin_for_tif(path)
    height, width = peek_tif_hw(path)
    return TileStub(
        path=path,
        height=height,
        width=width,
        west=west,
        north=north,
        pixel_m=pixel_m,
        name=name,
    )


def inspect_folder(input_path: Path) -> MosaicLayout:
    """Place tiles from names/.tfw + TIFF sizes. Does not decode pixels."""
    tiffs = list_tiffs(input_path)
    if not tiffs:
        raise ValueError(f"no TIFF files in {input_path}")
    stubs = [stub_tile(p) for p in tiffs]
    return layout_from_stubs(stubs)


def layout_from_stubs(stubs: list[TileStub]) -> MosaicLayout:
    if not stubs:
        raise ValueError("no tiles to mosaic")
    pixel_m = stubs[0].pixel_m
    for s in stubs[1:]:
        if not pixel_m_close(s.pixel_m, pixel_m):
            raise ValueError(
                f"mixed pixel sizes: {stubs[0].path.name}={pixel_m} m, "
                f"{s.path.name}={s.pixel_m} m"
            )
    west = min(s.west for s in stubs)
    north = max(s.north for s in stubs)
    east = max(s.east for s in stubs)
    south = min(s.south for s in stubs)
    width = _near_int((east - west) / pixel_m)
    height = _near_int((north - south) / pixel_m)
    shapes = {(s.height, s.width) for s in stubs}
    regular = len(shapes) == 1
    cells: dict[tuple[int, int], TileStub] = {}
    if regular:
        th, tw = next(iter(shapes))
        tile_cols = _near_int((east - west) / (tw * pixel_m))
        tile_rows = _near_int((north - south) / (th * pixel_m))
        for s in stubs:
            col = _near_int((s.west - west) / (tw * pixel_m))
            row = _near_int((north - s.north) / (th * pixel_m))
            cells[(row, col)] = s
    else:
        tile_rows, tile_cols = 1, len(stubs)
        for i, s in enumerate(sorted(stubs, key=lambda t: (-t.north, t.west))):
            cells[(0, i)] = s
    holes = 0
    if regular:
        holes = tile_rows * tile_cols - len(cells)
    zone = next((s.name.zone for s in stubs if s.name is not None), None)
    crs = f"EPSG:258{zone}" if zone in (32, 33) else None
    return MosaicLayout(
        stubs=stubs,
        pixel_m=pixel_m,
        west=west,
        north=north,
        east=east,
        south=south,
        height=height,
        width=width,
        tile_rows=tile_rows,
        tile_cols=tile_cols,
        cells=cells,
        holes=holes,
        regular=regular,
        crs=crs,
    )


def _near_int(value: float) -> int:
    rounded = round(value)
    if abs(value - rounded) > PIXEL_EPS:
        raise ValueError(
            f"tile origin is not on the mosaic pixel grid ({value:.6f} px) "
            "— refusing to resample"
        )
    return int(rounded)


def mosaic_for_tile_box(
    tiles: list[Tile],
    layout: MosaicLayout,
    box: tuple[int, int, int, int],
) -> tuple[np.ndarray, dict[str, Any]]:
    """Paste tiles into the selected cell rectangle. Missing cells stay 0."""
    if not layout.cells:
        raise ValueError("layout has no tiles")
    r0, c0, r1, c1 = norm_tile_box(box, layout.tile_rows, layout.tile_cols)
    sample = next(iter(layout.cells.values()))
    th, tw = sample.height, sample.width
    pixel_m = layout.pixel_m
    west = layout.west + c0 * tw * pixel_m
    north = layout.north - r0 * th * pixel_m
    nrows, ncols = r1 - r0 + 1, c1 - c0 + 1
    canvas = np.zeros((nrows * th, ncols * tw), dtype=np.float32)
    used: list[str] = []
    for t in tiles:
        col = _near_int((t.west - layout.west) / (tw * pixel_m))
        row = _near_int((layout.north - t.north) / (th * pixel_m))
        if row < r0 or row > r1 or col < c0 or col > c1:
            continue
        rr, cc = row - r0, col - c0
        dest = canvas[rr * th : (rr + 1) * th, cc * tw : (cc + 1) * tw]
        src = np.nan_to_num(t.array, nan=0.0)
        if src.shape != dest.shape:
            raise ValueError(f"{t.path.name}: shape {src.shape} != tile {dest.shape}")
        dest[:, :] = src
        used.append(t.path.name)
    east = west + ncols * tw * pixel_m
    south = north - nrows * th * pixel_m
    zone = next((t.name.zone for t in tiles if t.name is not None), None)
    info = {
        "pixel_m": pixel_m,
        "west": west,
        "north": north,
        "east": east,
        "south": south,
        "shape_hw": [int(canvas.shape[0]), int(canvas.shape[1])],
        "overlap_pixels": 0,
        "crs": f"EPSG:258{zone}" if zone in (32, 33) else None,
        "tiles": used,
        "tile_box": [r0, c0, r1, c1],
        "zero_padded": nrows * ncols - len(used),
    }
    return canvas, info


def mosaic_tiles(tiles: list[Tile]) -> tuple[np.ndarray, dict[str, Any]]:
    if not tiles:
        raise ValueError("no tiles to mosaic")
    pixel_m = tiles[0].pixel_m
    for t in tiles[1:]:
        if not pixel_m_close(t.pixel_m, pixel_m):
            raise ValueError(
                f"mixed pixel sizes: {tiles[0].path.name}={pixel_m} m, "
                f"{t.path.name}={t.pixel_m} m"
            )
    west = min(t.west for t in tiles)
    north = max(t.north for t in tiles)
    east = max(t.west + t.array.shape[1] * t.pixel_m for t in tiles)
    south = min(t.north - t.array.shape[0] * t.pixel_m for t in tiles)
    width = _near_int((east - west) / pixel_m)
    height = _near_int((north - south) / pixel_m)
    canvas = np.full((height, width), np.nan, dtype=np.float32)
    overlap = 0
    for t in tiles:
        c0 = _near_int((t.west - west) / pixel_m)
        r0 = _near_int((north - t.north) / pixel_m)
        th, tw = t.array.shape
        if r0 < 0 or c0 < 0 or r0 + th > height or c0 + tw > width:
            raise ValueError(f"{t.path.name}: paste {r0},{c0} {th}x{tw} outside {height}x{width}")
        dest = canvas[r0 : r0 + th, c0 : c0 + tw]
        hit = np.isfinite(dest) & np.isfinite(t.array)
        overlap += int(hit.sum())
        dest[:, :] = np.where(np.isfinite(t.array), t.array, dest)
    zone = next((t.name.zone for t in tiles if t.name is not None), None)
    info = {
        "pixel_m": pixel_m,
        "west": west,
        "north": north,
        "east": east,
        "south": south,
        "shape_hw": [int(height), int(width)],
        "overlap_pixels": overlap,
        "crs": f"EPSG:258{zone}" if zone in (32, 33) else None,
        "tiles": [t.path.name for t in tiles],
    }
    return canvas, info


def _valid_mask(z: np.ndarray) -> np.ndarray:
    return np.isfinite(z)


def quantize_u16cm(z: np.ndarray, z0: float | None = None) -> QuantizeResult:
    valid = _valid_mask(z)
    if not np.any(valid):
        raise ValueError("mosaic has no valid pixels")
    z_min = float(np.nanmin(z))
    z_max = float(np.nanmax(z))
    z0_m = float(np.floor(z_min) if z0 is None else z0)
    cm = np.rint((z - z0_m) * 100.0)
    if np.any(valid & (cm < 1)):
        z0_m -= 1.0
        cm = np.rint((z - z0_m) * 100.0)
    if np.any(valid & (cm > 65535)):
        relief = z_max - z0_m
        raise ValueError(
            f"u16cm overflow: relief {relief:.1f} m from z0={z0_m:g} exceeds 655.35 m"
        )
    out = np.zeros(z.shape, dtype=np.uint16)
    out[valid] = np.clip(cm[valid], 1, 65535).astype(np.uint16)
    n_clip = 0
    return QuantizeResult(
        out,
        {
            "mode": "u16cm",
            "dtype": "uint16",
            "nodata": 0,
            "z0_m": z0_m,
            "scale_m": 0.01,
            "z_min_m": z_min,
            "z_max_m": z_max,
            "clipped_pixels": n_clip,
            "reconstruct": "z_m = z0_m + pixel * scale_m  (pixel 0 = nodata)",
        },
    )


def quantize_u8stretch(z: np.ndarray) -> QuantizeResult:
    valid = _valid_mask(z)
    if not np.any(valid):
        raise ValueError("mosaic has no valid pixels")
    z_min = float(np.nanmin(z))
    z_max = float(np.nanmax(z))
    span = z_max - z_min
    out = np.zeros(z.shape, dtype=np.uint8)
    if span <= 0:
        out[valid] = 1
        scale = 0.0
    else:
        scaled = 1.0 + np.rint((z - z_min) / span * 254.0)
        out[valid] = np.clip(scaled[valid], 1, 255).astype(np.uint8)
        scale = span / 254.0
    return QuantizeResult(
        out,
        {
            "mode": "u8stretch",
            "dtype": "uint8",
            "nodata": 0,
            "z0_m": z_min,
            "scale_m": scale,
            "z_min_m": z_min,
            "z_max_m": z_max,
            "clipped_pixels": 0,
            "reconstruct": "z_m ≈ z0_m + (pixel - 1) * scale_m  (pixel 0 = nodata)",
        },
    )


def quantize_u8step(
    z: np.ndarray,
    *,
    step_m: float,
    ref: RefMode,
) -> QuantizeResult:
    if step_m <= 0:
        raise ValueError("--step-m must be > 0")
    valid = _valid_mask(z)
    if not np.any(valid):
        raise ValueError("mosaic has no valid pixels")
    z_min = float(np.nanmin(z))
    z_max = float(np.nanmax(z))
    z_ref = z_min if ref == "min" else float(np.nanmean(z))
    if ref == "min":
        raw = np.rint((z - z_ref) / step_m) + 1.0
        reconstruct = "z_m ≈ z0_m + (pixel - 1) * scale_m  (pixel 0 = nodata)"
        z0_m = z_ref
    else:
        raw = np.rint((z - z_ref) / step_m) + 128.0
        reconstruct = "z_m ≈ z0_m + (pixel - 128) * scale_m  (pixel 0 = nodata)"
        z0_m = z_ref
    would = valid & ((raw < 1) | (raw > 255))
    n_clip = int(would.sum())
    out = np.zeros(z.shape, dtype=np.uint8)
    out[valid] = np.clip(raw[valid], 1, 255).astype(np.uint8)
    relief = z_max - z_min
    suggested = relief / 254.0 if relief > 0 else step_m
    return QuantizeResult(
        out,
        {
            "mode": "u8step",
            "dtype": "uint8",
            "nodata": 0,
            "ref": ref,
            "z0_m": z0_m,
            "scale_m": step_m,
            "z_min_m": z_min,
            "z_max_m": z_max,
            "clipped_pixels": n_clip,
            "suggested_step_m": suggested,
            "reconstruct": reconstruct,
        },
    )


def quantize_f32(z: np.ndarray) -> QuantizeResult:
    valid = _valid_mask(z)
    z_min = float(np.nanmin(z)) if np.any(valid) else float("nan")
    z_max = float(np.nanmax(z)) if np.any(valid) else float("nan")
    return QuantizeResult(
        np.asarray(z, dtype=np.float32),
        {
            "mode": "f32",
            "dtype": "float32",
            "nodata": None,
            "z0_m": 0.0,
            "scale_m": 1.0,
            "z_min_m": z_min,
            "z_max_m": z_max,
            "clipped_pixels": 0,
            "reconstruct": "pixel is metres; NaN = nodata",
        },
    )


def quantize(
    z: np.ndarray,
    mode: DtypeMode,
    *,
    step_m: float = 0.25,
    ref: RefMode = "min",
    z0: float | None = None,
) -> QuantizeResult:
    if mode == "u16cm":
        return quantize_u16cm(z, z0=z0)
    if mode == "u8stretch":
        return quantize_u8stretch(z)
    if mode == "u8step":
        return quantize_u8step(z, step_m=step_m, ref=ref)
    if mode == "f32":
        return quantize_f32(z)
    raise ValueError(f"unknown mode {mode}")


def default_output_path(input_path: Path) -> Path:
    if input_path.is_dir():
        return input_path.parent / f"{input_path.name}_mosaic.npy"
    return input_path.with_name(f"{input_path.stem}_mosaic.npy")


def mosaic_array(
    input_path: Path,
    *,
    mode: DtypeMode = "u16cm",
    step_m: float = 0.25,
    ref: RefMode = "min",
    nodata: float = NODATA_DEFAULT,
    z0: float | None = None,
    tile_box: tuple[int, int, int, int] | None = None,
) -> tuple[np.ndarray, dict[str, Any]]:
    """Build the quantized mosaic in memory (no disk write)."""
    input_path = Path(input_path)
    layout = inspect_folder(input_path)
    if tile_box is None:
        tile_box = (0, 0, layout.tile_rows - 1, layout.tile_cols - 1)
    r0, c0, r1, c1 = norm_tile_box(tile_box, layout.tile_rows, layout.tile_cols)
    paths = []
    for r in range(r0, r1 + 1):
        for c in range(c0, c1 + 1):
            stub = layout.cells.get((r, c))
            if stub is not None:
                paths.append(stub.path)
    tiles = [load_tile(p, nodata=nodata) for p in paths]
    mosaic, geo = mosaic_for_tile_box(tiles, layout, (r0, c0, r1, c1))
    quantized = quantize(mosaic, mode, step_m=step_m, ref=ref, z0=z0)
    meta = {
        "format": "wetter.dgm_mosaic.v1",
        "layout": "row0_north_col0_west",
        "source": str(input_path.resolve()),
        **geo,
        **quantized.meta,
        "nbytes": int(quantized.array.nbytes),
    }
    n_clip = int(quantized.meta.get("clipped_pixels") or 0)
    if n_clip:
        sug = quantized.meta.get("suggested_step_m")
        extra = f" (try --step-m {sug:.3g})" if sug else ""
        print(f"warning: {n_clip} pixels clipped to 1..255{extra}")
    return quantized.array, meta


def convert(
    input_path: Path,
    output_path: Path | None = None,
    *,
    mode: DtypeMode = "u16cm",
    step_m: float = 0.25,
    ref: RefMode = "min",
    nodata: float = NODATA_DEFAULT,
    z0: float | None = None,
    tile_box: tuple[int, int, int, int] | None = None,
) -> Path:
    arr, meta = mosaic_array(
        input_path,
        mode=mode,
        step_m=step_m,
        ref=ref,
        nodata=nodata,
        z0=z0,
        tile_box=tile_box,
    )
    out = Path(output_path) if output_path else default_output_path(Path(input_path))
    out.parent.mkdir(parents=True, exist_ok=True)
    np.save(out, arr)
    meta = {**meta, "npy": out.name}
    sidecar = out.with_suffix(".json")
    sidecar.write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")
    _report(out, sidecar, arr, meta)
    return out


def _report(
    npy: Path,
    sidecar: Path,
    arr: np.ndarray,
    meta: dict[str, Any],
) -> None:
    h, w = arr.shape[:2]
    print(f"Mosaic {w}×{h} px  ({meta.get('pixel_m'):g} m)  {npy}")
    print(f"  tiles: {len(meta.get('tiles') or [])}")
    if meta.get("west") is not None:
        print(
            f"  extent: E {meta['west']:g}…{meta['east']:g}  "
            f"N {meta['south']:g}…{meta['north']:g}"
        )
    if meta.get("crs"):
        print(f"  CRS: {meta['crs']}")
    if meta.get("overlap_pixels"):
        print(
            f"  warning: {meta['overlap_pixels']} overlapping valid pixels "
            "(last tile wins)"
        )
    zmin, zmax = meta.get("z_min_m"), meta.get("z_max_m")
    if zmin is not None and zmin == zmin:
        print(f"  height: {zmin:.3f} … {zmax:.3f} m")
    print(
        f"  dtype {meta.get('mode')}: {arr.dtype}  "
        f"{arr.nbytes / 2**20:.1f} MB  → {npy.name} + {sidecar.name}"
    )
