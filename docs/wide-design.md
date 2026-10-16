# Wide characters on the Unicode page: design (2026-10-16)

CJK ideographs, kana, Hangul, fullwidth forms and most emoji take two terminal columns. Today the Unicode
page (docs/unicode-design.md) holds one character per cell, cannot take most of them, and squeezes the 76
it has (⌚ ☕ ✅ ⭐ ...) into one cell, so a file it writes with them shifts in a terminal. This design keeps
the 32-bit cell and gives a wide character two cells: a **lead** and a **continuation**.

How each fact is known: *verified* (run or measured here), *read* (source or standard), *proposed* (a
choice, or a belief still to check in phase 2).

## Which characters are wide

- **The rule: East Asian Width W or F, Unicode 17.0.0, nothing else.** Ambiguous (A) is narrow, every
  other class narrow, controls and combining marks take no cell (as today's `uniInvisible`).
  - Since Unicode 9.0 every character with Emoji_Presentation=Yes is EAW W itself, so the EAW table already
    covers single emoji; no separate UTS #51 table is needed (*read*, UAX #11; *verified*: 😀 ⌚ ✅ ☕ are W
    in EastAsianWidth-17.0.0.txt and 2 in glibc).
  - A must be narrow: box drawing and blocks (═ ▀ █) are A; a CJK-width terminal setting makes them wide
    and breaks all text art (*verified*: EAW A for U+2550, U+2580, U+2588; glibc wcwidth gives 1).
  - glibc 2.44's wcwidth agrees with this rule on every sample tried: W/F 2, A 1, ｱ (H) 1, VS16 0, flags
    1+1, 👍🏽 2+2, 👨‍👩‍👧 2+0+2+0+2, U+20000 2, Unicode 16's newly wide ☰ (U+2630) and ⚊ (U+268A) 2
    (*verified*, ctypes on this machine).
  - Terminals differ only on sequences: VS16 after a narrow base (❤️), ZWJ sequences, skin tones, flags.
    xterm, Kitty, Alacritty and GNOME's VTE add up the code points (🧑‍🌾 = 4); Ghostty, foot and WezTerm
    cluster graphemes (= 2) when mode 2027 is on; Kitty widens a narrow emoji followed by VS16 (*read*:
    mitchellh.com/writing/grapheme-clusters-in-terminals, Oct 2023; Kitty and Ghostty docs). A cell holds one
    code point, so Charcoal does what glibc and xterm do: **one code point at a time**. VS16, ZWJ and other
    invisibles are dropped on reading (as today), so ❤️ is a narrow ❤ and 👨‍👩‍👧 is three wide characters.
- **The table is our own**, generated from EastAsianWidth-17.0.0.txt by `tools/widetab.sh` into
  `widetab.go` (123 ranges, 182,876 code points; *verified*). Not Go's tables: the standard library has no
  East Asian Width at all, x/text/width is older (Unicode 15), and the knowledge base's go/testing-traps.md
  shows Go 1.24 and 1.27 already reading the same file differently. Unicode 17 vs 15.1: 355 code points
  became wide (U+2630-2637, U+268A-268F, U+4DC0-4DFF hexagrams, U+1D300..., *verified* by diffing the two
  files); none stopped being wide. Unifont 17.0.05 (what we embed) matches the table's version.

## The cell

- A **continuation** cell has character code `0xDFFF` (`uniCont`). The surrogates DD00-DFFF are never
  characters and no cell holds them today (`uniCodeOf` refuses them), so no page, file or autosave changes
  meaning. Its style is the lead's (a terminal gives a wide character one colour pair; writers use the
  lead's). A lead is an ordinary cell whose character is wide.
- **The invariant**: on a Unicode page a lead is followed by its continuation in the same row, and a
  continuation follows a lead. The last column never holds a lead.
