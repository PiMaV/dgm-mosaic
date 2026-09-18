# DGM mosaic

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue)](LICENSE)

Part of [WETTER](https://wetter.mess.engineering): mosaic LGL DGM GeoTIFF tiles and stream them to **BLITZ** or **DONNER**.

Download a release binary, `chmod +x`, run it. Click **Browse…**, select a tile rectangle, **Send to BLITZ**.

BLITZ → Stream: `http://127.0.0.1:5056`, token `dgm`.

## Features

- LGL 1 km tiles (`dgm025_32_{e}_{n}_…tif` + optional `.tfw`). No GDAL. No BigTIFF.
- Edge-to-edge preview (blue–white–red). Drag a rectangle of tiles to export.
- Formats: `u16cm` (default), `u8stretch`, `u8step`, `f32`.
- **Send** over the WETTER Viewer Contract (Socket.IO + HTTP `.npy`), or **Save** `.npy` + `.json`.

```bash
# GUI (opens browser)
./dgm-mosaic

# CLI
./dgm-mosaic path/to/tiles --dtype u16cm -o out.npy
```

From source:

```bash
go build -o bin/dgm-mosaic ./cmd/dgm-mosaic
go test ./...
```

Agents: [`docs/llm-brief.md`](docs/llm-brief.md). Architecture: [`architecture.md`](architecture.md).
