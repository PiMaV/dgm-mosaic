# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- Standalone GeoTIFF load without OpenCV (classic strips **and** tiled LGL
  DGM TIFFs via `tiffio`).
- Clear progress while loading tile previews and while building/sending.
- **u16dm** = fixed **1 dm/DN** (metres×10): 751.3 m → 7513. No auto-rescale.
  Overflow → error, use **f32**. Removed **u8stretch**. **u8step** errors
  instead of silent clip. Default format: **f32**.

### Changed

- Preview normalize defaults to **whole mosaic** (shared height scale); optional
  per-tile via checkbox off.
- Primary button is **Stream** (BLITZ or DONNER Viewer Contract), not “Send to BLITZ”.
- Selection UX: drag rectangle grows live; status shows output **W×H px** and
  **point count**.

### Added

- Preview colormaps: blue–white–red, grayscale, terrain, plasma, viridis.
- Optional per-tile contrast normalize (checkbox off = whole mosaic).

## [0.1.0] - 2026-09-18

### Added

- Early PyQt stage spike + PyInstaller CI release layout (port 5056 / `dgm`).
- Optional Go CLI mosaic + hub (not the product path).

### Known issues

- **Not functional:** OpenCV was not bundled; tile preview / TIFF load fails.
- Go CLI alone does not replace the DnD mosaic stage.
