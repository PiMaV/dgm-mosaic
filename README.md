# DGM mosaic

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue)](LICENSE)
[![Status](https://img.shields.io/badge/status-not%20yet%20functional-orange)](BACKLOG.md)

Part of [WETTER](https://wetter.mess.engineering).

> **Not yet functional (v0.1.0).** The GitHub release binary and the default
> dependency set omit OpenCV, so **tile preview / GeoTIFF load fails** at
> runtime. Treat this as an early spike on its own repo — not a usable tool.
> See [BACKLOG.md](BACKLOG.md).

Intended product: drop a folder of DGM GeoTIFF tiles, preview the mosaic,
drag a rectangle of tiles, **Send to BLITZ** or DONNER
(`http://127.0.0.1:5056`, token `dgm`).

## After it works

Download the release binary. Linux: `chmod +x`, then run. Drop a tile folder
(Browse is fallback only).

## Develop (local)

```bash
uv sync --group dev
uv run pytest -q
uv run dgm-mosaic
```

OpenCV (`opencv-python-headless`) must be installed for TIFF read/preview.

The Go CLI under `cmd/` is a headless experiment only — not the product path.

Agents: [`docs/llm-brief.md`](docs/llm-brief.md).
