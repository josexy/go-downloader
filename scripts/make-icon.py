#!/usr/bin/env python3
"""Draw the app icon: a flat cobalt tile with a white download-to-tray glyph.

Run with the workspace Python (needs Pillow):

    python scripts/make-icon.py
"""

from __future__ import annotations

import math
import pathlib

from PIL import Image, ImageDraw

SIZE = 1024
SS = 4  # supersampling factor for the glyph and the rounded corners


def main() -> None:
    big = SIZE * SS
    icon = Image.new("RGBA", (big, big), (0, 0, 0, 0))
    d = ImageDraw.Draw(icon)
    # Keep the brand color fixed in both themes, matching the light accent
    # in internal/app/style.go. Only the corners are transparent.
    d.rounded_rectangle(
        (0, 0, big - 1, big - 1), radius=int(big * 0.22), fill="#315DCE"
    )
    ink = "#FFFFFF"
    stroke = int(big * 0.06)

    def rounded_line(points: list[tuple[float, float]]) -> None:
        pixels = [(round(x * big), round(y * big)) for x, y in points]
        d.line(pixels, fill=ink, width=stroke, joint="curve")
        for x, y in pixels:
            r = stroke / 2
            d.ellipse((x - r, y - r, x + r, y + r), fill=ink)

    # Round both corners along the tray's centerline, keeping the same
    # stroke width through the curves and its two open ends.
    tray = [(0.26, 0.64)]
    for cx, start in ((0.31, math.pi), (0.69, math.pi / 2)):
        for i in range(17):
            angle = start - i * math.pi / 32
            tray.append((cx + 0.05 * math.cos(angle), 0.69 + 0.05 * math.sin(angle)))
    tray.append((0.74, 0.64))
    rounded_line(tray)

    # A 45-degree arrow, with the same stroke weight as the open tray.
    rounded_line([(0.50, 0.24), (0.50, 0.58)])
    rounded_line([(0.36, 0.44), (0.50, 0.58), (0.64, 0.44)])
    icon = icon.resize((SIZE, SIZE), Image.Resampling.LANCZOS)

    out = pathlib.Path(__file__).resolve().parent.parent / "resources"
    out.mkdir(parents=True, exist_ok=True)
    icon.save(out / "icon.png")
    print(f"wrote {out / 'icon.png'}")


if __name__ == "__main__":
    main()
