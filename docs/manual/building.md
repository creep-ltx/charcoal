[Charcoal](../../README.md) · [Manual](README.md)

# Building and the source

## Building

Working on Charcoal: start with [docs/HANDOFF.md](../HANDOFF.md) (state, wishes, testing) and
[docs/roadmap.md](../roadmap.md).

    make                      # builds charedit; needs Go and the SDL2 development files
    sudo make install         # charedit, charview, menu entries and icon under /usr/local
    make test                 # the core: parser, writer, SAUCE

The SDL2 development files are `libsdl2-dev` (Debian, Ubuntu), `sdl2` (Arch) or `SDL2-devel` (Fedora).
`make install PREFIX=$HOME/.local` installs for one user without root (`~/.local/bin` must be on `PATH`);
`make uninstall` with the same `PREFIX` removes it again, and `DESTDIR` is for packagers. Install puts
`charedit` in `bin` with `charview` as a link to it, CharEdit in the desktop's application menu, and CharView
as an "Open with" choice for art files (it is not in the menu, since it needs a file to show). Plain
`go build -o charedit .` still works and leaves the program here.

Windows and Mac programs are built by GitHub Actions (`.github/workflows/build.yml`) on every push, with SDL2
linked in so each is one file: Windows with the official SDL2 MinGW build, the Mac from SDL2's source as one
program for Apple Silicon and Intel, packed as `Charcoal.app` by `tools/release/package.sh`. Each run tests on
all three systems and keeps headless screenshots as artifacts. A tag `v*` makes a release (a prerelease if the
tag has a "b"). The Windows program also builds here, with MinGW and SDL2's MinGW package:

    CGO_ENABLED=1 GOOS=windows CC=x86_64-w64-mingw32-gcc CGO_CFLAGS=-I$SDL/include \
      CGO_LDFLAGS="-L$SDL/lib -lSDL2 -lsetupapi -lwinmm -limm32 -lversion -lole32 -loleaut32 -luuid -lgdi32 -luser32 -lshell32 -lcfgmgr32" \
      go build -ldflags '-s -w -H windowsgui -extldflags "-static"' -o charedit.exe .

(`$SDL` = `SDL2-x.y.z/x86_64-w64-mingw32` from `SDL2-devel-x.y.z-mingw.tar.gz`; Wine runs it.)

## Tested

`bigcheck_test.go` (see `tools/bigcheck/`) runs thousands of real files: on 2026-10-13, 4119 files from 16colo.rs
(PC, Amiga, XBin, PETSCII-style and animation packs), C64 BBS `.seq` files, Petmate `.petmate`/`.c` files from
GitHub and the Break Into Chat ATASCII collection. All open, all with a known type get the right page type, all
come back cell for cell after saving in their own format.

The files that run on a machine were run in emulators (`emu_test.go`, `tools/emucheck/`, 2026-10-16): C64
`.prg`/`.d64` (every mode, own sets, both cases) in VICE, Atari `.xex`/`.atr` (38/40 columns, ROM and own font,
800XL and 800) in atari800, Spectrum `.tap` in Fuse, Amstrad CPC `.bin` on a `.dsk` (modes 0-2) in MAME. Each
emulator's screen is CharEdit's PNG export pixel for pixel, border included (the colours as each emulator's
palette has them). MSX cartridges (SCREEN 0, SCREEN 1, own fonts, and 80 columns on an MSX2) ran in MAME on
C-BIOS, and MZ-700 `.mzf` files in MAME's mz700 with a stand-in monitor that loads them (`msx_test.go`,
`mz_test.go`): the same, colours by RGB too.

Drawing sessions (`collab_test.go`, `tools/collab/`, 2026-10-16) were run against the real thing on this
computer: CharEdit joined Moebius's own server (node) serving 42 of Moebius's test files (those with a SAUCE
record and a PC font) and got each page as Moebius's reader made it; a client built on Moebius's own page code
(libtextmode) and IcyDraw's client joined CharEdit's session and read its page cell for cell; CharEdit joined IcyDraw's server; drawing,
resizing and chat went both ways in each.

## Files

