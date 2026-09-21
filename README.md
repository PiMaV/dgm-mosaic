# DGM mosaic

[![License](https://img.shields.io/badge/license-GPL--3.0--or--later-blue)](LICENSE)

Part of [WETTER](https://wetter.mess.engineering).

Drop a folder of DGM GeoTIFF tiles, preview the mosaic, drag a rectangle of
tiles, **Send to BLITZ** or DONNER.

**Download:** [latest release](https://github.com/PiMaV/dgm-mosaic/releases/latest)
— `DGM-vX.Y.Z-linux-x86_64` or `DGM-vX.Y.Z-windows-x86_64.exe`.

1. Download the file for your system.
2. Linux: `chmod +x DGM-vX.Y.Z-linux-x86_64`
3. Run it. Drop a tile folder (or Browse…).
4. Wait for the preview bar to finish, pick tiles, **Send to BLITZ**.
5. In BLITZ → Stream: `http://127.0.0.1:5056`, token `dgm`.

Optional: **Normalize whole mosaic** (shared height scale; uncheck for per-tile)
and colormap (blue–white–red, grayscale, terrain, plasma, viridis). Drag a
rectangle on the mosaic to select tiles; **Stream** pushes to BLITZ or DONNER.

GNU GPL v3. See [LICENSE](LICENSE).

For agents: [`docs/llm-brief.md`](docs/llm-brief.md).
