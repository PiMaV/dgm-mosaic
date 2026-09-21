"""Classic single-band TIFF → float32 (no GDAL, no OpenCV).

Uncompressed strips **or** tiles. Same profile as the Go ``internal/tiff``
package for strips; tiles added for LGL DGM GeoTIFFs.
"""
from __future__ import annotations

import math
import struct
from pathlib import Path

import numpy as np

_TAG_WIDTH = 256
_TAG_LENGTH = 257
_TAG_BITS = 258
_TAG_COMPRESSION = 259
_TAG_STRIP_OFFSETS = 273
_TAG_SAMPLES = 277
_TAG_ROWS_PER_STRIP = 278
_TAG_STRIP_BYTES = 279
_TAG_TILE_WIDTH = 322
_TAG_TILE_LENGTH = 323
_TAG_TILE_OFFSETS = 324
_TAG_TILE_BYTES = 325
_TAG_SAMPLE_FORMAT = 339

_TYPE_BYTE = 1
_TYPE_SHORT = 3
_TYPE_LONG = 4


def peek_hw(path: Path) -> tuple[int, int]:
    """Return (height, width) from the first IFD without decoding pixels."""
    with path.open("rb") as f:
        bo, entries = _read_ifd(f, path.name)
    width = _entry_uint(entries, _TAG_WIDTH, bo)
    height = _entry_uint(entries, _TAG_LENGTH, bo)
    if width is None or height is None:
        raise ValueError(f"{path.name}: TIFF missing ImageWidth/Length")
    return int(height), int(width)


def read_float32(path: Path) -> np.ndarray:
    """Load a single-band uncompressed TIFF as float32 (H, W)."""
    with path.open("rb") as f:
        bo, entries = _read_ifd(f, path.name)
        width = _entry_uint(entries, _TAG_WIDTH, bo)
        height = _entry_uint(entries, _TAG_LENGTH, bo)
        if width is None or height is None:
            raise ValueError(f"{path.name}: TIFF missing ImageWidth/Length")
        width, height = int(width), int(height)

        comp = _entry_uint(entries, _TAG_COMPRESSION, bo) or 1
        if comp != 1:
            raise ValueError(
                f"{path.name}: compressed TIFF unsupported (compression={comp})"
            )
        spp = _entry_uint(entries, _TAG_SAMPLES, bo) or 1
        if spp != 1:
            raise ValueError(f"{path.name}: expected 1 sample/pixel, got {spp}")

        bps = _entry_uint_or_first(entries, _TAG_BITS, bo)
        if bps is None:
            raise ValueError(f"{path.name}: missing BitsPerSample")
        sf = _entry_uint(entries, _TAG_SAMPLE_FORMAT, bo) or 1

        elem = int(bps) // 8
        if bps % 8 != 0 or elem not in (1, 2, 4):
            raise ValueError(f"{path.name}: unsupported BitsPerSample {bps}")

        tile_w = _entry_uint(entries, _TAG_TILE_WIDTH, bo)
        tile_h = _entry_uint(entries, _TAG_TILE_LENGTH, bo)
        if tile_w and tile_h:
            return _read_tiled(
                f, path.name, bo, entries, width, height, int(tile_w), int(tile_h),
                elem, int(sf),
            )
        return _read_strips(
            f, path.name, bo, entries, width, height, elem, int(sf),
        )


def _read_strips(
    f, name: str, bo: str, entries: list, width: int, height: int, elem: int, sf: int,
) -> np.ndarray:
    offsets = _entry_uint_slice(f, entries, _TAG_STRIP_OFFSETS, bo)
    counts = _entry_uint_slice(f, entries, _TAG_STRIP_BYTES, bo)
    if not offsets or len(offsets) != len(counts):
        raise ValueError(f"{name}: bad strip tags")
    rows_per_strip = _entry_uint(entries, _TAG_ROWS_PER_STRIP, bo) or height
    out = np.empty(height * width, dtype=np.float32)
    row = 0
    for off, nbytes in zip(offsets, counts, strict=True):
        f.seek(int(off))
        buf = f.read(int(nbytes))
        rows = int(rows_per_strip)
        if row + rows > height:
            rows = height - row
        need = rows * width * elem
        if need > len(buf):
            need = len(buf) - (len(buf) % elem)
        dest = out[row * width : (row + rows) * width]
        _decode_strip(buf[:need], dest, width, rows, elem, sf, bo)
        row += rows
        if row >= height:
            break
    return out.reshape(height, width)


def _read_tiled(
    f,
    name: str,
    bo: str,
    entries: list,
    width: int,
    height: int,
    tile_w: int,
    tile_h: int,
    elem: int,
    sf: int,
) -> np.ndarray:
    offsets = _entry_uint_slice(f, entries, _TAG_TILE_OFFSETS, bo)
    counts = _entry_uint_slice(f, entries, _TAG_TILE_BYTES, bo)
    if not offsets or len(offsets) != len(counts):
        raise ValueError(f"{name}: bad tile tags")
    tiles_x = int(math.ceil(width / tile_w))
    tiles_y = int(math.ceil(height / tile_h))
    if len(offsets) < tiles_x * tiles_y:
        raise ValueError(
            f"{name}: expected {tiles_x * tiles_y} tiles, got {len(offsets)}"
        )
    out = np.full((height, width), np.nan, dtype=np.float32)
    idx = 0
    for ty in range(tiles_y):
        for tx in range(tiles_x):
            off, nbytes = offsets[idx], counts[idx]
            idx += 1
            f.seek(int(off))
            buf = f.read(int(nbytes))
            need = tile_w * tile_h * elem
            if need > len(buf):
                need = len(buf) - (len(buf) % elem)
            flat = np.empty(tile_w * tile_h, dtype=np.float32)
            _decode_strip(buf[:need], flat, tile_w, tile_h, elem, sf, bo)
            tile = flat.reshape(tile_h, tile_w)
            y0, x0 = ty * tile_h, tx * tile_w
            y1, x1 = min(y0 + tile_h, height), min(x0 + tile_w, width)
            out[y0:y1, x0:x1] = tile[: y1 - y0, : x1 - x0]
    return out


