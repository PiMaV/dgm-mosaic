# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Product

**Sidecar** for DGM GeoTIFF tile folders → mosaic → WETTER Viewer Contract
(port **5056**, token `dgm`). Clients: BLITZ, DONNER.

**Shipped binary:** PyInstaller one-file — PyQt6 DnD stage. No OpenCV: classic
TIFF via `dgm_mosaic/tiffio.py` (uncompressed strips or tiles, float/int).

## Do

- Prefer real filesystem paths (DnD). Browse is fallback only.
- Show progress while loading previews and while building/sending mosaic.
- Preview: **whole-mosaic** normalize by default (optional per-tile);
  colormaps `bwr|gray|terrain|plasma|viridis`.
- Primary action is **Stream** (Viewer Contract hub for BLITZ or DONNER).
- Heights: metres are authoritative. Formats only apply a **fixed** unit
  conversion (`u16dm` = ×10 dm from z0; `f32` = metres). Never stretch/auto-fit
  into the dtype. Large mosaics: default **f32**.
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.

## Don’t

- Do not reintroduce OpenCV/GDAL for the product path.
- Do not present the Go CLI as the user product.
- Do not merge HIK/DICOM into this binary.

## Key paths

| Path | Role |
|------|------|
| `dgm_mosaic/app.py` | DnD GUI, progress, preview options |
| `dgm_mosaic/mosaic.py` | Layout, quantize, colormaps |
| `dgm_mosaic/tiffio.py` | Classic TIFF reader (no OpenCV) |
| `DGM.spec` | PyInstaller entry |
| `cmd/dgm-mosaic` | Optional Go CLI (low priority) |

## Defaults

`127.0.0.1:5056`, token `dgm`, stack name `mosaic.npy`.
