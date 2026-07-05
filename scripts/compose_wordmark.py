# /// script
# requires-python = ">=3.11"
# dependencies = ["pillow"]
# ///
"""Compose assets/logo.png with the "rpup" wordmark into assets/logo-wordmark.png.

Run with `uv run scripts/compose_wordmark.py` (or `just wordmark`). The DM Sans
font is cached under tmp/fonts/ and downloaded on first run if absent, so the
script is self-contained and needs no machine-local font path.
"""
import urllib.request
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

BG = (245, 241, 232)
INK = (42, 38, 35)
TEXT = "rpup"
FONT_SIZE = 360

ROOT = Path(__file__).resolve().parent.parent
FONT_PATH = ROOT / "tmp" / "fonts" / "DMSans-Regular.ttf"
FONT_URL = "https://github.com/googlefonts/dm-fonts/raw/main/Sans/fonts/ttf/DMSans-Regular.ttf"


def font() -> ImageFont.FreeTypeFont:
    if not FONT_PATH.exists():
        FONT_PATH.parent.mkdir(parents=True, exist_ok=True)
        urllib.request.urlretrieve(FONT_URL, FONT_PATH)
    return ImageFont.truetype(str(FONT_PATH), FONT_SIZE)


mark = Image.open(ROOT / "assets" / "logo.png").convert("RGB")
mark_w, mark_h = mark.size

f = font()
bbox = f.getbbox(TEXT)
text_w = bbox[2] - bbox[0]
text_h = bbox[3] - bbox[1]

gap = 40
right_pad = 120
canvas_w = mark_w + gap + text_w + right_pad
canvas_h = mark_h

canvas = Image.new("RGB", (canvas_w, canvas_h), BG)
canvas.paste(mark, (0, 0))

draw = ImageDraw.Draw(canvas)
draw.text((mark_w + gap - bbox[0], (canvas_h - text_h) // 2 - bbox[1]), TEXT, font=f, fill=INK)

out = ROOT / "assets" / "logo-wordmark.png"
canvas.save(out)
print(f"Saved: {out.relative_to(ROOT)} ({canvas_w}x{canvas_h})")
