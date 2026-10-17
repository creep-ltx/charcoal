[Charcoal](../../README.md) · [Manual](README.md)

# Fonts and TheDraw fonts

## Fonts

![The font editor: the character ░ redrawn, the page beside it showing the change](../screenshots/fonteditor.png)

- **Built-in PC fonts**: IBM VGA, EGA 8x14, VGA 8x8 (80x50 pages), VGA with code page 850 and 866, from VileR's
  *Ultimate Oldschool PC Font Pack*. A SAUCE font name (`IBM VGA`, `IBM EGA`, `IBM VGA50`, `IBM EGA43`,
  `IBM VGA 850` ...) picks the font when a file opens; saving writes the font's name.
- **Your own font** for a page: the Font gadget's *Load a font...* (Ctrl+Shift+L), or drop the file on the window
  (it asks: font or art?). PC raw fonts (`.fnt .f08 .f14 .f16`: 256 characters of 8, 14 or 16 rows), PSF 1 and 2,
  an XBin's font; C64, VIC-20 and PET character sets (`.64c` with its load address, `.bin` of 2048 bytes a set or
  4096 for both; CharPad `.ctm` projects); Atari 8-bit `.fnt` (1024 bytes); ZX Spectrum sets (768 bytes, codes 32-127) and UDGs
  (168 bytes). The font is named after its file.
- **Kept with the art**: XBin holds the font inside, a Petmate workspace as its custom font (Petmate reads it),
  CharEdit's C64 `.prg`/`.d64` and the ACME and BASIC exports in the program. Formats that cannot hold a font
  (ANSI, `.seq`, `.ata`, `.scr` ...) say so when saved, and CharEdit
  remembers the font for that file: it comes back when the file opens again. Autosaves keep it too.
- **Font editor** (Ctrl+Shift+E, or *Edit the font...*): the brush character's pixels in a grid. Click or drag
  sets pixels (a set pixel clicked clears), right click clears; Invert, Flip, Move (Shift+arrows), Clear, Copy,
  Paste, Undo; arrows or the character map pick another character. The page shows every change at once. Editing a
  built-in font makes a copy, the page's own. *Save font...* writes it as `.psf`, `.f08/.f14/.f16`, `.64c`, `.fnt`,
  `.ch8` or `.udg`.

## TheDraw fonts (logos)

TheDraw fonts (`.tdf`) are the big letters ANSI artists type logos with; thousands exist, many in one file.
The **Logo tool** (Alt+G, the Logo button, Draw > Logo) types in one on PC, Amiga and Unicode pages:

- **Typing** puts the font's letter at the cursor and moves on past it (plus the font's spacing, or one you
  choose under the brush). Each letter is one undo step: **Backspace** takes the last letter back, Ctrl+Z works
  as always. **Return** goes to the next line of letters, under the first one typed. A letter that does not fit
  goes to the next line by itself. A click puts the cursor somewhere else. Most fonts have capitals only:
  small letters use them (and the other way round). A letter the font lacks is named in the status line.
- **The three kinds**: *Block* fonts draw in the brush's colours, *Colour* fonts keep their own colours,
  *Outline* fonts are drawn in one of TheDraw's 19 outline styles (single and double lines, half blocks: the
  Outline choice under the brush shows its corners).
- **The font list** (Alt+Shift+G, or the font's name under the brush): every font with a preview of your text
  in it, as the page will show it; Find narrows the list by name, file or kind. *Add fonts...* takes a `.tdf`,
  a `.zip` of them or a whole folder; opening or dropping a `.tdf` adds it too. *Download...* fetches Roy/SAC's
  collection (3,536 fonts, 3 MB, from roysac.com) into CharEdit's data folder: fonts are other people's art,
  so none come with the program.
- **A whole line** (Ctrl+Shift+G, *Line...*): type the text in a requester with a preview, *Stamp* hands it
  over as a block; a click puts it down (Shift: without the blanks), as many times as you like, Esc stops.
- **Amiga pages** show Latin-1, which has no block or line characters: only 118 of Roy/SAC's fonts can be
  typed there (the list shows those unless you tick *Also fonts an Amiga page cannot show*); a letter needing a
  character the page lacks is refused, not changed. **Unicode pages** take every font (CP437 to Unicode).
- **Spacing** is counted as TheDraw counts it: a font whose file says 2 puts one column between its letters
  (TheDraw's own font editor shows it as "1 character"). Earlier versions put one column too many.

### Making and changing TheDraw fonts

Draw > **New TheDraw font...** (name, Block / Colour / Outline, spacing) or **Edit a TheDraw font...** (the font
list: pick one, *Edit*) opens the font in a tab of its own. Each letter is a small PC page: draw it with every
tool, W and H set its width and height (1 to 255 columns). The **letter strip** under the page shows `!` to `~`
in two rows, as TheDraw's TDFONTS does: letters in the font in boxes, the others faint; a click goes to a letter
(one not yet in the font starts blank, the size of the last one, and joins the font once something is drawn on
it). Each letter keeps its own undo. On the strip's last row: the font's name (12 characters), its kind, its
spacing, the letter's size, its **descender lines** (the last lines of g, j, p, q, y that hang below: TheDraw
leaves them out when it lines letters up), *Copy to* (then click the letter to copy to) and *Delete*. At the
right, the letter as the Logo tool will type it (outline fonts in the Logo tool's outline style).

- Block fonts keep characters only (they are typed in the brush's colours); colour fonts keep each cell's
  colours. A space is see-through (a space on black, in a colour font): the page below shows; TheDraw's solid
  space is character 255. Outline fonts are drawn with the letters A-P and @, which become the outline style's
  lines when typed (TheDraw's manual, "Editing outline type fonts").
- **Ctrl+S** writes the `.tdf` in place. A font from a file of several goes back into that file, in its place,
  the other fonts byte for byte as they are on disk at that moment (so two tabs editing two fonts of one file
  both keep their changes; if the file changed so that the font is no longer where it was, the file is left
  alone and the font is saved as a file of its own, with a message); only a font from a `.zip` (or a file CharEdit may not write) is
  saved as a file of its own, `<name>.tdf`, and the status line says so. Letters you did not change are written
  byte for byte as they were (letters that shared their bytes with others in the old file each get their own).
  Character 13 and an `&` at a block letter's right edge cannot be stored (TheDraw would read a line's end):
  the save says how many became spaces.
- Checked: every letter of Roy/SAC's 3,547 fonts written anew draws the same; TheDraw 4.63's TDFONTS opens
  CharEdit's fonts; IcyDraw's font library reads them letter for letter.
