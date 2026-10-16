#!/usr/bin/env python3
"""Makes CharEdit's MSX and MZ-700 font files from their free sources.

  python3 tools/msxmz-fonts.py CBIOS_SRC_DIR MZ800EM_DIR

CBIOS_SRC_DIR: C-BIOS 0.29a's src/ (cbios.sourceforge.net, BSD licence):
  font_us.asm, font_jp.asm, font_br.asm -> fonts/msx-cbios{,-jp,-br}.font
MZ800EM_DIR: a checkout of github.com/mkoeppe/mz800em: font.txt, Russell Marks's
  public domain MZ-700-like set (display codes 0-255) -> fonts/mz700-rjm.font
Every file is 256 glyphs of 8 bytes, the top bit the leftmost pixel.
"""
import re, sys, os

def asm_font(path):
    out = bytearray()
    for line in open(path, encoding="latin-1"):
        m = re.match(r"\s*db\s+([^;]*)", line)
        if m:
            out += bytes(int(x.strip()[1:], 16) for x in m.group(1).split(","))
    assert len(out) == 2048, (path, len(out))
    return bytes(out)

def rjm_font(path):
    lines = open(path, encoding="latin-1").read().split("\n")
    out, i = bytearray(), 0
    for c in range(256):
        i += 1  # the glyph's name line
        for r in range(8):
            row = lines[i]; i += 1
            v = 0
            for x in range(8):
                if row[x * 2:x * 2 + 1] == "x":
                    v |= 0x80 >> x
            out.append(v)
    return bytes(out)

here = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "fonts")
cb, mz = sys.argv[1], sys.argv[2]
for src, dst in (("font_us.asm", "msx-cbios.font"), ("font_jp.asm", "msx-cbios-jp.font"), ("font_br.asm", "msx-cbios-br.font")):
    open(os.path.join(here, dst), "wb").write(asm_font(os.path.join(cb, src)))
open(os.path.join(here, "mz700-rjm.font"), "wb").write(rjm_font(os.path.join(mz, "font.txt")))