| Path | What |
|---|---|
| `core.go` | document, ANSI parser, writer, SAUCE (no window; `core_test.go`) |
| `editor.go` | the editor's state and editing, loading, saving, playing |
| `view.go` | the main window: gadget bar, page, bottom strip, status line |
| `dialogs.go` | open/save, info, colour, help, yes/no requesters |
| `ui.go`, `gfx.go` | the gadgets and the pixel drawing |
| `fonts.go`, `fonts/` | the embedded fonts and their mirror tables; `cp437.go` the CP437 table |
| `fontfile.go`, `fontedit.go` | a page's own font: font files read and written, kept with the art; the font editor (`fontfile_test.go`); `tools/pcfonts.py` makes the built-in EGA/VGA fonts |
| `viewer.go` | CharView |
| `export.go` | `.prg`, the `.c` array, XBin, HTML (`export_test.go` runs the `.prg` in a small 6502 emulator) |
| `c64x.go`, `c64x_prg.go`, `c64x_d64.go`, `c64x_ctm.go` | C64 multicolour and ECM, CharEdit's own `.prg`, `.d64`, the ACME/BASIC exports with modes and sets, CharPad `.ctm` (`c64x_test.go` runs every program in a 6502) |
| `c64bbs.go`, `boardcolours.go` | the C64 boards' pauses (Image BBS, C*Base) and Image's £ codes read and written; the PC boards' own colour codes in their animated files (`c64bbs_test.go` with models of Image's and C*Base's display code) |
| `bbscodes.go` | BBS colour codes: pipe, Synchronet Ctrl-A, WWIV, Wildcat!, Celerity, read in front of the ANSI terminal and written (`bbscodes_test.go`; real files when fetched, see HANDOFF) |
| `boardswhole.go`, `brdcodes.go`, `dirarthead.go` | the whole-second boards (FAME, CNet, Image BBS's `£W`); animated files in a board's colour codes, Telegard's codes, Mystic's theme and Renegade's user colours; the DirArt header line (`brd_test.go`) |
| `editorformats.go` | other editors' files: iCEDraw, ArtWorx, TundraDraw, IcyDraw, REXPaint, Playscii, Marq's `.pet`/`.bas`/`.s`, Petmate JSON (`editorformats_test.go`; `editorformats_ext_test.go` checks against ansilove, IcyDraw's engine and Playscii's loader when they are there) |
| `fed.go` | the TheDraw font editor: a `.tdf` font in a tab, each letter a page, the letter strip; `.td` writing is in `othformats.go` (`fed_test.go`) |
| `icymach.go`, `psciown.go`, `othformats.go`, `petmate9.go` | IcyDraw's PETSCII/ATASCII/ST pages, Playscii's other sets and palettes, PabloDraw `.pd` / TheDraw `.td` / BSAVE `.bsv`, Petmate9's machines (`oth_test.go`) |
| `skypix.go`, `skyfonts.go` | SkyPix pages and the Amiga fonts they name (`oth_test.go`) |
| `atari.go` | ATASCII: the Atari fonts, `.ata` reader and writer, recognising and width guessing (`atari_test.go`) |
| `gtia.go`, `atari8.go`, `zxtap.go` | Atari GTIA colours and the colour registers; the Atari `.xex`/`.atr` and the OS ROM's font; the Spectrum `.tap` (`mach_test.go` runs the Atari program in a small 6502 with a model of ANTIC and compares its screen with the PNG export pixel for pixel) |
| `pet.go` | PETSCII: the C64 fonts, the `.seq` reader and writer, typing (`pet_test.go`) |
| `msx.go`, `mz.go` | MSX pages (screens, colours, the `.rom`/`.bin` programs, Playscii's msx charset) and Sharp MZ-700 pages (two character sets, the `.mzf`); `tools/msxmz-fonts.py` makes their fonts (`msx_test.go`, `mz_test.go` run the programs in a small Z80) |
| `tedchars.go`, `dirart.go` | a Plus/4 page's own character set (CharEdit's Plus/4 program, TEDSE's `.chrs`, the BASIC/ACME exports, the TED's reverse in the font editor); C64 DirArt (the 16-column page, the `.d64` directory read and written); `tools/mz700-set2.py` makes the MZ-700's second set from `tools/mz700-set2.txt` (`chs_test.go`; with `CHARCOAL_EMU=1` both run in VICE) |
| `minitel.go`, `minitel_ui.go`, `viewdata.go` | Minitel pages (the screen, a small Minitel that reads `.vdt`, the `.vdt` writer) and Viewdata pages (streams, 960-byte frames, Telstar frames); the editor's side of both (`minitel_test.go`, `viewdata_test.go`) |
| `keys.go` | the table of actions and their default keys, the keys file, the help's key list |
| `rowscols.go`, `blockjobs.go` | insert/delete rows and columns; fill, move and crop a block |
| `frames.go`, `strip.go`, `streams.go` | frames and their undo, the frame strip and onion skin, cutting streams into frames |
| `layers.go`, `layers_ui.go`, `layers_keep.go`, `layers_formats.go`, `layers_lock.go`, `layers_frames.go` | layers: putting them together, working on them and their undo, the panel, autosave and the file beside a flat file, the `.icy`/`.xp`/`.psci` layers, locked layers, layers with frames (`layers_test.go`, `lay2_test.go`) |
| `anim.go`, `apng.go` | GIF and APNG export |
| `animcost.go`, `animcost_ui.go` | animation detail levels, the cost of a saved animation (size, time at a speed, weight), Play's line timing, the Animate requester's cost, marks and preview at a speed |
| `autosave.go` | autosave and offering it back, `.bak` on save, recent files |
| `compo.go`, `stages.go` | Check for a party (compo rules as data, the check, Save for the party); the work's thinned history and Export work stages (`compo_test.go`) |
| `paths.go`, `keymods.go`, `platform_*.go` | per system: the settings and data folders, Cmd and AltGr, drives, is a CharEdit still running |
| `menus.go`, `layout.go`, `panel.go` | the menu bar (built from the actions in keys.go), the Studio and Focus layouts, the drawing gadgets both arrange |
| `theme.go`, `uifont.go` | the themes, the interface text through a face (Topaz, or TrueType / OpenType: the system's, the Go font, a file) |
| `script.go` | `CHARCOAL_SCRIPT` test steps, for checking it headless with `SDL_VIDEODRIVER=offscreen` |
| `web/` | the first version, called amiansi, that ran in the browser (Python server + HTML/JS), kept for reference |
