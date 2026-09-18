# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Status

**Not yet functional.** Own GitHub repo `PiMaV/dgm-mosaic`. Tag `v0.1.0`
ships binaries, but GeoTIFF preview/load needs OpenCV (`cv2`) and that was
missing from the release dependency/PyInstaller set. Do not treat as a
shipping sidecar until `BACKLOG.md` blockers are cleared.

CLI (`cmd/dgm-mosaic`) is headless-only and not the product path.

## Product (intended)

**Sidecar** for DGM GeoTIFF tile folders → mosaic → WETTER Viewer Contract
(port **5056**, token `dgm`). Clients: BLITZ, DONNER.

**Shipped binary (when green):** PyInstaller one-file — PyQt6 DnD stage: drop
folder, tile mosaic preview, rectangle select, Send.

## Do

- Prefer real filesystem paths (DnD). Browse is fallback only.
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.
- Keep OpenCV (or a chosen TIFF reader) in the binary deps once unblocked.

## Don’t

- Do not claim the tool works while OpenCV is absent from the freeze.
- Do not present the Go CLI as the user product.
- Do not merge HIK/DICOM into this binary.

## Key paths

| Path | Role |
|------|------|
| `dgm_mosaic/` | DnD GUI + mosaic math (`cv2` for TIFF) |
| `DGM.spec` / `dgm_mosaic_main.py` | PyInstaller entry |
| `BACKLOG.md` | Blockers before “functional” |
| `cmd/dgm-mosaic` | Optional Go CLI (low priority) |

## Defaults

`127.0.0.1:5056`, token `dgm`, stack name `mosaic.npy`.
