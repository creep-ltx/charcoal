# Charcoal: where things stand (start here)

Charcoal = **CharEdit** (editor, `charedit`) + **CharView** (viewer, `charview`), one Go + SDL2 binary
(`go build -o charedit .`; `~/.local/bin/charedit` and `charview` link to it; the viewer is chosen by the
program's name or `--view`). History: first a browser app called amiansi (kept in `web/`), then CharCool,
renamed by the user on 2026-10-13. Public at github.com/creep-ltx/charcoal (remote origin, branch main).

## Now (2026-10-14): 0.1b7 released

- **Released:** v0.1b7 (prerelease) on GitHub with Linux, Windows and macOS zips. Roadmap steps 0, 1 and 2 are done
  (data-loss fixes, Windows/Mac + start screen, frames, reference picture, own fonts, shapes), plus things users
  asked for: animation templates (TheDraw's 14 scan patterns + 14 of our own), BBS pauses per board, Animate a
  folder (random effects, budgets, random screens per board), Moebius compatibility + key presets, files for
  other editors. **Next on the roadmap: step 3, compo-ready** (work stages from undo history, party rule check,
  runs-on-the-machine exports), then step 4/5 (docs/roadmap.md).
- **Release how-to:** bump `const version` in main.go (X.YbN = beta), commit, push main, `git tag -a vX.YbN -m ...`,
  push the tag; GitHub Actions builds the three zips and a prerelease (tags with "b"). Check the run with
  `gh run watch`, then `gh release view`. Before tagging: `go test ./...` and, after merges touching readers, the
  big check (tools/bigcheck) compared with the previous report.
- **How the user wants work done:** showy new features are built on a local branch (git worktrees, never pushed),
  shown first (a video on the NAS /mnt/nas/home/*.mp4 and screenshots opened with Read: they show on the user's
  phone), released only after the user says yes. Several helpers in parallel worktrees, then one local integration
  branch, then the full test run and the big check on the combination (it caught a real clash twice: see
  ~/Projects/Knowledge/claude-code/parallel-helpers-in-worktrees.md).
- **Who uses it:** the user posts Charcoal in the Ami-Express Sysops Facebook group. Smolly Danders (a sysop, not an
  artist, colours art in Moebius) asked for the animation templates ("exactly that"). Sysops want: effects that
  play on the board (BBS pauses), small files, whole folders at once.
- **Open, not done:** nothing has been played on a live board (Ami-Express 5 first: `~SMO`/`~w`/`~NS` are AE5
  only, /X 4.x lacks them); FAME and CNet pauses (whole seconds only) - the user was asked, no answer yet; an app
  icon; a CharView .app on the Mac (`--view` works); a 0x1A in a cell of an ANSI saved without SAUCE ends the file
  for readers (3 old files); the 4:3 aspect preview; roadmap step 3 onwards.

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
| PC ANSI | + .bin (BIOS colour order!), XBin, PCBoard .pcb, Avatar .avt; iCEDraw .idf, ArtWorx .adf, Tundra .tnd, IcyDraw .icy, REXPaint .xp, Playscii .psci (see Other editors' files) |
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
- Not done: the 4:3 preview (step 4; 9-pixel cells done, 0.1b7), 8x8/8x14 sets of other code pages, a .prg that carries a
  charset (Petmate's form has none), fonts on ST/teletext pages, more than the first screen's custom font in a
  Petmate file.

## Animation templates (2026-10-14, merged; 0.1b5-0.1b7)

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

## Other editors' files (2026-10-14, merged; 0.1b5-0.1b7)

- `editorformats.go`: writers `writeIDF` (runs within a row, 4+ cells or any character 1, count <= 255),
  `writeADF` (80 columns; EGA 64 with the 16 at `adfColours`), `writeTundra` (80 columns; the first cell sets both
  colours, characters 1-6 always inside a colour command), `writeIcy` (IcyDraw v1: PNG + icYD records ICED,
  PALETTE, FONT (PSF2), LAYER, END; uncompressed; no SAUCE record, IcyDraw refuses a file whose SAUCE it cannot
  read), `writeXP` (gzip, column-major), `writePsci` (Playscii JSON, one layer per frame, charset/palette by
  name), `writeMarqPet` / `writeMarqBas` / `writeMarqAsm` (Marq's C64 exports), `writePetmateJSON`. Readers:
  `parseIcyDraw` (v1 with zstd via klauspost/compress, and v0 base64 tEXt/zTXt) + `icyPage` (layers put
  together as IcyDraw's merge_layer_char, bold -> bright, blink -> background 8-15), `parseXP` and `parseTundra` (24-bit colours numbered by `rgbColours`: a VGA colour keeps
  its VGA number, so a page saved with VGA colours reads back with the same numbers), `parsePsci`
  (charsets dos, amiga_topaz, atari, c64_real_shifted/unshifted, commodore_pet, c64_petscii/c64_edscii/jpetscii through
  `psciC64Sets`; any other charset opens as a PC page with the glyph numbers and a note, never refused; a C64/Atari page's background is the commonest, reversed cells from swapped colours),
  `parseMarqPet`, `parsePetmateJSON`. Hostile sizes: IcyDraw pages/layers <= 4M cells, 256 MB unpacked; a
  Playscii picture must fit its file (w*h <= len/16); Petmate JSON needs w*h screencodes.
- Glue (the only edits to shared files): entries at the end of `formats` ("For other editors: ..."),
  `encodeEditor` at the end of `encodeFor`, `loadEditor` first in `loadBytes` (it also takes .idf/.adf/.tnd
  over, so their saveFmt is now their own format; before, Ctrl+S on an opened .idf wrote XBin bytes into it),
  `reparseEditor` first in `reparse`, `editorOwn` in `writeOut` (own + srcKind), `editorCarriesFont` in
  `carriesFont`, `isIcyDraw` in `isPicture` (an .icy is a PNG). `init` adds psci/pmjson to `framesFormats`.
  SAUCE of opened .idf/.adf/.tnd/.icy goes to the page's info; written back without comment lines (ansilove
  takes 129 bytes off the end, so comments would move its font/palette).
- Verified: `editorformats_test.go` (round trips, models of ansilove's and IcyDraw's idf/tnd readers, files
  IcyDraw 0.6 wrote in `testdata/icydraw`, Playscii's hello1.psci, CharEdit save/open/Ctrl+S); external
  `editorformats_ext_test.go`: ansilove renders .idf/.adf pixel-identical to CharEdit (+ SAUCE), .tnd colours;
  IcyDraw's engine (ccdump, built in a bwrap sandbox: ~/Projects/Knowledge/amiga/text-art-editor-formats.md)
  reads all five PC formats cell for cell; Playscii's own art.py loader reads the .psci tiles/frames/delays.
  `FuzzEditorFormats` (2.8M execs) and FuzzParsers (60 s; it stalls at 0/sec for long stretches on main too).
- Not done: IcyDraw PETSCII/ATASCII/Viewdata/ST pages, sixel layers, layer effects, tags; Playscii xform
  (flips) and other charsets; a Playscii charset for an own font (Playscii takes charsets only from its own
  folders); Petmate custom charsets in its JSON; PabloDraw / TheDraw / ACiDDraw / Moebius have no own art file
  beyond ANSI, BIN, XBin (TheDraw .tdf are fonts).

## Animate a folder (2026-10-14, merged; 0.1b5-0.1b7)

- For sysops (not artists): every screen of a folder animated for the board at once. `batch.go` (logic + the
  command line), `batch_ui.go` (the requester `BatchReq`: setup, progress, report; Alt+Shift+J `animate_folder`,
  and a button in the Open requester). `batchSpec` -> `newBatchRun` (checks, lists files sorted, refuses an
  output that is the originals' folder unless Replace) -> `step()` one file at a time (the GUI runs steps for
  40 ms per drawn frame; `dialogAnimates` is also true while `batchBusy`) -> `reportText` / `writeReport`
  (`animate-report.txt` in the output folder, with the command line that repeats the run).
- Each file is read by the normal `loadBytes` in a **scratch tab** (`withScratch`: a.Tab swapped for a
  `makeTab`, the tab, tool, F-key set, requester and message put back afterwards), given `a.order` and written by
  `encodeFor(fmt, board, nil)`: exactly what Save would write. Non-stream sources (.bin .xb .pcb .avt .idf .adf
  .tnd, one-screen .petmate) are written as the page's stream (.ans/.seq), `name-bin.ans` if the name is taken.
- Random: one `rand.Rand` from the seed, used in file order only for files that get animated (`pick`): the
  pattern (from the ticked pool, not the last one with NoRepeat; Fade in left out on pages that cannot fade),
  Reverse/Flip (Toggles), the effect's seed. So the same seed and the same folder give the same bytes (test).
- Versions (`variantNames`, `randomScreens`): Ami-Express `001.NAME.ext` ... + `NAME.ext` = `~<n>SR_<AEPath>NAME|`;
  Mystic `name.ans`, `name.an1`..`.an8` (9 at most); Synchronet `name.ans`, `name1.ans` ...; others `NAME-2.ext`.
  How each was established: ~/Projects/Knowledge/bbs/screens-variants-utf8-wide.md. Copies for other boards are
  named their board's way + `-<id>` (copyPath).
- Skipped with a reason: not art (`batchNotArt`: notArt's magics, Amiga icons, .bak, C source, no art ending and
  no ESC[), unreadable, ROMs missing for PETSCII, teletext/Spectrum, several screens, already an animation
  (`split.clears >= 2`, `fullest`), kind not ticked, a board with no codes for that format, too large, nothing drawn.
- Tests: `batch_test.go` (seed repeatability, no repeats, ticked pool, names per board, every output through
  bbspause_test's board models reads back as the original picture, skip reasons, originals untouched, replace +
  .bak, command-line exit codes 0/1/2, a real-art run over /tmp/bigcheck when it is there).
- Not done: the stub for a board copy of Ami-Express names the copy's versions only if renamed; Mystic needs the
  base name asked without an ending (as its menus do); PETSCII/ATASCII/VT52 versions have no board naming.

## Animation size, detail levels and cost (2026-10-14, merged; 0.1b5-0.1b7)

- Why: a 15 KB screen became ~630 KB with Matrix rain: minutes at 14400 bps. Measure with
  `CHARCOAL_MEASURE=1 go test -run TestMeasure -v` (animsize_test.go: every template at every level on real screens
  from /tmp/bigcheck and testdata; byte kinds: moves / SGR / characters).
- Writer trims (transitions.go, every level): `ansiWriter` keeps `screen` (what the terminal shows) and crosses a
  gap of <= 3 cells by reprinting them when no colour change is needed (`reprint`); BS for 1-3 steps left; a space
  keeps the pen's fg/bold/italic (`attrsOf`; only bg/blink/underline show). **No bare CR**: PCBoard ends a line at
  a CR and sends CR LF (the test model caught it). `stepStream` (write / cost / finish) replaced the loop of
  `writeStepsIn`; `writeDeltaFrames` writes each frame row-major, column-major (`byColumn`) or by colour
  (`byColour`), whichever `cost` says is shortest (writers have `clone()`). PCBoard: an art line starting with
  % ! $ gets `@DELAY:0@` first (layoutPCB; a latent bug the reordering exposed).
- Levels (animcost.go): `detailLarge` = 0 (zero value: old pages keep their look), Medium, Small; `drawOrder.Detail`
  ("" = large), `animSpec.detail`, `fx.detail`. Draw orders: `scanOpts.run` (`scanRun`: 1/3/8 cells of a row
  together, keyed at the run's middle cell; Reverse turns the run order, runs stay left to right) via
  `cellsInOrderAt`; Fade in `fadePassesAt` (4/3/2). Effects: `effectOwn(pat, detail)` frames (`effectFramesAt`)
  with holds scaled so frames x hold stays ~ Large's; Matrix churn 1/3, 1/8, none, trail 14/8/2 (Small: one colour
  behind the head); Decrypt glyphs re-rolled every frame / 1 in 3 / once, no flash at Small; Drop and Assemble
  `flight()` 1/0.7/0.4; Shine one glint level at Small; Shade in fewer steps. Large's random draws are unchanged
  (byte-identical files).
- Cost: `costOf(fmt, order, board)` = `animCost{size (the written file), sent (encodeFor with no board), pauseMs,
  plain}` with page/order/info/message restored; `at(bps)` = sent x 10 / bps + pauses (`playMs`; telnet = -1 =
  pauses only); `weight()` by `ratio()` = sent/plain: light <= 1.5, medium <= 4. Speeds: `bauds` now 2400 9600 14400
  33600 57600 115200 telnet (prefs keep -1); `lineBps` where a number is needed. Settings: `prefs.AnimWarn` (s, 0 =
  30), `prefs.AnimLevel` (Animate's last level, "" = medium).
- Animate requester (transitions_ui.go + animcost_ui.go): Detail (D) and Caller's speed (sets `a.baud`), the lists'
  weight bars (`fillCosts`: the chosen one, then one more per drawn frame; not on pages > 60000 cells), the cost
  panel under the lists (`drawCost`, warning triangle drawn by hand: Topaz has no U+26A0), `orderFor(pat)` = what
  Apply sets. Preview: `speedPreview` plays the saved stream of the first 50 rows through the page's terminal
  with the board's pauses (`drawPreviewAt`; frames mode keeps the frames-at-holds preview).
- Play: `Play.stepLine` (animcost.go) used by `playStepAt` and the preview: exact byte time carried between frames
  (before, rounding made 2400 bps play a fifth slow), stops at `Play.pauses` (`playPauses`: the page's board's
  pauses, if the bytes are the page's animation) and waits from the last byte's time.
- Animate a folder: `batchSpec.Detail/MaxKB/MaxSeconds/AtBps/MaxWeight`, `pickFitting` (candidates shuffled by the
  run's rng, each from the run's level downwards, first that fits; skip reason names the smallest) used only with
  a budget, so old seeds without one pick as before (now at Medium); `batchItem.orders/sizes/times`; report
  Detail/Budget/Times lines and per-output size + time; flags --detail --max-kb --max-seconds --at --light --weight.
  UI: Detail, Effects (any / light / light and medium), budget fields + speed; ticked list bars from
  `typicalWeights` (a made-up 80x25 screen, per level, cached).
- Tests (animcost_test.go): every level round-trips in every stream format; every frame of Small/Medium effects;
  Small <= Medium <= Large and Small draw orders <= 1.2x plain (bs-alove); `costOf` = the written file to the byte
  and the time formula for 4 boards; weights; Play/preview time at 2400/14400/telnet with Mystic pauses; requester
  cost = Apply's save; batch budget honoured (file sizes, times), lowering, skip reason, light only, CLI flags.
- Not done: Large could churn less in the stream than in the GIF (kept as the full look on purpose); PETSCII /
  ATASCII / VT52 writers have no reprint trick (their moves are 1 byte anyway); the weight of a frame strip saved
  as a stream is not shown.

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

## Moebius files and key presets (2026-10-14, merged; 0.1b5-0.1b7)

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
  With a fresh XDG_CONFIG_HOME the first screen is the "Commodore character sets" requester at 984x688: start the
  script with `click 740 452; wait 300;` (Not now) or every shot is that requester.
- Interface look (2026-10-14, step 1 of the GUI plan; mockups: https://claude.ai/artifact/RFZccPWKn8tEKNPt1ZqhHD):
  `theme.go`. The colours `cGrey`, `cText`, `cOrange`... are variables set by `useTheme(name)` from `themes`
  (only "Dark", today's look, so far). Interface text goes through a `face` (`draw`, `width`), today
  `bitmapFace` (Topaz); `setFace(f, h)` also resizes `lineH`, `gadH`, `smallH`, `tileH`. Layout measures text in
  pixels: `textW`, `digitsW(n)` for number fields and columns, `fitText`, `wrap(s, px)` (`wrapCols(s, n)` is for
  plain-text reports). No `charW` any more: never count characters for screen layout. Checked pixel-identical
  to 0.1b7 on 9 screens (start, page, Animate, Animate a folder, help, open, save as, info, typing).
  Next: the menu bar generated from `keys.go`, then the Studio layout, Light/Workbench themes and a system font
  (golang.org/x/image's opentype, already a dependency).
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

Last updated 2026-10-14 (0.1b7 released; "Now" section at the top).
