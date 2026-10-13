# Charcoal: where things stand (start here)

Charcoal = **CharEdit** (editor, `charedit`) + **CharView** (viewer, `charview`), one Go + SDL2 binary
(`go build -o charedit .`; `~/.local/bin/charedit` and `charview` link to it; the viewer is chosen by the
program's name or `--view`). History: first a browser app called amiansi (kept in `web/`), then CharCool,
renamed by the user on 2026-10-13. Local git only, no remote yet.

## The user's wishes (keep them)

- The interface stays quiet and small; the art comes first. Ctrl+H hides all gadgets.
- Pages for a machine's screen keep its width (PETSCII per machine, Atari 38/40, ST per resolution, Spectrum
  32x24, teletext 40x25); only Amiga and PC pages have a free width (W).
- A page has a real height (H): files open exactly as tall as they are, empty lines at the end included, the
  page end is visible (dark area below), every row is saved. Enter/Down past the last row adds a row.
- Commodore .prg: only Petmate's form is read/written (the user agreed general .prg needs no emulator).
- No picture import (PNG/HTML -> characters) for now.

## Page types and formats (all done, all tested)

| Page | Files |
|---|---|
| Amiga ANSI | .ans .asc .txt .nfo .diz, XBin, PNG/HTML; Amiga or PC guessed by learned byte weights (core.go) |
| PC ANSI | + .bin (BIOS colour order!), XBin, PCBoard .pcb, Avatar .avt; opens iCEDraw .idf, ArtWorx .adf, Tundra .tnd |
| Atari (ATASCII) | .ata (width 38/40 guessed); AltirraOS font built in |
| Atari ST (VT52) | .vt; low/medium/high; EmuTOS font downloaded on first use (GPL) |
| PETSCII | PET 40/80, VIC-20, C64, C128 40/80; .seq .c .petmate .prg (Petmate form); ROMs from VICE on first use |
| ZX Spectrum | .scr (6912 bytes); 48K ROM font built in (Amstrad allows passing it on) |
| Teletext | .tti .ep1, edit.tf links (.url), Mode 7 dumps; Bedstead font (CC0) built in |

CharView: n/p/arrows, q/Esc, a auto-scroll (smooth/step with s, speed [ ]), wide art shrunk to fit, m plays.

## Testing

- `go test ./...` (unit tests; `testdata/alove` = the same art in five PC formats, must give the same page).
  **Tests that reach savePrefs (writeOut, save) must set XDG_CONFIG_HOME to a temp folder**: on 2026-10-13
  one such test overwrote the user's real ~/.config/charcoal/prefs.json.
- `FuzzParsers` (fuzz_test.go): every reader, any length; fails on a page over maxPageCells or a slow input.
  Only under a memory cap: `systemd-run --user --scope -q -p MemoryMax=6G go test -run XXX -fuzz FuzzParsers
  -fuzztime 120s -parallel 4 .`
- Pages never grow past `maxRowsFor(cols)` (core.go): 100000 rows or 16M cells. The audit and its fixes:
  docs/audit-2026-10-13.md.
- The big check: `tools/bigcheck/README.md`. Files live in `/tmp/bigcheck/files` (lost on reboot; the fetch
  scripts there get them again). Last run 2026-10-13: 8196 files, only "issues" = 24 asciiarena.se pieces drawn
  with PC characters (fair as PC) and one test .prg.
- Headless screenshots: `SDL_VIDEODRIVER=offscreen XDG_CONFIG_HOME=/tmp/x CHARCOAL_SCRIPT="click x y; key ctrl+S;
  text abc; wait 300; shot /tmp/a.png; quit" ./charedit file` (script.go). Never on the user's display.
- Sources: 16colo.rs packs and tags, asciiarena.se (sitemap; files at /collections/<name>/<file>),
  ~/Amiga/BBS/BBS Style (412 files), NAS /mnt/nas/home/Unsorted/Junk/Latex/Petscii* (with the PETSCII editor's
  own PNGs: rendering is pixel-identical). **Not csdb.dk and not zxart.ee files: their robots.txt forbid it.**

## Next: Windows and Mac (the user asked for it after teletext)

Plan agreed in chat (2026-10-13), nothing built yet:
- go-sdl2 `-tags static` to link SDL2 in: one file per system.
- Settings/data dirs: `os.UserConfigDir` / per-OS data dir instead of ~/.config and ~/.local/share (prefs.go,
  pet.go dataDir, st.go stFontPath).
- CharView on Windows: a second .exe or `charedit --view` shortcut (no symlink habit there).
- File requester: drive letters on Windows, home/Volumes on Mac.
- Mac: Cmd instead of Ctrl for shortcuts; Retina (SDL_WINDOW_ALLOW_HIGHDPI and renderer output size); an .app.
- Builds: Windows from Linux with MinGW, tested under Wine; Mac only on GitHub's macOS runners (no Mac here) -
  run the headless script there and look at screenshot artifacts. See ~/Projects/Knowledge/go/
  cgo-windows-and-memory.md and ~/Projects/Knowledge/macos/.
- Unsigned programs warn (Gatekeeper / SmartScreen): tell the user.
- Needs a GitHub (or NAS) remote first: ask the user where the repository should live.

## Where the knowledge is

- ~/Projects/Knowledge/amiga/ansi-art-real-world-traps.md: sources, robots rules, all the format traps
  (0x1A in art, .bin width, BIOS colour order, page height, Amiga vs PC, ATASCII, teletext, Petmate .prg).
- ~/Projects/Knowledge/bbs/petscii-seq.md: PETSCII/.seq, C64 screens, ATASCII and ST notes.
- ~/Projects/Knowledge/go/sdl2-desktop-gui.md: the GUI approach and headless testing.

Last updated 2026-10-13 (after the audit fixes).
