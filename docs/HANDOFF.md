# Charcoal: where things stand (start here)

Charcoal = **CharEdit** (editor, `charedit`) + **CharView** (viewer, `charview`), one Go + SDL2 binary
(`go build -o charedit .`; `~/.local/bin/charedit` and `charview` link to it; the viewer is chosen by the
program's name or `--view`). History: first a browser app called amiansi (kept in `web/`), then CharCool,
renamed by the user on 2026-10-13. Public at github.com/creep-ltx/charcoal (remote origin, branch main).

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
| Teletext | .tti .ep1, edit.tf links (.url), Mode 7 dumps; Bedstead font (CC0) built in; a .tti carousel opens as frames (one per subpage) and saving rewrites only changed subpages' OL 0-24 rows, every other line kept (teletext.go writeTTIFrames) |

CharView: n/p/arrows, q/Esc, a auto-scroll (smooth/step with s, speed [ ]), wide art shrunk to fit, m plays.

## Frames and animation (2026-10-14, 0.1b3 + roadmap 2.1; plan: docs/frames-design.md)

- `frames.go`: `Tab.frames []Frame` (doc, hold ms, name, lower, seg (.tti subpage text), src (.petmate framebuf)),
  `Tab.cur`; nil = one picture (t.doc). **t.doc is always the live current frame; frames[cur].doc is stale**;
  frames that are not current are never changed in place (goFrame clones), so undo snapshots share them.
  Undo: one stack; `Snap.whole` steps keep the frame list (new, copy, delete, move, hold, width/crop of every
  frame, split), cell steps keep `cur` and undoing one jumps to its frame; budget by `Snap.cost`.
- Width (W, machine, ST resolution) and crop change every frame; height (H) is each frame's own.
- Files: `.c`/`.petmate` screens and `.tti` subpages open as frames (reparse builds them); `writeMarqFrames`
  (Petmate's form: frame0000.., one META), `writePetmateFrames` (keeps unknown fields per framebuf and top level,
  customFonts, framebufUIStates reordered), `writeTTIFrames` (ttiHead; changed hold writes CT,n,T; new/moved
  frames renumber PN/SC; `restructured`). Unchanged .c/.petmate save = the source bytes. Petmate9 and Marq store
  no per-screen delay (checked 2026-10-14). ANSI/SEQ/ATA/VT52: `streams.go` `splitStream` cuts at clears (or
  cursor homes: `beforeHome` hooks in Term/PetTerm), hold = bytes x 10 / baud; offered on the strip (Split), never
  automatic; saved as one stream with a clear between frames. Formats of one picture write the current frame as an
  export (page stays dirty). GIF (`anim.go`, image/gif) and APNG (`apng.go`, by hand) export every frame.
- UI (`strip.go`): the strip under the page (boxes, drag to move, Hold field, New/Copy/Delete/Onion/Play/Loop,
  speed), shown with several frames or Ctrl+Shift+F; onion skin (Alt+O) = the frame before at ~25% under empty
  cells; Ctrl+P plays (Play.frames, `playStepAt(now)`); keys in keys.go's Frames block.
- Autosave keeps every frame (`autosaveMeta.Frames`, cells one after another); 0.1b3's one-subpage/one-screen
  autosaves come back as the file's frames.
- Not done (design step 11): delta stream writing, CharView playing frames, onion of the next frame, timed capture
  of scrolling ANSImations.

## Reference picture, shapes and brushes (2026-10-14, roadmap 2.2 and 2.4)

- `refimage.go`: `Tab.ref *RefImage` (pixels ARGB, x/y in font pixels of the page at zoom 1, scale = page pixels
  per picture pixel, opacity %, hidden, over, grey). Pictures: image/png, jpeg, gif + golang.org/x/image bmp and
  webp (v0.30.0: later versions want Go 1.26; BSD, `third_party/x-image-LICENSE.txt`); over 16 M pixels refused.
  `drawRef` (called in `drawPage` before the onion) blends it into the cells `onionBlank` calls empty, or into
  all with `over`; per-column/row lookup tables, nearest neighbour at screen resolution. Exports never see it
  (`renderDoc` draws cells only; TestRefNeverExported). `openPath` asks "use as reference?" for a picture
  (`isPicture` by magic) before opening a tab; the Ref gadget's pop-up (`refMenu`) and `FileReq.pick` open one.
  Remembered: `prefs.Refs` (absolute art path -> `RefPref`, the 200 most recently used), written on every change
  and on Save as (`writeOut` calls `rememberRef`); `autosaveMeta.Ref`. Move mode (`a.refMove`): `refKey` takes the
  arrows / + / - / Return / Esc before the key table, typing is held; Ctrl+drag moves it any time (`refPress`).
- `shapes.go`: `drawOpts` (embedded in App: shape, filled, style, mirror bits, shade, paintOnly, drag state).
  `shapePoints` (line via `line`, box with box-piece kinds, ellipse = Zingl's rectangle algorithm with the tip
  loop `<= b`, filled rows), `symPoints` (mirror around the centre, a point on the centre line once),
  `mirrorCell` (Font.mapped), `putCell` (colour/char only; on PETSCII/Atari pages bit 7 = reverse counts as
  colour). A shape is one undo step: pressed = `pushUndo`, each motion copies the undo snapshot's cells back and
  draws again. Half blocks / sixels use `halfSym` / `sixelSym` (the Half and Sixel tools mirror too). Shade:
  `shadeSteps` per mode (PC 32 B0 B1 B2 DB; C64 screen codes 32 66 A0; Atari/ZX/Amiga/ST ASCII ramps; none on
  teletext), `prefs.Shades` overrides; `strokeSeen` makes a stroke step each cell once. Box pieces: PC CP437
  single/double, C64 screen codes 40 5D 70 6E 6D 7D, ATASCII 12 7C 11 05 1A 03 (all checked on screen).
- UI: one Shape button (tools pane), the options under the brush (`drawToolOptions`, where the block buttons go
  while a block is selected), the Ref button in the bar. Keys: two blocks in keys.go (Alt+Shift+L/R/O/F/B/S/H/V/K/C;
  Ctrl+Shift+I/R/M, Ctrl+[ ], Ctrl+Alt+= -, the rest without a default). Script: `drag x0 y0 x1 y1 shift`, `drop PATH`.
- Not done: Marq's left/right Shift for colour/character only, PETSCII rounded corners (upper set only), shapes
  on the Colour tool, a reference per frame.

## Keys and safety (2026-10-13, evening)

- `keys.go`: one table of actions (id, default keys, help line, what it does) drives every shortcut, the help's
  KEYS list (`keysHelp`) and the keys file (`~/.config/charcoal/keys` on Linux) (written with the defaults the first time; bad lines are
  skipped with a status-line message; a key the file gives an action is taken from the others' defaults). Status
  hints name keys with `keyName(id)` so they follow the file. Only the left Alt is Alt (AltGr types).
- Defaults follow Moebius/PabloDraw/TheDraw (survey: ~/Projects/Knowledge/bbs/text-art-editor-keys.md): Ctrl+arrows
  colours, Ctrl/Alt+0-7, Ctrl+F1-F10 sets, Alt+arrows rows/columns, Alt+U, Ctrl+D, Ctrl+Shift+X, Ctrl+K, Ctrl+I;
  tools Alt+T/D/K/L/P/S/B. Fit width moved to Ctrl+9. Alt+F fills the selection (no selection: Fill tool).
- Autosave (`autosave.go`): every 2 minutes, dirty tabs to `<data dir>/autosave/<id>.cells` (appDataDir) (raw cells,
  lossless) + `<id>.json` (path, mode, machine, font, info, pid). Dropped on save, on closing a tab, on a clean
  quit. At start, autosaves of CharEdits no longer running are offered back. An XBin's own font is not kept (the
  page falls back to the mode's font).
- `.bak` on saving over a file (`prefs.no_backup` switches it off; Settings). Recent files: `prefs.recent`.

## Start screen (2026-10-14)

- `start.go`: CharEdit started with no files (`startShownAtLaunch`) shows it in the page area of the empty
  tab (`Tab.start`; drawn by `drawPage` while the tab is `untouched()`). Tiles (`startTiles`): each draws an
  empty page of its type made by `makeTab` (the same the pick uses) with the machine's name through
  `drawCell`, and its colours; PET/C128 follow the last machine (40 or 80), ST the last resolution. Picking
  (`pickTile`) swaps the empty tab for that new page (an empty Spectrum page is not blank to `usedRows`, so
  `setMode` would keep its size); ROMs / EmuTOS go through the download question first. Typing, a tool, a
  page type gadget, F1-F10, Backspace/Delete or Esc let it go; Ctrl+T/N/+ show it in the new tab,
  Ctrl+Shift+N (`start_screen`) brings it back. `prefs.no_start_screen` = the "Show this at start" box off.
- Screenshot for the README: `docs/screenshots/start.png` (1320x900, recent files faked with HOME=/tmp/fh).

## Testing

- `go test ./...` (unit tests; `testApp` (editor_fixes_test.go) gives temp XDG_CONFIG_HOME and XDG_DATA_HOME and
  the default keys; `testdata/alove` = the same art in five PC formats, must give the same page).
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
- Big check 2026-10-14 (frames): every multi-screen .c/.petmate/.tti round-trips every frame (69 files, 4323
  frames); 235 stream files split at their clears (1271 frames) and come back frame for frame, but 3 (WWANS404/405,
  KS-CRI.ANS) with text after a DOS end-of-file 0x1A and no SAUCE: the 0x1A shows as a glyph and an ANSI
  without SAUCE writes it as a space (as for any page; not new with frames).
- Headless screenshots: `SDL_VIDEODRIVER=offscreen XDG_CONFIG_HOME=/tmp/x CHARCOAL_SCRIPT="click x y; key ctrl+S;
  text abc; wait 300; shot /tmp/a.png; quit" ./charedit file` (script.go). Never on the user's display.
  Ubuntu's SDL2 has no offscreen driver and on a Mac it wants OpenGL: use `SDL_VIDEODRIVER=dummy` there.
- Sources: 16colo.rs packs and tags, asciiarena.se (sitemap; files at /collections/<name>/<file>),
  ~/Amiga/BBS/BBS Style (412 files), NAS /mnt/nas/home/Unsorted/Junk/Latex/Petscii* (with the PETSCII editor's
  own PNGs: rendering is pixel-identical). **Not csdb.dk and not zxart.ee files: their robots.txt forbid it.**

## Windows and Mac (built 2026-10-14)

- **Builds**: `.github/workflows/build.yml` on every push: Linux (ubuntu-22.04, links the system's libSDL2),
  Windows (windows-latest gcc + the official SDL2 2.32.10 MinGW package, `-extldflags -static`, `-H windowsgui`:
  one .exe that needs only Windows' own DLLs), macOS (SDL2 2.32.10 built from source as a universal static
  library, cached; arm64 + amd64 builds `lipo`'d into one program; `tools/release/package.sh` makes
  `Charcoal.app`, ad-hoc signed). Each job runs `go test` and headless screenshots (artifacts
  `screenshots-*`). A `v*` tag makes a release with `charcoal-{linux-x64,windows-x64,macos}.zip` (prerelease when
  the tag holds a "b"). **No release tag made yet.**
- **Not go-sdl2 `-tags static`**: its bundled libraries are SDL 2.0.12 (2020), and the Windows one does not link
  with today's MinGW (`__ms_vsscanf`, `__imp___iob_func` missing). The official SDL2 packages are used instead.
- Windows from here: `README.md` Build has the MinGW command; `wine` runs it headless with
  `SDL_VIDEODRIVER=offscreen` and no DISPLAY (a throw-away WINEPREFIX). charview.exe is a copy of charedit.exe
  (the name picks the viewer); with no files it says so in a message box.
- Folders (`paths.go`): Linux unchanged (XDG); Windows `%AppData%\Charcoal`, macOS `~/Library/Application
  Support/Charcoal` (settings and data together). An absolute XDG_CONFIG_HOME / XDG_DATA_HOME wins everywhere
  (the tests). Help and questions show the real folder.
- Keys (`keymods.go`): Cmd = Ctrl on a Mac (Ctrl too; Cmd+Q/H/M are the Mac's menu); Alt is the left Alt/Option,
  the right one types (AltGr; Windows sends it as LCtrl+RAlt, never a shortcut).
- Retina: `SDL_WINDOW_ALLOW_HIGHDPI`; `pass()` draws at scale x (renderer output / window size), mouse stays in
  points / scale. Windows: `SetProcessDPIAware` before SDL starts, 2x by default at 175% scaling or more.
- File requester: Parent above a drive's top and the Drives / Disks button list `fileRoots()` (Windows A:-Z:
  that exist, Mac / + /Volumes/*, Linux / + /media, /run/media/$USER, /mnt). Started in / (the Finder) it opens
  in the home folder.
- Autosave owner check (`running`): Linux /proc comm, Mac `ps -o comm=`, Windows OpenProcess +
  GetExitCodeProcess + QueryFullProcessImageNameW. Only Linux's is tried for real.
- **Not seen yet**: a real Mac screen (Retina sharpness, Cmd keys, dropping on the .app icon, Gatekeeper's
  Open Anyway), a real Windows keyboard (AltGr on a Swedish layout; the logic is unit-tested in keymods_test.go)
  and Windows SmartScreen. Ask a Mac or Windows user to try a release.

## Roadmap

The goal and the plan: docs/roadmap.md (the go-to editor for text artists on every platform).

## Where the knowledge is

- ~/Projects/Knowledge/amiga/ansi-art-real-world-traps.md: sources, robots rules, all the format traps
  (0x1A in art, .bin width, BIOS colour order, page height, Amiga vs PC, ATASCII, teletext, Petmate .prg).
- ~/Projects/Knowledge/bbs/petscii-seq.md: PETSCII/.seq, C64 screens, ATASCII and ST notes.
- ~/Projects/Knowledge/go/sdl2-desktop-gui.md: the GUI approach and headless testing.

Last updated 2026-10-14 (reference picture, shapes and brushes; frames and animation; release 0.1b3 tagged before them).
