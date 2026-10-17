[Charcoal](../../README.md) · [Manual](README.md)

# ZX Spectrum, Amstrad CPC, MSX and Sharp MZ-700 pages

## ZX Spectrum page

The 32x24 screen, ink/paper/bright/flash per cell, the 48K ROM's font (built in: Amstrad
allows passing the ROM on, `fonts/zx-spectrum-NOTICE.txt`) with the block graphics and UDGs. Saved as `.scr`
(6912 bytes); a `.scr` opens with each 8x8 cell matched to a character (drawings get the nearest one).
**`.tap`**: a tape with a BASIC loader (`10 BORDER b: LOAD ""SCREEN$` / `20 PAUSE 0: GO TO 20`) and the screen,
so `LOAD ""` on a Spectrum or in any emulator shows the page in its border; a `.tap` opens with its first
SCREEN$ (any game's loading screen too).

## Amstrad CPC page (AMSCII)

**Amstrad CPC page** (AMSCII): the CPC's text screen in its three modes (the **Mode** gadget): mode 1, 40x25
with 4 inks (the AMSCII compos' main category), mode 0, 20x25 with 16 inks (wide pixels), mode 2, 80x25 with 2
inks (narrow pixels: cells 8x16). Each cell has a character of the ROM's 256 (codes 0-31 are glyphs too) with a
pen and a paper; each ink is one of the CPC's 27 colours. The Colours panel shows the mode's inks (left: pen,
right: paper) and the 27 colours (left: the pen's ink becomes that colour, right: the paper's ink, middle: the
border). The font is the CPC firmware ROM's, built in (Amstrad allows passing it on,
`fonts/amstrad-cpc-NOTICE.txt`). Saved as a `.bin` that runs on a CPC (`RUN"NAME.BIN"`) and in emulators
(drop it on Caprice Forever): the AMSDOS header, a 21-byte program and the firmware's text codes (mode, border,
inks, pen and paper per cell); as a `.dsk` disk image holding that `.bin`; and Playscii `.psci` with Logiker's
Amstrad ASCII Exporter's charsets (`CPC_Mode0/1/2`) and `cpc` palette. Opens the exporter's own `.bin` files,
`.dsk` images (the first art program on the disk) and its `.psci`; Open as Amstrad CPC reads any file as the
CPC's text codes. All 2024 AMSCII Compo entries open. Pictures (`.png`, GIF, APNG) are 768x544 in every mode,
the size and place of Caprice Forever's screenshots that the compos use, so a saved PNG matches the other
entries' pixel for pixel. Typing `^` gives the caret (0xA0; the CPC's 0x5E is an up arrow).

## MSX page

The MSX's text screens (the **Screen** gadget): SCREEN 0, 40x24 characters of 6x8 pixels in two
colours for the whole screen; SCREEN 1, 32x24 of 8x8 with a colour pair for each group of 8 characters (0-7,
8-15, ...) and a border; the MSX2's SCREEN 0 WIDTH 80 (cells 6x16: its pixels are half as wide). A cell is a
character only: the colours are the screen's, as on the machine, so in SCREEN 1 a typed letter takes its
group's colours. Colours (the TMS9918's 15 and transparent, which shows the border): left click is the
foreground, right click the background, of the screen in SCREEN 0 or of the brush character's group in
SCREEN 1; middle click the border (SCREEN 1). Palettes: MSX1 (TMS9918, as MAME has it) and MSX2 (V9938). The
font is C-BIOS's (international, Japanese, Brazilian; BSD licence, `fonts/msx-cbios-NOTICE.txt`), since
Microsoft's MSX BIOS may not be passed on; put an MSX BIOS ROM in CharEdit's data folder as `msx.rom` and
its font is offered too ("MSX (BIOS ROM)"). An own font (2048 bytes, or the font editor) goes into the files.
Saved as a **cartridge** (`.rom`, 16 KB: runs on any MSX and in emulators on C-BIOS) or a **BLOAD file**
(`.bin`: `BLOAD"ART.BIN",R` from MSX BASIC; a key goes back to BASIC): a small program that sets the screen up
through the BIOS (ERAFNK, INITXT/INIT32, CHGCLR) and copies the page, the colour table and an own font into
the VRAM (LDIRVM). Both open again. Playscii `.psci` with its `msx` charset and palette is read and written.
Pictures are openMSX's size: 320x240 (640x480 at 80 columns), the backdrop round the screen. Open as MSX
reads text (code 1 + a letter for the characters 0-31, as MSX BASIC prints them).

## Sharp MZ-700 page

**Sharp MZ-700 page** (MZ): the MZ-700's 40x25 screen, each cell a character of either of its two sets with a
foreground and a background of the 8 colours. The character table shows both sets (512 characters; the
second from 256 on); typing gives the first set (Sharp's display codes: letters, digits, lower case). The
font: Sharp's character ROM may not be passed on, so the first set is Russell Marks's public domain
look-alike (`fonts/mz700-NOTICE.txt`) and the second set is CharEdit's own look-alike, drawn for it from the
character table (CC0, `fonts/mz700-set2-NOTICE.txt`; real MZ art such as "Princess" looks right); with Sharp's
ROM (`mz700fon.int`, 4096 bytes) in CharEdit's data folder or MAME's `roms` (`mz700.zip`), CharEdit draws
both sets with it ("MZ-700 (Sharp ROM)"). Saved as a **tape file** (`.mzf`: load it with the monitor's `L`,
or in an emulator): a program at $1200 that copies the display codes to $D000 and the attributes to $D800.
Opens CharEdit's `.mzf` and other `.mzf` pictures that copy two 1000-byte blocks there (as the Demodulation
2021 entry "Princess" does). Pictures are the screen, 320x200. UTF-8 text keeps the second set's characters
(Unicode's Symbols for Legacy Computing). Open as Sharp MZ-700 reads text.
