# Makes minitel_font.go from Frédéric Bisson's Minitel font (github.com/Zigazou/Minitel-Canvas, Minitel.sfd,
# CC0): the Minitel's G0 set (codes 0x20-0x7F, as the STUM 1B's table shows them) and the G2 characters and
# accented letters a Minitel 1B (VGP5) shows, as 10 rows of 8 bits (bit 7 = the left pixel).
# The font's glyphs are squares on a 128-unit grid: x 0-1024 is 8 columns, y 1024 down to -256 is 10 rows.
#   python3 tools/minitel-font.py Minitel.sfd
import re
import sys

src = open(sys.argv[1], encoding='latin-1').read()
glyph = {}
for m in re.finditer(r'StartChar: (.*?)\nEncoding: (-?\d+) .*?\n(.*?)EndChar', src, re.S):
    polys, cur = [], None
    ss = re.search(r'SplineSet\n(.*?)EndSplineSet', m.group(3), re.S)
    for line in (ss.group(1).splitlines() if ss else []):
        p = line.split()
        if len(p) >= 3 and p[2] in ('m', 'l'):
            if p[2] == 'm':
                cur = []
                polys.append(cur)
            cur.append((float(p[0]), float(p[1])))
    glyph[int(m.group(2))] = polys
    glyph[m.group(1)] = polys


def inside(polys, px, py):  # even-odd rule
    w = 0
    for poly in polys:
        for i in range(len(poly)):
            (x1, y1), (x2, y2) = poly[i], poly[(i + 1) % len(poly)]
            if (y1 > py) != (y2 > py) and x1 + (py - y1) * (x2 - x1) / (y2 - y1) > px:
                w ^= 1
    return w


def rows(key):
    if key not in glyph:
        sys.exit('missing glyph %r' % (key,))
    out = []
    for r in range(10):
        b = 0
        for c in range(8):
            if inside(glyph[key], 128 * c + 64, 1024 - 128 * r - 64):
                b |= 0x80 >> c
        out.append(b)
    return out


# G0 as the STUM 1B's table 2.5 shows it: 0x5E an up arrow (the font's "^" is drawn so), 0x60 a long
# dash, 0x7B-0x7D bars on the left, in the middle and on the right, 0x7E a bar at the top, 0x7F the
# full block (the "oblitération").
special = {0x60: 8722, 0x7B: 'barre gauche', 0x7C: 'barre centrale', 0x7D: 'barre droite'}
g0 = []
for code in range(0x20, 0x80):
    if code == 0x7E:
        g0.append([0xFF] + [0] * 9)
    elif code == 0x7F:
        g0.append([0xFF] * 10)
    else:
        g0.append(rows(special.get(code, code)))
# The rest, in the order of mtExtra (minitel.go).
extra = '£§←↑→↓°±÷¼½¾Œœßàâäèéêëîïôöùûüç'
names = {'ß': 'beta'}
lines = ['// Generated from the Minitel font of Minitel-Canvas (github.com/Zigazou/Minitel-Canvas, by Frédéric Bisson,',
         '// CC0 public domain) by tools/minitel-font.py - do not edit.',
         '// 10 rows of 8 bits each (bit 7 = the left pixel).', '',
         'package main', '',
         '// mtG0Glyphs is the G0 set, codes 0x20-0x7F.',
         'var mtG0Glyphs = [96][10]byte{']
for i, g in enumerate(g0):
    lines.append('\t{' + ', '.join('0x%02X' % r for r in g) + '}, // 0x%02X' % (0x20 + i))
lines += ['}', '', '// mtExtraGlyphs are the characters of mtExtra.', 'var mtExtraGlyphs = [%d][10]byte{' % len(extra)]
for ch in extra:
    g = rows(names.get(ch, ord(ch)))
    lines.append('\t{' + ', '.join('0x%02X' % r for r in g) + '}, // ' + ch)
lines.append('}')
open('minitel_font.go', 'w', encoding='utf-8').write('\n'.join(lines) + '\n')
