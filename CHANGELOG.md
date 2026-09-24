# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Dual Stream hubs** on Stream: **BLITZ** keeps `:5056` / token `dgm` with
  plane `(1, H, W)` (`mosaic.npy`). **DONNER** gets `:5057` / same token with an
  elevation **surface** `(nZ, H, W)` (`surface.npy`) — one voxel per map cell
  along Z, no filled columns. Full Bin / full selection is sent as-is (no
  preflight size refuse).

### Removed

- Go CLI binary no longer built or attached on GitHub Releases (product path is
  the PyQt `DGM` binary only).
- Preflight “surface would be N MB” refuse on the DONNER hub — Stream sends
  the requested size; DONNER decides what it can load.

## [0.3.0] - 2026-09-21

### Added

- **Bin** factor 1/2/4/8/16 (block mean) before quantize; GUI default **2×**
  (own group with radios); CLI `--bin`. Sidecar `pixel_m` scales with the factor.

### Changed

- Stream / `.npy` always Viewer Contract **`(1, H, W)`** (`axes: THW`). Primary
  client is **BLITZ**. DONNER accepts the shape but has no DEM/height mode yet.

## [0.2.0] - 2026-09-21

### Added

- Standalone GeoTIFF load without OpenCV (classic strips **and** tiled LGL
  DGM TIFFs via `tiffio`).
- Preview colormaps: blue–white–red, grayscale, terrain, plasma, viridis.
- App icon (window + PyInstaller exe).
- README screenshot; LGL BW DGM25 GeoTIFF as example tile source.

### Changed

- Preview normalize defaults to **whole mosaic** (shared height scale); optional
  per-tile via checkbox off.
- Primary button is **Stream** (BLITZ or DONNER Viewer Contract), not “Send to BLITZ”.
- Selection UX: drag rectangle grows live; status shows output **W×H px** and
  **point count**.
- **u16dm** = fixed **1 dm/DN** (metres×10): 751.3 m → 7513. No auto-rescale.
  Overflow → error, use **f32**. Removed **u8stretch**. **u8step** errors
  instead of silent clip. Default format: **f32**.

### Fixed

- Clear progress while loading tile previews and while building/sending.
