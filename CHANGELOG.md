# Changes

Each version's section is also its release page on GitHub. Newest first.

## Unreleased

- **Wide characters on Unicode pages:** Chinese, Japanese and Korean characters, fullwidth forms and emoji take
  two cells, as in a terminal. Type them (an input method works too), paste them, or pick them from the new kana,
  CJK, Hangul and fullwidth blocks of the character table. The cursor, Delete, Backspace, fills, lines, selections,
  Mirror and the page width all keep them whole; writing over half of one leaves a space, as a terminal does.
- UTF-8 ANSI, durdraw, mIRC, text and web page files read and write them as terminals show them. durdraw
  itself shows CharEdit's files with them as they are.
- **Changed:** the 76 wide symbols a Unicode page could already type (⌚ ☕ ✅ ⭐ ☰ and the like) take two cells
  now, so files with them line up in a terminal. Files that count a kanji as one column, padding the line
  with blanks, open as a terminal shows them.
- **Changed:** a UTF-8 file with any wide character opens on a Unicode page (a PC page could only show `?`),
  unless its SAUCE record names a PC or Amiga font.
- The program is about 1 MB larger: every wide character's picture is built in (GNU Unifont).

## 0.1b12 (2026-10-16)

- **Layers** on PC, Amiga, Unicode, PETSCII, Atari and Atari ST pages. Every tool draws on the current layer;
  empty cells show the layers below. A Layers section in the panel: show/hide, rename, new, copy, up/down,
  merge down, delete, and the other layers shown, dim or hidden. "Black shows below" puts letters over a picture.
  Keys: Alt+Shift+N new, Ctrl+Shift+PgUp/PgDn switch, Alt+Shift+Up/Down move, Alt+Shift+M merge, Alt+Shift+E
  show/hide. Edit > Copy what is seen.
- IcyDraw `.icy`, REXPaint `.xp` and Playscii `.psci` keep their layers when saved. Every other format saves what
  you see (hidden layers left out, with a message), and the layers come back when you open the file again.
- A page with one layer works exactly as before.

## 0.1b11 (2026-10-16)

- **TheDraw fonts:** the Logo tool (Alt+G) types big logos in `.tdf` fonts (Block, Colour and Outline with
  TheDraw's 19 outline styles). A font list with previews (Alt+Shift+G), a Line requester to type and place a
  whole line (Ctrl+Shift+G). Roy/SAC's collection of 3,536 fonts can be downloaded on request.
- **Drawing together:** join a Moebius or IcyDraw drawing session, or host one they can join (File menu).
  Edits, cursors and chat go both ways; by default only this computer can join a session you host.
- Sharp MZ-700: its second character set now shows without Sharp's ROM (a free look-alike).
- Plus/4: pages with their own character set, carried in `.prg`, `.d64`, `.scrn`, BASIC and ACME files.
- C64 DirArt: art as a disk's directory listing (`.d64`; `LOAD"$",8` and `LIST` show it).

## 0.1b10 (2026-10-16)

- **MSX** pages (SCREEN 0, SCREEN 1, 80 columns; `.rom` cartridges and BASIC `BLOAD` `.bin` files) and **Sharp
  MZ-700** pages (`.mzf` tape files).
- **Check for a party:** what a page breaks in ten compos' 2026 rules (Revision, Evoke, Lovebyte, Zoo, Deadline,
  X, Logiker's AMSCII and ATASCII, the Teletext Advent Calendar). **Save for the party** writes the files a compo
  wants. **Export work stages** writes pictures of the work from start to now.
- More of other editors' files: IcyDraw on every machine, Playscii's other character sets (Apple II, ZX81, BBC
  Master and more), PabloDraw `.pd`, TheDraw `.td`, BSAVE `.bsv`, Petmate9's other machines, SkyPix (Amiga BBS
  graphics).
- CharView browses the newer formats too.

## 0.1b9 (2026-10-16)

- **Commodore BBS boards:** animated `.seq` files saved with Image BBS or C*Base pauses; Image BBS `£` codes read
  and written.
- Animations saved for Mystic, Synchronet and PCBoard use the board's own colour codes (about a quarter smaller).
- **Plus/4, C16 and C116** pages: 121 colours, flash, the Plus/4 character set; TED Screen Editor, Marq and
  Petmate9 files.
- Files that run on the machine: Atari `.xex` and bootable `.atr`, ZX Spectrum `.tap`.
- Atari 8-bit pages: the real GTIA colours (Altirra's palette by default), the colour registers, the OS ROM font
  when you have the ROM.
- Every machine file CharEdit writes is checked in real emulators (VICE, atari800, Fuse, MAME).
- Fixed: text like "@X1F" in art was taken as a PCBoard colour code.

## 0.1b8 (2026-10-16)

- **Amstrad CPC (AMSCII)** pages: modes 0, 1 and 2, the 27 colours, `.bin` and `.dsk` that run on a CPC,
  Playscii files; PNGs at the compos' screenshot size.
- **Unicode / terminal** pages: blocks, sextants, Braille, 256 and 24-bit colours; UTF-8 ANSI, durdraw `.dur`
  (with frames) and mIRC colour codes.
- **BBS colour codes** on PC pages: pipe codes (Mystic, Renegade), Synchronet Ctrl-A, WWIV, Wildcat!, Celerity.
- C64: multicolour, extended background colours (ECM), own character sets, CharPad `.ctm`, `.d64` disks.
- **Minitel** (`.vdt`) and **Viewdata/Prestel** pages.
- Atari ST **IGS** pictures shown, with text typed over them.
- Fixed: a UTF-8 file opened and saved again was written in CP437, so a terminal showed garbage.
- Linux: `make` and `make install`.

## 0.1b7 (2026-10-14)

- Animations trimmed with Small/Medium/Large detail, play time per connection speed and a warning for heavy
  effects, previews at a caller's speed.
- Animate a folder: random effects, budgets and random screens per board.
- Moebius files open as Moebius shows them; keys like Moebius, PabloDraw, TheDraw or ACiDDraw; 9-pixel cells.
- Save for iCEDraw, ArtWorx, TundraDraw, IcyDraw, REXPaint, Playscii, Marq's PETSCII editor and Petmate.

## 0.1b6 (2026-10-14)

- Ten more animation templates (Matrix rain, Decrypt, Spiral, Drop, Assemble, Blinds, Shine, Shade in, Melt in,
  Power on).
- Frame effects saved as delta streams that play on a BBS.
- BBS pauses for Ami-Express 5, Mystic, PCBoard, Synchronet and Tempest.

## 0.1b5 (2026-10-14)

- Animation templates (TheDraw's scan patterns), a reference picture to trace over, shapes and brushes, own fonts
  and a font editor, more PC fonts.

## 0.1b4 (2026-10-14)

- Frames and animation: a frame strip, onion skin, playback, GIF and APNG export; C64 screens and teletext
  carousels as frames; ANSI and PETSCII animations split into frames.

## 0.1b3 (2026-10-14)

- Multi-screen C64 files keep every screen when saved; the start screen.

## 0.1b2 (2026-10-14)

- Marq's PET and VIC-20 pictures open on their own machine; a readable character picker.

## 0.1b1 (2026-10-14)

- The first public beta, for Linux, Windows and Mac.
