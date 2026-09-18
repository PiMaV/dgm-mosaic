# BACKLOG

## Blockers (not yet functional)

- [ ] **OpenCV in the shipped binary** — `dgm_mosaic.mosaic` reads / resizes
      GeoTIFFs via `cv2`. v0.1.0 PyInstaller build and `pyproject.toml` at tag
      time omitted it → preview and load fail. Add `opencv-python-headless`,
      rebuild, re-test tile folder drop end-to-end.
- [ ] **Honest release** — do not present CLI or incomplete GUI as a ready
      sidecar until preview + Send to BLITZ work on a real LGL tile set.

## Product polish (after green path)

- [ ] Drop or demote Go CLI in README / release assets (low value vs DnD stage).
- [ ] Confirm PyInstaller hiddenimports for `cv2` / OpenCV plugins.
- [ ] WWM: drop folder → preview → rectangle → Send → BLITZ Stream 5056/`dgm`.
- [ ] Landing / suite docs: only link once status is functional.

## Parked elsewhere

HIKMICRO and DICOM sidecars stay parked (see suite `HIKMICRO/PARKED.md`,
`DICOM/PARKED.md`).
