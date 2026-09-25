# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.4.0] - 2026-09-25

### Changed

- **u16dm** defaults to **absolute** decimetres (`z0=0`): 403.2 m → 4032. No
  more `floor(z_min)` offset. Fails clearly above ~6553.5 m NN (use f32).
  Optional CLI `--z0` remains for rare relative encoding.

### Removed

- Go CLI binary no longer built or attached on GitHub Releases (product path is
  the PyQt `DGM` binary only).
- Dual Stream hub / DONNER elevation **surface** (`:5057`, `surface.npy`).
  Terrain stays a `(1, H, W)` plane for **BLITZ** only — no voxel shell path.

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
