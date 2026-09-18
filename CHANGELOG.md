# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed

- (pending) Add `opencv-python-headless` so GeoTIFF preview/load works in
  `uv run` and the next PyInstaller release.

### Changed

- Documented **v0.1.0 as not yet functional** (OpenCV missing from the binary;
  CLI is not the product path). See `BACKLOG.md`.

## [0.1.0] - 2026-09-18

### Added

- Early PyQt stage spike + PyInstaller CI release layout (port 5056 / `dgm`).
- Optional Go CLI mosaic + hub (not the product path).

### Known issues

- **Not functional:** OpenCV was not bundled; tile preview / TIFF load fails.
- Go CLI alone does not replace the DnD mosaic stage.
