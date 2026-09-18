# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed

- Product UI is the PyQt stage in suite `converters/` (`uv run dgm-mosaic`):
  tile preview, rectangle select, drag-drop. Embedded browser UI removed.
- Default Go build is CLI + hub; GUI stub points at converters. Optional Fyne
  behind `-tags fyne` (needs system OpenGL/X11).

### Added

- Go CLI mosaic + Viewer Contract hub (port 5056 / token `dgm`).
- Classic TIFF float/int reader without GDAL; LGL name + `.tfw` placement.

## [0.1.0] - 2026-09-18

### Added

- Initial public product line as a WETTER Converted-path sidecar.
