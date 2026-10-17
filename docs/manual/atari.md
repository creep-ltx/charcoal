[Charcoal](../../README.md) · [Manual](README.md)

# Atari pages: 8-bit ATASCII and Atari ST

## Atari 8-bit page (ATASCII)

ATASCII (`.ata`, and `.txt` files recognised by their bytes): inverse characters, one
background and one text colour for the screen, border; EOL line ends, ESC before control characters, cursor
moves for animations. Width 38 (the screen editor's margins) or 40 (terminals) is guessed: a 39-40 character
line settles it, else the width where characters stack best. Checked on the Break Into Chat collection: all 203
real files recognised, none of 853 ANSI/NFO files on this PC mistaken for ATASCII. The font is AltirraOS's
(Altirra emulator, all-permissive licence, `fonts/AltirraOS-LICENSE.txt`), built in. With a real Atari OS ROM
on this computer (`ATARIXL.ROM` or `ATARIOSB.ROM` in CharEdit's data folder, `~/.atari800` or
`/usr/share/atari800`; Atari's ROMs may not be passed on, so CharEdit brings none) the Font gadget also offers
"Atari (OS ROM)" and the XL's international set. Own fonts: Load a font (`.fnt`, 1024 bytes), the font editor.
Colours are the machine's registers, from GTIA's 128 colours (Palette: Atari PAL or NTSC, Altirra's or atari800's): text
(COLOR1, 709: only its luminance counts, the text takes the screen's hue, as in GRAPHICS 0), screen (COLOR2,
710) and border (COLOR4, 712); left / right / middle click in the 16 x 8 colour grid, the values to poke shown
under it. Start-up colours 202 on 148, border 0. The default colours are Altirra's PAL palette (what compo screenshots
are taken with); Altirra's NTSC and atari800's PAL and NTSC palettes are in the Palette gadget.
**Runs on the machine**: `.xex` (a program for any Atari 400/800/XL/XE: the page in its colours, with its own
font when it has one; DOS, a loader or an emulator runs it) and `.atr` (a single density disk that boots straight
into it). Both open again. The first 24 rows are shown; a 38-wide page sits at the screen editor's column 2.

## Atari ST page (VT52 and IGS)

The ST's VT52 terminal in low (40x25, 16 colours), medium (80x25, 4) or high resolution
(80x25, mono); each cell its own colours; `.vt` streams read and written, the resolution guessed from the pens
and columns used. Font: EmuTOS's, downloaded on first use (GPL, so not built in), SHA-1 checked.
**IGS** (`.ig`, Instant Graphics and Sound, the ST BBS graphics language): the drawing is shown as the ST
draws it (lines, fills and patterns, VDI text, blits, palettes, loops; checked pixel for pixel against
IcyDraw's renders), as a picture under the page's cells; the resolution is the file's own or guessed from
where it draws. Type over it: empty cells show the picture, text is VT52 in the file's colours. Saving keeps
the drawing and its sounds, pauses and closing clear byte for byte and adds the text after the drawing (an
unchanged page saves as the same file). The drawing itself is not editable; sound is not played.
Design: [docs/igs-design.md](../igs-design.md).
