# Charcoal roadmap: the go-to editor for text artists on every platform

The goal (the user, 2026-10-13): Charcoal is what a text artist picks, whatever machine they draw for.

Based on two surveys made 2026-10-13: ~/Projects/Knowledge/bbs/text-art-platforms-and-tools.md (which
scenes are alive, which tools they use, what artists ask for in issue trackers) and a review of Charcoal
against each platform's best editor. Each step says what it wins.

## Where we stand

- **Almost all living text art is on platforms Charcoal already covers**: PC ANSI/ASCII (most of it), C64
  PETSCII, Amiga, ATASCII, teletext. Revision 2025: 12 DOS, 8 C64, 1 Amiga text entries; 2025 Teletext
  Advent Calendar: 38 entries.
- **The rival is IcyDraw** (Windows/Mac/Linux, very active): ANSI, PETSCII, ATASCII, VT52, IGS, RIP, with
  layers, TheDraw fonts, a font editor, animation, collaboration. Moebius is dormant but still named first
  by ANSI artists; its browser fork teXt0wnz runs on phones. PETSCII: Petmate9 and Marq. Teletext: edit.tf,
  QTeletextMaker. Nobody else does teletext + Spectrum + Amiga-first.
- **Our edge to keep and show**: exact round trips (8196 real files), the most formats in one small
  program, teletext and Spectrum, Amiga as a first-class page, Marq's smart flips for every font,
  configurable keys, crash autosave, a quiet interface, a fast viewer.
- **The one blocker for everybody**: no release yet (Windows and Mac builds exist since 2026-10-14), no website.

## Step 0: data loss, now (done 2026-10-14)

- [x] Teletext: a .tti with several subpages (a carousel) kept only the first; saving dropped the rest.
  Done: each subpage opens in a tab of its own; saving a tab writes the whole file back with only its
  subpage's OL rows 0-24 changed, every other line (PN, SC, PS, CT, DE, FL, RE, OL 25-29 enhancement
  packets, unknown commands) kept byte for byte; an unchanged page saves byte-identical. Frames come in step 2.
  Also fixed: a .tti whose rows hold ESC E / ESC Y opened as Atari ST VT52.

## Step 1: everyone can get it (wins: every artist not on Linux - most of them)

- [x] Windows and Mac builds, one file each (2026-10-14): GitHub Actions builds and tests Linux, Windows and
  macOS (Charcoal.app, Apple Silicon + Intel) on every push; a v* tag makes the release page. Details in HANDOFF.md.
- [ ] Tried by a real Windows and a real Mac user: v0.1b3-v0.1b7 released 2026-10-14 (prereleases, three zips each); not yet confirmed on real hardware.
- [x] A start screen (2026-10-14): "New page for" with a tile per machine (Amiga, PC, Atari 8-bit, Atari ST,
  C64, PET, VIC-20, C128, ZX Spectrum, teletext) drawn in its own font and colours with its size, the last 10
  files, Open, "Drop a file here". Typing, a tool or Esc starts on the last used type. Not done: an example
  to open (no art of our own to ship yet).
- A short manual with screenshots, and "coming from Moebius / Petmate / edit.tf" pages (their keys
  mapped to ours). A small website later.
  - [x] Coming from Moebius, PabloDraw, TheDraw, ACiDDraw (2026-10-14): Settings > Keys like (a `preset =` line in
    the keys file), the help's and README's section with their keys and ours. Moebius files open exactly as Moebius
    shows them and CharEdit's saves read back in Moebius the same (69 files written by Moebius's own code).
    Still to do: Petmate and edit.tf pages.

## Step 2: what every rival has and artists ask for most

