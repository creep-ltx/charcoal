[Charcoal](../../README.md) · [Manual](README.md)

# Unicode pages (terminal and IRC art)

**Unicode page** (Uni, for terminal and IRC artists): any width; any character GNU Unifont draws from the blocks
text art uses (full, half, quarter and eighth blocks, sextants and octants from Symbols for Legacy Computing,
Braille, box drawing, shades, shapes, arrows, symbols, Latin, Greek, Cyrillic), in the 16 colours, xterm's 256
or any 24-bit colour; italic and underline. The character table shows one block at a time (< >, the wheel,
Ctrl+Alt+PgUp / PgDn), typing takes any character it has, the colour panel has the 256 and a 24-bit
requester (sliders or `#RRGGBB`), Half paints half blocks in any colours, the Shape tool has single, double,
rounded and heavy lines, flips know Braille, sextants and the box pieces. Files: **UTF-8 ANSI** (`.ans`; SGR
30-37/90-97/40-47/100-107, 38/48;5;n, 38/48;2;r;g;b; no SAUCE record, as durdraw: the lines give the width, so only a page under 80
columns gets one), **durdraw
`.dur`** (gzipped JSON, formats 5-8, 16 and 256 colours, frames with their delays), **mIRC colour codes**
(`.irc`: ^C 0-98, ^D hex, ^B ^] ^_ ^V ^O; a warning when a line passes what IRC carries, about 450 bytes), web
page, PNG, GIF/APNG, plain UTF-8. A UTF-8 file opens on a Unicode page when it needs one (characters CP437
lacks, colours beyond 16, or durdraw's `.utf8.ans` name); Open as Unicode takes any ANSI. **Page > Convert to
Unicode** (or choosing Unicode for a PC or Amiga page) converts every frame exactly, as one undo step: the page
looks the same, pixel for pixel ("VGA + Unifont" draws CP437's characters in the VGA font). A PC page also
saves as mIRC text directly. Checked: every durdraw example opens cell for cell as durdraw's own ANSI export
shows it, and durdraw itself reads CharEdit's `.dur` files back as the page (`uni_ext_test.go`).

## Wide characters

**Wide characters** (CJK ideographs, kana, Hangul, fullwidth forms, emoji: the characters a terminal shows two
columns wide, by Unicode 17's East Asian Width) take two cells, as in a terminal: type them (an input method
works), paste them, or pick them in the character table's kana, CJK, Hangul and fullwidth blocks. The cursor
moves over them whole, Delete and Backspace take both cells, writing over either half leaves a space in the
other (as a terminal does), lines and fills with one come out side by side, a selection always holds whole
ones, Mirror keeps them readable and Turn refuses them. Files: UTF-8 ANSI, mIRC, text and HTML write them once
(the terminal moves on by two); one that does not fit at the end of a row goes to the next, as a terminal
wraps it. Unifont draws them 16 pixels wide, emoji in one colour (the cell's). Ambiguous-width characters (box
drawing, blocks) stay narrow, and so do emoji sequences like ❤️ (as glibc and xterm count them).
