# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Product

**Sidecar** for DGM GeoTIFF tile folders → mosaic → WETTER Viewer Contract
(port **5056**, token `dgm`). Clients: BLITZ, DONNER.

**Shipped binary:** PyInstaller one-file from this repo (`DGM` /
`DGM-vX.Y.Z-…`). PyQt6 stage: drag-drop folder, tile mosaic preview, rectangle
select, Send. CI builds release assets on tag `v*`.

**Also in-repo:** Go CLI + Viewer Contract hub (`cmd/dgm-mosaic`) — headless
export / hub; not the primary download.

Not part of BLITZ core. Not WOLKE. HIKMICRO / DICOM sidecars are parked.

## Do

- Placement from LGL names and/or `.tfw`; refuse rotation / off-grid where encoded.
- Quantize: `u16cm` | `u8stretch` | `u8step` | `f32`. Preview blue–white–red.
- Prefer real filesystem paths (DnD). Browse is fallback only.
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.

## Don’t

- Do not ship browse-only or browser-only as the product stage.
- Do not merge HIK/DICOM into this binary.
- Do not invent a second live protocol.

## Key paths

| Path | Role |
|------|------|
| `dgm_mosaic/` | Product DnD GUI + mosaic math |
| `DGM.spec` / `dgm_mosaic_main.py` | PyInstaller entry |
| `cmd/dgm-mosaic` | Optional Go CLI + hub |
| `internal/hub` | Go Viewer Contract server |
| `.github/workflows/` | test + release binaries |

## Defaults

`127.0.0.1:5056`, token `dgm`, stack name `mosaic.npy`.
