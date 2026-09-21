"""Create icon_64.ico from icon_256.png. Run: uv run python icon/build_ico.py"""
from pathlib import Path

from PIL import Image

ICON_DIR = Path(__file__).resolve().parent
PNG_256 = ICON_DIR / "icon_256.png"
OUT = ICON_DIR / "icon_64.ico"
SIZES = [(16, 16), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]

img = Image.open(PNG_256).convert("RGBA")
if img.size != (256, 256):
    img = img.resize((256, 256), Image.Resampling.LANCZOS)
for s, _ in SIZES[:-1]:
    img.resize((s, s), Image.Resampling.LANCZOS).save(ICON_DIR / f"icon_{s}.png")
img.save(OUT, format="ICO", sizes=SIZES)
print(f"Written {OUT}")
