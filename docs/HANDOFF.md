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
- Not done (design step 11): delta stream writing for the strip's frames (the effects have it: writeDeltaFrames), CharView playing frames, onion of the next frame, timed capture
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

## Own fonts and the font editor (2026-10-14, roadmap 2.3 + PC fonts of step 4)

- `fontfile.go`: `Tab.ownFont` (+ `ownLower`, a PETSCII page's lower case set) is the page's own font, shown when
  `fontFor[mode]` is `ownID` ("xbin", the old XBin id, kept for autosaves) or `ownLowerID` ("xbinlower";
  `lower()` still works by the "lower" suffix; `setCase` keeps the own set). `a.ownInUse()`; `useOwnFont(up, low,
  changed)`. `Font.h` = real rows (8 kept doubled, 9-16 as they are; 0 = the kind's own: 8 for c64/atari/zx, else
  16), `rows()`, `row()`/`setRow()`, `native()` (256 x rows bytes); `cellH()` of a PC/Amiga page is the font's `h`
  when set (14 for EGA). `src`, `format` ("psf1", "psf2", "64c", "petmate:custom_1" ...).
- Files: `parseFontFile(name, bytes, mode, cur)` (PC/Amiga raw 256 x 6-16 rows, PSF1/PSF2 8 wide, XBin; C64 .64c
  (2-byte address, up to 512 glyphs), .bin 1024/2048/4096 (1024 = 128 glyphs, top half reversed; 4096 = both
  sets); Atari 1024 in ROM order; ZX 768 (32-127) and 168 (UDGs 144-164) onto the current font); `fontFileBytes`
  writes them back by ending. ST and teletext pages keep their fonts.
- Kept with the art: XBin (`writeXBin` height = cellH, 14 works), Petmate `customFonts` (`petmateOwnFont` reads
  the first screen's; `petmateWithFont` writes it as custom_N, reusing the id it came with; the lower set only if a
  screen shows it; with the ROM chosen, screens naming a custom font go back to upper/lower). Other formats:
  `keepArtFont` after `writeOut` puts a copy in `<data dir>/fonts/<sha1>.font` and `prefs.art_fonts[path]`; the
  status line says "a .ans cannot hold the font, CharEdit keeps it for the file"; `restoreArtFont` after
  `openPath`. A `.scr` is written and matched in the page's font (`writeSCRFont`, `parseSCRFont`).
- `fontedit.go`: `FontEd` (the window at the right, the page not dimmed: main.go), its own undo of font bytes;
  editing a built-in font copies it (`editableFont`; PETSCII: both sets). `FileReq.pick/title/note/back` make the
  open/save requester a font requester. Dropped files: `dropFile` asks (`FontAsk`) when `looksLikeFont`.
- Built-in PC fonts: `fonts/IBM_EGA_8x14.f14`, `IBM_VGA_8x8.f08`, `IBM_VGA_8x16_850.f16`, `_866.f16`, made by
  `tools/pcfonts.py` from the pack's **.otb** bitmap fonts (the Px .ttf moves capitals' accents by a row);
  checked byte for byte against DOSBox's ROM tables. `fontBySauce` with aliases (IBM VGA 437, IBM EGA43 ...).
