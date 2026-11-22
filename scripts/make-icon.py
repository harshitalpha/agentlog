#!/usr/bin/env python3
"""Draws plugin/assets/icon.png (512x512): a vertical timeline with three commit dots.

Pure stdlib (zlib + struct), so the icon can be regenerated anywhere:
    python3 scripts/make-icon.py plugin/assets/icon.png
"""
import struct
import sys
import zlib

N = 512
SS = 3  # supersampling per axis for smooth edges
BG = (13, 15, 18)
PANEL = (24, 28, 34)
ACCENT = (232, 165, 75)
LINE = (91, 99, 112)
GREEN = (63, 185, 80)


def rounded_rect(x, y, x0, y0, x1, y1, r):
    cx = min(max(x, x0 + r), x1 - r)
    cy = min(max(y, y0 + r), y1 - r)
    return (x - cx) ** 2 + (y - cy) ** 2 <= r * r


def shade(x, y):
    """Colour and alpha at a point (floating-point coordinates)."""
    if not rounded_rect(x, y, 0, 0, N, N, 96):
        return None
    col = PANEL if rounded_rect(x, y, 36, 36, N - 36, N - 36, 64) else BG
    if abs(x - 150) <= 5 and 110 <= y <= 402:  # the timeline
        col = LINE
    for i, cy in enumerate((130, 256, 382)):
        d2 = (x - 150) ** 2 + (y - cy) ** 2
        ring = ACCENT if i != 2 else GREEN
        if d2 <= 30 ** 2:
            col = ring if d2 > 18 ** 2 or i == 0 else PANEL
        # a short "message" bar to the right of each dot
        if 208 <= x <= (380 if i != 1 else 330) and abs(y - cy) <= 11:
            col = ring if i == 0 else LINE
        if 208 <= x <= (300 if i != 1 else 360) and abs(y - (cy + 34)) <= 6 and i != 2:
            col = (60, 66, 76)
    return col


def main(out):
    raw = bytearray()
    for py in range(N):
        raw.append(0)  # filter type: none
        for px in range(N):
            r = g = b = a = 0
            for sy in range(SS):
                for sx in range(SS):
                    c = shade(px + (sx + .5) / SS, py + (sy + .5) / SS)
                    if c:
                        r += c[0]; g += c[1]; b += c[2]; a += 1
            if a:
                raw += bytes((r // a, g // a, b // a, 255 * a // (SS * SS)))
            else:
                raw += b"\0\0\0\0"

    def chunk(tag, data):
        body = tag + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)

    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", N, N, 8, 6, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(bytes(raw), 9))
    png += chunk(b"IEND", b"")
    with open(out, "wb") as f:
        f.write(png)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "plugin/assets/icon.png")
