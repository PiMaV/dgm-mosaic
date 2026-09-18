"""DGM mosaic window + one-shot BLITZ push (WOLKE contract, not WOLKE)."""
from __future__ import annotations

import argparse
import io
import logging
import sys
import threading
from pathlib import Path

import numpy as np
from PyQt6.QtCore import QObject, QPoint, QRect, QThread, Qt, pyqtSignal
from PyQt6.QtGui import (
    QColor,
    QDragEnterEvent,
    QDropEvent,
    QImage,
    QMouseEvent,
    QPainter,
    QPen,
    QPixmap,
)
from PyQt6.QtWidgets import (
    QApplication,
    QButtonGroup,
    QComboBox,
    QDoubleSpinBox,
    QFileDialog,
    QGroupBox,
    QHBoxLayout,
    QLabel,
    QLineEdit,
    QMainWindow,
    QMessageBox,
    QPushButton,
    QRadioButton,
    QVBoxLayout,
    QWidget,
)

from dgm_mosaic.mosaic import (
    NODATA_DEFAULT,
    DtypeMode,
    MosaicLayout,
    compose_preview_rgb,
    convert,
    default_output_path,
    float_thumbs_to_unit,
    fmt_mb,
    inspect_folder,
    legend_rgb,
    mosaic_array,
    tile_thumbnail,
)

log = logging.getLogger("dgm_mosaic")

DEFAULT_HOST = "127.0.0.1"
DEFAULT_PORT = 5056
DEFAULT_TOKEN = "dgm"
STACK_NAME = "mosaic.npy"

_MODES: tuple[tuple[DtypeMode, str], ...] = (
    ("u16cm", "uint16 centimetres (default)"),
    ("u8stretch", "uint8 min…max → 1…255"),
    ("u8step", "uint8 fixed step"),
    ("f32", "float32 metres"),
)


class MosaicPublisher:
    """Holds the latest mosaic; Socket.IO + HTTP .npy like the Event reader."""

    def __init__(
        self,
        host: str = DEFAULT_HOST,
        port: int = DEFAULT_PORT,
        token: str = DEFAULT_TOKEN,
    ) -> None:
        from flask import Flask, abort, request
        from flask_socketio import SocketIO

        self.host = host
        self.port = port
        self.token = token
        self._stack: np.ndarray | None = None
        self._lock = threading.RLock()
        self._thread: threading.Thread | None = None
        self._clients = 0
        self._app = Flask("dgm_mosaic")
        self._sio = SocketIO(
            self._app, cors_allowed_origins="*", async_mode="threading"
        )
        self._register(abort, request)

    @property
    def base_url(self) -> str:
        return f"http://{self.host}:{self.port}"

    @property
    def connect_hint(self) -> str:
        return f"{self.base_url}  token {self.token}"

    def set_stack(self, stack: np.ndarray, push: bool = True) -> None:
        arr = np.ascontiguousarray(stack)
        with self._lock:
            self._stack = arr
        if push:
            self.push()

    def push(self) -> None:
        self._sio.emit("send_file_message", {"file_name": STACK_NAME})

    def start_background(self) -> None:
        if self._thread and self._thread.is_alive():
            return

        def _run() -> None:
            self._sio.run(
                self._app,
                host=self.host,
                port=self.port,
                debug=False,
                use_reloader=False,
                allow_unsafe_werkzeug=True,
            )

        self._thread = threading.Thread(
            target=_run, name="dgm-mosaic-http", daemon=True
        )
        self._thread.start()

    def _register(self, abort, request) -> None:
        publisher = self
        app = self._app
        sio = self._sio

        @app.get("/<tok>")
        def get_file(tok: str):
            if tok != publisher.token:
                return abort(404)
            if not request.args.get("filename"):
                return abort(400)
            with publisher._lock:
                stack = publisher._stack
            if stack is None:
                return abort(404)
            buf = io.BytesIO()
            np.save(buf, stack)
            raw = buf.getvalue()
            from flask import Response

            return Response(
                raw,
                mimetype="application/octet-stream",
                headers={
                    "Content-Disposition": f'attachment; filename="{STACK_NAME}"',
                    "Content-Length": str(len(raw)),
                },
            )

        @sio.on("connect")
        def on_connect():
            publisher._clients += 1
            sio.emit("Connected successfully")
            with publisher._lock:
                ready = publisher._stack is not None
            if ready:
                sio.emit("send_file_message", {"file_name": STACK_NAME})

        @sio.on("disconnect")
        def on_disconnect():
            publisher._clients = max(0, publisher._clients - 1)


