# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Product

**Sidecar** for DGM GeoTIFF tile folders → mosaic → WETTER Viewer Contract
(port **5056**, token `dgm`). Clients: BLITZ, DONNER.

**Product UI (mandatory):** PyQt stage in suite `../converters/` —
`uv run dgm-mosaic`. Drag-drop a tile folder, see the mosaic of tiles, drag a
rectangle to select, Send to BLITZ. Browse is fallback only. Do not ship a
browse-only or browser-only stage as the main path.

**This Go repo:** CLI mosaic + Viewer Contract hub. Optional experimental Fyne
UI (`go build -tags fyne`) needs system OpenGL/X11; default build has no GUI
and points at converters.

Not part of BLITZ core. Not WOLKE. EVT stays a separate Python product.

## Do

- Placement from LGL names and/or `.tfw`; refuse rotation / off-grid / BigTIFF / compressed TIFF.
- Quantize: `u16cm` | `u8stretch` | `u8step` | `f32`. Preview blue–white–red.
- Prefer real filesystem paths (DnD / native dialog). Never depend on browser File APIs without a path.
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.

## Don’t

- Do not treat HTML browse-only as acceptable product UX for this sidecar.
- Do not embed GDAL.
- Do not invent a second live protocol.

## Key paths

| Path | Role |
|------|------|
| `../converters/dgm_mosaic/` | Product DnD GUI (tile preview + box select) |
| `cmd/dgm-mosaic` | CLI + optional Fyne entry |
| `internal/hub` | Viewer Contract server |
| `internal/mosaic` | Layout, mosaic, quantize, preview |
| `internal/tiff` | Classic single-band TIFF reader |
| `internal/ui` | Fyne (`-tags fyne`) or stub → converters |

## Defaults

`127.0.0.1:5056`, token `dgm`, stack name `mosaic.npy`.