- **Beyond the BMP** (emoji, CJK Extension B and later) the 16-bit code has no room. Codes DD00-DFFE (767)
  stand for such characters through one process-wide, append-only table (`uniExtra`), as the style table
  does for colours: undo, frames, the clipboard and other tabs share them; an autosave keeps the table and
  remaps it (like `uniRemapPairs`); preferences (F-keys) store the character, not the code. Full table: the
  character becomes `?` and the status line says so. 767 different characters beyond the BMP in a session
  is plenty for art. (Rejected: putting the plane number in the continuation cell. The lead's code would
  then mean another character when read alone, and every reader of a single cell would have to look right.)
- `wideOK(d)` checks the invariant; tests run it after every operation and in the fuzz target.
- A lone lead can only come from an autosave made by 0.1b12 (which typed ⭐ into one cell). Restoring one
  makes a blank right neighbour its continuation; otherwise the lead stays alone and is drawn squeezed into
  its cell, as 0.1b12 drew it.

## What each operation does

The rule everywhere is the terminal's: **writing into either half of a wide character leaves a space in
the other half** (lead's or continuation's style). (*proposed*: xterm and VTE behave so as far as I know;
phase 2 checks it with a pty test.) One helper `wideNormal(cells, x0, x1)` applies it to a stretch of a row
(a lone lead or a lone continuation becomes a space); operations call it on the edges they cut.