class _Work(QObject):
    finished_array = pyqtSignal(object, object)
    finished_path = pyqtSignal(str)
    failed = pyqtSignal(str)

    def __init__(self, kind: str, kwargs: dict) -> None:
        super().__init__()
        self.kind = kind
        self.kwargs = kwargs

    def run(self) -> None:
        try:
            if self.kind == "send":
                arr, meta = mosaic_array(**self.kwargs)
                self.finished_array.emit(arr, meta)
            else:
                path = convert(**self.kwargs)
                self.finished_path.emit(str(path))
        except Exception as exc:  # noqa: BLE001
            self.failed.emit(str(exc))


class _ThumbWork(QObject):
    finished = pyqtSignal(int, object)
    failed = pyqtSignal(int, str)

    def __init__(self, gen: int, cells: dict, rows: int, cols: int) -> None:
        super().__init__()
        self.gen = gen
        self.cells = cells
        self.rows = rows
        self.cols = cols

    def run(self) -> None:
        try:
            raw = {}
            for key, path in self.cells.items():
                raw[key] = tile_thumbnail(path)
            unit = float_thumbs_to_unit(raw)
            rgb = compose_preview_rgb(self.rows, self.cols, unit)
            self.finished.emit(self.gen, rgb)
        except Exception as exc:  # noqa: BLE001
            self.failed.emit(self.gen, str(exc))


