# The Unicode page: design (2026-10-16)

For terminal artists (durdraw, UTF-8 ANSI) and IRC artists (mIRC colour codes). A page of any width whose
cells hold any Unicode character Charcoal can draw and 256-colour or 24-bit colours, without changing the
cell layout every other page and reader depends on.

## The cell

A cell stays one `uint32`. Bits 0-17 mean what they mean on a PC page; the Unicode page also uses the high
bits, which every other page leaves at 0 (except a Spectrum page's flash bit, 18: that page is never Unicode).

| Bits | PC page | Unicode page |
|---|---|---|
| 0-7 | CP437 byte | character code, low byte |
| 8-11 | foreground 0-15 | style, bits 0-3 |
| 12-15 | background 0-15 | style, bits 4-7 |
| 16, 17 | italic, underline | style, bits 8-9 |
| 18-25 | 0 | character code, high byte |
| 26-31 | 0 | style, bits 10-15 |

- **Character code** (16 bits): the Unicode character itself for the BMP. The 2048 surrogate codes
  U+D800-U+DFFF are never characters, so they stand for the two plane-1 blocks text art uses: D800-D8FF =
  U+1FB00-1FBFF (Symbols for Legacy Computing: sextants, wedges, eighth blocks) and D900-DCFF = U+1CC00-1CFFF
  (its supplement, octants). A fixed mapping: no table to save. Anything else outside the BMP cannot be typed
  and reads as `?` (wide CJK and emoji are not terminal-art cells anyway).
- **Style** (16 bits): styles 0-255 are fixed: foreground = style & 15, background = style >> 4, the 16
  colours in ANSI order, no italic or underline: exactly a PC cell's bits. So a PC cell of an ASCII
  character is already the same Unicode cell, and converting a PC page only maps characters 1-31 and
  128-255 (and puts italic/underline into a style). Styles 256-65535 come from one **process-wide,
  append-only table** (`uniStyles`) of {foreground, background, italic|underline}, where a colour is 0-255
  (xterm-256 index) or `rgbFlag|0xRRGGBB` (24-bit). Append-only means a style number keeps its meaning, so
  undo steps, frames, the clipboard and other tabs share cells freely; process-wide means a cell copied
  between tabs keeps its colours. 65280 styles: a 24-bit 160x100 picture has at most 16000.
  `uniCompact` drops the styles no cell uses any more (renumbering every tab's pages, frames, undo steps,
  the floating block and the clipboard) when the table is three quarters full, only at a safe point (a
  file being opened, no requester open, nothing playing). When the table is full anyway, a new style takes
  the nearest existing one with the same italic/underline and the status line says so.
- `Doc.uni` (set by `applyLook` like `glyph0`, kept by `clone` and `withCols`) tells `blankAt` to read the
  cell this way: a space is blank only when its background is colour 0 and it is not underlined.
- `Snap.mode` keeps the page type with each undo step, so undoing a conversion to or from Unicode
  brings the old page type back with its cells (`restoreUniMode`; only between Unicode and the others).

## What the page has

- **Mode `uni`** ("Unicode"), any width, real height, like a PC page. Colours 0-15 come from the page's
  palette (VGA by default, any other: xterm, mIRC); 16-255 are xterm's, 24-bit are themselves. Bright
  backgrounds always (no blink on terminals).
- **Font**: GNU Unifont (dual licensed OFL-1.1 / GPL-2+ with the font exception; we use it under the OFL),
  a subset of the blocks text art uses, built in (`fonts/unifont-art.hex.gz`, made by `tools/unifont.sh`).
  The default "VGA + Unifont" draws CP437's characters in the IBM VGA font (so a converted PC page looks the
  same) and the rest in Unifont; "Unifont" draws all in Unifont. Glyphs 16 pixels wide are squeezed into
  one cell.
- **Editing**: typing takes any character the font has; the character table shows 256 codes of a chosen
  block (Latin, box and blocks, Braille, legacy computing, shapes, arrows ...); the F-keys hold Unicode;
  colours: the 16, a 256 grid, and the colour requester for 24-bit; the Half tool draws ▀ ▄ █ in any colours;
  flips use a mirror table found from the glyphs' own bitmaps.
- **Conversion**: Page > "Convert to Unicode" (also choosing Unicode as the machine): every frame's cells
  CP437 (or Latin-1) -> Unicode, exact, one undo step. Back to PC: characters not in CP437 become `?`,
  colours the nearest of 16 (said in the status line, undoable).

## Files

| Format | Read | Write |
|---|---|---|
| UTF-8 ANSI `.ans`/`.txt`/`.utf8ans` (SGR 30-37 90-97 40-47 100-107, 38/48;5;n, 38/48;2;r;g;b) | yes: a UTF-8 file with characters CP437 lacks, or with 256/24-bit colours, opens as a Unicode page | yes (`uniansi`) |
| durdraw `.dur` (gzipped JSON, format 7 and 8, 16 and 256 colours, frames with delay) | yes, frames as Charcoal frames | yes (format 8) |
| mIRC colour codes `.irc`/`.txt` (^C fg,bg 0-98, ^B ^] ^_ ^V ^O, ^D hex) | yes | yes; also from a PC page (CP437 -> Unicode); warns when a line passes IRC's limit |
| HTML (`.html`) | | yes |
| PNG, GIF, APNG, plain UTF-8 text | | yes (as every page) |

## Where the code is

`uni.go` (cell, pair table, conversions, the font), `uniformats.go` (the readers and writers),
`uni_ui.go` (character table, colours, Half, status). Glue in shared files: a `uni` case where a page type
switches (drawCell, cell/brushCell, charToByte/runeOf, blankAt, setMode), `loadUni` first in loadBytes,
`reparseUni` first in reparse, `encodeUni` before encodeFor's switch, entries appended to the shared lists
(`formats`, `modeOrder`, `openAs`, `pageModes`, start tiles) from `uni.go`'s `init`.
