# -*- mode: python ; coding: utf-8 -*-
from pathlib import Path

repo_root = Path.cwd()
_icon_dir = repo_root / "icon"
_icon_ico = _icon_dir / "icon_64.ico"
_datas = []
if _icon_ico.is_file():
    _datas.append((str(_icon_ico), "icon"))

a = Analysis(
    ["dgm_mosaic_main.py"],
    pathex=[str(repo_root)],
    binaries=[],
    datas=_datas,
    hiddenimports=[
        "engineio.async_drivers.threading",
        "flask_socketio",
        "PyQt6",
        "socketio",
        "dgm_mosaic",
        "dgm_mosaic.app",
        "dgm_mosaic.app_icon",
        "dgm_mosaic.mosaic",
        "dgm_mosaic.tiffio",
    ],
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=["eventlet"],
    noarchive=False,
    optimize=0,
)
pyz = PYZ(a.pure)

exe = EXE(
    pyz,
    a.scripts,
    a.binaries,
    a.zipfiles,
    a.datas,
    [],
    name="DGM",
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=True,
    upx_exclude=[],
    runtime_tmpdir=None,
    console=True,
    disable_windowed_traceback=False,
    argv_emulation=False,
    target_arch=None,
    codesign_identity=None,
    entitlements_file=None,
    icon=str(_icon_ico) if _icon_ico.is_file() else None,
)
