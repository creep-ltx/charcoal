# amiansi

An Amiga and PC ANSI/ASCII art viewer and editor, drawn with the real bitmap fonts: Topaz 1/1+/2/2+,
MicroKnight, MicroKnight+, P0T-NOoDLE and mO'sOul on an Amiga page (Latin-1), IBM VGA on a PC page (CP437).

    amiansi [FILE]        # opens its own window

A native program (Go + SDL2, one binary; needs only the SDL2 library that every Linux desktop has). The whole
interface is drawn in Topaz, kept dark and small so the art comes first; **Ctrl+H** hides every gadget.
Press the **?** gadget for every key.

## Build

    go build -o amiansi .     # needs Go and the SDL2 development files
    go test ./...             # the core: parser, writer, SAUCE

`~/.local/bin/amiansi` links to the binary here.

## What it does

- **Amiga page**: Latin-1, the 8-bit CSI byte 0x9B as well as `ESC [`, form feed clears the screen, LF line ends,
  italic (SGR 3) and underline (SGR 4) drawn like the Amiga console, bold as bright colours or as the console's
  smeared bold ("Bold smear"), 8×16 (line doubled) or 8×8.
- **PC page**: CP437 in the VGA font, CRLF line ends, blink or iCE colours, half-block painting, `.bin` files.
- Both: cursor movement, erase, insert/delete lines and characters, save/restore cursor, SyncTERM's font switch
  (`CSI 0;n SP D`), SAUCE read and written (width, font name, title, author, group, iCE flag). Amiga or PC is guessed
  from SAUCE and the bytes; "Open as" overrides. UTF-8 text files are read too.
- Bottom strip: tools, all 256 characters (click = brush), 5 F-key sets (F1–F10), 16 colours (left fg, right bg,
  Shift+click edits the colour).
- Tools: Type, Draw (Shift+click = line), Colour, Fill, Pick (or middle click), Select (copy/cut/paste/stamp), Half.
- Undo/redo, play at modem speed, save as ANSI, plain text, UTF-8, `.bin` or PNG; dropping a file on the window opens it.
- Settings in `~/.config/amiansi/prefs.json` (`"uiscale": 2` doubles the window for big screens).

## Files

| Path | What |
|---|---|
| `core.go` | document, ANSI parser, writer, SAUCE (no window; `core_test.go`) |
| `editor.go` | the editor's state and editing, loading, saving, playing |
| `view.go` | the main window: gadget bar, page, bottom strip, status line |
| `dialogs.go` | open/save, info, colour, help, yes/no requesters |
| `ui.go`, `gfx.go` | the gadgets and the pixel drawing |
| `fonts.go`, `fonts/` | the embedded fonts; `cp437.go` the CP437 table |
| `script.go` | `AMIANSI_SCRIPT` test steps, for checking it headless with `SDL_VIDEODRIVER=offscreen` |
| `web/` | the earlier browser version (Python server + HTML/JS), kept for reference |

## Fonts

The Amiga fonts are TrueSchool's *Multi Platform Amiga Fonts* raw conversions (dMG/t!s^dS!, GPL-FE,
github.com/rewtnull/amigafonts): 256 glyphs × 16 rows, every row of the 8×8 original doubled. The VGA font is
rendered from Px437_IBM_VGA_8x16 (VileR, *The Ultimate Oldschool PC Font Pack*, CC BY-SA 4.0); the licence is in
`fonts/`.