- Not done: the 4:3 preview (step 4; 9-pixel cells done on branch moebius), 8x8/8x14 sets of other code pages, a .prg that carries a
  charset (Petmate's form has none), fonts on ST/teletext pages, more than the first screen's custom font in a
  Petmate file.

## Animation templates (2026-10-14, branch transitions)

- TheDraw 4.63's "Easy Animation" (manual: The Animation System, Fadeout): `transitions.go` (logic),
  `transitions_ui.go` (the Animate requester, Alt+J `animate`, with a live preview: ~40 frames of the page's first
  50 rows through `renderDoc`, scaled by `blitScaled`; main.go redraws while it is open, `dialogAnimates`).
- `scanOrder(pat, scanOpts{reverse, flip, aspect}, cols, rows, used)`: cells sorted by a pattern key (`scanKey`;
  aspect = cellH/cellW so circles look round), Flip = the key of (x, rows-1-y), Reverse = the order turned round,
  Scramble a fixed seed (same file every save). Interpretations of the one-line descriptions: Gate = columns from
  both sides inward, Door = rows from top and bottom inward with both ends of each row at once, Checkerboard =
  8-column squares, even ones first, Squares = rings clockwise from the upper-left corner.
- `usedCell(mode, v, paper)`: only cells that show something are drawn (spaces only with their own background;
  teletext: letters/mosaics only, the codes always stay).
- Draw order: `Tab.order *drawOrder` (pattern id, Reverse, Flip), part of `Snap` (undo), autosave (`draw_order`),
  status line "Draws: ...". `encode` writes it for ansi/seq/ata/vt52 when the page has one frame
  (`orderedSave`, `writeOrdered`), `playBytes` sends it. `writeDrawSteps` + one `cursorWriter` per format: ANSI
  (shortest of CSI H / A-D / CR LF), PETSCII (cursor keys, HOME, RETURN; a quote turns on quote mode: closed
  with a second quote on a cell drawn later, or RETURN, or past the last row quote + DEL), ATASCII (cursor keys,
  left/right wrap in the row, EOL), VT52 (ESC Y within the screen's rows, ESC B below). The stream ends with
  the cursor on the last row so the file opens as tall as the page. Fade in = 4 passes. Fade out / scroll / Erase:
  frames only (the file's last picture would not be the page).
- Frames: `transitionFrames` (every n cells, or every ms at the baud through the stream's byte offsets), replace
  the current frame as one undo step (`applyFrames`); the finished picture holds at least 2 s.
- Tests: `transitions_test.go` (coverage of every pattern/toggle, shapes, round trip of every pattern through
  parseArt/parseSeq/parseAta/parseVT52 on random pages, PETSCII quotes, undo/save/play/autosave, frames).
- Ten more templates (2026-10-14, branch templates): `effects.go`. Spiral and Blinds are scan keys (`patSpiral`,
  `patBlinds`, `blindRows` = 4; `isScan`). The frame effects (Matrix rain, Decrypt, Drop, Assemble, Shine, Shade
  in, Melt in, Power on; `isEffect`, `effectDefaults` = frames and hold) build k frames from `fx` (`base` = the
  clear screen with teletext codes kept, `cellOf` per page type, `glint` = white / BRIGHT+inverse on ZX / reverse
  without colours / block on teletext, `inkOf`, `shadeSeq`), time 0..1, the last frame forced to the picture,
  random parts from `drawOrder.Seed` (also `Frames`; the requester's Shuffle / S). As the saved file they are
  `writeDeltaFrames` (transitions.go): clear, then per frame the cells that look different (`usedCell` both
  ways), through `writeStepsIn` with `seg` so a PETSCII quote is closed only on a cell of the same frame. The
  frame strip's normal multi-frame save keeps a clear between frames: Split can cut that back into frames, a delta
  stream cannot be. Requester: two columns, Frames + Hold fields per effect (`syncDefaults`), preview at the holds.
  Video: `tools/preview.sh` (preview_test.go with CHARCOAL_PREVIEW) -> preview/templates.mp4 + sheet.png
  (preview/ is excluded from git). Test trap: on random ANSI pages a 0x1A ends the plain reference file and a
  0x9B before digits reads as CSI, so stream-vs-plain comparisons of intermediate frames clean those first.
- BBS pauses (2026-10-14, branch templates): `bbspause.go`. `bbsBoards` (Ami-Express (/X), Mystic, PCBoard,
  Synchronet, Tempest: head, delay code, unit/max with the rounding carried (`units`), escape of the board's
  own character, `layout` for boards that read in pieces: `layoutAE` (998-byte Fgets pieces, filled with empty
  `~|` / a last `~`), `layoutPCB` (2047-byte dosfgets lines, filled with `@DELAY:000@`)). `pace(stream,
  pauses)`; the pauses come from `writeOrderedPaced` (every N cells / each effect frame's hold) and
  `writeStreamFramesPaced` (the strip's holds). `encodeFor(fmtID, board, &ps)`; `encode` uses `pageBoard()`
  = `drawOrder.BBS` ("none" = none) else `prefs.MyBBS` (Settings' My BBS, default ""). `drawOrder` keeps
  BBS, Copies (ids, comma separated), Hold, Every, Pause (autosave/undo with the order). `writeOut` writes the
  copies (`copyPath`: name-<id>.ext) and a status note. Animate: `drawPauses`. Tests: `bbspause_test.go`, a
  model of each board's display code (from its source) gives the plain stream's bytes and the asked time.
  Codes per board and how they were established: ~/Projects/Knowledge/bbs/ansi-draw-order-animation.md.
  Not tried on a real board yet (the user's Ami-Express next).
- Not done: reading a draw order back from a file (it opens as the picture; the order is not recognised), Amiga
  real-terminal check, TheDraw's own animation editing (limiting, markers, include files, pauses).

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
  quit. At start, autosaves of CharEdits no longer running are offered back. The page's own font goes in
  `<id>.font` (since 2026-10-14).
- `.bak` on saving over a file (`prefs.no_backup` switches it off; Settings). Recent files: `prefs.recent`.

## Moebius files and key presets (2026-10-14, branch moebius)

- Corpus: `tools/moebius/gen.js` builds pages and writes them with Moebius's own encoders (a separate checkout of
  github.com/blocktronics/moebius, `app/libtextmode/{ansi,binary_text,xbin,textmode,palette,encodings}.js`, pure
  JS, run with node from another folder; never `npm install`), then reads each back with Moebius's decoder into
  `testdata/moebius/<file>.json` (cells as base64 char + BIOS attribute; SAUCE fields; XBin palette/font).
  `moebius_test.go`: every file opens as that page (`TestMoebiusFilesOpen`), saves and reopens the same with the
  same SAUCE (`TestMoebiusRoundTrip`), 9-pixel drawing (`TestNinePixelCells`). The other way round:
  `CHARCOAL_MOEBIUS_SAVE=/tmp/x go test -run TestMoebiusSaveBack .` then `node tools/moebius/check.js
  <moebius>/app testdata/moebius /tmp/x` (all 69 read back in Moebius as the page, 2026-10-14).
- What changed in reading (core.go `parseArt`, `Term`): SAUCE width from 1 column (was 8); the SAUCE height when
  the record has the aspect-ratio bits (Moebius always sets 0x10) and is taller than the stream (only those: older
  editors wrote 25 or one row too many, 599 PabloDraw-flag files would grow); `glyphCtl`: with a SAUCE record
  (DataType 1, FileType 0/1) on PC pages 0x00 0x07 0x08 0x09 are glyphs (real files: MM-ERRORIN0RDERRZ.ANS's 2305
  NULs now line up); `sauceText` reads UTF-8 fields, `utf8Comments` finds a COMNT block whose lines are 64
  characters of UTF-8 (Moebius) and `Sauce.utf8` makes `makeSauce` write it back so; XBin's SAUCE is read
  (`finishLoad`) and written (`SauceInfo.dataType` 6); UTF-8 files (`Tab.sourceUTF8`, `ParseOpts.utf8`):
  `lateWrap` (VT wrap), width = longest line up to 256, real tabs become spaces and ○ ◘ • stay glyphs
  (`fromUtf8`), SGR 38/48;5;n and 38/48;2 as the nearest of 16 (`xterm256`, `nearest16`, `hiBg` so 49 ends a
  bright background), ice on for 48;5/100-107; `utf8LooksPc` counts only characters one set lacks; `looksAtascii`
  is not fooled by ¢ (0x9B) in a file full of ESC [.
- Kept on save: `Tab.sauceFlags` (TFlags bits 1-4, letter spacing and aspect) for .ans and .bin; a SAUCE font
  name CharEdit lacks (`Info.font`/`fontID`, `sauceFont()`) while the page shows the stand-in; `fontBySauce` gives
  stand-ins for other code pages (IBM VGA50 866 -> VGA50, IBM VGA25G -> VGA). 0x00/07/08/09 are saved raw only when
  the file had them so (`Tab.rawCtl`, `rawControls`; writeAnsi's second keep flag), else as spaces (a BBS terminal
  acts on them; draw-order streams always write spaces). Big check after all this: 29 issues, the same as before.
- 9-pixel cells: `cellW()` 9 on a PC page with `ninePx()`; `drawCell` repeats column 8 for 0xC0-0xDF; PNG export
  too. `ninepx` action (Alt+9; Moebius preset Ctrl+F) toggles and dirties the page. The aspect-ratio (4:3) preview
  is not done; its bits are kept.
- Key presets (`keypresets.go`): `keyPresets` (charedit, moebius, pablodraw, thedraw, aciddraw), each a map of
  action -> keys from the editor's source/manual; `presetKeys` (the preset's keys, the rest CharEdit's minus keys
  the preset took); the keys file's `preset = id` line (`keyFilePreset`, read by `applyKeyFile`; lines that only
  repeat a CharEdit default do not override a preset, since the first-run file lists every default);
  `setKeyPreset` (Settings > Keys like, `drawKeysLike`) rewrites the line, keeps the rest, old file in keys.bak.
  Tests: `TestDefaultKeysHaveNoClashes` runs every preset, `TestKeyPresets`.
- Moebius facts (verified by running its code): ~/Projects/Knowledge/bbs/moebius-file-format.md.

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

Last updated 2026-10-14 (Moebius files and key presets; reference picture, shapes and brushes; own fonts and the font editor; animation templates; frames and animation before).
