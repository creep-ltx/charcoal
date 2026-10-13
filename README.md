# Charcoal

Drawing with characters: **CharEdit**, the editor, and **CharView**, the viewer.

**Start here: [docs/HANDOFF.md](docs/HANDOFF.md)** (state, the user's wishes, testing, next steps).

An art viewer and editor for Amiga ANSI, PC ANSI (and PCBoard, Avatar, XBin, iCEDraw, ArtWorx, TundraDraw),
Atari ATASCII, Atari ST VT52, Commodore PETSCII (PET, VIC-20, C64, C128), ZX Spectrum screens and teletext
(BBC Micro Mode 7, Ceefax), drawn with the real bitmap fonts: Topaz
1/1+/2/2+, MicroKnight, MicroKnight+, P0T-NOoDLE and mO'sOul on an Amiga page (Latin-1), IBM VGA on a PC page
(CP437), the C64's two character sets on a C64 page.

    charedit [FILE...]     # the editor: its own window, one tab per file

    charview FILE|FOLDER...   # just looks: n / Right / Space next, p / Left previous, q / Esc quits

**CharView** is the same program started as `charview` (a link to `charedit`, or `charedit --view`): the art
fills the window, fitted to its width, with one line at the bottom (i hides it). Up/Down/PgUp/PgDn/Home/End
and the wheel scroll (Shift+Left/Right sideways), +/- zoom (0 fits again), g grid, m plays the file at modem
speed, e opens it in CharEdit. **a** scrolls by itself (tall art down, wide art sideways; at the end it waits two
seconds and goes to the next file), **s** switches smooth / a text row at a time, **[ ]** set the speed (0.5-48
rows a second); these are remembered. Art wider than the window is shown smaller (1/2, 1/3 ...; - and + step it), drawn once so scrolling stays
smooth. A folder gives every art file in it (.ans .asc .nfo .diz .txt .bin .seq
.pet .ice ...). It never changes the editor's other settings.

A native program (Go + SDL2, one binary; needs only the SDL2 library that every Linux desktop has). The whole
interface is drawn in Topaz, kept dark and small so the art comes first; **Ctrl+H** hides every gadget.
Press the **?** gadget for every key.

## Build

    go build -o charedit .     # needs Go and the SDL2 development files
    go test ./...             # the core: parser, writer, SAUCE

`~/.local/bin/charedit` and `~/.local/bin/charview` link to the binary here.

## What it does

- **Amiga page**: Latin-1, the 8-bit CSI byte 0x9B as well as `ESC [`, form feed clears the screen, LF line ends,
  italic (SGR 3) and underline (SGR 4) drawn like the Amiga console, bold as bright colours or as the console's
  smeared bold ("Bold smear"), 8×16 (line doubled) or 8×8.
- **PC page**: CP437 in the VGA font, CRLF line ends, blink or iCE colours, half-block painting, `.bin` files.
- Both: cursor movement, erase, insert/delete lines and characters, save/restore cursor, SyncTERM's font switch
  (`CSI 0;n SP D`), SAUCE read and written (width, font name, title, author, group, iCE flag). Amiga or PC is guessed
  from SAUCE and the bytes; "Open as" overrides. UTF-8 text files are read too.
- **Atari page**: ATASCII (`.ata`, and `.txt` files recognised by their bytes): inverse characters, one
  background and one text colour for the screen, border; EOL line ends, ESC before control characters, cursor
  moves for animations. Width 38 (the screen editor's margins) or 40 (terminals) is guessed: a 39-40 character
  line settles it, else the width where characters stack best. Checked on the Break Into Chat collection: all 203
  real files recognised, none of 853 ANSI/NFO files on this PC mistaken for ATASCII. The font is AltirraOS's
  (Altirra emulator, all-permissive licence, `fonts/AltirraOS-LICENSE.txt`), built in.
- A page has a real height (H): a file opens as tall as it is, its empty lines at the end included; the page
  ends where the dark area starts; every row is saved. Enter or Down past the last row adds a row.
- Pages for a machine's screen keep its width: PETSCII the machine's, Atari 38 or 40; only Amiga and PC pages
  can be any width (W).
- **PETSCII page** (the Machine gadget): PET 40/80 columns (green, amber or white screen), VIC-20 (22x23, 8 text
  colours), C64 (40x25; Pepto or Colodore colours), C128 40 columns, C128 80 columns (VDC, RGBI colours). Each
  machine's own character set, upper/graphics and lower/upper, from VICE (first run or Settings, 18 KB, SHA-1
  checked). Rendering checked against 125 pictures exported by Marq's PETSCII editor: all of the same version
  identical, cell by cell.
- **Atari ST page**: the ST's VT52 terminal in low (40x25, 16 colours), medium (80x25, 4) or high resolution
  (80x25, mono); each cell its own colours; `.vt` streams read and written, the resolution guessed from the pens
  and columns used. Font: EmuTOS's, downloaded on first use (GPL, so not built in), SHA-1 checked.
- **Teletext page**: 40x25 cells of 7-bit codes in 12x20 cells like the SAA5050's; attributes (text/graphics
  colour, double height, flash, conceal, separated, hold, new/black background) act from their cell on, read
  left to right as the chip reads a row (set-at / set-after as in ETS 300 706 and edit.tf). **Codes** shows the
  attributes' names; the **Sixel** tool paints a cell's 2x3 graphics pixels. Opens and saves `.tti`, `.ep1`,
  edit.tf links (`.url`; any text with an edit.tf link opens), opens BBC Mode 7 dumps (1000-byte `.bin`). Font:
  Bedstead (public domain), 5x9 dots doubled with the chip's character rounding (`tools/teletext-font.py`).
