[Charcoal](../../README.md) · [Manual](README.md)

# Commodore pages: PETSCII, C64, Plus/4, DirArt

## PETSCII page

**PETSCII page** (the Machine gadget): PET 40/80 columns (green, amber or white screen), VIC-20 (22x23, 8 text
colours), C64 (40x25; Pepto or Colodore colours), C128 40 columns, C128 80 columns (VDC, RGBI colours), Plus/4,
DirArt (below).
Each machine's own character set, upper/graphics and lower/upper, from VICE (first run or Settings, 34 KB, SHA-1
checked). Rendering checked against 125 pictures exported by Marq's PETSCII editor: all of the same version
identical, cell by cell.

## C64 page and .seq files

PETSCII `.seq` files (the stream a C64 BBS sends): 40 columns, colour and reverse codes, both
character sets, one screen colour, cursor moves, clear/home, insert/delete. The character set is the C64's
ROM, not shipped: the first run offers to download it from the VICE emulator project (SourceForge, or its
GitHub mirror), checked by SHA-1, into `~/.local/share/charcoal/chargen.bin`; Settings can do it too. bbmp's
and an installed VICE's copies are used if present.

## C64 BBS codes (Image BBS)

A `.seq` written with **Image BBS**'s £ codes (`£C1`-`£CO` colours, `£F1` clear, `£R1`/`£R0`
reverse, `££` a £) opens with them acting, as Image shows the file (found by its codes, or **Open as** *C64:
Image BBS £ codes*); its other codes (`£V2` the caller's name, `£S` speed, `£W` waits ...) stay on the page as
text, and Save (*PETSCII with Image BBS £ codes*) writes the file back with them, a £ that would start a code as
`££`. Plain PETSCII art draws with £ too: none of the 239 petscii-bbs screens or the big check's 627 C64 files
is taken for Image's.

## C64 beyond PETSCII: multicolour, ECM, own character sets

**C64 beyond PETSCII** (C64 and C128 40 columns, for graphics compos that take custom character sets): the
page's **Screen** gadget picks the VIC-II's text mode. *Multicolour*: a cell of colour 8-15 is drawn in pixel
pairs (00 the screen colour, 01 MC 1 = `$D022`, 10 MC 2 = `$D023`, 11 the cell's colour 0-7), colours 0-7 stay
hi-res. *Extended background (ECM)*: characters 0-63, each on one of four backgrounds (BG 0 = the screen colour,
BG 1-3 = `$D022`-`$D024`) chosen by the screen code's top two bits; *Paint on* picks the background typing and
drawing give, the Colour tool's right button paints it. Drawn as the VIC-II shows them (PNG/GIF too). A
**custom character set** is the page's own font: load a `.64c`/`.bin` or a **CharPad project** (`.ctm`,
versions 5-9: its character set, mode and colours), edit it in the font editor (in multicolour it draws pixel
pairs with three pens, keys 1-3). A CharPad project also opens as a page (its map, tiles laid out; Save as
writes a C64 program). Kept in: the `.prg` and `.d64` below, the ACME source and the BASIC listing; other
formats say so when saved and CharEdit remembers mode and set for the file.

## Plus/4 page

**Plus/4 page** (also the C16 and C116; the Plus/4 start tile, or Model: Plus/4): 40x25 like the C64, but
every cell has the TED's whole attribute: one of 16 hues in 8 luminances (121 colours, black has no
luminance) and **Flash** (the tool rail's toggle; flashing cells blink on the page). The screen colour and the
border are TED colours too; a new page has the Plus/4's power-on colours (black on white, a light blue
border). The Colours panel is the hue x luminance grid (left: the brush, right: the screen colour, middle:
the border; the wheel over it makes the brush lighter or darker, as in Marq's PETSCII editor); Ctrl+0-7 and
Ctrl+arrows change the hue and keep the luminance. Palettes: VICE's own (the default: VICE 3.9's xplus4
shows exactly these), YAPE's and Colodore's (VICE's `.vpl` files). The character set is the Plus/4 ROM's
(from VICE, like the other Commodore ROMs). Files: **`.prg`** (Marq's PETSCII editor's Plus/4 program: `RUN`
shows the page; checked in VICE's xplus4: its screenshot and CharEdit's PNG are identical, pixel for pixel,
border included), a **`.d64`** holding it, **`.scrn`** (TED Screen Editor's screen, with its `.proj` for the
size and colours, written next to it), Marq's **`.c`** (`PLUS4`), Petmate9's **`.petmate`** (`c16Upper` /
`c16Lower` screens), Marq's BASIC listing and ACME source (both also checked in xplus4), **`.seq`** as a Plus/4 prints it (the colour codes give the KERNAL's luminance of each
hue, FLASH ON / OFF are 0x82 / 0x84; the save says how many cells show another luminance), PNG / GIF / APNG
as VICE's screenshots (384x288, the screen at 32,40). Open as "Plus/4 PETSCII" reads any file as a Plus/4
prints it; changing a page between the C64 and the Plus/4 turns its colours into the nearest (Ctrl+Z
undoes it). **A character set of its own** (Load a font..., or the font editor; TED Screen Editor's `.chrs`,
`.64c`, `.bin` of 1024 bytes for 128 characters or 2048 for 256): the font editor changes a character and its
reverse together, as the TED shows them. It goes with the page in CharEdit's own Plus/4 `.prg` (and `.d64`:
the TED reads the set from RAM at `$2000`, 128 characters with the TED's own reverse or, when the reversed
half is drawn differently, all 256), in the `.scrn` (TEDSE's project: `NAME.chrs` next to it, 128 characters,
as TEDSE saves it) and in the BASIC listing (the set POKEd to `$E000`) and ACME source (the set at `$2000`);
all checked in VICE's xplus4, pixel for pixel.

## DirArt

**DirArt** (Model: DirArt): art made of a C64 disk's directory. `LOAD"$",8` and `LIST` show one line per file,
its 16-character name in quotes: the page is 16 columns wide, light blue on blue. A page opened from a disk has
**the disk's header line as its first row** (its 16-character name, which `LIST` shows reversed before the
files), then a row per file (up to 144); saved, the first row is the disk's name again. A page made in CharEdit
or opened from Marq's or Petmate9's files has no header row: every row is a file and the disk is named after
the saved file. Save as *C64 disk whose directory shows it* (`.d64`): each file row a DEL file of 0 blocks; the
disk ID and DOS type (`CE 2A` on a new disk) are kept from the disk the page came from. Characters a name cannot hold (the quote, and reversed characters other than the control codes: as Marq's
editor and Petmate9 mark them) become spaces, and the save says how many. A `.d64` without a screen program
of CharEdit's opens as a DirArt page (Open as "C64 DirArt" opens any disk's directory); saved again, it is
written over that disk: its programs, and the names that did not change, stay as they were. Marq's `DIRART`
`.c` files and Petmate9's `dirart` screens open on it. Checked in VICE: the listing shows the page.
