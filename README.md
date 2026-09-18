# DGM mosaic

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue)](LICENSE)

Part of [WETTER](https://wetter.mess.engineering).

## Product path (drag-and-drop GUI)

The working DGM mosaic **stage** is the PyQt tool in the suite converters tree — tile preview, rectangle select, drop a folder, Send to BLITZ:

```bash
cd ../converters
uv sync
uv run dgm-mosaic
```

BLITZ Stream: `http://127.0.0.1:5056`, token `dgm`.

Drag-and-drop of a tile folder is the primary ingest. Browse is fallback only.

## This Go repo

CLI mosaic + Viewer Contract hub (and experimental Fyne UI under `internal/ui`). Native Fyne needs system OpenGL/X11 headers (`libgl1-mesa-dev`, `xorg-dev`, …). Until that builds on every machine, **use `uv run dgm-mosaic` above**.

```bash
go test ./...
go build -o bin/dgm-mosaic ./cmd/dgm-mosaic
./bin/dgm-mosaic path/to/tiles -o out.npy   # CLI
```

Agents: [`docs/llm-brief.md`](docs/llm-brief.md).
