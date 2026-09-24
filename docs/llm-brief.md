# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Product

**Sidecar** for DGM GeoTIFF tile folders → mosaic → **two** WETTER Viewer
Contract hubs (token `dgm` on both):

| Port | Client | Stack | Shape |
|------|--------|-------|-------|
| **5056** | BLITZ | `mosaic.npy` | `(1, H, W)` height plane |
| **5057** | DONNER | `surface.npy` | `(nZ, H, W)` elevation surface |

**Shipped binary:** PyInstaller one-file — PyQt6 DnD stage. No OpenCV: classic
TIFF via `dgm_mosaic/tiffio.py` (uncompressed strips or tiles, float/int).

## Do

- Prefer real filesystem paths (DnD). Browse is fallback only.
- Show progress while loading previews and while building/sending mosaic.
- Preview: **whole-mosaic** normalize by default (optional per-tile);
  colormaps `bwr|gray|terrain|plasma|viridis`.
- Primary action is **Stream** — publishes **both** hubs from one click.
- Heights: metres are authoritative. Formats only apply a **fixed** unit
  conversion (`u16dm` = ×10 dm from z0; `f32` = metres). Never stretch/auto-fit
  into the dtype. Large mosaics: default **f32**.
- BLITZ stack: **`(1, H, W)`**. DONNER stack: sparse elevation **surface**
  (one voxel per map column along Z; no filled pillars). Built in
  `height_surface_from_plane` / `height_surface_thw`. No preflight size refuse —
  requested Bin / selection is what gets streamed.
- Optional **bin** 1|2|4|8|16 = block-mean downsample before quantize (GUI
  default **2×**, prominent radios).
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.

## Don’t

- Do not reintroduce OpenCV/GDAL for the product path.
- Do not present the Go CLI as the user product.
- Do not merge HIK/DICOM into this binary.
- Do not fill Z columns from ground to height — surface only.

## Key paths

| Path | Role |
|------|------|
| `dgm_mosaic/app.py` | DnD GUI, progress, dual hubs |
| `dgm_mosaic/app_icon.py` | Window icon helper |
| `dgm_mosaic/mosaic.py` | Layout, quantize, height surface, colormaps |
| `dgm_mosaic/tiffio.py` | Classic TIFF reader (no OpenCV) |
| `icon/` | App icon (`icon_64.ico` + PNGs) |
| `DGM.spec` | PyInstaller entry |
| `cmd/dgm-mosaic` | Optional Go CLI (low priority) |

## Defaults

- BLITZ: `127.0.0.1:5056`, token `dgm`, `mosaic.npy`
- DONNER: `127.0.0.1:5057`, token `dgm`, `surface.npy`