1. [x] **Frames and animation** (done 2026-10-14, docs/frames-design.md): a frame strip, copy, onion skin,
   hold time, playback; ANSImation built frame by frame or split from a file, PETSCII/ATASCII/VT52 animations,
   teletext carousels; export GIF and APNG (and the stream for real time). Multi-screen .c/.petmate open as
   frames, not tabs. Later: delta streams, CharView playing frames, onion of the next frame.
   - [x] **Animation templates (TheDraw's scan patterns)** (done 2026-10-14, transitions.go): a request from the
     Ami-Express sysops; TheDraw's 14 Easy Animation patterns + Reverse/Flip and Charcoal's own fades, typewriter
     and scroll, as a stream file's draw order or as frames.
   - [x] **Animate a folder** (done 2026-10-14, batch.go): every screen of a folder animated for the board at
     once (one effect or a random one each, from a seed), versions named the way each board picks a random
     screen, a report; also `charedit --animate`.
2. [x] **Reference image** to trace over (done 2026-10-14): open (Ref gadget, Ctrl+Shift+I, or drop a PNG / JPEG
   / GIF / BMP / WebP), move (drag, arrows, Ctrl+drag), scale (fit width, fit page, 1:1, steps), fade, greys,
   hide, under the art or faintly over it; remembered per art file and in the autosave; never converted into
   characters (the scene bans conversions) and never exported.
3. [x] **Own fonts and character sets** (done 2026-10-14): load .fnt/.f08/.f14/.f16/.psf (PC), Atari .fnt,
   C64 .64c/charset .bin, Spectrum 768-byte sets and UDGs (file requester, or a dropped file that asks), kept with
   the art (XBin, Petmate customFonts; a .prg in Petmate's form cannot hold one, so it and the stream formats have
   CharEdit remember the font per file), autosaved; a small font editor that saves the font as a file.
4. [x] **Shapes and brushes** (done 2026-10-14): the Shape tool (line, box, ellipse, outlined or filled) in the
   brush, in box-drawing lines (PC single/double, PETSCII, ATASCII) or in half blocks / sixels; a shade brush
   (░▒▓█, per page type, own sequences in prefs.json); symmetry (left-right, top-bottom); colour-only /
   character-only painting. Later: Marq's left/right Shift for colour/character only, rounded PETSCII corners.

## Step 3: compo-ready (wins: party entrants, the most active artists)

- [x] **Work stages** (2026-10-16, stages.go, docs/workstages-design.md): File > Export work stages writes N
  pictures (PNG 1x/2x, the page's format too) spread evenly over the work, from a thinned history kept with the
  file across sessions and in the autosave. (Revision oldskool wants exactly 10, Zoo 2026 at least 15.)
- [x] **Party rule check** (2026-10-16, compo.go): File > Check for a party against 10 compos' 2026 rules (size,
  iCE/blink, own font/palette, animation, C64 modes, page type, teletext rows); quiet report, no fixing; "Save
  for the party" writes the files a compo wants (+ work stages) into a folder.
- **Runs on the machine**: C64 .prg (done) and .d64 (done 2026-10-16), Atari .xex, Spectrum .tap, a matching screenshot PNG
  with the border.
  - [x] Atari .xex and a boot .atr (2026-10-16, atari8.go): the page in its colour registers and font on any
    400/800/XL/XE, checked in a 6502 + ANTIC model against the PNG pixel for pixel.
  - [x] Spectrum .tap (2026-10-16, zxtap.go): BASIC loader + SCREEN$, blocks checked byte for byte; any tape's
    SCREEN$ opens.
  - [x] Run in real emulators (2026-10-16, emu_test.go, tools/emucheck): C64 .prg/.d64 in VICE, Atari .xex/.atr in
    atari800, Spectrum .tap in Fuse, CPC .dsk in MAME; every screen pixel for pixel as the PNG export. No writer bug found.

## Step 4: PC artists feel at home (the biggest group)

- [x] 9-pixel VGA cells (2026-10-14): from the SAUCE letter-spacing flag, Alt+9, kept on save with the aspect bits.
- The 4:3 aspect preview (from the SAUCE aspect bits, which are kept on save).
- [x] BBS colour codes (2026-10-16, bbscodes.go): pipe codes (Mystic, Renegade, WWIV 5), Synchronet Ctrl-A,
  WWIV hearts, Wildcat! @XX@, Celerity |x read (also mixed with ANSI) and written; Open as lists PCBoard, Avatar
  and each of them. "Save for board X" writes the board's own colour codes too (2026-10-16, see Step 5).
- [x] More PC fonts (done 2026-10-14): EGA 8x14, VGA50 8x8 (80x50), code pages 850 and 866; SAUCE font names
  honoured on opening and written on saving (IBM VGA, IBM EGA, IBM VGA50, IBM EGA43, IBM VGA 850 ...). Not yet:
  other code pages' 8x8/8x14 sets (Moebius's names open in the CP437 font of the same size and are kept on save), the 4:3 aspect above.
- [x] TheDraw fonts (logo typing) (done 2026-10-16, tdf.go tdf_ui.go): the Logo tool, the font list with previews,
  a line to stamp, Roy/SAC's collection downloaded on request; PC, Amiga (Latin-1 fonts only) and Unicode pages.
