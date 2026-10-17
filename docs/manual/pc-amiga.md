[Charcoal](../../README.md) · [Manual](README.md)

# PC and Amiga pages, BBS codes and SkyPix

## Amiga page

Latin-1, the 8-bit CSI byte 0x9B as well as `ESC [`, form feed clears the screen, LF line ends,
italic (SGR 3) and underline (SGR 4) drawn like the Amiga console, bold as bright colours or as the console's
smeared bold ("Bold smear"), 8×16 (line doubled) or 8×8.

## PC page

CP437 in the VGA font, CRLF line ends, blink or iCE colours, half-block painting, `.bin` files.

## Amiga and PC pages alike

Cursor movement, erase, insert/delete lines and characters, save/restore cursor, SyncTERM's font switch
(`CSI 0;n SP D`), SAUCE read and written (width, font name, title, author, group, iCE flag). Amiga or PC is guessed
from SAUCE and the bytes; "Open as" overrides. UTF-8 files are read too, and Save writes them in UTF-8 again
("ANSI in UTF-8, for terminals": every row in full and ended by LF, no SAUCE record, as Moebius's UTF-8 export
and durdraw write them), so a terminal artist's file stays readable with `cat`.

## PC BBS formats

PCBoard `@X` (.pcb) and Avatar (.avt) open and save (a file with both `@X00` and `@XFF` uses
them as PCBoard does, to keep a colour and bring it back; alone each is a colour, as art editors wrote them; a
save never writes both); iCEDraw (.idf), ArtWorx (.adf) and
TundraDraw (.tnd) open with their own fonts and colours and save (see [Files for other editors](formats.md#files-for-other-editors)). The same art
in five formats (ansilove's test files, `testdata/alove`) gives the same page in all of them. PC attribute
bytes are in BIOS colour order and are converted.

## BBS colour codes

**BBS colour codes** (PC pages, open and save): the codes boards turn into ANSI for their callers, as sysops
keep menus and screens in them. Found by their bytes (and `.msg` / `.bbs` endings), also in files that mix
them with ANSI escapes (common on Mystic); **Open as** picks one for any file, **Save as** writes each with the
shortest codes. A file saves back in its codes, also one with ANSI escapes too (Save as has ANSI). Pipe codes
write bright backgrounds and blink as `|24`-`|31` (WWIV, Synchronet and IcyDraw show them, Mystic 1.10 and
Renegade do not: the save says how many cells), a `|` as Mystic's `|PI`. A board's code CharEdit cannot show (a pause, the caller's name, `|PO`, Ctrl-A `P`) stays on the
page as text and is written back as it came, so it still works on the board. Colours a sysop or caller sets
up (Mystic's theme colours `|T0`-`|T9`; Renegade's `^0`-`^9` with Open as *PC: Renegade pipe codes*, since Mystic
prints them) show in the boards' default colours and are written back as those codes for the cells drawn in
them (a cell given another colour gets an ordinary code).

| Format | Codes | Read as |
|---|---|---|
| Pipe codes (`.ans`) | `\|00`-`\|15` foreground, `\|16`-`\|23` background, `\|24`-`\|31` bright background / blink, `\|CL` clear, `\|PI` a `\|` | Mystic, Renegade, WWIV 5, Synchronet, IcyDraw's `.an1` (a `\|` that could join a code is written `\|PI`, Mystic's way) |
| Synchronet Ctrl-A (`.msg`) | Ctrl-A + `K R G Y B M C W`, `H` bright, `I` blink, `E` bright background, `0`-`7` background, `N` normal, `+ - _`, `L` clear, cursor moves | Synchronet's `ctrl_a()`; a `☺` is written Ctrl-A `A` |
| WWIV (`.msg`) | Ctrl-C (♥) + `0`-`9` (each caller's colours: the defaults are read), `\|nn`, `\|#n` | WWIV 5's `outstr`; written with `\|nn` only, since hearts differ per caller (a ♥ in the art becomes a space) |
| Wildcat! (`.bbs`) | `@BF@`, background and foreground as capital hex digits, `@CLS@` | Synchronet's Wildcat! mode |
| Celerity (`.ans`, Open as only) | `\|k \|b \|g \|c \|r \|m \|y \|w`, `\|d \|B ... \|W` bright, `\|S` swap | Synchronet's Celerity mode (no blink: it is dropped, and the save says so) |
| Renegade pipe codes (`.ans`, Open as only) | `\|00`-`\|23` as pipe codes (`\|24`-`\|99` show nothing), `^0`-`^9` the caller's colours | Renegade's own reading (no blink: dropped, and the save says so) |
| Telegard (`.msg`) | `` ` `` + the attribute as two capital hex digits (`` `0E `` yellow, `` `1F `` white on blue) | Telegard 3's sample screens (its `~XX` codes stay text) |

## SkyPix

**SkyPix** (`.spx`, or an `.ans` with SkyPix commands; the Amiga BBS graphics of SkyPix and JR-Comm): opens
on a PC page of 80x25 in the 8x8 font with the drawing (lines, fills, rectangles, ellipses, brushes, palettes,
text placed in pixels, the Amiga fonts it names) as a picture under the cells, as IcyDraw's engine draws it
(pixel for pixel on 9 of its 10 test files). Type over it; saving keeps the file as it came and adds the typed
cells after it. The Amiga fonts are fetched on first use (Not now: the text shows in the VGA's 8x8). Open as
"SkyPix (Amiga)" reads any file so.
