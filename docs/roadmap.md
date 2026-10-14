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
- [ ] The first release, tried by a Windows and a Mac user: v0.1b3 tagged 2026-10-14 (prerelease, three zips).
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

- **Work stages**: export 6-10 stages from the undo history (Revision wants exactly 10, Deadline 6).
- **Party rule check**: size limits (Revision 80x500), no iCE/blink for PC, ROM font for PETSCII.
- **Runs on the machine**: C64 .prg (done) and .d64, Atari .xex, Spectrum .tap, a matching screenshot PNG
  with the border.

## Step 4: PC artists feel at home (the biggest group)

- [x] 9-pixel VGA cells (2026-10-14): from the SAUCE letter-spacing flag, Alt+9, kept on save with the aspect bits.
- The 4:3 aspect preview (from the SAUCE aspect bits, which are kept on save).
- [x] More PC fonts (done 2026-10-14): EGA 8x14, VGA50 8x8 (80x50), code pages 850 and 866; SAUCE font names
  honoured on opening and written on saving (IBM VGA, IBM EGA, IBM VGA50, IBM EGA43, IBM VGA 850 ...). Not yet:
  other code pages' 8x8/8x14 sets (Moebius's names open in the CP437 font of the same size and are kept on save), the 4:3 aspect above.
- TheDraw fonts (logo typing), select by character or colour, fill limited to a colour.
- Layers (later: they touch everything, so after frames).

## Step 5: platform by platform

- **Teletext**: edit.tf's Esc+letter code entry and Esc ? table; reveal concealed text; Viewdata/Prestel
  pages (40x24, same characters); Minitel/Videotex (.vdt) reusing the teletext code.
- **PETSCII**: Plus/4 and C16 (TED, 121 colours); C64 custom charsets, multicolour and ECM; dirart and
  .d64; pages wider than the screen for scrollers; asm/BASIC exports.
- **Amstrad CPC (AMSCII)**: the liveliest platform we lack (compos 2022-24): mode 1 40x25 4 inks, mode 0
  20x25 16 inks, mode 2 80x25; 27-colour palette; ROM font licence to check (as the Spectrum's).
- **Spectrum**: UDG/charset editing; attributes-only painting.
- **Atari 8-bit**: the real OS ROM font when present, the 128-colour palette, custom fonts.
- **Atari ST**: IGS (ST BBS graphics), ST/STE palettes.
- **Terminals**: a UTF-8/Unicode page (blocks, Braille, 256/24-bit colour) for terminal artists (durdraw's
  crowd), mIRC colour export.

## Later

- Collaboration in Moebius's protocol (IcyDraw and Moebius speak it).
- Palette editing beyond 16 colours.
- Not planned: Shift-JIS AA (proportional, scene gone), RIPscrip (vector, not text).

Last updated 2026-10-14 (coming from Moebius/PabloDraw/TheDraw, 9-pixel cells; steps 2.1 frames and animation templates, 2.2 reference picture, 2.3 own fonts and 2.4 shapes and brushes done; 0.1b3 released).