| Operation | What happens |
|---|---|
| `setCell` (typing, Paint, shapes, Half, Shade, Logo/TDF, stamping, dropping a moved block) | The choke point. Writing a wide character writes lead + continuation; at the last column it is not written (status: does not fit). Writing over half of a pair blanks the other half. Writing a wide character where it already stands (the cell is the continuation of the same character, or the cell to the right is the same lead) does nothing, so a line, a filled shape or a fill drawn with a wide brush in any direction comes out as whole characters side by side. Writing a continuation cell (from a block) keeps it only right after a lead, else a space. |
| Typing | The cursor moves on by 2. At the last column the character goes to the next row (as a terminal wraps it). Insert mode shifts the row by 2. |
| Delete, Backspace | On either half, the whole character goes (the row is pulled left by 2). Backspace steps over a pair. |
| Cursor | Never rests on a continuation: Right skips it, Left/Up/Down/a click land on its lead. The cursor box is drawn 2 cells wide on a lead. |
| Pick (eyedropper), status line | A continuation gives its lead's character and colours. |
| Paint drag | Skips the continuation of the character it has just painted (else each step would cut the last). |
| Fill selection, flood fill | Through the `setCell` rule, left to right; a wide character is put only where both its cells are inside the area. |
| Select, copy, cut | The selection grows to whole characters (a left edge on a continuation takes the lead, a right edge on a lead takes the continuation), so a block never holds half a character; crop to selection the same. Copy as text skips continuations. |
| Paste text from the system clipboard | Character by character through `setCell`: wide ones take two cells. |
| Mirror (block) | Cells are reversed, then every pair turned back round (continuation-lead becomes lead-continuation). Wide characters themselves are not mirrored (letters never are; Unifont's 16-wide glyphs are not in the mirror table). |
| Upside down | Rows swap: pairs unchanged. |
| Turn | Refused when the block holds a wide character ("Wide characters cannot be turned: each is 2 cells wide and 1 high"), as Minitel mosaics are (`mtBlockOpRefused`). |
| Insert/delete column, W field narrower, crop | `wideNormal` at the cut: a pair cut at the new last column or at the shifted column becomes a space. |
| Layers: show and flatten | Cells merge as now (a continuation is not blank, so it covers); then `wideNormal` on the merged row, in both the drawn view and `flatten`, so what is shown is what is saved. |
| Transitions/effects | `wideNormal` on every frame they make; their random glyphs leave out wide characters. |
| Convert to PC/Amiga | A lead becomes `?`, its continuation a space; counted in "cells had characters ... the page lacks". |
| Stamping a Unicode block on a PC page | The same conversion (`uniToPC`). |

## The font

- **Unifont draws them** at 16x16, the two halves in the two cells, each half in its own cell's colours;
  italic shears the 16-pixel row as one (bits move from the left half into the right), underline runs
  under both. A wide character Unifont lacks gets a 16-wide missing box. The 30 wide characters Unifont
  draws 8 wide are centred.
- **Size** (*verified*, Unifont 17.0.05 + EastAsianWidth 17.0.0): every wide character Unifont has is 52,701
  glyphs, **1.03 MB gzipped**: Han ideographs 35,791 glyphs / 862 KB (U+4E00-9FFF alone 484 KB, planes 2-3
  193 KB), everything else 16,910 glyphs / 166 KB (kana, CJK punctuation, fullwidth forms, Hangul 70 KB,
  plane 1 emoji and symbols 52 KB). Today's built-in subset is 55 KB, CharEdit 10.4 MB.
- **Proposal: build all of it in** as `fonts/unifont-wide.hex.gz` (made by `tools/unifont.sh`, same OFL
  terms, NOTICE and README licence row extended), parsed lazily the first time a wide character is drawn
  (1.7 MB of glyphs in memory then). No download code, works offline, CI tests draw kanji. The alternative,
  downloading the Han part (862 KB) on first use like the VICE ROMs, saves 8% of the binary for a network
  path and tests that cannot draw kanji. (Question for the user.)
- **Emoji are drawn in Unifont's monochrome 16x16 glyphs**, in the cell's foreground colour. Colour emoji
  fonts are not 16-pixel bitmaps (Noto Color Emoji is 136x128 colour bitmaps) and a cell has two colours;
  text art colours its characters itself. Unifont has 1,107 of the 1,184 wide code points in
  U+1F000-1FAFF (*verified*).
- The character table gains blocks: CJK symbols and kana (U+3000), Hangul compatibility jamo and
  more (U+3100), fullwidth forms (U+FF00), and, with `uniExtra`, emoji pages. Wide characters are squeezed
  in the table (its cells are 8 pixels). Typing works through SDL's text input, so an input method gives
  kanji (*proposed*: check that the IME window is placed at the cursor, `SDL_SetTextInputRect`).

## Files

- **UTF-8 ANSI read**: a wide character advances the cursor by 2 (`putCode`). At the last column it wraps to
  the next row first, leaving the last column as it was (as xterm with autowrap; *proposed*, checked in phase
  2). Cursor moves and erases may cut pairs, so the reader ends with `wideNormal` over the page (the terminal
  rule). The page width is the longest line's **width** (wide = 2), not its character count.
  - Real evidence (*verified*, a file in the big check, MC_FRESHER-RX-78.ANS, Mistigris 2025-08): all 32 lines
    are 59 characters, one of them with four kanji spaced for a terminal, padded with spaces to 59
    characters, so 63 columns wide in a terminal. The tool that made it counted a kanji as one column. Rule:
    blanks (spaces on the default background) that run past the last column and are followed only by the
    line end are dropped, not wrapped; anything else past the column wraps as a terminal does. That file
    then opens 59 wide with the kanji where a terminal shows them.
- **UTF-8 ANSI, mIRC, plain text write**: the lead's character, nothing for the continuation (the terminal
  advances 2). mIRC reads like ANSI (width 2), its line measure counts bytes as now.
- **HTML**: each wide character in `<span class=w>` with `display:inline-block;width:2ch`, because a
  browser's monospace font draws CJK at its own width (*proposed*), and nothing for the continuation.
