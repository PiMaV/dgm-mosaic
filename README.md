# DGM mosaic

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue)](LICENSE)

Part of [WETTER](https://wetter.mess.engineering).

Drop a folder of DGM GeoTIFF tiles, preview the mosaic, drag a rectangle of
tiles, **Send to BLITZ** or DONNER.

## Download and run

Get the latest release binary. Linux: `chmod +x`, then run. Optional: pass a
tile folder path. Click **Open…** / drop a folder if you start empty.

BLITZ → Stream: `http://127.0.0.1:5056`, token `dgm`.

Drag-and-drop of a tile folder is the primary ingest. Browse is fallback only.

## Develop

```bash
uv sync --group dev
uv run pytest -q
uv run dgm-mosaic
```

Go CLI / hub (optional, same Viewer Contract):

```bash
go test ./...
go build -o bin/dgm-mosaic-cli ./cmd/dgm-mosaic
./bin/dgm-mosaic-cli path/to/tiles -o out.npy
```

Agents: [`docs/llm-brief.md`](docs/llm-brief.md).