def _endian_char(bo: str) -> str:
    return "<" if bo == "II" else ">"


def _read_ifd(f, name: str) -> tuple[str, list[tuple]]:
    hdr = f.read(8)
    if len(hdr) < 8:
        raise ValueError(f"{name}: truncated TIFF header")
    if hdr[:2] == b"II":
        bo = "II"
    elif hdr[:2] == b"MM":
        bo = "MM"
    else:
        raise ValueError(f"{name}: not a TIFF")
    e = _endian_char(bo)
    magic = struct.unpack(e + "H", hdr[2:4])[0]
    if magic == 43:
        raise ValueError(f"{name}: BigTIFF is unsupported")
    if magic != 42:
        raise ValueError(f"{name}: not a TIFF")
    ifd_off = struct.unpack(e + "I", hdr[4:8])[0]
    f.seek(ifd_off)
    n_raw = f.read(2)
    if len(n_raw) < 2:
        raise ValueError(f"{name}: truncated IFD")
    n = struct.unpack(e + "H", n_raw)[0]
    entries: list[tuple] = []
    for _ in range(n):
        raw = f.read(12)
        if len(raw) < 12:
            break
        tag, typ, count = struct.unpack(e + "HHI", raw[:8])
        entries.append((tag, typ, count, raw[8:12]))
    return bo, entries


def _parse_inline_uint(typ: int, count: int, raw: bytes, bo: str) -> int | None:
    if count != 1:
        return None
    e = _endian_char(bo)
    if typ == _TYPE_SHORT:
        return int(struct.unpack(e + "H", raw[:2])[0])
    if typ == _TYPE_LONG:
        return int(struct.unpack(e + "I", raw[:4])[0])
    if typ == _TYPE_BYTE:
        return int(raw[0])
    return None


def _entry_uint(entries: list[tuple], tag: int, bo: str) -> int | None:
    for t, typ, count, raw in entries:
        if t != tag:
            continue
        v = _parse_inline_uint(typ, count, raw, bo)
        if v is not None:
            return v
    return None


def _entry_uint_or_first(entries: list[tuple], tag: int, bo: str) -> int | None:
    for t, typ, count, raw in entries:
        if t != tag:
            continue
        if count == 1:
            v = _parse_inline_uint(typ, count, raw, bo)
            if v is not None:
                return v
        if typ == _TYPE_SHORT:
            e = _endian_char(bo)
            return int(struct.unpack(e + "H", raw[:2])[0])
    return None


def _entry_uint_slice(f, entries: list[tuple], tag: int, bo: str) -> list[int]:
    e = _endian_char(bo)
    for t, typ, count, raw in entries:
        if t != tag:
            continue
        size = {1: 1, 2: 1, 3: 2, 4: 4, 5: 8}.get(typ, 1)
        nbytes = size * count
        if nbytes <= 4:
            blob = raw[:nbytes]
        else:
            off = struct.unpack(e + "I", raw)[0]
            f.seek(off)
            blob = f.read(nbytes)
            if len(blob) < nbytes:
                raise ValueError("truncated TIFF offset/count table")
        out: list[int] = []
        if typ == _TYPE_SHORT:
            for i in range(count):
                out.append(struct.unpack_from(e + "H", blob, i * 2)[0])
        elif typ == _TYPE_LONG:
            for i in range(count):
                out.append(struct.unpack_from(e + "I", blob, i * 4)[0])
        elif typ == _TYPE_BYTE:
            out.extend(blob[:count])
        else:
            raise ValueError(f"unsupported TIFF type {typ} for tag {tag}")
        return out
    return []


def _decode_strip(
    buf: bytes,
    dest: np.ndarray,
    width: int,
    rows: int,
    elem: int,
    sf: int,
    bo: str,
) -> None:
    e = _endian_char(bo)
    n_pix = min(rows * width, dest.size)
    if elem == 4 and sf == 3:
        vals = np.frombuffer(buf, dtype=np.dtype(e + "f4"), count=n_pix)
        dest[: len(vals)] = vals
        return
    for i in range(n_pix):
        off = i * elem
        if off + elem > len(buf):
            break
        chunk = buf[off : off + elem]
        if elem == 4 and sf == 1:
            dest[i] = float(struct.unpack(e + "I", chunk)[0])
        elif elem == 4 and sf == 2:
            dest[i] = float(struct.unpack(e + "i", chunk)[0])
        elif elem == 2 and sf == 1:
            dest[i] = float(struct.unpack(e + "H", chunk)[0])
        elif elem == 2 and sf == 2:
            dest[i] = float(struct.unpack(e + "h", chunk)[0])
        elif elem == 1 and sf == 1:
            dest[i] = float(chunk[0])
        elif elem == 1 and sf == 2:
            dest[i] = float(struct.unpack("b", chunk)[0])
        else:
            dest[i] = np.nan