- **ZX Spectrum page**: the 32x24 screen, ink/paper/bright/flash per cell, the 48K ROM's font (built in: Amstrad
  allows passing the ROM on, `fonts/zx-spectrum-NOTICE.txt`) with the block graphics and UDGs. Saved as `.scr`
  (6912 bytes); a `.scr` opens with each 8x8 cell matched to a character (drawings get the nearest one).
- **PC BBS formats**: PCBoard `@X` (.pcb) and Avatar (.avt) open and save; iCEDraw (.idf), ArtWorx (.adf) and
  TundraDraw (.tnd) open with their own fonts and colours. The same art in five formats (ansilove's test files,
  `testdata/alove`) gives the same page in all of them. PC attribute bytes are in BIOS colour order and are
  converted.
- **C64 page**: PETSCII `.seq` files (the stream a C64 BBS sends): 40 columns, colour and reverse codes, both
  character sets, one screen colour, cursor moves, clear/home, insert/delete. The character set is the C64's
  ROM, not shipped: the first run offers to download it from the VICE emulator project (SourceForge, or its
  GitHub mirror), checked by SHA-1, into `~/.local/share/charcoal/chargen.bin`; Settings can do it too. bbmp's
  and an installed VICE's copies are used if present.
- **Tabs and two pages at once**: every file opens in a tab; show one page, two side by side or one over the other
  (Ctrl+L), optionally scrolling together.
- **Grid** (Ctrl+G, brighter every 10 columns and every 25 rows), **zoom** 1x-8x, fit to width (Ctrl+9),
  Ctrl+wheel zooms around the mouse.