- **PNG, GIF, APNG**: through the drawing above (renderDoc uses `paintCells`).
- **durdraw `.dur`**: durdraw has no width model at all: a line is a list of one character per cell, its
  ANSI reader advances 1 per character, its export writes cell by cell (*read*, durdraw 0.31.0 main
  2026-10-15, durdraw_ansiparse.py and durdraw_ui_curses.py; none of its 18 example files holds a wide
  character, *verified*). The colour map is per cell, so the grid must stay: a wide character in cell x
  takes x+1 as its continuation (whatever durdraw had there is hidden, as on a terminal), a wide character in
  the last column becomes a space. Writing, the continuation is a space (durdraw needs a character per cell).
  Round trips are exact; durdraw itself shows such a line one column longer in a terminal. Phase 2 runs
  durdraw on a Charcoal `.dur` with kanji (`CHARCOAL_DURDRAW_SRC`, uni_ext_test.go) to see what it shows.
- Detection is unchanged: a few Japanese words in a long text still do not make it Unicode art (needsUni's
  letter rule; its test of an English text with a few Japanese words keeps passing).

## Memory, styles, collab, CharView

- No new memory per cell; the continuation shares the lead's style, so wide characters add no styles.
  `uniCompact` renumbers continuations like any cell. `uniExtra` is at most 767 runes.
- **Collab**: sessions are PC pages only (`collabTick` ends one whose page is not "pc"), so no wide character
  ever reaches Moebius; nothing to do.
- **CharView** draws through `paintCells` and gets wide characters with the drawing work; nothing else.

## Pages without wide characters stay byte for byte the same

Every new path is entered only for a wide character or a continuation: `setCell` checks the old and new cell,
readers branch in `putCode` on the width, `wideNormal` finds nothing to do, writers skip only `uniCont`,
`uniLongestLine` counts as before. The existing tests stay as they are, except one whose premise is the
missing feature (uni_test.go: `textInput("😀") // no cell holds it`), which changes in the emoji step.
**The one intended change**: the 76 wide characters the art subset already has (U+231A ⌚ ... U+2B55 ⭕,
including Unicode 16's ☰-☷ and ⚊-⚏) become two cells, so files with them now read as a terminal shows them.

## Order of work (each a commit with its tests)

1. `tools/widetab.sh` + `widetab.go` (`uniWide`), `uniCont`, `wideNormal`, `wideOK`. Tests: known widths
   (kanji, kana, ｱ narrow, ═ ▀ █ narrow, ☰ wide, 😀 wide) independent of Go's tables. No behaviour change.
2. Font + drawing: `fonts/unifont-wide.hex.gz`, the 16-wide glyphs, halves in `paintCellsAt`, the page loop and
   the layers row, the 2-wide cursor. Tests: pixels of a pair, a PNG export, a lone lead squeezed.
3. Readers and writers: UTF-8 ANSI (width, last-column wrap, blank overflow, normalise, longest line), plain
   text, mIRC, HTML. Round trips, an RX-78-like test file of our own, the fuzz target checks `wideOK` after
   every reader.
4. Editing: `setCell`, typing, Delete/Backspace, cursor, pick, paint drag, fills, selection growth, mirror,
   turn refusal, column ops, W, crop, clipboard text. A table test runs every operation over a page with
   pairs at both edges and checks `wideOK`.
5. Layers, transitions, PC conversion, block conversion.
6. durdraw `.dur` read/write, checked against durdraw.
7. Character table blocks and input method placement.
8. `uniExtra`: characters beyond the BMP (emoji, CJK Extension B+), autosave, F-keys in preferences.

**If it grows, cut from the end**: 8 first (emoji and rare ideographs then read as `?`, as today), then 7
(pasting and the input method still type wide characters), then 6 (a `.dur` writes wide characters as `?`).
Steps 1-5 are the feature.

## The user's decisions (2026-10-16)

1. All of Unifont's wide glyphs are built in (+1.03 MB), loaded the first time a wide character is drawn.
2. Emoji are Unifont's monochrome glyphs in the cell's colour.
3. VS16 sequences (❤️) stay narrow, as glibc and xterm count them.
4. The 76 wide characters the art subset already had become two cells (as terminals show them).
5. Turn is refused for a block with wide characters, with a message.
6. The 767-code table beyond the BMP is enough; when full, `?` and a note in the status line.