- Select by character or colour, fill limited to a colour.
- Layers (later: they touch everything, so after frames).

## Step 5: platform by platform

- **Teletext**: edit.tf's Esc+letter code entry and Esc ? table; reveal concealed text.
- [x] **Viewdata/Prestel** (done 2026-10-16, viewdata.go): a teletext page of 24 rows; .cpt streams (carousels as
  frames), 960-byte .bin frames, Telstar .json frames (other fields kept); Play. Later: Telstar markup content,
  the Prestel character set's differences if any (it is the SAA5050's here).
- [x] **Minitel/Videotex** (done 2026-10-16, minitel.go): 40x25 with the status row, G0/G2/accents, G1 mosaics,
  zones (background, underline, conceal) as the STUM 1B gives them, flash, inverse, big letters, grey levels or
  colour; .vdt read and written (149 real pages round-trip), Play; Bisson's CC0 font. Later: DRCS (downloaded
  characters, skipped when read), the 80-column mode, a run on a real Minitel (VDT2BMP draws 25 of 149 pages
  differently: see HANDOFF).
- **PETSCII**: Plus/4 and C16 (TED, 121 colours); C64 custom charsets, multicolour and ECM; dirart and
  .d64; pages wider than the screen for scrollers; asm/BASIC exports.
  - [x] **C64 beyond PETSCII** (2026-10-16, c64x*.go): multicolour and ECM pages drawn as the VIC-II shows them
    (Screen, MC/BG gadgets), custom character sets (.64c/.bin, CharPad .ctm v5-v9 as a font or a page, pixel
    pairs in the font editor), CharEdit's own .prg (set at $2000, VIC registers) and .d64, the ACME source and
    BASIC listing with sets and modes; [x] run in VICE (2026-10-16). Later: CharPad .ctm writing, Petmate files with modes.
  - [x] **DirArt** (2026-10-16, dirart.go): a 16-column DirArt page; a `.d64` whose directory listing is the page
    (DEL entries), written over the disk it came from (its programs kept); any `.d64` without a screen program
    opens as one; Marq's and Petmate9's DirArt read. `LOAD"$",8` + `LIST` in VICE shows the page.
  - [x] **Plus/4, C16, C116 (TED)** (done 2026-10-16, ted.go, ted_formats.go): a PETSCII model with the TED's
    121 colours (hue x luminance grid in the Colours, the wheel for luminance) and flash per cell, screen and
    border colours, the ROM's characters from VICE; VICE's computed palette (identical to xplus4's screenshots),
    YAPE's and Colodore's; Marq's Plus/4 .prg (run in VICE's xplus4: pixel-identical to CharEdit's PNG) and .c,
    a .d64, TED Screen Editor's .scrn + .proj, Petmate9's c16 screens, .seq with the KERNAL's colours and FLASH
    codes, Marq's BASIC and ACME exports (run in xplus4 too), PNG at VICE's 384x288. Later: the screen
    editor's ESC commands in .seq, multicolour text mode.
  - [x] **Plus/4 custom character sets** (2026-10-16, tedchars.go): a page's own set (128 characters with the
    TED's reverse, or 256) in CharEdit's Plus/4 .prg/.d64, TEDSE's .chrs beside the .scrn, the BASIC and ACME
    exports; the font editor keeps a character and its reverse together; all run in xplus4, pixel for pixel.
- [x] **Amstrad CPC (AMSCII)** (done 2026-10-16, cpc.go): mode 1 40x25 4 inks, mode 0 20x25 16 inks, mode 2
  80x25 2 inks; the 27 colours, inks and border in the Colours panel; the ROM font built in (Amstrad's
  permission, as the Spectrum's); .bin that runs on a CPC, .dsk, Playscii .psci with Logiker's exporter; reads
  the exporter's files (all 2024 compo entries, pixel-identical to their Caprice Forever screenshots). [x] CharEdit's .bin run in MAME (2026-10-16).
  Later: a real-CPC run, Caprice's 768x544 screenshot size, play the stream, custom
  characters (SYMBOL, not allowed in the compos).
- [x] **MSX** (2026-10-16, msx.go): SCREEN 0 (40x24, two colours), SCREEN 1 (32x24, colours per 8 characters, border)
  and the MSX2's 80 columns; the TMS9918 and V9938 palettes; C-BIOS's fonts (BSD) and an MSX BIOS ROM's when the
  user has one; own fonts; a cartridge (.rom) and a BLOAD .bin that show the page (run in MAME on C-BIOS: pixel
  for pixel), Playscii's msx charset. Later: a .dsk/.cas, an MSX BASIC listing, MSX2 blink.
- [x] **Sharp MZ-700** (2026-10-16, mz.go): 40x25, both character sets, 8 colours per cell; .mzf that shows the
  page (run in MAME's mz700: pixel for pixel), other .mzf pictures read; a public domain look-alike of the first
  set, Sharp's ROM when the user has it. [x] The second set's look without the ROM (2026-10-16): CharEdit's own
  CC0 look-alike, drawn from the character table (fonts/mz700-set2.font).
- **Spectrum**: UDG/charset editing; attributes-only painting.
- [x] **Atari 8-bit** (2026-10-16, gtia.go, atari8.go): the real OS ROM font when present, the 128-colour palette
  (PAL and NTSC, atari800's model) with the machine's colour registers, custom fonts (load, edit, save .fnt; in the .xex).
- [x] **Atari ST IGS** (done 2026-10-16, igs.go, docs/igs-design.md): IGS files (the ST BBS graphics language) shown
  exactly (IcyDraw's engine ported; every one of IcyDraw's passing test renders matched pixel for pixel) as a
  picture under the ST page's cells; type VT52 text over it and save `.ig` with the drawing, sounds and closing
  kept byte for byte; ST/STE palettes from the file's S / X 1 / X 12 (STE's 4-bit levels). Later: editing the
  drawing itself (IcyDraw has a script editor), playing the sounds and pauses, the palette on plain ST pages.
- [x] **Terminals** (2026-10-16, docs/unicode-design.md): a Unicode page (blocks, sextants and octants, Braille,
  box drawing, any Unifont character; 16, 256 and 24-bit colours) for terminal artists (durdraw's crowd): UTF-8
  ANSI and durdraw `.dur` (frames) read and written, mIRC colour codes read and written (also from a PC page),
  HTML; PC/Amiga pages convert exactly in one undoable step. Later: wide (CJK, emoji) characters as two cells,
  durdraw's own 16-colour iCE backgrounds checked in durdraw's screen (its export cannot write them).

- [x] **BBS boards** (2026-10-16, boardcolours.go): "Save for board X" (bbspause.go) writes the board's own colour
  codes as well as its pauses: Mystic pipe codes, Synchronet Ctrl-A, PCBoard @X (the user said yes, 2026-10-16);
  a stream with a colour the board has no code for (blink on Mystic) keeps ANSI.
- [x] **C64 BBS graphics** (2026-10-16, c64bbs.go): pauses in animated .seq files for Image BBS (£S, tenths) and
  C*Base (Ctrl-P, ~1/30 s), each board's own characters escaped and its line reading respected; .seq files with
  Image BBS's £ codes read (colours, clear, reverse) and saved back with them. Later: Image's whole-second £W and
  Color 64 (open questions to the user), C-Net/DMBBS/Centipede codes (not found), a run on a real board.

- [x] **The rest of other editors' files** (2026-10-16, icymach.go, psciown.go, othformats.go, petmate9.go,
  skypix.go): IcyDraw's PETSCII (every Commodore model), ATASCII and ST pages read and written (IcyDraw's engine
  reads and draws all 18 kinds as CharEdit does); Playscii's other sets as the page's own font (fetched) and its
  palettes; PabloDraw's .pd; TheDraw 4's .td (read, page layers as frames); BSAVE .bsv; Petmate9's VIC-20, PET
  and C128 screens; SkyPix (Amiga BBS graphics) as a picture under the cells, the Amiga fonts fetched. Later:
  IcyDraw's Viewdata/Mode 7 pages, TheDraw 3 and .tdl, Playscii's teletext set on the teletext page.
  [x] Marq's DirArt as a .d64 (2026-10-16, dirart.go). (Playscii's msx set opens on the MSX page; sharp, the Japanese MZ set, stays a PC page.)
- [x] **Sharp MZ-700**: a free look-alike of the second character set (2026-10-16, fonts/mz700-set2.font, CC0;
  drawn by hand from the character table in tools/mz700-set2.txt).

## Later

- Collaboration in Moebius's protocol (IcyDraw and Moebius speak it).
- Palette editing beyond 16 colours.
- Not planned: Shift-JIS AA (proportional, scene gone), RIPscrip (vector, not text).

Last updated 2026-10-14 (steps 0-2 done, plus animation templates, BBS pauses, Animate a folder, Moebius and other editors' files; 0.1b7 released; next: step 3).
