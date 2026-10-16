#!/usr/bin/env python3
"""Makes fonts/mz700-set2.font, CharEdit's own look-alike of the Sharp MZ-700's
second character set, from the drawings in tools/mz700-set2.txt.

  python3 tools/mz700-set2.py [tools/mz700-set2.txt] [fonts/mz700-set2.font]
  python3 tools/mz700-set2.py --png sheet.png   (a 16x16 table, display codes
                                                  down the columns, to look at)

The font is 256 glyphs of 8 bytes in display code order, the top bit the
leftmost pixel (as fonts/mz700-rjm.font). Every code must be drawn exactly once.
"""
import sys, os

HERE = os.path.dirname(os.path.abspath(__file__))


def turn(g, op):
    """g: 8 strings of 8. Turned or mirrored."""
    if op == "flipx":
        return [r[::-1] for r in g]
    if op == "flipy":
        return g[::-1]
    if op == "180":
        return [r[::-1] for r in g[::-1]]
    if op == "cw":  # a quarter turn clockwise
        return ["".join(g[7 - x][y] for x in range(8)) for y in range(8)]
    if op == "ccw":
        return ["".join(g[x][7 - y] for x in range(8)) for y in range(8)]
    raise ValueError(op)


def outline(skel):
    """The wall around a letter's middle line (6 rows of 5 at x 1-5, y 1-6)."""
    mid = {(x + 1, y + 1) for y, r in enumerate(skel) for x, c in enumerate(r) if c == "#"}
    g = [["."] * 8 for _ in range(8)]
    for x, y in mid:
        for dx in (-1, 0, 1):
            for dy in (-1, 0, 1):
                p = (x + dx, y + dy)
                if p not in mid and 0 <= p[0] < 8 and 0 <= p[1] < 8:
                    g[p[1]][p[0]] = "#"
    return ["".join(r) for r in g]


def pie(side):
    """The large black circle less the quarter towards side."""
    g = []
    for y in range(8):
        row = ""
        for x in range(8):
            dx, dy = x - 3.5, y - 3.5
            ink = dx * dx + dy * dy <= 4.3 * 4.3 - 1.5
            cut = {"up": -dy >= abs(dx), "down": dy >= abs(dx), "left": -dx >= abs(dy), "right": dx >= abs(dy)}[side]
            row += "#" if ink and not cut else "."
        g.append(row)
    return g


def parse(path):
    glyphs = {}
    lines = [l.rstrip() for l in open(path, encoding="utf-8")]
    lines = [l for l in lines if l and not (l.startswith("# ") or l == "#")]  # glyph rows have no spaces
    i = 0

    def put(code, g):
        assert code not in glyphs, "drawn twice: %02X" % code
        assert len(g) == 8 and all(len(r) == 8 and set(r) <= set(".#") for r in g), "bad glyph %02X" % code
        glyphs[code] = g

    def rows(n, w):
        nonlocal i
        out = lines[i:i + n]
        assert len(out) == n and all(len(r) == w and set(r) <= set(".#") for r in out), lines[i - 1]
        i += n
        return out

    while i < len(lines):
        head = lines[i].split()
        i += 1
        if head[0] == "skel":
            put(int(head[1], 16), outline(rows(6, 5)))
        elif head[0] == "big":
            pic = rows(16, 16)
            for q, name in enumerate(head[1:5]):
                if name != "-":
                    ox, oy = q % 2 * 8, q // 2 * 8
                    put(int(name, 16), [pic[oy + y][ox:ox + 8] for y in range(8)])
        elif head[0] == "pie":
            put(int(head[1], 16), pie(head[2]))
        elif len(head) == 4 and head[1] == "=":
            src = int(head[3], 16)
            assert src in glyphs, "%s before %s" % (head[0], head[3])
            put(int(head[0], 16), turn(glyphs[src], head[2]))
        else:
            assert len(head) == 1, head
            put(int(head[0], 16), rows(8, 8))
    missing = [c for c in range(256) if c not in glyphs]
    assert not missing, "not drawn: " + " ".join("%02X" % c for c in missing)
    return glyphs


def font_bytes(glyphs):
    out = bytearray()
    for c in range(256):
        for r in glyphs[c]:
            out.append(sum(0x80 >> x for x, ch in enumerate(r) if ch == "#"))
    return bytes(out)


def sheet(glyphs, path, scale=4):
    from PIL import Image
    cell = 10 * scale
    im = Image.new("RGB", (16 * cell, 16 * cell), (255, 255, 255))
    px = im.load()
    for c, g in glyphs.items():
        ox, oy = (c >> 4) * cell + scale, (c & 15) * cell + scale
        for y in range(8):
            for x in range(8):
                if g[y][x] == "#":
                    for a in range(scale):
                        for b in range(scale):
                            px[ox + x * scale + a, oy + y * scale + b] = (0, 0, 0)
    im.save(path)


if __name__ == "__main__":
    args = sys.argv[1:]
    src = os.path.join(HERE, "mz700-set2.txt")
    if args and args[0] == "--png":
        sheet(parse(src), args[1])
        sys.exit(0)
    if args:
        src = args[0]
    dst = args[1] if len(args) > 1 else os.path.join(HERE, "..", "fonts", "mz700-set2.font")
    open(dst, "wb").write(font_bytes(parse(src)))
    print("wrote", dst)