class MosaicView(QWidget):
    """Edge-to-edge mosaic preview; drag a rectangle of tiles to export."""

    selectionChanged = pyqtSignal()

    def __init__(self) -> None:
        super().__init__()
        self.setMinimumHeight(320)
        self.setCursor(Qt.CursorShape.CrossCursor)
        self._pixmap: QPixmap | None = None
        self._rows = 1
        self._cols = 1
        self._labels: dict[tuple[int, int], str] = {}
        self._box = (0, 0, 0, 0)
        self._drag: tuple[int, int] | None = None

    def tile_box(self) -> tuple[int, int, int, int]:
        return self._box

    def set_mosaic(
        self,
        rgb: np.ndarray,
        rows: int,
        cols: int,
        labels: dict[tuple[int, int], str],
        *,
        keep_selection: bool = False,
    ) -> None:
        arr = np.ascontiguousarray(rgb, dtype=np.uint8)
        h, w = arr.shape[:2]
        qimg = QImage(arr.data, w, h, 3 * w, QImage.Format.Format_RGB888).copy()
        self._pixmap = QPixmap.fromImage(qimg)
        self._rows = max(1, rows)
        self._cols = max(1, cols)
        self._labels = labels
        if not keep_selection:
            self._box = (0, 0, self._rows - 1, self._cols - 1)
        else:
            r0, c0, r1, c1 = self._box
            self._box = (
                min(r0, self._rows - 1),
                min(c0, self._cols - 1),
                min(r1, self._rows - 1),
                min(c1, self._cols - 1),
            )
        self.update()
        self.selectionChanged.emit()

    def select_all(self) -> None:
        self._box = (0, 0, self._rows - 1, self._cols - 1)
        self.update()
        self.selectionChanged.emit()

    def _image_rect(self) -> QRect:
        if self._pixmap is None or self._pixmap.isNull():
            return QRect()
        w, h = self._pixmap.width(), self._pixmap.height()
        wr, hr = max(self.width(), 1), max(self.height(), 1)
        scale = min(wr / w, hr / h)
        nw, nh = max(1, int(w * scale)), max(1, int(h * scale))
        x = (wr - nw) // 2
        y = (hr - nh) // 2
        return QRect(x, y, nw, nh)

    def _tile_at(self, pos: QPoint, *, clamp: bool = False) -> tuple[int, int] | None:
        rect = self._image_rect()
        if rect.isEmpty():
            return None
        if not rect.contains(pos):
            if not clamp:
                return None
            x = min(max(pos.x(), rect.x()), rect.right())
            y = min(max(pos.y(), rect.y()), rect.bottom())
            pos = QPoint(x, y)
        fx = (pos.x() - rect.x()) / max(rect.width(), 1)
        fy = (pos.y() - rect.y()) / max(rect.height(), 1)
        c = min(self._cols - 1, max(0, int(fx * self._cols)))
        r = min(self._rows - 1, max(0, int(fy * self._rows)))
        return r, c

    def mousePressEvent(self, event: QMouseEvent) -> None:  # noqa: N802
        if event.button() != Qt.MouseButton.LeftButton:
            return
        tile = self._tile_at(event.position().toPoint())
        if tile is None:
            return
        self._drag = tile
        self._box = (tile[0], tile[1], tile[0], tile[1])
        self.update()

    def mouseMoveEvent(self, event: QMouseEvent) -> None:  # noqa: N802
        if self._drag is None:
            return
        tile = self._tile_at(event.position().toPoint(), clamp=True)
        if tile is None:
            return
        r0 = min(self._drag[0], tile[0])
        c0 = min(self._drag[1], tile[1])
        r1 = max(self._drag[0], tile[0])
        c1 = max(self._drag[1], tile[1])
        box = (r0, c0, r1, c1)
        if box != self._box:
            self._box = box
            self.update()

    def mouseReleaseEvent(self, event: QMouseEvent) -> None:  # noqa: N802
        if event.button() != Qt.MouseButton.LeftButton:
            return
        if self._drag is not None:
            self._drag = None
            self.selectionChanged.emit()

    def paintEvent(self, event) -> None:  # noqa: N802
        del event
        p = QPainter(self)
        p.fillRect(self.rect(), QColor("#141414"))
        dest = self._image_rect()
        if self._pixmap is None or dest.isEmpty():
            p.setPen(QColor("#888"))
            p.drawText(self.rect(), Qt.AlignmentFlag.AlignCenter, "Drop a tile folder")
            return
        p.drawPixmap(dest, self._pixmap)
        cell_w = dest.width() / self._cols
        cell_h = dest.height() / self._rows
        font = p.font()
        font.setPixelSize(max(10, int(min(cell_w, cell_h) * 0.11)))
        font.setBold(True)
        p.setFont(font)
        for r in range(self._rows):
            for c in range(self._cols):
                text = self._labels.get((r, c), "—")
                tr = QRect(
                    int(dest.x() + c * cell_w) + 4,
                    int(dest.y() + r * cell_h) + 3,
                    max(8, int(cell_w) - 8),
                    max(14, int(cell_h * 0.22)),
                )
                p.setPen(QColor(0, 0, 0, 200))
                p.drawText(
                    tr.translated(1, 1),
                    Qt.AlignmentFlag.AlignLeft | Qt.AlignmentFlag.AlignTop,
                    text,
                )
                p.setPen(QColor("#ffffff"))
                p.drawText(
                    tr,
                    Qt.AlignmentFlag.AlignLeft | Qt.AlignmentFlag.AlignTop,
                    text,
                )
        r0, c0, r1, c1 = self._box
        sel = QRect(
            int(dest.x() + c0 * cell_w),
            int(dest.y() + r0 * cell_h),
            max(1, int((c1 - c0 + 1) * cell_w)),
            max(1, int((r1 - r0 + 1) * cell_h)),
        )
        p.fillRect(sel, QColor(255, 255, 255, 32))
        p.setPen(QPen(QColor("#ffe066"), 2))
        p.drawRect(sel.adjusted(1, 1, -1, -1))


