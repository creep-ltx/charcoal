[Charcoal](../../README.md) · [Manual](README.md)

# Saving and file formats

## Save as

ANSI, ANSI in UTF-8 (for terminals), plain text, UTF-8, PC `.bin`, **XBin** (`.xb`, font and palette inside, so it looks the same in
any viewer; opens again too), PETSCII `.seq`, a **C64 program** (`.prg`: LOAD and RUN on a C64 or in VICE, a key
in Petmate's form, so Petmate and Marq's PETSCII editor open it too; any .prg in that form opens; a page with its
own character set, multicolour or ECM gets CharEdit's own program, which sets the VIC-II up and puts the set at
`$2000`), a **C64 disk** (`.d64`, a 1541 image with that program on it, named after the file; the first screen
program on any `.d64` opens), the **C array** of Marq's PETSCII editor and Petmate (`.c`), a
**Petmate workspace** (`.petmate`), **PNG**
(normal or twice the size; C64 pages with their border) and a **web page** (`.html`). A typed file ending picks
the format, and an **animated GIF or APNG** of the frames. Everything that holds characters opens too.
(Web pages do not open; a picture can be the page's reference, below.)

## Files for other editors

Save as lists them at the end, as "For other editors: ...", only for the page types they fit. The ones that
open again become the page's file (Save writes the same format again).

| Program | File | Pages | Saves | Opens | Checked against |
|---|---|---|---|---|---|
| iCEDraw | `.idf` | PC, Amiga | cells (runs as iCEDraw packs them), font, palette | yes | ansilove (picture identical), IcyDraw's reader; files IcyDraw wrote |
| ArtWorx | `.adf` | PC, Amiga, 80 columns | cells, font, the 16 colours in the EGA's 64 | yes | ansilove (picture identical), IcyDraw's reader; files IcyDraw wrote |
| TundraDraw | `.tnd` | PC, Amiga, 80 columns | cells with 24-bit colours (no font) | yes | ansilove (colours; it cannot draw characters 1, 2, 4, 6), IcyDraw's reader; files IcyDraw wrote |
| IcyDraw | `.icy` | PC, Amiga | every layer (name, shown or hidden, locked; empty cells invisible, black that shows below transparent), font, palette, iCE or blink; a picture of the first 80 rows | yes (version 1 with or without zstd, and the older version 0; layers kept when CharEdit's show them as IcyDraw does, else put together; picture layers and effects left out) | IcyDraw's reader; files IcyDraw 0.6 wrote; IcyDraw's engine reads a layered file cell for cell as CharEdit shows it |
| IcyDraw | `.icy` | PETSCII (C64, C128 40/80, VIC-20, PET 40/80, Plus/4 as IcyDraw's C16), Atari, Atari ST | IcyDraw's page of that machine: its machine and case, screen colour and border, the character set (a page's own set too), the machine's colour numbers (the C16's 128, the VDC's own order), flash; every layer (name, shown or hidden, locked; empty cells invisible; on an ST page black that shows below is written as the colour seen) | yes, on that machine's page (an XEP80 page of 80 columns shows its first 40; the VDC's per-cell set: the one most cells use; layers kept when CharEdit's show them as IcyDraw does) | IcyDraw's engine reads all 18 page kinds cell for cell and draws them pixel for pixel as CharEdit, also with layers; files its engine wrote |
| PabloDraw | `.pd` | PC, Amiga | PabloDraw's own file: its font (by PabloDraw's name, or the font itself), palette, cells with 24-bit colours; SAUCE for other widths | yes (an Amiga font opens on an Amiga page) | its source (Pablo.cs, Tundra.cs, BitFont.cs, Eto's Palette) |
| TheDraw | `.td` | PC | TheDraw 4's own save file: the frames as its page layers (8 at most), 80 or 160 columns, as tall as the tallest frame; Ctrl+S on an opened `.td` writes it again | yes, every page layer as a frame | TheDraw 4.63 itself (run in a PC emulator): it loads CharEdit's files and saves them as `.bin` cell for cell, every page layer; TheDraw's example files; abydos' reader |
| TheDraw, ACiDDraw | `.bsv` | PC, 80 columns | a BASIC BSAVE screen dump (`BLOAD` loads it into the text screen) | yes | TheDraw's manual |
| REXPaint | `.xp` | PC | every shown layer (empty cells in REXPaint's transparent pink; 9 at most), 24-bit colours | yes (layers kept when CharEdit's show them as REXPaint does, else put together) | IcyDraw's reader |
| Playscii | `.psci` | PC (dos), Amiga (amiga_topaz), PETSCII (c64_real_*, commodore_pet), Atari (atari) | every frame with its hold; a page of layers (PC, Amiga, ST): every layer of every frame, empty cells transparent; Playscii's own charset and palette by name | yes, also c64_petscii, c64_edscii (Playscii's defaults) and jpetscii; other charsets open as a PC page; the layers (of every frame) kept when CharEdit's show them as they are, else put together | Playscii's own loader (art.py); its glyph order compared with the ROM fonts |
| Playscii | `.psci` | PC with Playscii's set as its font: Apple II, Agat, BBC Master, Intellivision, Pac-Man, Sharp MZ (Japanese), ZX81 (`speccy`); Atari ST (`atari_st`) | the same charset and palette names and numbers as read (a palette other than ansi/ega is the page's own colours: the 16 the art uses most of a bigger one) | yes; the sets' pictures and Playscii's palettes are fetched on first use (see [Licences](licences.md)) | Playscii's charset.py/palette.py; its own art (56 files) |
| Marq's PETSCII editor | `.pet` | C64 | screen codes, colours, border, background, case | yes | its source (m_c64.pde) |
| Marq's PETSCII editor | `.bas`, `.s` | C64 | a BASIC listing (for petcat) and an ACME program of a 40x25 screen (data only for other sizes); with a custom set, multicolour or ECM: the registers and the set (BASIC: VIC bank 3, screen `$C400`, set `$C800`; ACME: the set at `$2000`) | no | its source; the ACME output assembled by ACME 0.97 and run in a 6502 (c64x_test.go) |
| CharPad | `.ctm` (v5-v9) | C64 | (read only) the character set, mode, colours; the map laid out as characters | yes, as a font or a page | CharPad 2.7's format pages; c64lib's test projects (`testdata/charpad`), every version the same page |
| Petmate | `.json` | PETSCII | every screen (Petmate's JSON export) | yes | its source (exporters/json.ts) |
| Marq's PETSCII editor | `.prg`, `.c`, `.bas`, `.s` | Plus/4 | the program of its Plus/4 template (screen codes, attributes, border, background, case); the C array with `PLUS4` (no flash: the editor has none); its BASIC 3.5 listing (`petcat -w3`) and ACME source (data only for sizes other than 40x25), attributes with their flash bit | `.prg` and `.c` | its source (m_plus4.pde) and template; the `.prg`, the BASIC listing (tokenised by VICE's petcat) and the ACME program (ACME 0.96.4) run in VICE's xplus4: screenshots identical to CharEdit's PNG |
| TED Screen Editor | `.scrn` + `.proj` (+ `.chrs`) | Plus/4 | attributes, its 24-byte signature, screen codes ("save screen"); the project's size, colours, case, and its character set (`NAME.chrs`, 128 characters from `$B800`) when the project says it changed | yes (without a `.proj`: 40 columns, black) | its source and README; its own title and help screens |
| Petmate9 | `.petmate` | Plus/4 | `c16Upper` / `c16Lower` screens with the TED's attribute bytes and colours | yes | its source |
| Marq's PETSCII editor, Petmate9 | `.c`, `.petmate` | DirArt | Marq's `DIRART` machine (16 columns, no colours) and Petmate9's `dirart` charset open on the DirArt page | yes | their sources (m_dirart.pde, Petmate9's d64 exporter) |
| Petmate9 | `.petmate` | VIC-20, PET 40/80, C128 40/80 | the machine's charset name (`vic20Upper`, `petBiz`, `c128Lower` ...); the VDC's colour numbers and attribute byte | yes, on that machine's page | its source (presetExport.ts, vdcAttr.ts) |

Traps: Petmate's `.pet` export says 0 for lower case where Marq's says 1 (CharEdit follows Marq); IcyDraw
evens out colours that do not show when it saves and writes NUL as a space; an ArtWorx or TundraDraw page is
80 columns; Playscii and REXPaint files cannot hold a font (CharEdit keeps a page's own font for the file, as
for `.ans`); PabloDraw cannot store characters 1-6 in a `.pd` (left out, the save says how many). Moebius and
ACiDDraw have no files of their own beyond ANSI/BIN/XBin (ACiDDraw's `.adf` is ArtWorx's); TheDraw's `.tdf`
are fonts (see [TheDraw fonts](fonts.md#thedraw-fonts-logos); CharEdit edits them too). TheDraw shows as much of a `.td` as its worksheet holds
(80x25 unless its Setup says 160 columns or up to 100 lines). Not done: TheDraw 3's `.td` and libraries (`.tdl`), its COM/OBJ/C/Pascal/ASM exports; IcyDraw's
Viewdata and Mode 7 pages, sixel layers, live text and effects; Playscii's teletext_uk, ultima4, ui and
osamu-micro sets (glyphs wider than a cell: they open by number); Marq's DirArt as a disk. Playscii's `msx` set
opens on the MSX page, its `sharp` set (the Japanese MZ's, with kana and kanji) on a PC page with the set as its font.

## Moebius files

Moebius files open as Moebius shows them, checked cell for cell against Moebius's own reader on files written
by Moebius's own save code (`testdata/moebius`, `tools/moebius/`): widths from 1 column, the height in the SAUCE
record (Moebius writes nothing for empty rows at the end), iCE colours, 9-pixel letter spacing, the font name (also
ones CharEdit draws with a near font: IBM VGA25G, other code pages), the SAUCE title and comments in UTF-8 as
Moebius writes them, XBin's own font and palette and its SAUCE record, and Export As UTF-8 files (256-colour codes,
VT-style wrap, their width). CharEdit's saves read back in Moebius as the same page, with the same SAUCE. Not the
same: characters 0, 7, 8 and 9 (• ◘ ○) are drawn when the file has SAUCE, as Moebius and PabloDraw draw them,
and saved as they were; on a new page CharEdit saves them as spaces, because a BBS terminal beeps, backs up and
tabs on them. Without SAUCE they act as a terminal's controls (text and NFO files use tabs). Moebius pads a page without SAUCE to 25 rows; CharEdit
opens it as tall as it is.