- **Flip** a block left-right (Alt+H) or upside down (Alt+V): characters become their mirror images (known
  pairs first, then by comparing the glyphs' pixels, so it works in every font).
- Bottom strip: tools, all 256 characters (click = brush), 5 F-key sets (F1–F10), 16 colours (left fg, right bg,
  Shift+click edits the colour).
- Tools: Type, Draw (Shift+click = line), Colour, Fill, Pick (or middle click), Select (copy/cut/paste/stamp), Half.
- Undo/redo, play at modem speed; dropping a file on the window opens it.
- **Safety**: pages with changes are kept aside every two minutes and offered back after a crash; saving over a
  file keeps the old one as `name.bak` (Settings); the Open requester lists the last 10 files.
- **Save as**: ANSI, plain text, UTF-8, PC `.bin`, **XBin** (`.xb`, font and palette inside, so it looks the same in
  any viewer; opens again too), PETSCII `.seq`, a **C64 program** (`.prg`: LOAD and RUN on a C64 or in VICE, a key
  in Petmate's form, so Petmate and Marq's PETSCII editor open it too; any .prg in that form opens), the **C array** of Marq's PETSCII editor and Petmate (`.c`), a
  **Petmate workspace** (`.petmate`), **PNG**
  (normal or twice the size; C64 pages with their border) and a **web page** (`.html`). A typed file ending picks
  the format. Everything that holds characters opens too; a `.c` or `.petmate` with several screens opens each
  screen in its own tab. (Pictures and web pages do not open.)
- Settings in `~/.config/charcoal/prefs.json` (`"uiscale": 2` doubles the window for big screens).

## Keys

Every shortcut can be changed in `~/.config/charcoal/keys` (`action = key, key`; written with the defaults and
comments on the first start; a line it cannot read is skipped and named in the status line). Alt is the left Alt
only (the right one is AltGr). Fixed: typing, F1-F10 (the F-key set), the cursor keys, Return, Tab, Backspace,
Delete, Insert, Esc. The defaults, the PC ANSI editors' (Moebius, PabloDraw, TheDraw) where they agree:

| Keys | Action |
|---|---|
| Ctrl+O, Ctrl+Shift+O | open, open the most recent file again |
| Ctrl+S, Ctrl+Shift+S, Ctrl+I | save, save as, info (SAUCE) |
| Ctrl+T / Ctrl+N, Ctrl+W, Ctrl+Tab / Ctrl+Shift+Tab (Ctrl+PgDn / PgUp) | new page, close page, next / previous page |
| Ctrl+L, Ctrl+Q | one page / side by side / top and bottom, quit |
| Ctrl+Z, Ctrl+Y / Ctrl+Shift+Z | undo, redo |
| Ctrl+A, Ctrl+C, Ctrl+X, Ctrl+V | select all, copy, cut, paste |
| Shift+arrows / Home / End / PgUp / PgDn | select from the cursor (any tool); Esc lets go |
| Alt+H, Alt+V, Alt+R, Alt+I | mirror, upside down, turn, invert the block (nothing selected: H/V/R turn the brush character) |
| Alt+F, Ctrl+K | fill the selection with the brush (nothing selected: Fill tool), crop to the selection (Amiga, PC) |
| Ctrl+Home, Ctrl+End | top, bottom of the page |
| Alt+Up / Alt+Down, Alt+Left / Alt+Right | delete / insert a row, delete / insert a column |
| Alt+T, D, K, L, P, S, B | tools: Type, Draw, Colour, Fill, Pick, Select, Half (PC) / Sixel (teletext) |
| Ctrl+Up / Ctrl+Down, Ctrl+Left / Ctrl+Right | foreground -/+, background -/+ |
| Ctrl+0-7, Alt+0-7 | foreground, background colour (again: the bright one) |
| Alt+U, Ctrl+D, Ctrl+Shift+X, Ctrl+E | take the colours under the cursor, default colours, swap, iCE on/off |
| Ctrl+F1-F10, Ctrl+Shift+Up / Down | F-key set 1-10, next / previous set |
| Ctrl+= / Ctrl++, Ctrl+-, Ctrl+9 | zoom in, out, fit the width |
| Ctrl+G, Ctrl+R, Ctrl+H | grid, reverse (PETSCII, Atari), hide the gadgets |

Dragging inside a selection with the Select tool moves it (one undo step).

## Tested

`bigcheck_test.go` (see `tools/bigcheck/`) runs thousands of real files: on 2026-10-13, 4119 files from 16colo.rs
(PC, Amiga, XBin, PETSCII-style and animation packs), C64 BBS `.seq` files, Petmate `.petmate`/`.c` files from
GitHub and the Break Into Chat ATASCII collection. All open, all with a known type get the right page type, all
come back cell for cell after saving in their own format.

## Files

| Path | What |
|---|---|
| `core.go` | document, ANSI parser, writer, SAUCE (no window; `core_test.go`) |
| `editor.go` | the editor's state and editing, loading, saving, playing |
| `view.go` | the main window: gadget bar, page, bottom strip, status line |
| `dialogs.go` | open/save, info, colour, help, yes/no requesters |
| `ui.go`, `gfx.go` | the gadgets and the pixel drawing |
| `fonts.go`, `fonts/` | the embedded fonts and their mirror tables; `cp437.go` the CP437 table |
| `viewer.go` | CharView |
| `export.go` | `.prg`, the `.c` array, XBin, HTML (`export_test.go` runs the `.prg` in a small 6502 emulator) |
| `atari.go` | ATASCII: the Atari fonts, `.ata` reader and writer, recognising and width guessing (`atari_test.go`) |
| `pet.go` | PETSCII: the C64 fonts, the `.seq` reader and writer, typing (`pet_test.go`) |
| `keys.go` | the table of actions and their default keys, the keys file, the help's key list |
| `rowscols.go`, `blockjobs.go` | insert/delete rows and columns; fill, move and crop a block |
| `autosave.go` | autosave and offering it back, `.bak` on save, recent files |
| `script.go` | `CHARCOAL_SCRIPT` test steps, for checking it headless with `SDL_VIDEODRIVER=offscreen` |
| `web/` | the first version, called amiansi, that ran in the browser (Python server + HTML/JS), kept for reference |

## Licences

Charcoal's own code is under the MIT licence (`LICENSE`), copyright Tobias Karlsson. These parts keep theirs:

| Part | From | Licence |
|---|---|---|
| Amiga fonts (Topaz 1/1+/2/2+, MicroKnight/+, P0T-NOoDLE, mO'sOul) | TrueSchool's *Multi Platform Amiga Fonts* raw conversions, dMG/t!s^dS!, github.com/rewtnull/amigafonts (256 glyphs × 16 rows, each row of the 8×8 original doubled) | GPL with the font exception, `fonts/amigafonts-NOTICE.txt` |
| IBM VGA 8×16 | rendered from Px437_IBM_VGA_8x16, VileR, *The Ultimate Oldschool PC Font Pack* | CC BY-SA 4.0, `fonts/Px437-LICENSE.TXT` |
| Atari 8-bit fonts | AltirraOS, from the Altirra emulator | all-permissive, `fonts/AltirraOS-LICENSE.txt` |
| ZX Spectrum font | the 48K ROM's character set | © Amstrad, who allow passing it on, `fonts/zx-spectrum-NOTICE.txt` |
| Teletext font | Bedstead, Ben Harris | CC0, `fonts/bedstead-NOTICE.txt` |
| PETSCII flip and turn tables (`cbmturn.go`) | Marq's PETSCII editor, Marq / Fit^L!T, tables by Dr. TerrorZ | WTFPL, `third_party/marq-petscii-LICENSE.txt` |
| Petmate's file formats | Petmate, C. T. Lin | MIT, `third_party/petmate-LICENSE.txt` |
| Test art in `testdata/alove` | ansilove's tests | BSD 2-clause, `testdata/alove/LICENSE-ansilove.txt` |

Not included, fetched on first use and checked by SHA-1: the Commodore character ROMs (from the VICE
project) and the EmuTOS font sources (GPL), for PETSCII and Atari ST pages.
