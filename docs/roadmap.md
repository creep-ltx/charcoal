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
- **The one blocker for everybody**: Linux only, no release, no website.

## Step 0: data loss, now (done 2026-10-14)

- [x] Teletext: a .tti with several subpages (a carousel) kept only the first; saving dropped the rest.
  Done: each subpage opens in a tab of its own; saving a tab writes the whole file back with only its
  subpage's OL rows 0-24 changed, every other line (PN, SC, PS, CT, DE, FL, RE, OL 25-29 enhancement
  packets, unknown commands) kept byte for byte; an unchanged page saves byte-identical. Frames come in step 2.
  Also fixed: a .tti whose rows hold ESC E / ESC Y opened as Atari ST VT52.

## Step 1: everyone can get it (wins: every artist not on Linux - most of them)

- Windows and Mac builds (plan in HANDOFF.md), one file each, a public release page.
- A start screen: "New page for ..." with each machine's picture, recent files, an example to open.
  Today a PC artist lands on an Amiga page and must know to press PC.
- A short manual with screenshots, and "coming from Moebius / Petmate / edit.tf" pages (their keys
  mapped to ours). A small website later.

## Step 2: what every rival has and artists ask for most

1. **Frames and animation** (the most-asked feature everywhere): a frame strip, duplicate, onion skin,
   hold time, playback; ANSImation built frame by frame, PETSCII/ATASCII animations, teletext carousels;
   export GIF and APNG (and the stream for real time). Multi-screen .c/.petmate open as frames, not tabs.
2. **Reference image** to trace over: open, move, scale, fade, hide (not converted into characters: the
   scene bans conversions, so this stays a tracing aid).
3. **Own fonts and character sets**: load .fnt/.psf/.f16 (PC), Atari .fnt, C64 .64c/charset .bin,
   Spectrum 768-byte sets and UDGs, kept with the art (XBin, .prg); a small font editor.
4. **Shapes and brushes**: line (drag), rectangle, ellipse (filled or not), a shade brush (░▒▓█),
   symmetry drawing, colour-only / character-only painting.

## Step 3: compo-ready (wins: party entrants, the most active artists)

- **Work stages**: export 6-10 stages from the undo history (Revision wants exactly 10, Deadline 6).
- **Party rule check**: size limits (Revision 80x500), no iCE/blink for PC, ROM font for PETSCII.
- **Runs on the machine**: C64 .prg (done) and .d64, Atari .xex, Spectrum .tap, a matching screenshot PNG
  with the border.

## Step 4: PC artists feel at home (the biggest group)

- 9-pixel VGA cells and the 4:3 aspect preview (from SAUCE flags).
- More PC fonts: EGA 8x14, VGA50 8x8 (80x50), other code pages; SAUCE font names honoured.
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

Last updated 2026-10-13 (from the two surveys; nothing in Steps 1-5 built yet).
