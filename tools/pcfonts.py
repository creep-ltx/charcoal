#!/usr/bin/env python3
"""Make the built-in PC fonts (fonts/IBM_*.f08/.f14/.f16) from VileR's
Ultimate Oldschool PC Font Pack (int10h.org/oldschool-pc-fonts, CC BY-SA 4.0).

The pack holds no raw bitmaps, so each font is drawn from its "Bm" .otb bitmap
font through FreeType (Pillow), 256 glyphs of h bytes, bit 7 = the left pixel:
the raw form DOS font tools use. The code pages other than 437 come from the
"BmPlus" fonts, each byte through Python's codec for the page (0x00-0x1F and
0x7F are the same glyphs on every page).

Checked 2026-10-14: the EGA 8x14 and VGA 8x8 made so are byte for byte DOSBox's
ROM tables (int10_font_14, int10_font_08 in dosbox-staging's int10_memory.cpp),
and the VGA 8x16 the same way is fonts/IBM_VGA_8x16.raw. Drawing the "Px"
TrueType fonts instead moves the accents of capitals (Ä Ö Â ...) by a row:
use the .otb.

    python3 -I tools/pcfonts.py PACK_DIR OUT_DIR

PACK_DIR holds the pack's .otb files (Bm437_IBM_EGA_8x14.otb ...), unzipped flat.
"""
import os
import sys

from PIL import Image, ImageDraw, ImageFont

# CP437 0x00-0x1F and 0x7F as glyphs (the codecs give control characters).
LOW = '\u0000☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼'

# out file, pack font, rows, rows the strike is drawn higher, cell width, code page.
# The 14-row fonts are 16-pixel strikes with the cell 2 rows down.
FONTS = [
    ('IBM_EGA_8x14.f14', 'Bm437_IBM_EGA_8x14', 14, 2, 8, 'cp437'),
    ('IBM_VGA_8x8.f08', 'Bm437_IBM_VGA_9x8', 8, 0, 9, 'cp437'),  # the 8x8 ROM font, drawn 9 wide: the left 8
    ('IBM_VGA_8x16_850.f16', 'BmPlus_IBM_VGA_8x16', 16, 0, 8, 'cp850'),
    ('IBM_VGA_8x16_866.f16', 'BmPlus_IBM_VGA_8x16', 16, 0, 8, 'cp866'),
]


def char(b, cp):
    if b < 32:
        return LOW[b]
    if b == 0x7F:
        return '⌂'
    return bytes([b]).decode(cp)


def strike(path):
    for s in range(1, 40):
        try:
            return ImageFont.truetype(path, s)
        except OSError:
            pass
    sys.exit('no bitmap size in ' + path)


def render(path, size, top, width, cp):
    font = strike(path)
    out = bytearray()
    for b in range(256):
        img = Image.new('L', (width, size + 16), 0)
        if b not in (0, 0x20, 0xFF):
            ImageDraw.Draw(img).text((0, 8 - top), char(b, cp), font=font, fill=255)
        px = img.load()
        for y in range(8, 8 + size):
            row = 0
            for x in range(8):
                if px[x, y] >= 128:
                    row |= 0x80 >> x
            out.append(row)
    return bytes(out)


def main():
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    pack, out = sys.argv[1], sys.argv[2]
    for name, src, size, top, width, cp in FONTS:
        bits = render(os.path.join(pack, src + '.otb'), size, top, width, cp)
        if bits[0xDB * size:0xDB * size + size] != b'\xff' * size:
            sys.exit(f'{name}: the full block is not the whole cell: wrong offset')
        with open(os.path.join(out, name), 'wb') as f:
            f.write(bits)
        print(f'{name}: {len(bits)} bytes')


if __name__ == '__main__':
    main()
