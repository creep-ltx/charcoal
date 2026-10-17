[Charcoal](../../README.md) · [Manual](README.md)

# Teletext, Viewdata and Minitel pages

## Teletext page

40x25 cells of 7-bit codes in 12x20 cells like the SAA5050's; attributes (text/graphics
colour, double height, flash, conceal, separated, hold, new/black background) act from their cell on, read
left to right as the chip reads a row (set-at / set-after as in ETS 300 706 and edit.tf). **Codes** shows the
attributes' names; the **Sixel** tool paints a cell's 2x3 graphics pixels. Opens and saves `.tti`, `.ep1`,
edit.tf links (`.url`; any text with an edit.tf link opens), opens BBC Mode 7 dumps (1000-byte `.bin`). Font:
Bedstead (public domain), 5x9 dots doubled with the chip's character rounding (`tools/teletext-font.py`).

## Viewdata page

**Viewdata page** (VD): a teletext page of 24 rows, as Prestel and today's Viewdata boards (Telstar, CCl4,
NXtel) send it: the same codes, attributes, Codes view and Sixel tool. Files: a **stream** (`.cpt`, as the
Amiga's vtx datatype names it: CS, the rows with trailing spaces left out and CR LF, attributes as ESC and a
letter; a `.car` carousel or any stream with several CS opens as frames), a **960-byte frame** (`.bin`;
attributes read as 0x00-0x1F, 0x80-0x9F or MacViewdata's 0xC0-0xDF, written as 0x80-0x9F so no CS byte
appears) and a **Telstar frame** (`.json`: the page in `content` as rawV; rawT and edit.tf content are read;
one read from Telstar keeps every other field when saved, a new one is numbered by its name, `200a.json` is
page 200 frame a). Teletext files open as Viewdata with Open as; a Viewdata page also saves as `.tti` and an
edit.tf link. **Play** sends the stream to a Viewdata terminal on screen.

## Minitel page

**Minitel page** (MT): the Minitel's 40x25 screen (row 0 is its status row) in 8x10 cells, as the STUM 1B
(the Minitel's specification) describes it. A cell is a letter (G0, the G2 signs £ § ← ↑ → ↓ ° ± ÷ ¼ ½ ¾ Œ œ ß
and the accented letters à â ä è é ê ë î ï ô ö ù û ü ç) in a colour, or a mosaic of 2x3 blocks (the character
table's last quarter, the **Sixel** tool) in a colour on a background. A letter shows the background,
underline and concealing of its zone: a zone starts at a space with other zone attributes (type a space with
a background to start one) or at a mosaic, as on the Minitel. The tool rail's toggles go with what you type:
**Flash**, **Inv** (inverse), **Line** (underline; separated blocks on mosaics), **Mask** (conceal), the size
(**1x**, **Wide**, **Tall**, **2x**: big letters are written from their bottom row, the cursor stays there) and
**Reveal** (show concealed cells). Palettes: the Minitel 1's grey levels (the default) or colour. Files:
**`.vdt`**, the Videotex stream MiniPavi, MiEdit and the 3615 servers keep pages in: read by running it on a
small Minitel (screens a clear ends become frames), written so a Minitel shows the page (FF, the rows with REP
for runs, the status row last; the stream is run on the same Minitel as it is made and checked cell by cell).
**Play** (m) sends it to a Minitel on screen at the chosen speed. 149 `.vdt` pages from MiniPavi's examples,
MiEdit-era servers and VDT2BMP's samples open and come back cell for cell after saving. The font is Frédéric
Bisson's Minitel font (CC0), `fonts/minitel-NOTICE.txt`.