class DgmMosaicWindow(QMainWindow):
    def __init__(self, folder: Path | None = None) -> None:
        super().__init__()
        self.setWindowTitle("DGM mosaic → BLITZ")
        self.setAcceptDrops(True)
        self.resize(820, 900)
        self._folder: Path | None = None
        self._layout: MosaicLayout | None = None
        self._thread: QThread | None = None
        self._worker: _Work | None = None
        self._thumb_thread: QThread | None = None
        self._thumb_worker: _ThumbWork | None = None
        self._thumb_gen = 0
        self._publisher: MosaicPublisher | None = None
        self._push_error: str | None = None
        try:
            self._publisher = MosaicPublisher()
            self._publisher.start_background()
        except Exception as exc:  # noqa: BLE001
            self._push_error = str(exc)

        root = QWidget()
        self.setCentralWidget(root)
        v = QVBoxLayout(root)

        folder_row = QHBoxLayout()
        self.folder_edit = QLineEdit()
        self.folder_edit.setPlaceholderText("Drop a tile folder, or Browse…")
        self.folder_edit.setReadOnly(True)
        browse = QPushButton("Browse…")
        browse.clicked.connect(self._browse)
        folder_row.addWidget(self.folder_edit, 1)
        folder_row.addWidget(browse)
        v.addLayout(folder_row)

        self.meta_label = QLabel("No folder loaded.")
        self.meta_label.setWordWrap(True)
        v.addWidget(self.meta_label)

        north = QLabel("N ↑")
        north.setAlignment(Qt.AlignmentFlag.AlignCenter)
        v.addWidget(north)

        self.mosaic_view = MosaicView()
        self.mosaic_view.selectionChanged.connect(self._on_selection)
        v.addWidget(self.mosaic_view, 1)

        east = QLabel("west  ←  tiles  →  east")
        east.setAlignment(Qt.AlignmentFlag.AlignCenter)
        v.addWidget(east)

        sel_row = QHBoxLayout()
        self.sel_label = QLabel("Drag a rectangle of tiles to export.")
        self.sel_label.setWordWrap(True)
        all_btn = QPushButton("All tiles")
        all_btn.clicked.connect(self.mosaic_view.select_all)
        sel_row.addWidget(self.sel_label, 1)
        sel_row.addWidget(all_btn)
        v.addLayout(sel_row)

        legend_row = QHBoxLayout()
        self.legend_img = QLabel()
        self.legend_img.setFixedHeight(14)
        self._set_legend()
        legend_cap = QLabel("low  blue   ·   mid  white   ·   high  red")
        legend_cap.setStyleSheet("color: #bbb; font-size: 11px;")
        legend_row.addWidget(self.legend_img, 1)
        legend_row.addWidget(legend_cap)
        v.addLayout(legend_row)

        fmt = QGroupBox("Format  (size = selected tiles)")
        fmt_l = QVBoxLayout(fmt)
        self.step_m = QDoubleSpinBox()
        self.step_m.setRange(0.01, 50.0)
        self.step_m.setDecimals(3)
        self.step_m.setValue(0.25)
        self.step_m.setSuffix(" m")
        self.ref = QComboBox()
        self.ref.addItems(["min", "mean"])
        self.size_label = QLabel()
        self.mode_group = QButtonGroup(self)
        self.mode_buttons: dict[DtypeMode, QRadioButton] = {}
        for i, (mode, label) in enumerate(_MODES):
            btn = QRadioButton(label)
            self.mode_group.addButton(btn, i)
            self.mode_buttons[mode] = btn
            fmt_l.addWidget(btn)
            btn.toggled.connect(self._refresh_sizes)
        step_row = QHBoxLayout()
        step_row.addWidget(QLabel("u8step:"))
        step_row.addWidget(self.step_m)
        step_row.addWidget(QLabel("ref"))
        step_row.addWidget(self.ref)
        step_row.addStretch(1)
        fmt_l.addLayout(step_row)
        fmt_l.addWidget(self.size_label)
        v.addWidget(fmt)
        self.mode_buttons["u16cm"].setChecked(True)

        if self._publisher is not None:
            connect = (
                f"BLITZ Stream → Connect  {self._publisher.connect_hint}\n"
                "Not WOLKE — same contract as the Event reader (one mosaic push)."
            )
        else:
            connect = self._push_error or "Push server unavailable."
        self.connect_label = QLabel(connect)
        self.connect_label.setWordWrap(True)
        v.addWidget(self.connect_label)

        self.send_btn = QPushButton("Send to BLITZ")
        self.send_btn.setEnabled(False)
        self.send_btn.clicked.connect(self._send)
        v.addWidget(self.send_btn)

        save_row = QHBoxLayout()
        self.out_edit = QLineEdit()
        self.out_edit.setPlaceholderText("Optional: also write .npy")
        save_btn = QPushButton("Save .npy…")
        save_btn.clicked.connect(self._save)
        save_row.addWidget(self.out_edit, 1)
        save_row.addWidget(save_btn)
        v.addLayout(save_row)
        self._save_btn = save_btn
        save_btn.setEnabled(False)

        self.status = QLabel("Drop a folder of DGM TIFFs.")
        self.status.setWordWrap(True)
        v.addWidget(self.status)

        if folder is not None:
            self._load_folder(folder)

    def dragEnterEvent(self, event: QDragEnterEvent) -> None:  # noqa: N802
        if event.mimeData().hasUrls():
            event.acceptProposedAction()

    def dropEvent(self, event: QDropEvent) -> None:  # noqa: N802
        urls = event.mimeData().urls()
        if not urls:
            return
        path = Path(urls[0].toLocalFile())
        if path.is_file():
            path = path.parent
        if path.is_dir():
            self._load_folder(path)

    def _browse(self) -> None:
        start = str(self._folder or Path.home())
        picked = QFileDialog.getExistingDirectory(self, "DGM tile folder", start)
        if picked:
            self._load_folder(Path(picked))

    def _mode(self) -> DtypeMode:
        for mode, btn in self.mode_buttons.items():
            if btn.isChecked():
                return mode
        return "u16cm"

    def _kwargs(self, output: Path | None = None) -> dict:
        kw: dict = {
            "input_path": self._folder,
            "mode": self._mode(),
            "step_m": float(self.step_m.value()),
            "ref": self.ref.currentText(),
            "tile_box": self.mosaic_view.tile_box(),
        }
        if output is not None:
            kw["output_path"] = output
        return kw

    def _load_folder(self, folder: Path) -> None:
        try:
            layout = inspect_folder(folder)
        except Exception as exc:  # noqa: BLE001
            QMessageBox.warning(self, "Cannot read folder", str(exc))
            return
        self._folder = folder
        self._layout = layout
        self.folder_edit.setText(str(folder))
        self.out_edit.setText(str(default_output_path(folder)))
        self.send_btn.setEnabled(self._publisher is not None)
        self._save_btn.setEnabled(True)
        km = (layout.east - layout.west) / 1000.0
        kn = (layout.north - layout.south) / 1000.0
        hole = f", {layout.holes} hole(s)" if layout.holes else ""
        crs = f", {layout.crs}" if layout.crs else ""
        self.meta_label.setText(
            f"{len(layout.stubs)} tiles → {layout.width}×{layout.height} px "
            f"({layout.pixel_m:g} m)  ·  {km:g}×{kn:g} km{hole}{crs}"
        )
        self._rebuild_grid(layout)
        self._refresh_sizes()
        self._start_thumbs(layout)
        if self._publisher is None:
            self.status.setText("Loading tile previews…")
        else:
            self.status.setText(
                f"Loading tile previews…  Then Connect BLITZ Stream to "
                f"{self._publisher.connect_hint} and Send to BLITZ."
            )

    def _set_legend(self) -> None:
        rgb = legend_rgb(280, 12)
        h, w = rgb.shape[:2]
        qimg = QImage(rgb.data, w, h, 3 * w, QImage.Format.Format_RGB888).copy()
        self.legend_img.setPixmap(
            QPixmap.fromImage(qimg).scaled(
                280,
                12,
                Qt.AspectRatioMode.IgnoreAspectRatio,
                Qt.TransformationMode.SmoothTransformation,
            )
        )

    def _on_selection(self) -> None:
        layout = self._layout
        if layout is None:
            self.sel_label.setText("Drag a rectangle of tiles to export.")
            return
        r0, c0, r1, c1 = self.mosaic_view.tile_box()
        nr, nc = r1 - r0 + 1, c1 - c0 + 1
        holes = 0
        for r in range(r0, r1 + 1):
            for c in range(c0, c1 + 1):
                if layout.cells.get((r, c)) is None:
                    holes += 1
        hole = f", {holes} hole(s) → 0" if holes else ""
        self.sel_label.setText(
            f"Export {nr}×{nc} tiles  (rows {r0}…{r1}, cols {c0}…{c1}){hole}"
        )
        self._refresh_sizes()

    def _rebuild_grid(self, layout: MosaicLayout) -> None:
        labels = {
            key: stub.label_km for key, stub in layout.cells.items()
        }
        ph = np.zeros(
            (max(1, layout.tile_rows) * 64, max(1, layout.tile_cols) * 64, 3),
            dtype=np.uint8,
        )
        self.mosaic_view.set_mosaic(ph, layout.tile_rows, layout.tile_cols, labels)

    def _start_thumbs(self, layout: MosaicLayout) -> None:
        self._thumb_gen += 1
        gen = self._thumb_gen
        jobs = {key: stub.path for key, stub in layout.cells.items()}
        if not jobs:
            return
        self._thumb_thread = QThread()
        self._thumb_worker = _ThumbWork(
            gen, jobs, layout.tile_rows, layout.tile_cols
        )
        self._thumb_worker.moveToThread(self._thumb_thread)
        self._thumb_thread.started.connect(self._thumb_worker.run)
        self._thumb_worker.finished.connect(self._on_thumbs)
        self._thumb_worker.failed.connect(self._on_thumbs_fail)
        self._thumb_worker.finished.connect(self._thumb_thread.quit)
        self._thumb_worker.failed.connect(self._thumb_thread.quit)
        self._thumb_thread.start()

    def _on_thumbs(self, gen: int, rgb: object) -> None:
        if gen != self._thumb_gen or self._layout is None:
            return
        layout = self._layout
        labels = {key: stub.label_km for key, stub in layout.cells.items()}
        self.mosaic_view.set_mosaic(
            np.asarray(rgb),
            layout.tile_rows,
            layout.tile_cols,
            labels,
            keep_selection=True,
        )
        hint = ""
        if self._publisher is not None:
            hint = f" Connect BLITZ Stream to {self._publisher.connect_hint}, then Send."
        self.status.setText(f"Previews ready.{hint}")

    def _on_thumbs_fail(self, gen: int, message: str) -> None:
        if gen != self._thumb_gen:
            return
        self.status.setText(f"Preview failed: {message}")

    def _refresh_sizes(self) -> None:
        layout = self._layout
        step_on = self._mode() == "u8step"
        self.step_m.setEnabled(step_on)
        self.ref.setEnabled(step_on)
        if layout is None:
            self.size_label.setText("Load a folder to see output size.")
            return
        chosen = self._mode()
        box = self.mosaic_view.tile_box()
        lines = []
        for mode, _label in _MODES:
            mark = "←" if mode == chosen else " "
            lines.append(f"{mark} {mode}: {fmt_mb(layout.nbytes_box(mode, box))}")
        self.size_label.setText("\n".join(lines))

    def _busy(self, on: bool) -> None:
        self.send_btn.setEnabled(
            (not on) and self._publisher is not None and self._folder is not None
        )
        self._save_btn.setEnabled((not on) and self._folder is not None)

    def _start_work(self, kind: str, kwargs: dict) -> None:
        self._busy(True)
        self.status.setText("Reading TIFFs and building mosaic…")
        self._thread = QThread()
        self._worker = _Work(kind, kwargs)
        self._worker.moveToThread(self._thread)
        self._thread.started.connect(self._worker.run)
        self._worker.finished_array.connect(self._on_sent)
        self._worker.finished_path.connect(self._on_saved)
        self._worker.failed.connect(self._on_fail)
        self._worker.finished_array.connect(self._thread.quit)
        self._worker.finished_path.connect(self._thread.quit)
        self._worker.failed.connect(self._thread.quit)
        self._thread.start()

    def _send(self) -> None:
        if self._folder is None or self._publisher is None:
            if self._push_error:
                QMessageBox.warning(self, "Send to BLITZ", self._push_error)
            return
        self._start_work("send", self._kwargs())

    def _save(self) -> None:
        if self._folder is None:
            return
        start = self.out_edit.text() or str(default_output_path(self._folder))
        picked, _ = QFileDialog.getSaveFileName(
            self, "Mosaic .npy", start, "NumPy (*.npy)"
        )
        if not picked:
            return
        self.out_edit.setText(picked)
        self._start_work("save", self._kwargs(output=Path(picked)))

    def _on_sent(self, arr, meta: dict) -> None:
        assert self._publisher is not None
        self._publisher.set_stack(arr, push=True)
        mb = arr.nbytes / (1024 * 1024)
        mode = meta.get("mode")
        self._busy(False)
        self.status.setText(
            f"Pushed {mode} {arr.shape[1]}×{arr.shape[0]} ({mb:.1f} MB) to BLITZ.\n"
            f"Stream should already be connected to {self._publisher.connect_hint}."
        )

    def _on_saved(self, path: str) -> None:
        self._busy(False)
        self.status.setText(f"Wrote {path}")

    def _on_fail(self, message: str) -> None:
        self._busy(False)
        self.status.setText(f"Error: {message}")
        QMessageBox.warning(self, "Mosaic failed", message)


