"""Window/taskbar icon from icon/icon_64.ico (bundled under PyInstaller)."""
from __future__ import annotations

import sys
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from PyQt6.QtWidgets import QWidget


def get_icon_path(basename: str = "icon_64.ico", relative_to: Path | None = None) -> Path | None:
    if getattr(sys, "frozen", False):
        base = Path(sys._MEIPASS)  # type: ignore[attr-defined]
    else:
        base = (relative_to or Path(__file__).resolve().parent.parent)
    p = base / "icon" / basename
    return p if p.is_file() else None


def set_window_icon(window: "QWidget", basename: str = "icon_64.ico", relative_to: Path | None = None) -> None:
    from PyQt6.QtGui import QIcon

    p = get_icon_path(basename=basename, relative_to=relative_to)
    if p:
        window.setWindowIcon(QIcon(str(p)))
