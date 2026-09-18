# DGM mosaic — LLM brief

Machine-oriented product brief. Humans use [README.md](../README.md).

## Product

Standalone **sidecar**: DGM GeoTIFF folder → mosaic → WETTER Viewer Contract hub (port **5056**, token `dgm`). Clients: BLITZ, DONNER. One binary, embedded web UI + Socket.IO hub.

Not part of BLITZ core. Not WOLKE. EVT stays a separate Python product.

## Do

- Placement from LGL names and/or `.tfw`; refuse rotation / off-grid / BigTIFF / compressed TIFF.
- Quantize: `u16cm` | `u8stretch` | `u8step` | `f32`. Preview blue–white–red.
- Folder pick via native dialog (zenity/kdialog); no browser drop without a real path.
- Wire: Socket.IO `send_file_message` + GET `/{token}?filename=…` → `.npy`.

## Don’t

- Do not embed GDAL or Qt.
- Do not merge into `converters/` or EVT.
- Do not invent a second live protocol.

## Key paths

| Path | Role |
|------|------|
| `cmd/dgm-mosaic` | CLI + GUI entry |
| `internal/hub` | Viewer Contract server |
| `internal/mosaic` | Layout, mosaic, quantize, preview |
| `internal/tiff` | Classic single-band TIFF reader |
| `internal/ui` | Embedded HTML stage |

## Defaults

`127.0.0.1:5056`, token `dgm`, stack name `mosaic.npy`.
