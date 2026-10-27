#!/usr/bin/env python3
"""Draw the app icon: a rounded deep-petrol square with a paper download glyph.

Run with the workspace Python (needs Pillow and numpy):

    python scripts/make-icon.py
"""

from __future__ import annotations

import pathlib

import numpy as np
from PIL import Image, ImageDraw

SIZE = 1024
SS = 4  # supersampling factor for the glyph and the rounded corners


def background(size: int) -> Image.Image:
    y, x = np.mgrid[0:size, 0:size].astype(np.float32)
    t = np.clip((x / size) * 0.30 + (y / size) * 0.70, 0.0, 1.0)
    # One hue, from lit petrol to deep petrol: depth, not a rainbow gradient.
    start = np.array([0x17, 0x69, 0x7E], dtype=np.float32)
    end = np.array([0x08, 0x3E, 0x4C], dtype=np.float32)
    rgb = start + (end - start) * t[..., None]
    # A soft highlight near the top-left corner.
    dx, dy = (x - size * 0.22) / size, (y - size * 0.10) / size
    glow = np.clip(1.0 - np.sqrt(dx * dx + dy * dy) * 2.1, 0.0, 1.0) ** 2 * 0.16
    rgb = rgb * (1.0 - glow[..., None]) + np.array([0xF5, 0xF2, 0xEA]) * glow[..., None]
    return Image.fromarray(np.clip(rgb, 0, 255).astype(np.uint8), "RGB")


def main() -> None:
    big = SIZE * SS
    base = background(SIZE).resize((big, big), Image.Resampling.BICUBIC)

    icon = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    mask = Image.new("L", (big, big), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, big - 1, big - 1), radius=int(big * 0.225), fill=255
    )
    icon.paste(base, (0, 0), mask)

    glyph = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    d = ImageDraw.Draw(glyph)
    paper = (0xF5, 0xF2, 0xEA, 255)
    cx = big // 2
    w = int(big * 0.088)
    top = int(big * 0.215)
    spread = int(big * 0.150)
    tip = int(big * 0.615)
    # The shaft runs into the chevron's tip, so the V has no gap inside it.
    d.line((cx, top, cx, tip), fill=paper, width=w)
    d.ellipse((cx - w // 2, top - w // 2, cx + w // 2, top + w // 2), fill=paper)
    for sx in (-spread, spread):
        d.line((cx + sx, int(big * 0.435), cx, tip), fill=paper, width=w)
        y = int(big * 0.435)
        d.ellipse((cx + sx - w // 2, y - w // 2, cx + sx + w // 2, y + w // 2), fill=paper)
    d.ellipse((cx - w // 2, tip - w // 2, cx + w // 2, tip + w // 2), fill=paper)

    tray = int(big * 0.195)
    ty = int(big * 0.730)
    d.rounded_rectangle(
        (cx - tray, ty - w // 2, cx + tray, ty + w // 2), radius=w // 2, fill=paper
    )

    icon = Image.alpha_composite(icon, glyph)
    icon = icon.resize((SIZE, SIZE), Image.Resampling.LANCZOS)

    out = pathlib.Path(__file__).resolve().parent.parent / "resources"
    out.mkdir(parents=True, exist_ok=True)
    icon.save(out / "icon.png")
    print(f"wrote {out / 'icon.png'}")


if __name__ == "__main__":
    main()