def run_gui(folder: Path | None = None) -> int:
    app = QApplication.instance() or QApplication(sys.argv)
    win = DgmMosaicWindow(folder)
    win.show()
    return app.exec()


def main() -> None:
    parser = argparse.ArgumentParser(
        description=(
            "Mosaic DGM GeoTIFF tiles for BLITZ. "
            "No args or --gui opens the window; a folder without --gui stays CLI."
        )
    )
    parser.add_argument(
        "input",
        nargs="?",
        default=None,
        help="Tile folder or a single .tif",
    )
    parser.add_argument("--gui", action="store_true")
    parser.add_argument("-o", "--output", default=None)
    parser.add_argument(
        "--dtype",
        choices=("u16cm", "u8stretch", "u8step", "f32"),
        default="u16cm",
    )
    parser.add_argument("--step-m", type=float, default=0.25)
    parser.add_argument("--ref", choices=("min", "mean"), default="min")
    parser.add_argument("--nodata", type=float, default=NODATA_DEFAULT)
    parser.add_argument("--z0", type=float, default=None)
    args = parser.parse_args()
    if args.gui or args.input is None:
        sys.exit(run_gui(Path(args.input) if args.input else None))
    try:
        convert(
            Path(args.input),
            Path(args.output) if args.output else None,
            mode=args.dtype,
            step_m=args.step_m,
            ref=args.ref,
            nodata=args.nodata,
            z0=args.z0,
        )
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
