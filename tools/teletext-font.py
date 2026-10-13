# Makes teletext_font.go from Bedstead's source (https://bjh21.me.uk/bedstead/bedstead.c, CC0):
# the SAA5050 English set, codes 0x20-0x7F, as 9 rows of 5 bits.
#   python3 tools/teletext-font.py bedstead.c
import re
import sys

src = open(sys.argv[1], encoding='latin-1').read()
glyph = {}
# Entries look like {"\16\21...", U(0410) } or {"\00\00\17...", 0x0063, "c" }.
entry = re.compile(r'\{\s*"((?:\\[0-7]{1,3})+)"\s*,\s*(?:U\(([0-9A-Fa-f]+)\)|0x([0-9A-Fa-f]+))')
for m in entry.finditer(src):
    rows = [int(x, 8) for x in re.findall(r'\\([0-7]{1,3})', m.group(1))]
    cp = int(m.group(2) or m.group(3), 16)
    if len(rows) == 9 and cp not in glyph:
        glyph[cp] = rows
# The English set differs from ASCII in these places.
special = {0x23: 0xA3, 0x5B: 0x2190, 0x5C: 0xBD, 0x5D: 0x2192, 0x5E: 0x2191, 0x5F: 0x23, 0x60: 0x2014,
           0x7B: 0xBC, 0x7C: 0x2016, 0x7D: 0xBE, 0x7E: 0xF7, 0x7F: 0x25A0}
lines = ['// Generated from Bedstead (bjh21.me.uk/bedstead, CC0 public domain) by tools/teletext-font.py - do not edit.',
         '// The SAA5050 English character set, codes 0x20-0x7F: 9 rows of 5 bits each (bit 4 = the left pixel).', '',
         'package main', '', 'var teletextGlyphs = [96][9]byte{']
missing = []
for code in range(0x20, 0x80):
    cp = special.get(code, code)
    if cp not in glyph and code != 0x20:
        missing.append(hex(code))
    g = glyph.get(cp, [0] * 9)
    lines.append('\t{' + ', '.join('0x%02X' % r for r in g) + '}, // 0x%02X' % code)
lines.append('}')
open('teletext_font.go', 'w').write('\n'.join(lines) + '\n')
print('missing:', missing)
