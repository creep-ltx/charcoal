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
- **Release how-to:** add the version's section to CHANGELOG.md (plain words for artists and sysops; the
  release page is made from it by tools/release/notes.sh, and the release job fails without it), bump
  `const version` in main.go (X.YbN = beta), commit, push main, `git tag -a vX.YbN -m ...`,
  push the tag; GitHub Actions builds the three zips and a prerelease (tags with "b"). Check the run with
  `gh run watch`, then `gh release view`. Before tagging: `go test ./...`, the same as CI runs it (Go 1.24 from
  go.mod, no downloaded ROMs/fonts: `HOME=<empty> GOMODCACHE=$(go env GOMODCACHE) GOCACHE=$(go env GOCACHE)
  GOPATH=$(go env GOPATH) XDG_DATA_HOME=<empty> XDG_CONFIG_HOME=<empty> GOTOOLCHAIN=go1.24.0 go test ./...` -
  0.1b8's first tag failed in CI on exactly that), after merges touching readers the big check (tools/bigcheck)
  compared with the previous report, and for machine programs `CHARCOAL_EMU=1 go test -run TestEmu .`.
- **How the user wants work done:** showy new features are built on a local branch (git worktrees, never pushed),
  shown first (a video on the NAS and screenshots opened with Read: they show on the user's
  phone), released only after the user says yes. Several helpers in parallel worktrees, then one local integration
  branch, then the full test run and the big check on the combination (it caught a real clash twice: see
  the knowledge base, claude-code/parallel-helpers-in-worktrees.md).
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
| Atari (ATASCII) | .ata (width 38/40 guessed); .xex and boot .atr (2026-10-16); AltirraOS font built in, an OS ROM's when present; GTIA colour registers |
| Atari ST (VT52) | .vt; low/medium/high; EmuTOS font downloaded on first use (GPL) |
| PETSCII | PET 40/80, VIC-20, C64, C128 40/80; .seq .c .petmate .prg (Petmate form, or CharEdit's for sets/modes) .d64, CharPad .ctm read; multicolour and ECM (C64, C128 40); ROMs from VICE on first use |

| Atari ST (VT52) | .vt; low/medium/high; EmuTOS font downloaded on first use (GPL); IGS .ig (2026-10-16): the drawing as a picture under the cells, see "Atari ST IGS" below |
| PETSCII | PET 40/80, VIC-20, C64, C128 40/80; .seq .c .petmate .prg (Petmate form); ROMs from VICE on first use |
| ZX Spectrum | .scr (6912 bytes), .tap (2026-10-16: loader + SCREEN$); 48K ROM font built in (Amstrad allows passing it on) |
| Amstrad CPC | modes 0/1/2; .bin (AMSDOS program), .dsk, Playscii .psci (Logiker's exporter); firmware ROM font built in (Amstrad's permission) |
| Teletext | .tti .ep1, edit.tf links (.url), Mode 7 dumps; Bedstead font (CC0) built in; a .tti carousel opens as frames (one per subpage) and saving rewrites only changed subpages' OL 0-24 rows, every other line kept (teletext.go writeTTIFrames) |
| Minitel (2026-10-16) | .vdt (Videotex stream; screens a clear ends become frames); Bisson's Minitel font (CC0) built in; see "Minitel and Viewdata pages" |
| Viewdata (2026-10-16) | a teletext page of 24 rows: .cpt streams (.car carousels as frames), 960-byte .bin frames, Telstar .json frames, and .tti / edit.tf |
| Plus/4, C16 (2026-10-16) | a PETSCII model: Marq's Plus/4 .prg and .c, .d64, TED Screen Editor .scrn (+ .proj), Petmate9 c16 screens, .seq; TED colours and flash; see "Plus/4 and C16 pages" |
| MSX (2026-10-16) | SCREEN 0 (40x24), SCREEN 1 (32x24, colours per 8 characters), MSX2 WIDTH 80; .rom cartridge and BLOAD .bin (both read back), Playscii .psci; C-BIOS fonts (BSD) built in; see "MSX and Sharp MZ-700 pages" |
| Sharp MZ-700 (2026-10-16) | 40x25, two character sets, 8 colours per cell; .mzf (CharEdit's, and other pictures that copy to $D000/$D800); a public domain look-alike of the first set and CharEdit's own (CC0) of the second built in, Sharp's ROM when found |
| Unicode (2026-10-16) | UTF-8 ANSI (256/24-bit SGR), durdraw .dur (frames), mIRC .irc (also written from PC pages), HTML; GNU Unifont's art blocks built in (OFL); see "Unicode page" below |
| SkyPix (2026-10-16) | Amiga BBS graphics (.spx, .ans with `ESC[n!`) on a PC page: the drawing under the cells; also IcyDraw's PETSCII/ATASCII/ST .icy, PabloDraw .pd, TheDraw .td, BSAVE .bsv, Playscii's other sets: see "The rest of other editors' files" |

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
- Not done: the 4:3 preview (step 4; 9-pixel cells done, 0.1b7), 8x8/8x14 sets of other code pages, fonts on ST/teletext pages (a .prg that carries a charset:
  done 2026-10-16, c64x_prg.go), more than the first screen's custom font in a
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
  Codes per board and how they were established: the knowledge base (bbs/ansi-draw-order-animation.md).
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
  IcyDraw's engine (ccdump, built in a bwrap sandbox: the knowledge base, amiga/text-art-editor-formats.md)
  reads all five PC formats cell for cell; Playscii's own art.py loader reads the .psci tiles/frames/delays.
  `FuzzEditorFormats` (2.8M execs) and FuzzParsers (60 s; it stalls at 0/sec for long stretches on main too).
- Not done: IcyDraw Viewdata pages (PETSCII/ATASCII/ST: done 2026-10-16, see "The rest of other editors' files"), sixel layers, layer effects, tags; Playscii xform
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
  How each was established: the knowledge base (bbs/screens-variants-utf8-wide.md). Copies for other boards are
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

## Unicode page (2026-10-16, branch r1-unicode; design: docs/unicode-design.md)

- Mode `uni` ("Unicode", the Uni button, Page > Unicode / Convert to Unicode, a start tile). `uni.go` (cell,
  style table, conversions, the font, mirror tables, list registration in `init`), `uniformats.go` (readers and
  writers, `loadUni` first in loadBytes, `reparseUni` first in reparse, `encodeUni` before encodeFor's switch),
  `uni_ui.go` (drawUniCell, character table by blocks, 256 grid + 24-bit requester, Half, setModeUni,
  convertFromMachine, restoreUniMode).
- **Cell**: character code in bits 0-7 + 18-25 (BMP; surrogates D800-D8FF = U+1FB00-1FBFF, D900-DCFF =
  U+1CC00-1CFFF), style in bits 8-17 + 26-31 (16 bits). Styles < 256 = a PC cell's fg | bg<<4 (so ASCII PC cells
  are already Unicode cells); >= 256 from `uniStyles`, one process-wide append-only table of {fg, bg, italic |
  underline}, colours 0-255 or `rgbFlag|RRGGBB`. **Never** build a Unicode cell with makeCell beyond ASCII or put
  italic/underline bits in it: use `uniCell(code, styleOf(fg, bg, flags))`, `uniRestyle`, `uniWithCode`.
  `Doc.uni` (applyLook, clone, withCols) makes blankAt use `uniBlank`. `Snap.mode` lets undo bring the page type
  back across a conversion. `Block.uni`: a block pasted between a PC and a Unicode page is converted (stamp).
- `uniCompact` renumbers all tabs' docs, frames, undo/redo docs, float and clip when the table is 3/4 full,
  only from loadUni (a safe point; skipped with a requester open or while playing). Full anyway: nearest style
  with the same flags, `uniFullNote` in the status line. Tests must not flood the table with random 24-bit
  colours (uniFColour uses 40 RGB values).
- Brush: `a.fg/a.bg` stay 0-15 (other pages index the palette with them); `uniOpts` (ufg/ubg + on flags)
  hold 256/24-bit colours; `setFg/setBg` (the 16) turn them off. `a.brush` and F-keys hold codes (> 255 on
  Unicode pages; prefs validate with `validUniFkeys`). `halfFg/halfBg` give the Half tool full colours.
- Terminal: `Term.uni` keeps 38/48;5 and ;2 colours exact (`uniSgr`, `fgX/bgX`), 90-97 are bright colours
  (`hiFg`, ended by 30-37/39) and 40-47 end a 100-107 background, as terminals do (only on Unicode pages; PC
  parsing unchanged). `putCode` puts a code. The writer uses 90-97/100-107, 38;5, 38;2, a reset + LF per row.
- Detection: `.dur`/gzip with "DurMovie" -> dur; `.irc` or 2+ `^C<digit>` without ESC[ -> mirc (palette set to
  mIRC's); UTF-8 that `needsUni` (a rune > 0xFF CP437 lacks, or 38/48;5;16+ / ;2) or a `.utf8.ans` name ->
  uniansi. CP437-only UTF-8 (Moebius's .utf8ans exports) still opens as PC (and saves as UTF-8 again: see "UTF-8
  files ..." above). Unicode pages write no SAUCE (terminal art; cat would show it; durdraw writes none): a page
  wider than its longest line gets its first row in full so the lines give the width; only a page under 80
  columns needs the record (readers take 80 at least). The user's choices, 2026-10-16.
- durdraw (read from its source, verified by running it): 16-colour files fg = 1 + BIOS colour, bg BIOS 0-7
  (8 = black, 9-16 durdraw's iCE order, which its export cannot write); 256-colour files fg 1-16 through
  color_256_to_ansi_16, bg xterm (durdraw strips them; kept here); formats < 7 through its legacy tables;
  v7 colorMap is [column][line]; non-printable characters become spaces; hold = delay > 0 ? delay : 1/framerate.
  `testdata/durdraw`: 6 examples + durdraw's own `--export-ansi` output (TestUniDurdrawMatchesItsExport);
  `CHARCOAL_DURDRAW=dir` compares more, `CHARCOAL_DURDRAW_SRC=checkout` has durdraw read CharEdit's .dur
  (uni_ext_test.go; runs durdraw in script(1)).
- mIRC: 0-15 map one to one onto the 16 (mircToAnsi; the mIRC palette shows them as mIRC does), 16-98 are
  their RGB (modern.ircdocs.horse), 99 default; ^B is read and dropped (no bold bit); writer: two digits only
  when a digit or comma follows, the longest line measured, warning past `ircLineMax` (450).
- Font: `fonts/unifont-art.hex.gz` (5280 glyphs, 55 KB; `tools/unifont.sh` remakes it), parsed once
  (`uniFont`), 16-wide glyphs squeezed; "VGA + Unifont" (`univga`, default) draws CP437's characters in the
  VGA font (TestUniConvertedLooksTheSame: pixel-identical after converting). Missing glyph: a small box.
- Tests: `uni_test.go` (cell, styles, conversions, all three formats round trip, real files, detection, editing,
  flips, paste, autosave, style table compaction/full, HTML, drawing, Animate, own files), FuzzParsers has the
  three readers. TestStartKeys now works for any number of tiles.
- Not done: wide characters as two cells (CJK, emoji: not typable), Animate's draw orders in UTF-8 ANSI (frames
  work), a Unicode page's own fonts (Load a font... loads a 256-glyph font), PC fonts other than VGA for
  "VGA + Unifont", 9-pixel cells, CharView playing .dur frames.

## UTF-8 files on PC and Amiga pages are saved in UTF-8 (2026-10-16, on next)

- Before: a UTF-8 ANSI whose characters all fit CP437 opened as a PC page (right), but Ctrl+S wrote it in CP437,
  so `cat` in a terminal showed garbage (*verified* on 0.1b7). Now `loadBytes` turns saveFmt "ansi" into "ansiutf8"
  and "text" into "utf8" when the file was read as UTF-8 (`fromUTF8`).
- "ansiutf8" = `writeAnsi(..., keep1A[2]=true)`: every cell as its CP437 (or Latin-1) character in UTF-8, control
  glyphs too (☺ ○ ◙ ← cannot be taken for controls in UTF-8, so nothing becomes a space), every row in full and
  ended by LF, colours as 1;3x/4x like the ANSI save, **no SAUCE** (durdraw's ANSI save writes none - *read* in its
  saveAnsiFile; Moebius's UTF-8 export has none - its test files): the lines give the width. Moebius's UTF-8 test
  files round-trip cell for cell (TestMoebiusRoundTrip), `utf8save_test.go` pins the save format.
- Not done: a multi-frame page in ansiutf8 writes the shown frame only (the ANSI stream writers are CP437).

## BBS colour codes (2026-10-16, branch r1-bbscodes)

- **The user's decisions (2026-10-16):** a file mixing codes and ANSI saves back in its codes; bit 7 as `|24`-`|31`
  (the save counts cells Mystic 1.10 / Renegade will not show: `bbsLoss.bright`); a literal `|` as `|PI` (Mystic
  first); detection stays strict (3+ codes; Open as for short files); "Save for board X" writing the board's own
  colour codes: wanted, on the roadmap.

- `bbscodes.go`: five kinds, ids = format ids = Open as values (`bbsCodeIDs`): `pipe` (|00-|31, |CL, |PI),
  `ctrla` (Synchronet Ctrl-A), `wwiv` (Ctrl-C hearts, |nn, |#n), `wildcat` (@BF@, @CLS@), `celerity` (|k..|W,
  |S). Reading: `codeTerm` wraps the PC `Term` and takes each kind's codes out of the byte stream before the
  terminal (only while `Term.state == 0`, so ANSI escapes pass untouched: mixed files work); colours are set on
  the Term's own fields (`setFg/setBg/setAttr` in BIOS order, `bios()` reads them back), clears/moves are fed to
  the Term as ANSI. A code cut by writeWatching's chunks waits in `pend`; `flush` at the end. Hooked in by one
  field, `ParseOpts.codes` (parseArt; UTF-8 width guessing is off with codes) and `Tab.codes` (reparse passes it).
- **Shown as text**: a code the board acts on but CharEdit cannot show (pauses, MCI like |UN/|PO, Ctrl-A P , ; .
  T D, WWIV |@ macros, ☼ macros, @FIRST@) is not a code to the reader; the writers write those characters raw
  (e.g. ☺ before a letter `ctrlaActs` rejects), so the bytes go back as they came. `pipeActs`, `ctrlaActs`,
  `wildcatActs` are shared by reader and writer: keep them in step.
- **Writing** (`writeBBSCodes`): codes where the attribute changes (`appendCodes`; Ctrl-A: N only when bright or
  blink must go off, I or E (ice) for bit 7), CR LF after rows that do not fill the width, the board's clear if
  Info's clear is set, normal colours at the end. Escapes: pipe `|` -> `|PI` when it could join a code or meet one
  within two bytes (Mystic takes any two bytes after a `|`, printing all three when they are no code; also at
  the end of a full row: `cellAt` looks into the next row); WWIV `|` before a digit/`#`, Wildcat `@` before two
  hex digits or CLS, Celerity `|` before a code letter: the same colours written again right after it
  (`force`); Ctrl-A ☺ -> Ctrl-A A, 0x1A -> Ctrl-A z. Loss (`bbsLoss`): Celerity has no bit 7, WWIV cannot write ♥
  (a heart + printable byte is always a code there): spaces; the save's message says how many.
- **Loading** (`loadBytes`): `bbsCodesKind` after the PCBoard/Avatar check, or Open as a kind. `saveFmt` = the
  kind, also with ANSI escapes in the file (the user's choice 2026-10-16: a Mystic file keeps its pipe codes) -
  but only when the codes hold the whole page (`writeBBSCodes` loses nothing: no ♥ for WWIV, no bit 7 for
  Celerity); else ANSI (the big check's 1990 WWIV ANSIs with heart footers that also draw ♥). Frames are written
  one after another, each after the board's clear; `splitStream(..., codes)` reads a codes file's frames with its
  codes (the app's Split did not before: fixed 2026-10-16). A .d64 with no screen of ours is a refusal
  (`errD64NoScreen`, the same words as a foreign .prg), not a load failure. Ctrl-A E sets the page's iCE. Save in another own format clears
  `Tab.codes` (writeOut). Open as now also lists PCBoard and Avatar; the C64 entry is skipped (not cut off) when
  the ROMs are missing (`shown` in dialogs.go).
- **Detection** rules (checked on the big check's 8479 files: only 13 WWIV ANSIs from 1990 with real hearts are
  taken, test `TestBBSCodesBigCheck`): 3 codes (1 with .msg/.bbs), at least 2 different and more than different+1
  (text art numbers its boxes |01| |02|), codes >= 1/4 of the file's | or @, Ctrl-A/hearts >= 1/2 of the ☺/♥
  bytes, a heart takes one digit not followed by a digit or comma (mIRC), no other machines' endings, no binary.
  Celerity is never guessed.
- Tests: `bbscodes_test.go` (each board's table, round trips of random pages heavy in | @ ☺ digits and code
  letters, unknown codes kept, shortest codes, losses, detection, open/save/Open as in the app, a model of Mystic
  1.10's OutFile showing the pipe writer's files as the page). Real files when fetched (skipped otherwise; set
  `CHARCOAL_BBS_FILES`, default /tmp/charcoal-bbs): IcyDraw's benches/data Members01 in .pcb/.avt/.an1/.msg give
  one page (`TestBBSCodesIcyDrawMembers`; IcyDraw writes @ as @@ in Ctrl-A, let off), Synchronet's text/menu
  (85 Ctrl-A files, every board's look) and WWIV's install/gfiles round-trip (`TestBBSCodesRealFiles`). Fuzz:
  every kind in FuzzParsers. How to fetch them and the board facts: the knowledge note on BBS colour codes in the knowledge base (bbs/).
- Not done: PCBoard's @X00/@XFF save/restore (pcformats.go reads them as colours);
  Telegard's backtick codes; Renegade's ^0-^9 user colours and Mystic's |T0-|T9 theme colours (shown as text);
  Animate a folder counts a codes file as "other" (written as .ans with ANSI colours).

## C64 BBS graphics and the boards' own colour codes (2026-10-16, branch r3-c64bbs)

- **The user's decisions (2026-10-16):** Image BBS is in "Save for board" with `£Sx` tenths only; no `£W` (its
  first second is short, and 2.0 does not wait); Color 64 stays out (no timed pause); Image colours as plain
  PETSCII bytes, not `£C`.

- **What**: "Save for board X" (bbspause.go's boards: Settings' My BBS, Animate's choice and copies, Animate a
  folder) now covers two C64 boards for animated `.seq` files, and the PC boards write their own colour codes.
  `.seq` files with Image BBS's £ codes open with them and save back with them (format "imageseq").
- `c64bbs.go`: boards appended to `bbsBoards` in `init` with a new field `paceFn` (the board paces its own way;
  `pace` calls it first). **Image BBS** (`paceImage`, `imageLayout`): a pause of n tenths is `£S`+digit before
  the next byte that shows (`printable`: &0x7F >= 0x20; Image 1.2 slows only those, 2.0 every byte) and `£S0`
  after it; more than 15 tenths spread over the next characters; the slowed characters and their codes are one
  piece, so no filler and no control byte is ever sent at a speed. Every £ of the art is `££`. Lines: Image's
  dskin reads 80 bytes or up to a RETURN (not kept), so no piece is cut (filled with `£B0`, 0 bells = nothing,
  moving 1-2 art bytes to the next line so the fill is whole codes; reverse no-op bytes only when nothing can
  move), `£B0` before a line that would start with ↑ (read0 stops there), and a RETURN at the end (the last
  byte comes with EOF status, and dskin then turns a line without RETURN into "↑": lost). `£W` is not used (first
  second short; in the 2.0 source `comw` compares the seconds register with itself and returns at once).
  **C*Base** (`paceCBase`, `cbaseLayout`): a unit is Ctrl-P (0x10) + a reverse code that changes nothing
  (`petState.noop`); waiter2 waits for the jiffy clock's `& 6` to change, so ~1/30 s (its manual says 1/10 s).
  After 0x10 C*Base prints the next byte without the MCI check: in quote mode (where a no-op reverse would print)
  the 0x10 goes before the next art byte instead, and a £/@ there is written after a 0x10. Elsewhere every £ and
  @ gets a no-op reverse after it (`£N T S V O P R L W I B` and `@:` are codes; both look at the next byte, even
  past the line). Lines of 255 bytes with the RETURN kept.
- `boardcolours.go`: `boardColourer(kind, stream)` (nil unless every SGR has a code: italic/underline never,
  blink not for Mystic 1.10 which eats `|24`-`|31`), `code()` turns each SGR into `appendCodes` (pipe, ctrla) or
  `@Xnn`, written only when the attribute changes. Boards get it by the `colours` field (Mystic "pipe", Synchronet
  "ctrla", PCBoard "pcb"). PCBoard also: art `@X` + two hex digits gets `@DELAY:0@` after the `@` (before, the
  board took it for a colour; the old test model did not look at @X).
- **Image's codes read** (`imageTerm` in front of `PetTerm`, like `codeTerm`): `£C1`-`£CO` colour (the
  command is the byte's low 6 bits, the number its low 4: `£Ca` = `£C1`), `£F` clear, `£R` reverse, `££` a £.
  `£C0` (the caller's default colour) and everything else (`£V2` name, `£S`, `£W` ...) stay on the page as text.
  `imageEscape` (the writer) doubles a £ only where `imageCodeAt` would read a code, so text codes go back as they
  came. `isImageCodes` (detection, only on C64 pages with the ROMs): >= 3 codes typed as letters with a digit/J-O
  after them, >= 2 acting, >= 2 kinds, at least half of the £ in codes; 0 of 239 petscii-bbs screens and 627 big
  check C64 files taken. Glue: `parseSeq(b, cols, codes...)`, `splitStream` c64 case, `docOf`, `loadBytes`
  (`as == "imageseq"`, detection after the mode, saveFmt), `encodeFor` hook `encodeImageSeq` (an animated page
  writes its `.seq` animation; escaped unless a C64 board paced it), `writeOut` (own + codes), Open as list
  (hidden without the ROMs), `framesFormats`, `animatedSave`/`copyBoards`/`writeCopies` treat imageseq as seq.
- Tests (`c64bbs_test.go`): models of Image's read0/dskin/MCI and C*Base's readln3/bufout2 (fail on a cut code,
  art taken for a code, a lost last line, a control byte at a speed); every draw order, effect and frame strip of
  random pages full of £ @ : ↑ " through both, the terminal bytes = the plain stream minus no-op reverses, the
  time within half a unit; layouts and escapes byte for byte; colour codes in Mystic/Synchronet/PCBoard files,
  blink and italic keep ANSI; reading/writing Image codes, in the editor (open, Ctrl+S, Open as, animated saves +
  a C*Base copy); Animate a folder for both boards; the 239 petscii-bbs `.seq` (`CHARCOAL_C64BBS_FILES`, default
  ~/.cache/charcoal-c64bbs/petscii-bbs, skipped when missing). bbspause_test.go's models now act on the colour
  codes (sending an SGR), `checkPaced` compares through `cbbsCanon`. FuzzParsers reads "seq image".
- **Traps**: a real `.seq` with a stray `"` leaves quote mode on to the end of its line; a filler there would print
  (only when a 2-byte C*Base piece meets the end of a 255-byte line in quote mode; the real-file test puts its
  pauses after a RETURN). Test pages need `glyph0 = true` (screen code 0 is @) or `writeSeq` drops trailing @s.
- **Open questions to the user**: Image's `£W` for long holds (whole seconds, unreliable per the 2.0 source); Color
  64 (no pause code, only `£v` speed); writing Image's `£C` colours instead of PETSCII's (CharEdit writes PETSCII's:
  the board passes them to C/G callers, and £C is for typists on other machines). Nothing was run on a real board.

## Amstrad CPC pages (2026-10-16, branch r1-cpc; roadmap step 5)

- `cpc.go`. Page type "cpc" (`a.cpc()`). `Tab.cpcSet cpcSetup` = mode (0/1/2), `ink [16]int` (firmware colours
  0-26), border, `ok`; it belongs to the page, not to a Doc (Doc copies in withCols/blankLike/... would lose it).
  `Snap.cpc` (pointer, nil on other pages) makes ink, border and mode changes undoable; `restore` applies it.
  Cells: `makeCell(ch, pen, paper)`, pens/papers are ink numbers, masked to the mode (`cpcSetup.mask`) when drawn
  and written, as the firmware does. `setPalette` makes `a.pal` from the inks (no palette choice on the page).
  Empty cell `cpcBlank()` = space, pen 1, paper 0. Mode 0 cells are 16x8 (`cpcWide` doubles glyph bits in
  `drawCell`), mode 1 8x8, mode 2 8x16 (doubled rows): each mode in the CPC's shape. 25 rows fixed, width by mode.
- Colours: `firmware n = 9 green + 3 red + blue`, levels 00/80/FF (`cpcColour`), names in `cpcColourNames`.
  Power-on inks `cpcStartInks`, border 1.
- Font: `fonts/amstrad-cpc-charset.font` = OS ROM 0x3800-0x3FFF (same in 464/664/6128, from Caprice32's ROMs),
  notice in `fonts/amstrad-cpc-NOTICE.txt` (the COPYRIGHT.TXT condition: keep the acknowledgement). Own fonts and
  the font editor are refused on CPC pages (custom characters break the compo rules).
- .bin: `writeCPCBin` = AMSDOS header (`amsdosHeader`, checksum of bytes 0-66) + `cpcStub` (21 bytes at 0x1000:
  LD HL,stream / LD BC,len / loop TXT OUTPUT with BC pushed / JP KM WAIT CHAR) + `cpcStream` (4 mode, 29 border, 28
  every ink of the mode, then cells with 14 paper / 15 pen on change, no pen for a space, no paper for 0x8F, 1 before
  0-31). Reading (`parseCPCBin`): `cpcArtStream` finds `LD HL,nn` + a CALL BB5A in the first bytes (Logiker's two
  stubs and ours), `LD BC` after it gives our length; `cpcScreen` is the text VDU (codes and parameter counts from
  the Firmware Guide; scroll, LOCATE, CLS, clears, swap, transparent, SYMBOL/windows/flash noted in `note()`).
  **Trap**: Logiker's streams end with 0x1F and a file on a disk has junk after it: `print(b, program=true)` stops at
  a 0x1F where a code is due; a bare stream (Open as Amstrad CPC, `srcKind ""`) reads 0x1F as LOCATE.
- .dsk: `writeDSK` Extended DSK, 40 tracks x 9 x 512, sectors C1-C9 interleaved C1 C6 C2 C7 ..., directory in
  logical sectors 0-3, 1 KB blocks from 2, extents of 16 KB; the file padded with 0x1A to 128. `readDSK` reads
  Extended and standard images (side 0), `dskArt` the first art .bin. Name inside = the saved file's name
  (`cpcTarget`, set in writeOut).
- Playscii: `parsePsci` hands CPC charsets (`isCPCCharset`: CPC_Mode0/1/2, glyph 0 = space; old "cpc" = codes
  32-255) to `psciCPC` (own layer merge; inks allocated in order of first use, paper before pen, as the exporter
  does; border from selected_bg_color-1); `writePsci` hands cpc pages to `writePsciCPC`. `psciArt.cpc` carries the setup.
- Glue (kept small for merges): `init()` in cpc.go appends to pageModes, modeOrder, modeNames, openAs, modeNoStream,
  editorOwn and adds " cpc" to the modes of the utf8/png/png2/gif/apng/psci formats; editor.go: Tab/Snap fields,
  the cell size/font/palette/printable/runes/maxRows/nColours/defaultColours/setMode/makeTab/setRows/setCols
  cases, `loadCPC` after loadEditor, `reparseCPC` with reparseEditor, `encodeCPC` before encodeFor's switch, two
  format lines; small cases in view.go (drawCell, Mode gadget, no palette gadget), panel.go (`drawCPCColours`),
  anim.go (`borderPx`), rowscols, strip, frames, effects, shapes (box lines 9A 95 96 9C 93 99, shades 20 90 CF 8F),
  fontfile/fontedit, start (tile), menus, main, autosave (`cpcAutosave`), fuzz_test.
- Tests (`cpc_test.go`): font SHA-1 and glyphs, colours, runes; .bin round trip in every mode run through a small
  Z80 interpreter (`cpcFZ80`: the stub prints exactly the stream); the exporter's two stubs + its stream layout
  read back and agree with what its program prints; control codes; .dsk round trip and 3 extents; .psci round trip
  and the old charset; the editor (typing, undo of inks/border/mode, .bin/.dsk/.psci save and reopen, Ctrl+S,
  autosave, PNG border); `TestCPCRealFiles` with `CHARCOAL_CPC_FILES=dir` (2024 compo entries from
  files.scene.org/browse/parties/2024/amstrad_ascii_compo24/: 26 files open; renders were pixel-identical to the
  entries' Caprice Forever PNGs, offset 64,82 in their 768x544); `FuzzCPC` and FuzzParsers' cpc checks.
- **Pictures** (PNG, GIF, APNG) are Caprice Forever's screenshot (2026-10-16, the user's choice): 768 x 544 in every
  mode (`cpcZoom` doubles modes 0 and 1), the screen at 64,82 (`borderSize`: left/right w/10, top h*82/400, bottom
  h*62/400); "twice the size" is 1536 x 1088. *Verified*: the whole PNG, border included, is byte-identical to 9
  compo entries' own Caprice screenshots (the others' screenshots came from other tools: 1280x800, 768x576,
  320x200). A .psci has no border colour, so its picture differs there.
- Not done: CharEdit's .bin has not run on a real CPC (it runs in MAME: see "Machine files in real emulators"); play (m) of the stream;
  BASIC listing; flashing inks shown as their first colour; custom characters (SYMBOL) ignored.

## Minitel and Viewdata pages (2026-10-16, branch r2-videotex)

- **Minitel** (`minitel.go`, `minitel_ui.go`): page type "mt" (`a.mt()`), 40x25 cells of 8x10 (`mtW`, `mtH`), row 0
  the status row. The model is the Minitel's page memory as the STUM 1B gives it (the spec is online as text:
  jbellue.github.io/stum1b; its tables are SVGs under img/p100-p104): a cell is a letter (`cellCh` 0x20-0x7F G0,
  0x80+ the 30 characters of `mtExtra`: G2 signs and the accented letters a VGP5 Minitel shows) with colour,
  `mtBlink`, `mtInv`, `mtDW`/`mtDH` and which part (`mtRight`, `mtTop`); or with `mtG1` a mosaic (code as teletext
  writes sixels, 0x20-0x3F/0x60-0x7F) with colour, background, `mtLine` (= separated) and flash. **Zones**: a
  letter has no background of its own; `mtDecode`/`mtRow` give it the background, underline (`mtLine`) and
  conceal (`mtMask`) of the nearest space or mosaic to its left (a space whose zone bits differ from the zone it is
  in starts a zone; a mosaic gives its background and ends underlining). `mtSettle` writes those into letters and
  plain spaces, so a page's cells say what is shown. Empty cell `mtBlank` = black mosaic space (what FF fills
  with). Colours in Minitel order (0 black, 1 red, 2 green, 3 yellow, 4 blue, 5 magenta, 6 cyan, 7 white);
  palettes `minitelgrey` (the STUM's grey levels, the default) and `minitel`.
- Brush codes 0xC0-0xFF are the 64 mosaics (0xC0 + blocks): the character table's last quarter, the F-keys
  (`mtFkeys`), `mtCellOf`/`mtBrushOf`; `mtCellAlone` draws one. Typing (`mtType`) puts big letters on two or four
  cells, written from the bottom row like the Minitel (not on rows 0-1, not at column 39); toggles in the tool
  rail (`drawMTToggles`): Flash, Inv, Line, Mask, size, Reveal (= `showCodes`). Sixel tool: `mtPaintSixel`.
- **Reading** (`mtTerm`, `parseVDT`): a small Minitel: C0 codes (BS HT LF VT CR FF RS US CAN SO SI REP SS2 SUB),
  ESC + C1 attributes, CSI (cursor, J/K clears, P/@ characters, M/L rows), PRO1-3, the whole-screen mask, US to the
  status row and LF back, DRCS definitions skipped (US 0x23 ... up to the next US). It keeps which spaces were
  delimiters (`delim`) and settles at the end. Traps found on real pages: **G1 codes 0x40-0x5F show the mosaics
  0x60-0x7F** (pages use `_` for the full block; the STUM table leaves them blank, MiEdit and VDT2BMP show them);
  **0x16 works as SS2** (TF1's page writes "Télématique" so; VDT2BMP does the same); a control code after SS2
  drops the SS2 (the STUM; a fuzz find). Screens a clear (FF, CSI 2J) ends become frames (`maxSplitFrames` cap).
- **Writing** (`mtWrite`): FF, rows 1-24 left to right, then the status row (US 0x40 0x41 ... LF back). The writer
  runs its own stream on an `mtTerm` as it goes (`mtWriter.emit`), so it always knows the Minitel's state: cursor
  moves by HT, CR LF or US (US resets the attributes, CSI C is not used: VDT2BMP ignores it); a space that must
  start a zone gets its ESC 0x5x/Z/Y/X/_ first (and one forced if nothing differs: the STUM's "the first space
  after an attribute is a delimiter"); runs of 4+ use REP; big letters are written from their origin. Before a
  letter, `context` makes the terminal's current background/underline/conceal those of the letter's zone by
  writing the zone's start again, so a terminal that keeps attributes per cell (VDT2BMP) shows it the same.
  Then up to 4 passes put right any cell still wrong (big letters over each other, lone parts: written from
  their origin). 149 real pages: all come back cell for cell, CharEdit's streams are 15% smaller.
- **What is not certain** (verify on a real Minitel or MAME's minitel2): VDT2BMP (jfdelnero's converter,
  github.com/jfdelnero/minitel) draws 25 of the 149 pages differently from CharEdit and MiEdit: it keeps the
  background and underline current when a letter is written (per cell) and resets the mosaic background on CR,
  where the STUM's zones take them from the page. Example for the STUM's reading: 3614.vdt cycles a delimiter's
  background to white so that "3614" shows white on the banner; VDT2BMP never shows it. CharEdit's own streams
  differ in VDT2BMP on 19 of 148 pages.
- **Viewdata** (`viewdata.go`): page type "vd", and `a.tt()` is true on it too (everything teletext does); only
  24 rows (`newVDDoc`, `maxRows`, `vdCut` for a teletext file opened as Viewdata). `vdScreen`/`parseVDStream`
  (CS starts a frame), `writeVDStream` (rows trimmed, CR LF only after a row shorter than 40, as Telstar's
  RawTToRawV; 7 bits), `parseVDBin`/`writeVDBin` (960 bytes; attributes written 0x80-0x9F), Telstar frames
  (`isTelstar`, `parseTelstar`: content types rawV/raw, rawT, edit.tf; markup is not read; `writeTelstar` keeps
  the old frame's fields, a new one gets pid from the name `200a.json`). Telstar keeps row 0 for its header and
  row 23 for system messages. Hold times on Viewdata frames are in seconds, as teletext's.
- Glue (kept small): `init()` in both files adds to pageModes, modeOrder, modeNames, openAs, modeNoStream,
  mirrorPairs, formats (and " mt"/" vd" to png/png2/gif/apng/utf8, " vd" to tti/tturl), fonts and palettes;
  editor.go: `tt()` covers "vd", cell size/font/palette/printable/runes/maxRows/nColours/typing/pick/sixel/
  setMode/makeTab/setRows/setCols/blockOp cases, `loadVideotex` after loadCPC, loadC64x and loadIGS, `reparseVideotex` with the
  others, `encodeVideotex` before encodeFor's switch, Play (`videotexPlayer`, `videotexPlayBytes`); small cases
  in view.go, panel.go (eight colours on Minitel), rowscols, start (two tiles; tile text at 2x for 10-row
  cells), menus, main, frames, shapes, transitions, streams, fontfile/fontedit, uni_ui, prefs (defaults),
  dialogs (help), viewer (`.vdt .cpt .car` in a folder).
- Tests: `minitel_test.go` (font, stream basics, random pages round trip, big letters, `TestMinitelRealFiles`
  with `CHARCOAL_VDT_FILES=dir` (and `CHARCOAL_VDT_OUT=dir` to keep CharEdit's streams), `TestMinitelLooksRight`
  with `CHARCOAL_VDT_REF=dir` of x.vdt + x.bmp made by VDT2BMP (`-bmp:x.bmp x.vdt`; it reports, the models
  differ; `CHARCOAL_VDT_DUMP=dir` writes side-by-side PNGs), the editor, Play, `FuzzVideotex` (any stream is
  written back so it shows the same); `viewdata_test.go` (round trips of all three formats, MacViewdata bytes,
  carousels, Telstar fields, `TestViewdataTelstarFrames` with `CHARCOAL_TELSTAR_FILES`); FuzzParsers reads
  .vdt/.cpt/.bin/Telstar and checks the .vdt write-back. Samples used (not in the repo): 151 .vdt from
  github.com/ludosevilla/minipaviCli, BwanaFr/minitel-server, jfdelnero/minitel, cquest/pynitel,
  iodeo/Minitel-ESP32; Telstar's own frames from github.com/johnnewcombe/telstar-2.
- Not done: DRCS characters (skipped), the 80-column (Mixte/Téléinformatique) mode, Telstar markup, a .car of
  960-byte frames as frames (only streams), Viewdata pages from IcyDraw files, a run on a real Minitel. Play
  goes at the toolbar's speed (2400 is the slowest; a Minitel is 1200).

## C64 beyond PETSCII (2026-10-16, branch r2-c64plus)

- **The user's decisions (2026-10-16):** formats that cannot hold a custom set or mode (.seq .c .petmate ...) save
  anyway with a note, the mode remembered for the file (as fonts); CharEdit's own .prg (set at $2000) only when
  the page needs it, Petmate's form otherwise; ECM: codes 0-63 take the "Paint on" background, characters picked
  from the table keep theirs; no KickAssembler export until its output can be checked (needs Java); CharPad
  multicolour follows the VIC rule (colour 8-15), which the c64lib projects agree with in every version 5-8.2.

- `c64x.go` (mode, drawing, gadgets, kept-for-the-file, font editor pairs), `c64x_prg.go` (CharEdit's .prg, ACME
  and BASIC with sets/modes, `encodeC64x`), `c64x_d64.go`, `c64x_ctm.go` (CharPad). `Tab.vic vicSetup{Mode,
  Regs}` (0 hi-res, 1 multicolour, 2 ECM; Regs = $D022-$D024; `defaultVIC` = KERNAL's 1,2,3, set in makeTab and
  by `loadC64x` at the start of every load); `Snap.vic` (undo), `autosaveMeta.VIC`, `prefs.VICModes` (path ->
  setup for formats that cannot hold it: `keepVICMode` after writeOut, `restoreVICMode` after openPath).
  `vicII()` = C64 or C128 40 (by `mach().id`: `a.machine` may be "" = C64). `App.c64xOpts.ecmBg`.
- Drawing: `drawCell` hands any non-hi-res VIC page to `drawVICCell` -> `vicCell` (MCM: colour 8-15 = pairs 00
  $D021 / 01 $D022 / 10 $D023 / 11 colour&7, 0-7 hi-res in colour&7; ECM: glyph code&63, bg = [$D021,
  $D022-$D024][code>>6], fg 0-15). From the VIC-II data sheet / PRG ch.3 (*read*); VICE not installed here,
  so not compared with a VICE screenshot.
- ECM brush: `ecmChar` gives codes 0-63 the "Paint on" background (`a.cell` and `brushCell`); codes >= 64
  picked from the table keep theirs; the Colour tool's right button sets bits 6-7 (`paintAt`). Reverse is
  ignored in ECM (bit 7 is a background bit there).
- **.prg**: plain ROM screens keep Petmate's form (`writePrg`); a page with its own set or MCM/ECM gets
  `c64xPrg`: 10 SYS2061, code at $080D (blank, copy 8 pages of set to $2000 when present, copy screen/colours
  like Petmate, set $D020-$D024, $D018 ($18 own set, $14/$17 ROM), $D016 ($C8/$D8), $D011 ($1B/$5B), JMP *),
  data at $088C/$0C74/$105C. Fixed template + slots (`c64xSlot`), so `parseC64xPrg` reads only that form
  (4189 or 2141 bytes). $2000 because bank 0 shows the char ROM at $1000-$1FFF. `readC64Prg`, `isOurC64Prg`.
- **.d64**: `writeD64` (35 tracks, one PRG $82, 1541 order: tracks 17 down then 19 up, interleave 10, BAM with
  "2A", disk id "CE", names PETSCII upper, $A0 padded; name = saved file via `cpcTarget`), `d64Dir` (loops
  guarded, 35/40 tracks with or without error bytes), `d64Art` = first PRG CharEdit can open. srcKind "d64" via
  `editorOwn`; `c64xAfterLoad` gives prg/d64 pages their mode and set.
- **ACME / BASIC**: `writeC64Asm` = Marq's source plus the registers after `sta $d021`, `#91` for ECM, and the
  set at `*=$2000` (ACME fills the gap); data-only (not 40x25) adds a mode comment and `charset:`.
  `writeC64Bas`: a set goes to VIC bank 3 (screen $C400, set $C800, `poke 56576,...and252`, `poke 53272,20`,
  `poke 648,196`) since 20 KB of DATA fill BASIC past $2000/$3800. Plain pages give Marq's bytes unchanged.
- **CharPad .ctm** (v5-v9; formats in c64x_ctm.go's head): `parseCTM` -> `ctmProject`; as a font
  (`parseFontFile` .ctm, then `ctmFontLoaded` sets mode/regs/bg); as a page (`loadC64x` -> `loadCTM`: srcKind
  "ctm", saveFmt "prg", path cleared so Ctrl+S asks; `reparseCTM`). v8 prerelease (4-byte colour entries) told
  by the byte after the colours not being $DA (retro-image's rule). v9's bg4 position and ECM maps are *proposed*.
  MC colours follow the VIC (8-15 = multicolour); CharPad itself was not run. Writing .ctm is refused.
- Font editor: `fontEdChar` (ECM: brush&63), `fontEdPairs` (MCM and fg >= 8): `drawPairs`, `penGadgets`,
  keys 1-3.
- Tests (`c64x_test.go`): a 6502 interpreter (`c64xF6502`, checked on Petmate's own program) runs every
  .prg combination (own/ROM x 3 modes x case) and ACME's output of `writeC64Asm` (`CHARCOAL_ACME=path` or acme
  in PATH; built from github.com/meonwax/acme for this, skipped otherwise); BASIC DATA parsed back; .d64 BAM /
  chain / directory; CharPad files of every version give version 5's page; editor (undo, ECM painting, .prg/.d64
  save/open/Ctrl+S, .seq keeps the mode, autosave); `TestC64xMutations` (20000 mutated files; the fuzz engine
  stalls in this package), `FuzzC64x`, FuzzParsers has prg/d64/ctm.
- Not done: ~~DirArt (directory art) on the .d64~~ (done 2026-10-16: "Character sets: MZ-700, Plus/4, DirArt"); a run on a real C64 (VICE: see "Machine files in real emulators"); Petmate/Marq .c files with modes
  (neither editor has them); CharPad writing; Petmate JSON with a custom charset.

## Atari ST IGS (2026-10-16, branch r2-igs; design: docs/igs-design.md)

- What: IGS files (`G#` commands drawing with GEM VDI inside a VT52 stream) open as an Atari ST page whose
  drawing is a **picture under the cells** (`Tab.igs *igsLayer`, `a.igsPage()`). An empty cell (`isBlank`:
  space/0 on pen 0) shows the picture (`drawCellXY`, called by drawPage and `paintCellsAt`, so PNG/GIF exports
  and CharView have it); other cells are VT52 text drawn over it. `setPalette` takes the file's registers.
  Typing goes over it; saving `.ig` (`writeIGS`) = the drawing's bytes + CR + the cells as VT52 (`ESC q ESC w`,
  `ESC Y` per run, pens where they change; a run is cut before a '#' that follows a 'G') + the kept commands
  (`G#...:` + CR each) + the closing. **Unchanged cells save the source bytes exactly.** Not editable: the drawing.
- `igs.go`: `igsParse` cuts the file into items (commands, VT52 runs, ESC E alone). Values are read IG 2.18's
  way (CR/LF inside a value ignored, `_` too); a command ends at `:` (then chained: the next letter needs `>` or
  a space and a value - so `p>24,19:We all` prints "We all", the spec's example, and `k 1:` is a command), or after
  its value count (`igsWant`). W's text runs to `@` (CR/LF dropped, IG 2.20). Loops (`&`): tokens, chain gangs
  (`>CL@`, index then values, `)` or the letter separating), `W@` texts, `x y + - !`; every group of a chain gang
  runs (IcyDraw runs only the first: its 00-IGNIT render differs there, 99.6%). X 7 patterns, X 11 bitmaps (the
  32000/4000 raw bytes after the command, ST planar, `planar`), X 12 palettes. `igsVM` runs commands into
  `pix` (registers); VT52 on the pixels (`vt52`, `putGlyph`, scrolling). Random values: fixed seed. A work budget
  (`igsWorkMax`, 40M; real files <= 18M) stops hostile files; the note says so.
- `igs_vdi.go`: the port of IcyDraw's vdi_paint (icy_tools' own code, MIT; nothing from EmuTOS but the fonts, downloaded): Bresenham lines with masks (always replace mode,
  as IcyDraw), wide lines with the pen's quarter circle and arrow/round ends, polygon scan fill bottom-up,
  `igsCurve` (1800-step sine table), rounded boxes, flood fill, polymarkers, text (6x6/8x8/8x16, scale 2 above
  size 15; thickened/ghosted/skewed/underlined/outlined; 4 rotations), blits (logic ops masked to the planes).
- `igs_page.go`: `readIGS` splits the file: **head** = up to the last drawing command or wipe (`s`, `I`, `R`,
  ESC E) before the last visible thing; **text part** = VT52 + IG's text commands (`c p m l d i r v w`) into
  cells via `STTerm` from the head's cursor/colours, other commands **kept**; **closing** = from the first wipe
  after the last visible thing (BBS screens end "pause, sound, clear, reset"). Palette shown: of those the
  kept commands go through (fade-out, reveal, cycling), the one with most distinct colours on the picture, then
  the brightest, later wins ties (`igsBestPalette`). Resolution: R before any drawing, else a probe run
  (`vm.probe`, loops expanded) of the coordinates (x > 319 medium, y > 199 high), else low; the Resolution
  gadget re-reads in another (`reparseIGS` uses `a.stres` when `forceCols` is set). A head that ends in an X 11
  cut short is padded with zeros so text added after it is not read as bitmap.
- Fonts: EmuTOS `fnt_st_6x6.c` added to `stFontFiles` (SHA-1 `stFont6SHA1`; `loadIGSFont6`, 192-byte rows);
  `hasSTFont` still means 8x8 + 8x16 (old downloads keep working); opening an IGS file asks for the download when
  the 6x6 is missing. All three are byte-identical to IcyDraw's Atari fonts (checked).
- Glue: `init()` in igs_page.go (formats "igs" `.ig`, editorOwn, Open as "Atari ST IGS"); editor.go: Tab.igs,
  `loadIGS` after loadCPC (and `a.igs = nil`), `reparseIGS` with reparseEditor/CPC, `encodeIGS` after encodeCPC,
  setPalette; view.go: drawCellXY in drawPage, paintCells -> paintCellsAt (the float passes its page position);
  rowscols.go: fixedHeight; st.go: the 6x6 font; dialogs.go: "three small files".
- Tests (`igs_test.go`): IcyDraw's 14 test scripts + renders in `testdata/igs/icydraw` pixel for pixel; parsing
  (chains, W text over lines, values over lines, chain gang, W@); drawing (lines, loops, fill, palette words, X 11
  planar, X 7 patterns, a hostile loop's time); resolution guesses; the split (head/cells/kept/closing, fade-out,
  reveal, ESC E before text); app round trip (open, unchanged save = source, typed + erased cells, PNG shows the
  picture, reopen identical, .vt, plain page as .ig); detection; `FuzzIGS` (typed cell must read back; 2 min
  clean; its 5 regression inputs in testdata/fuzz/FuzzIGS) and FuzzParsers' "igs". External:
  `CHARCOAL_IGS_ICY=<icy_tools>/crates` compares all 79 of IcyDraw's renders (65 at 100%; the rest are files
  IcyDraw itself skips), `CHARCOAL_IGS_FILES=dir` round-trips real files (46 from IcyDraw's benches, the IG 2.20
  disk, GoF: all pass), `CHARCOAL_ST_FONTS=dir` gives the tests the three EmuTOS font files.
- Not done: editing the drawing (only text over it), sound/music (`b n N`), pauses and timed playback (`t q`,
  loop delays: everything is drawn at once), XOR stepping in loops (as IcyDraw: ignored), mouse zones/input
  (`X 3-6`, `<`, `?`), `X 9` MIDI buffer, `g` scaling, `I 5`, text below the screen's last row (cut, the ST would
  scroll), autosave keeps the cells only (the picture comes back when the file is opened again), Split/frames
  on IGS pages, the Animate draw orders in .ig. A space on pen 0 in the text part shows the picture (on the ST it
  paints pen 0): the only place the page differs from the ST.

## Plus/4 and C16 pages (2026-10-16, branch r3-ted)

- **The user's decisions (2026-10-16):** a new Plus/4 page starts in the power-on colours (black on white,
  border $6E); a .seq opened on a Plus/4 page starts in white; Petmate9's 0x16 full-colour code is read, never
  written; a .scrn save writes NAME.proj beside it; YAPE/Colodore colour values from VICE's .vpl files are fine;
  flash stays in the BASIC and ACME exports.

- **Model**: machine "plus4" in `machines` (mode "c64", `a.ted()`), so every PETSCII path (typing, F-keys,
  shapes, frames, Animate, Play, Split) works. A cell's bits 8-15 are the **TED attribute byte**: cellFg = hue,
  cellBg = luminance | flash<<3 (`tedAttr`, `tedCell`); `Doc.bg`/`Doc.border` are whole TED colours (0-127), so
  **never index `a.pal` with a Plus/4 page's bg/border**: use `a.screenRGB(c)` (TED colour or `a.pal[c&15]`) or
  `a.tedRGB`. `a.pal` on a Plus/4 page is the KERNAL's 16 colours (`kernal16`) for code that knows only 16.
  The brush: `a.fg` is a whole TED colour (0-127) on a Plus/4 page, `a.flash` (the ZX's toggle) is flash;
  `tedBrush()`. Colour keys/steps change the hue and keep the luminance (`tedKey`, `tedStep`), the wheel over the
  Colours grid `tedLum`. `Snap.mach` makes undo of a C64 <-> Plus/4 switch bring machine and colours back
  (`convertMachineColours`: nearest by RGB, one undo step, done before `a.machine` changes so the snapshot
  holds the old machine).
- **Colours** (*verified*): `tedVICE()` computes VICE 3.9's internal TED palette (ted-color.c angles/luminances,
  video-color.c: basesat 56/1.75, phase -9.5, UV->CbCr, saturation 1250 for the TED, float32 as VICE): an
  xplus4 screenshot of all 128 gives exactly these values. YAPE and Colodore are VICE's .vpl files. KERNAL
  colour table ($E133 codes, $E143 colours), power-on $FF15=$71 $FF19=$6E ($F338 table), FLASH ON 0x82 / OFF
  0x84 ($053C, RETURN clears it), all *verified* by disassembling kernal-318004-05.bin.
- **ROM**: `cbmROMs` gets PLUS4/kernal-318004-05.bin (16 KB, SHA-1 7c7e07f0...); `addCBMFonts` takes the sets
  at $D000/$D400 (128 glyphs each, 128-255 reversed: the TED's hardware reverse). The download question now
  says 34 KB. `tedNeedsROM` asks when a Plus/4 file opens without it.
- **Files** (ted_formats.go): `.prg` = Marq's template-plus4.prg form (`tedPrgHead`, 2088 bytes; `isTEDPrg`
  by the template), also on a `.d64` (`d64Art`, `readScreens` via `tedScreen`); the same format ids "prg" and
  "d64" as the C64's, with entries "c64 (Plus/4)" in `formats` (`tedHides` in modeFormats; psci and pmjson are
  hidden on Plus/4 pages); `.scrn` (TEDSE "save screen": $C000, attributes, 24-byte signature, codes, 24 more)
  with `NAME.proj` read next to it and written by `tedSaved` after writeOut; `.seq`: `parseSeqOn`/`writeSeqOn`
  with ted (PetTerm.ted: `tedByte`; also Petmate9's 0x16+byte "tedFull" colour and ESC + one byte skipped),
  `petWriter.ted` for Animate's draw orders, `splitStream(... "plus4" ...)` via `a.tedMode()`, `newPetTermOn`;
  Marq `.c` (META PLUS4, colours 0-127, no flash) and Petmate (`c16Upper`/`c16Lower`, attribute bytes) in
  export.go by `PetScreen.machine`. Marq's BASIC 3.5 listing (`writeTEDBas`, POKE 65305/65301/65299, DATA
  code,attribute x4 a line) and ACME source (`writeTEDAsm`; data only off 40x25) as "marqbas"/"marqasm" with
  "c64 (Plus/4)" entries; attributes keep the flash bit. PNG/GIF border 32/40/48 (VICE's 384x288). Open as
  "Plus/4 PETSCII".
  A PNG (every page type) now draws flashing and blinking cells in their "on" look, as GIF/APNG did; before,
  it showed the phase the screen was in when saved.
- **Tests** (`ted_test.go`, helpers `tedF*`): palette spot values and order, KERNAL tables and ROM glyphs (with
  the ROM: `CHARCOAL_TED_ROM=kernal-318004-05.bin`, else a made-up set so the rest still runs), .seq round
  trips (KERNAL luminance, any luminance as hue + flash), the .prg run in c64x_test's 6502, TEDSE with and
  without .proj (`CHARCOAL_TEDSE_FILES=` a TEDScreenEdit checkout: its title and help screens), Marq's
  plus4-stripes.c, Petmate c16 and Petmate9's own c16_colorbars.petmate (`testdata/petmate9`; with
  `CHARCOAL_PETMATE9=` a checkout, its 17-screen c16-5frame.petmate too), the editor (typing, flash, pick, Colour tool, keys, undo, picture, every
  format saved and opened), machine switch + undo, Animate (draw orders, effects, fade), autosave,
  `TestTEDMutations`, `FuzzTED`, FuzzParsers' plus4 readers. **`TestTEDInXplus4`** (`CHARCOAL_XPLUS4=` a program
  taking prg and png, e.g. a headless VICE 3.9 build: `./configure --enable-headlessui --disable-html-docs
  --without-alsa --without-pulse`, needs xa and dos2unix; run `xplus4 -default -sounddev dummy -warp
  -autostartprgmode 1 -limitcycles 30000000 -exitscreenshot out.png -autostart in.prg` from a folder holding the
  ROMs, with the real HOME: another HOME made it crash): CharEdit's PNG and VICE's screenshot are identical,
  pixel for pixel, on a random page in both cases and on the 128-colour grid; with `CHARCOAL_PETCAT` (VICE's
  petcat) and ACME the BASIC listing and the ACME program too (`TestTEDAsmAndBasic` checks DATA and runs ACME's
  program in the 6502 without VICE).
- Not done: ~~custom character sets on Plus/4 pages~~ (done 2026-10-16: "Character sets: MZ-700, Plus/4,
  DirArt"), the screen editor's ESC commands (skipped when read),
  Petmate9's own c16 .prg player output (assembled at run time; not read), multicolour text mode (TED $FF07
  bit 4), blocks pasted from a C64 page keep their 4-bit colours (shown at luminance 0), Marq's editor not run (needs Java).

## Files that run on the machine, Atari colours (2026-10-16, branch r3-machines)

- **The user's decisions (2026-10-16, "I trust you"):** PAL default; the GTIA model re-implemented from atari800's
  numbers (no code) is fine; the OS ROM font is the default when a ROM is found (prefs.go); a .xex shows 24 rows
  with a note, no scrolling; ~~a measured palette becomes the default~~ **done 2026-10-16 (r3-emu): Altirra's
  default PAL palette, computed from its model's constants, is the default** (compo screenshots are Altirra's).

- **Atari colours** (`gtia.go`): an Atari page's `Doc.bg/ink/border` are now the machine's registers as values
  0-255: COLOR2 (710, the screen), COLOR1 (709: only its luminance shows, the text takes the screen's hue:
  `atariText(d)` = bg&$F0 | ink&$0E) and COLOR4 (712). Start-up 148 / 202 / 0 (`atariBg`, `atariInk`,
  `atariBorder`). Before, they were numbers 0-15 into a 16-colour list: old autosaves (no `gtia` flag in the
  meta) go through `atariFromOld`. RGB: `a.atariRGB(v)` = `a.gtia()[v&$FE]` (luminance bit 0 ignored in text
  modes); **never `a.pal[d.bg]` on an Atari page** (it panics on values > 15): the sites that did (drawCell,
  borderPx, HTML, strip, paperRGB, paperColour, convertFromMachine, Playscii) have an atari case. `a.pal` on
  an Atari page is 16 hues at luminance 8 (for code that wants some palette). **Palettes (2026-10-16, r3-emu):**
  `atari` "Atari PAL (Altirra)" = `gtiaPAL`, the default; `atarintscalt` "Atari NTSC (Altirra)" = `gtiaAltNTSC`;
  `atari800pal` "Atari PAL (atari800)" = `gtia800PAL`; `atarintsc` "Atari NTSC (atari800)" = `gtiaNTSC`
  (`gtiaPalettes`, `isGTIAPalette`, `gtiaFor`). The ids are old: `atari` meant atari800's PAL before, so old
  prefs/autosaves naming it now get Altirra's; `atariFromOld` still converts with `gtia800PAL` (the table of the
  time). `altirraTable` = Altirra 4.40's palettegenerator.cpp model with presets default_pal/default_ntsc (XL luma
  ramp, hue start -12 / -57, range 18.3x15 / 27.1x15, saturation 0.29 / 0.20, PAL even/odd phase table; NTSC also
  brightness -0.04, contrast 1.04, gamma 2.2 + NTSC primaries to sRGB), only constants taken; PAL is pinned by
  `TestEmuAltirraPalette` (all 256 values; $94 #005F8A, grey 14 #EEEEEE). Its NTSC is *computed, not compared
  with Altirra* (greys tint slightly: its primaries' white is illuminant C). atari800's tables were made at start by its model (standard preset: gamma 2.35, levels
  16-235, delay 26.8 / 23.2 degrees, PAL's even/odd delay table; *read* from its colours_ntsc.c / colours_pal.c,
  written anew). They come out darker than measured palettes (atari800's act/jakub.act, Real.act: $94 is
  #2D769D there, #034653 here): the model is what atari800 shows by default, not a measurement.
- Colour UI (`drawAtariColours`): 16 hues x 8 luminances; left = COLOR1, right = COLOR2, middle = COLOR4 (all
  undoable); under it "709: n 710: n 712: n". Keys: `atariColourKey` (Ctrl+0-7 = text luminance 0-14, background
  keys = screen hue), `atariStep` (luminance / the 128 in order); `defaultColours` resets the start-up registers.
  Playscii: its `atari` palette is GTIA's 128 in order (16 hues a row, a row a luminance): `psciGTIA` /
  `gtiaOfPsci` map exactly.
- **.xex / .atr** (`atari8.go`): `atariXexCode` at $2000 sets SDMCTL 0, SDLSTL/H = $2060, COLOR1/2/4, CHBAS ($E0 =
  ROM font, else $24 = the own font at $2400), CHACT 2, SDMCTL $22, then `loop: LDA #0 STA ATRACT JMP loop`
  (attract mode off), an RTS (the .atr's DOSINI) and "CharEdit" + width + rows (how it is read back). Display list
  3x$70, $42 $3000, 23x$02, $41. Screen $3000 in ANTIC's screen codes (`atariInternal`); a 38-wide page from
  column 2. Segments: code+DL, font (if any), screen, RUNAD. `.atr`: 720 x 128-byte sectors, boot header at
  $1FFA (count, load, DOSINI) so the image from $2000 is the same memory; the OS calls $2000, which never
  returns. The font goes in the file unless it is the standard set (`atari`, `atarirom`): the international set
  is carried (no $CC set on a 400/800). Reading: `readAtariProgram` (only ours: code template + mark), opens as
  the page (`loadAtari8`, srcKind xex/atr, own font restored by `useOwnFont`).
- **OS ROM font**: `loadAtariROMFonts` (main, headlessApp) looks for ATARIXL.ROM / ATARIOSB.ROM / ATARIOSA.ROM in
  the data dir, ~/.atari800, ~/.config/atari800, /usr/share/atari800, /usr/local/share/atari800; 10 KB (set at
  $0800) or 16 KB (set at $2000, international at $0C00); fonts `atarirom` / `atarixlrom`. Not offered as a
  download (Atari's ROMs may not be passed on). Not the default font: the user picks it.
- **.tap** (`zxtap.go`): program header + `10 BORDER b: LOAD ""SCREEN$` / `20 PAUSE 0: GO TO 20` (autostart 10)
  + CODE header (6912 at 16384) + the .scr (`writeSCRFont`, so the page's own font). Name = the saved file's
  (`cpcTarget`), 10 characters. Reading: the first SCREEN$ (header + data, or a headerless 6912-byte block) and
  the BORDER of a BASIC block before it.
- Glue: formats inserted after `ata` / `scr` by `init()`; editorOwn xex/atr/tap; `loadAtari8 || loadZXTap` after
  loadCPC; `reparseAtari8 || reparseZXTap` after reparseVideotex; `encodeAtari8`, `encodeZXTap` after
  encodeCPC; `loadAtariROMFonts` in main.go and batch.go's headlessApp; help text in dialogs.go.
- Tests (`mach_test.go`): palettes (greys, rising luminance, blue start-up screen, gold hue 1); colour UI and
  keys, undo, NTSC switch, PNG colours; old autosave; **every .xex/.atr combination (38/40, ROM/own font, short
  pages) loaded as DOS / booted as the OS does, run in a 6502 (`machF6502`), drawn by a model of ANTIC
  (`machFAntic`: display list, CHBAS, CHACT, colour registers) and compared with the PNG export pixel for
  pixel, border included**; round trips; editor save/open/Ctrl+S with an own font; ROM font detection;
  .tap blocks byte for byte (headers, the tokenised BASIC, checksums), other tapes' loading screens, editor round
  trip with the border. FuzzParsers has xex/atr and tap.
- Run in atari800 and Fuse since (see "Machine files in real emulators"). atari800 was built from source in ~/.cache/charcoal-mach/src
  (`--target=libatari800`, needs `CFLAGS=-Wno-error`), but running it was not allowed in that session; fuse is
  not installed. Not done: pages taller than 24 rows in the .xex (only the first 24), a DOS 2 disk with the .xex
  as a file, a tape loader for an own Spectrum font (the .scr carries the pixels anyway), .tzx.

## Machine files in real emulators (2026-10-16, branch r3-emu; tools/emucheck/README.md)

- `emu_test.go` (off unless `CHARCOAL_EMU=1`): writes pages with `writeOut` as a user saves them, runs the file in
  an emulator without any window, and compares the emulator's screen with `renderDoc` (the PNG export) pixel for
  pixel, border ring included. Colours are matched as a one-to-one pairing (palettes differ), so what is checked is
  every cell's glyph and which colour index it has. `emuFMatch` finds the screen at any offset and with a scale
  (MAME's CPC is 1:1 across, 1:2 down); on a failure it lists the cells that differ.
- **Result: every file shows exactly CharEdit's page; no writer bug found.** 14 C64 cases (Petmate's form upper
  and lower, own set hi-res/MCM/ECM, ROM MCM/ECM in both cases, .d64 through the 1541, a 10-row page), 9 Atari
  (.xex/.atr, 38/40, ROM/own font, 6-row page, 800XL and 800 with AltirraOS), 3 Spectrum .tap (ROM font, own font),
  4 CPC .dsk (modes 0, 1, 2). Pictures in /tmp/charcoal-emu (`*-text-emu.png` are readable pages).
- How each runs: VICE `x64sc -console -warp -limitcycles -exitscreenshot` (`.prg` injected); atari800's core
  through `tools/emucheck/a800shot.c` (libatari800, built by `build-a800shot.sh` in ~/.cache/charcoal-emu); Fuse
  (GTK) on GTK's Broadway backend (`broadwayd`, no screen) recording an uncompressed FMF movie, read by
  `emuFFMF`; MAME `cpc6128 -video none -seconds_to_run` with Caprice32's ROMs (`get-cpc-roms.sh`). All with
  `env -i`, a temporary HOME, `timeout -k 5` and a 4 GB scope.
- **Colours** (logged by the tests): atari800's default PAL and NTSC palettes are CharEdit's `gtia800PAL`/`gtiaNTSC`
  (until the switch to Altirra's, `gtiaPAL`) exactly (0 difference over all 128 colours each), so "darker than measured" is atari800's own default look.
  atari800's act/ files are brighter (mean brightness 136-146 against our 99; $94 jakub #2D769D, Real #3E75A4,
  ours #034653); Altirra 4.40's default PAL, computed from its palettegenerator.cpp (not run: Windows), is between
  (mean 120, $94 #005F8A, grey 14 #EEEEEE against our #D3D3D3). VICE's default C64 palette differs from ours
  (e.g. blue 2C3DEC vs 352879); MAME's CPC half level is 0x60, ours (Caprice's) 0x80. Licences: the act files are
  part of atari800 (GPL-2+, no own notice; jakub.act by Jakub Bogusz, Real.act by Zdenek Eisenhammer per
  DOC/CREDITS); Altirra is GPL-2+ and its palette is a model (constants), the position gtia.go took for atari800.
- Traps: VICE's default (virtual drive) autostart is too slow for 4M cycles; Fuse ignores TERM (hence
  `timeout -k`); Broadway's socket is abstract; systemd-run needs the session's XDG_RUNTIME_DIR, so the emulator's
  own env goes through `env -i`; Arch's /usr/bin/mame writes ~/.mame (use /usr/lib/mame/mame); FMF has no flash
  phase; Spectrum bright black = black.
- Not done: flash on the Spectrum (not in the movie), C128/VIC-20/PET programs (x128/xvic/xpet not installed),
  Plus/4 (another helper's area), a real machine, Altirra itself.

## The rest of other editors' files (2026-10-16, branch r4-others)

- **Decided (2026-10-16, the user trusts these calls):** Playscii's sets/palettes and SkyPix's Amiga fonts are
  downloads, not built in (machines' own glyphs); IcyDraw XEP80 pages show their first 40 columns; a C128 80
  page with mixed sets uses the most-used set and says how many differ; a PETSCII .icy gets CharEdit's palette
  written in; SkyPix follows the spec for ESC[row;colH (IcyDraw is one row high); opening SkyPix makes VGA 8x8
  the PC default like any 8x8 file.

- **IcyDraw's other pages** (`icymach.go`): an `.icy` with buffer type 2 PETSCII / 3 ATASCII / 5 Atari ST opens on
  that machine's page; the `SCREEN` record (IcyDraw 0.6.0, version 1: tag 1 machine u16 + mode u16, 2 border,
  3 screen colour, each 4 bytes; unknown or repeated fields refuse the file, as IcyDraw) gives machine and case
  (`icyMachines`: 1 C64, 2 C128, 3 VIC-20, 4 PET, 5 C16 = CharEdit's Plus/4, 6 PET 80, 7 C128 VDC; 8 ST mode
  0-2, 9 Atari 8-bit (1 = XEP80), 10 Viewdata, 11 Mode 7), else the font's name as IcyDraw's `petscii_charset`.
  Cells: PETSCII = screen code (128+ reversed), colour = machine number (C16 lum*16+hue: palette tags 19 n, ICED
  version 2; VDC = RGBI order, `vdcOfC64`/`c64OfVDC`; font page 1 = lower set per cell), blink = TED flash;
  ATASCII = code, bg != 0 also inverse, colours = palette entries 0 (screen) and 7 (text) -> `gtiaNearest`;
  ST = pens. Fonts: IcyDraw writes the ROM's sets (identical to VICE's, checked: C128's lower = its chargen at
  6144); a PETSCII file whose set is not the ROM's opens with it as the page's own font (`icyAfterLoad`).
  Writing (`writeIcyMachine`, `icyOut.encode`; `writeIcy` for PC pages now goes through `icyOut` too,
  byte-identical to before): ICED ice mode 0, font mode 1 as IcyDraw, the page's palette (TED's 128 as shown,
  VDC reordered), the page's fonts (PET 80 and VDC rows doubled, 8x16 as IcyDraw; VDC both sets), SCREEN.
  Glue: `icyMachineLoad` in loadEditor's icy case, `reparseIcyMachine` in reparseEditor, `writeIcyMachine` in
  encodeEditor, Save as "IcyDraw" offered on c64/atari/st pages (init). XEP80 (80 columns) shows its first 40;
  VDC's per-cell set: the one most cells use (note says how many differ).
- **Playscii** (`psciown.go`): sets with no page (`psciSets`: agat, apple2, bbc_master, intellivision,
  pacman, sharp, speccy = ZX81) open on a PC page with the set's picture as the page's own font (glyph i = the
  picture's i-th cell; on = not black; <= 8 wide, 6-16 high; **glyph 0 and 32 trade places**, `psciSwap`, since a
  PC page draws nothing for 0 and types space as 32); a palette other than ansi/ega becomes the page's own colours
  (`psciOwnColours`: all of <= 16, else the 16 most used, the rest nearest). Ctrl+S writes the same names and
  numbers (`psciPage.own`, from `a.psciOwnOf()` re-reading the source). atari_st opens on an ST page (low <= 40
  columns, high for 2 colours, else medium); it saves back with the file's palette, each pen as a palette colour
  that reads back as that pen (`psciPenColour`; ansi alone does not round-trip yellow/light yellow). The pictures
  (7 sets, 21 palettes) are fetched on first use from github.com/michael-lazar/playscii at `psciVersion`, SHA-1
  checked (`psciNeeds` asks; `psciURL` is a var for tests). Not done: teletext_uk (12x20 glyphs; the Unicode or
  teletext page would fit), ultima4/ui/osamu-micro (too wide): by number as before. Playscii's `msx` opens on the MSX page (msx.go,
  checked before psciSets); `sharp` stays a PC page with its own font: it is the Japanese MZ's set (kana, kanji,
  352 glyphs in an order of its own), the MZ-700 page holds the European set's display codes and no ROM to
  compare glyphs with, so no exact mapping exists.
- **PabloDraw `.pd`, TheDraw `.td`, BSAVE `.bsv`** (`othformats.go`): `.pd` = .NET string font id (or "" and the
  font: int16 w, h, count, rows) + int16 palette count + RGB + TundraDraw body without header (PabloDraw's
  `Tundra.Save`: position after chars 0-6, fg not written for spaces, both start at attribute 0); width from
  SAUCE (written when not 80, FileType 8). PabloDraw ids map to CharEdit fonts (`pdFonts`); amiga-* open on an
  Amiga page. `.td` (TheDraw 4.x only; 3.x refused with a message): header 367 bytes (w at 39, h at 41), then
  chunks u16 size + crunched data (UNCRUNCH.PAS) read until they stop fitting; byte 56 is **not** the page count
  (SHADING.TD says 2, holds 3). Pages -> frames; read only: path cleared, Ctrl+S asks for an `.ans`. TheDraw
  writes 255 between words (shown as blank). `.bsv` = FD, seg B800, offset, length + screen bytes (80 columns,
  at most 409 rows). ACiDDraw has no own format (ANSI/BIN/ASM/...); its `.adf` is ArtWorx's: no clash.
- **Petmate9** (`petmate9.go`, export.go): charsets name the machine (`petmate9Machine`: c16*, vic20*, petGfx/
  petBiz (80 wide: PET 80), c128* (80 wide: VDC)); petBiz is lower case; the writer names the machine's set
  (`petmate9Charset`) when the kept name is a machine set of another machine (`petmate9Known`; custom font names
  kept). VDC screens: colours are RGBI numbers and pixels carry `attr` (reverse -> reversed glyph); converted only
  when the page itself is the VDC's (a big-check file mixes a C128 40 screen and a VDC one).
- **SkyPix** (`skypix.go`, `skyfonts.go`): `ESC[n;...!` commands; parsed as IcyDraw's SkypixParser (text after
  "!" for 0, 10 (not size 0) and 16), drawn as its sky_paint/skypix_impl (Bresenham, flood fill with a seen set,
  RectFill inclusive, midpoint ellipse, filled ellipse by rows, brushes, palettes $0BGR, pens, cursor in pixels;
  every command switches text to JAM1 and CR/LF off until SGR 0 or `ESC[10;0!`). Text: the 8x8 IBM set (fetched)
  or VGA 8x8; fonts by name from IcyDraw's yaff files (`parseYaff`: cells min(bbox w, 8) wide, pixel-size / size
  / line-height / bbox height; glyphs left-aligned; labels end in ":", a char label "':':" too). IcyDraw quirks
  followed: `ESC[10;0!` does not move the cursor (IcyDraw means to, but measures both fonts in the one in use);
  **not followed**: IcyDraw puts `ESC[row;colH` one row higher (the parser and the sink both subtract 1) - the
  one bench file that differs (skypix1, 1.8%). Page: PC 80x25, font vga50, `Tab.sky`, picture under blank cells
  (`drawSkyCell` from igs_page.go's `drawCellXY`), `setPalette` takes the file's colours, fixed height. Saving
  (`writeSkyPix`): unchanged = the source; else the drawing, `skyMarker` (a comment), then per run `ESC[15;fg!
  ESC[18;bg! ESC[10;0!` + `ESC[r;cH` + text (control bytes as spaces). Detection `isSkyPix`: 3 commands, no NUL,
  not a picture/program (a GIF in the big check had them by chance). Work limit 50M pixels.
- Glue in shared files: editor.go (`Tab.sky`, `a.sky` reset in loadBytes, `loadSkyPix` after loadIGS,
  `reparseSky`, `encodeSky`, `setPalette`), rowscols.go (fixedHeight), igs_page.go (drawCellXY), export.go
  (Petmate), fuzz_test.go (seeds and checks), editorformats.go (hooks for icy/psci/pd/td/bsv).
- Tests (`oth_test.go`): IcyDraw round trips of every machine page, own charset, SCREEN vectors (IcyDraw's own);
  env-gated: `TestOthIcyDrawMachineFiles` (files IcyDraw's engine wrote + its dumps, default
  ~/.cache/charcoal-others/icyfiles), `TestOthIcyExternal`/`TestOthIcyRender` (CHARCOAL_ICYMACH /
  CHARCOAL_ICYRENDER = `~/.cache/charcoal-others/sandbox.sh target/release/examples/ccicy dump|render`,
  CHARCOAL_ICYDIR): **18 of 18 page kinds read cell for cell and drawn pixel for pixel**. Playscii: own set and
  palette (made-up set registered with its SHA-1), fetch flow with a test server, real pictures and Playscii's
  56 own files (CHARCOAL_PLAYSCII), ST open/save. `.pd`/`.bsv` round trips (own font, widths 40/80/132), .NET
  strings, uncrunch codes, TheDraw 4.63's DEMO.TD/SHADING.TD (CHARCOAL_THEDRAW). Petmate9 machines. SkyPix
  spot checks, detection, app open/save/reopen; `TestOthSkyPixIcyDraw` (CHARCOAL_SKYPIX_FILES/ICY/FONTS) against
  IcyDraw's renders. `FuzzOthReaders` (60 s clean) and FuzzParsers' new checks.
- ccicy (the check program on IcyDraw's engine, not in the repo): `~/.cache/charcoal-others/icy` (icy_tools
  eddbe237 = IcyDraw 0.6.0) + `crates/icy_engine/examples/ccicy.rs` (dump / make / render / skypix), built with
  `sandbox.sh cargo build --offline --release -p icy_engine --example ccicy` (bwrap: no network, only that folder
  writable; PATH must not hold ~/.cargo/bin and /dev, /proc must be given). Recipe in the knowledge note.
## Compos: party check and work stages (2026-10-16, branch r4-compo; design: docs/workstages-design.md)

- **The user's decisions (2026-10-16, "I trust your judgement"):** Lovebyte's 256 counts the box round what is
  drawn on machine pages; stages never include the empty page, are spread by edit steps, and are fewer rather than
  repeated; iCE on with no bright backgrounds passes "no iCE"; File menu, no default keys, Save for the party
  writes beside the file.

- **Party check** (`compo.go`): `compoRules` is the data table (id, name, edition, URL, the key sentences quoted
  in comments, `plats` = the kinds of entry with page modes, machine, max cols/rows, exact cols, max cells, what is
  allowed (iCE, blink, own font, own palette, animation, VIC modes), rows to keep empty, formats to hand in; stages
  and `atLeast`; `also` = rules CharEdit cannot check). `compoCheck` returns only the problems; `compoPlatFor` picks
  the kind by page mode and machine. Lovebyte counts the drawing (`drawnSize`: the page on PC/Amiga, the box
  around `usedCell`s on machine pages). Blink = bright background without iCE on PC/Amiga pages. A draw order
  (Animate) counts as an animation when Save would write it. UI: `PartyReq` (File > Check for a party,
  action `party_check`, no key; Up/Down change the compo; `prefs.Party` remembers it). Rules read 2026-10-16
  (Revision 2026, Evoke 2026, Lovebyte 2025 (no 2026 page), Zoo 2026, Deadline 2026 at demoparty.berlin, X 2026,
  AMSCII 2024, ATASCII 2026, Yle teletext 2026): the KB survey's older numbers were wrong in places (Zoo now >= 15
  stages, Deadline's 6 stages are not for textmode, Revision textmode wants none).
- **Save for the party** (`saveForParty`): needs a saved page; writes `deliver` formats through `encodeStage`
  (a copy of the tab with only the picture: no source, frames, order, never "unchanged") into
  `<dir>/<name>-<compo id>/`, CPC names cut to 8 characters (`cpcTarget` set for the .dsk), and the compo's
  stages in `workstages/`. The status line counts broken rules.
- **Work stages** (`stages.go`): `Tab.steps` = depth (pushSnap: s.step + 1; restore: s.step; `Snap.step`),
  `Tab.stages *stageLog` = pictures at depths that are multiples of `every`; `recordStage` (from pushSnap) drops
  pictures deeper than the step (an undone branch) and skips other frames' steps; past 64 pictures or 24 MB
  `every` doubles. Pictures are flate-packed little-endian cells (`packDoc`; Unicode styles renumbered into the
  picture's own `Pairs`, so `uniCompact` cannot break them). Measured: 63 real PC ANSIs pack to 7.5% (worst 16%);
  80x500 typical art ~2-12 KB a picture. Kept: `writeStages` after an own-format save, to
  `<data dir>/stages/<sha1 of the absolute path>.json` with the SHA-1 of the bytes written; `restoreStages` in
  openPath takes it only if the file still has those bytes and the page type matches (openPath clears
  steps/stages before loading). Autosave: `<id>.stages`. `stageCands` = log + undo stack (same mode, current
  frame) + now; `pickStages` = stage k nearest depth k*D/N, strictly deeper than the one before, leaving room for
  the rest; depth 0 (the empty page) is not a stage; fewer pictures -> fewer stages. `exportStages` writes
  `name-stageNN.png` (+ the page's format). UI: `StagesReq` (File > Export work stages, action `work_stages`).
- script.go: `do ACTION` runs any action by id (for headless shots of key-less actions).
- Tests (`compo_test.go`): log thinning/spread/content over 1000 steps, undo branches, pickStages cases, the log
  kept with the file across a restart (and refused after an outside change), autosave carries it, Unicode
  pictures, packing, the rules table (every format id and page mode exists), the check on PC/C64/Atari/teletext
  pages, Save for the party (PC, CPC 8-char names, 10 stages for Revision oldskool), `FuzzCompoStageLog`;
  `CHARCOAL_COMPO_FILES=dir` measures packing on real .ans.
- Not done: a log is not kept for pages only ever exported; pictures keep cells only (font, CPC inks, C64 modes
  are the page's now); no check of the colour count beyond the page type's own; Coloured ATASCII and AMSCII's
  mode 0/2 categories are not separate entries; no timelapse video.

## MSX and Sharp MZ-700 pages (2026-10-16, branch r4-msxmz)

- **Decided (2026-10-16, the user trusts these calls):** C-BIOS's font as the MSX default (a real MSX shows its
  ROM font; only an own font travels); MSX pictures at openMSX's 320x240, MZ at the bare 320x200; the MSX .bin
  loads at $9000 and resets to SCREEN 0 on a key, the cartridge never returns; MZ display codes 0x00/0x20 swapped
  inside CharEdit (files hold the real codes). The MZ second set without Sharp's ROM: drawing a free look-alike is
  on the roadmap (real MZ art uses it a lot; hex boxes until then). **Done 2026-10-16**: CharEdit's own look-alike. CharView lists .mzf .xex .atr .tap .dur .irc
  .ig .igs .psci .icy .xp .msg too.

- **MSX** (`msx.go`): page type "msx" (`a.msx()`). `Tab.msxSet msxSetup` = mode (0 SCREEN 0 40x24, 1 SCREEN 1
  32x24, 2 SCREEN 0 WIDTH 80), fg/bg (SCREEN 0: the whole screen; border = bg), border (SCREEN 1's backdrop),
  `ct [32]byte` (SCREEN 1's colour table, fg<<4|bg per 8 characters). **A cell holds only its character**
  (`makeCell(ch, 0, 0, 0)`): `drawMSXCell` takes the colours from the setup (`msxSetup.colours(ch)`; colour 0 =
  transparent = the backdrop, `a.msxRGB`), so cells are blank to `blankAt` and the colours cannot drift. `setFg`/
  `setBg` (keys, Swap, the Colours) go to `msxSetColour`: the screen's pair in SCREEN 0, the brush character's
  group in SCREEN 1, undoable (`Snap.msx`, also in frames.go's snapshot); `msxBrushColours` keeps `a.fg/a.bg` as
  the brush shows. Cells 6x8 (SCREEN 0, the glyph's left 6 pixels as the TMS9918 shows), 8x8, 6x16 (80
  columns, pixels half as wide). 24 rows fixed. Palettes `msx1` (MAME's TMS9918 table, R. Nabet's from TI's data
  book; *verified* equal to MAME's screenshots) and `msx2` (openMSX's V9938_PALETTE, 3-bit GRB levels); the
  Palette gadget lists only those two.
- **Font**: C-BIOS 0.29a's three sets (`fonts/msx-cbios*.font`, BSD, made by `tools/msxmz-fonts.py` from
  font_us/jp/br.asm). Microsoft's MSX ROM font may not be shipped: `loadMSXROMFont` (main, headlessApp) adds
  "MSX (BIOS ROM)" from `msx.rom`/`msx1.rom`/`msx2.rom` in the data folder (32 KB, CGTABL at 0x0004). An own font
  (2048 bytes in `parseFontFile`, or the font editor) is carried by the programs; C-BIOS's sets and the ROM's
  are not (`msxProgramFont`): a real MSX shows its own ROM font then.
- **Files**: `.rom` (16 KB, "AB", INIT 0x4010) and `.bin` (BLOAD: FE start end exec, at 0x9000; a key returns
  to BASIC on a cleared SCREEN 0). Same program (`msxProgram`): ERAFNK; FORCLR/BAKCLR/BDRCLR (SCREEN 0: BDRCLR =
  bg, since C-BIOS's CHGCLR takes BDRCLR where MSX BASIC's takes BAKCLR); LINL40 (40/80) + INITXT or LINL32 +
  INIT32; CHGCLR; LDIRVM of the own font to (CGPBAS), the names to (NAMBAS), SCREEN 1's colours to (T32COL);
  then `JR $` (cartridge) or KILBUF/CHGET/INITXT/RET (.bin). After it "CharEdit MSX", a version byte, mode, fg,
  bg, border, flags (bit 0 own font), the names, the 32-byte colour table (always), the font: how
  `readMSXProgram` reads it back. Other people's .rom/.bin are not read (`loadMSX` returns false on auto).
  Playscii `msx` charset = codes 32-255 then 0-31 (`psciMSXIndex`), `msx` palette = 15 colours in Playscii's own
  order (`psciMSXColours`, matched to the MSX2 palette); reading picks the mode by width and the commonest
  colours (per group in SCREEN 1). Open as MSX: text, code 1 + 0x40-0x5F for 0-31 (`msxText`), saved as UTF-8.
- **Sharp MZ-700** (`mz.go`): page type "mz". Cells: character + fg + bg (0-7, colour code = G4 R2 B1), and
  `mzSet2` (1<<20) for the second character set. **Display code 0x00 (space) and 0x20 (the digit 0) change
  places inside CharEdit** (`mzCode`, an involution used for fonts, files and runes), so the space is 32 as
  everywhere; `Doc.glyph0` is set on MZ pages (`applyLook`) so the digit 0 is not empty. The brush and F-keys
  hold 0-511 (256+ = second set): `mzCell`, `mzChar`, `validMZFkeys` (prefs.go accepts them), the character
  table shows 512 (panel.go `drawCharset`). Empty cell: space, white on blue (the monitor's $71).
- **Font**: Sharp's CG ROM is copyrighted (MAME needs it, nobody may ship it). Built in: Russell Marks's public
  domain look-alike of the first set (mz700em's font.txt, in github.com/mkoeppe/mz800em; `fonts/mz700-NOTICE.txt`);
  the second set is CharEdit's own look-alike, `mzFreeSet2` (2026-10-16; until then its code in a box). `loadMZROMFonts` finds `mz700fon.int` (or `.jpn`)
  in the data folder, ~/.mame/roms, /usr/share/mame/roms, as a file or in `mz700.zip`, and adds "MZ-700 (Sharp
  ROM)" with its second set (`mzROMSet2`; MAME's layout: 512 glyphs, the low bit the leftmost pixel). Own fonts
  and the font editor are refused (the MZ-700 has no programmable characters).
- **Files**: `.mzf` (`writeMZF`): header (1, name ended by 0x0D, size, load = start = $1200), program `DI; LD
  HL,codes; LD DE,$D000; LD BC,1000; LDIR; LD HL,attrs; LD DE,$D800; ...; JR $`, "CharEdit MZ-700", 1000 display
  codes, 1000 attributes (bit 7 set 2, 6-4 fg, 2-0 bg). `readMZScreen`: our mark, or any program that names
  $D000 and $D800 within 12 bytes of an LD HL/DE/BC of an address with 1000 bytes in the file (z88dk's memcpy
  pushes, LDIR): pixelrat's "Princess" (Demodulation 2021, from demodulation.retroscene.org) opens. Open as
  Sharp MZ-700: UTF-8/Latin-1 text. UTF-8 export (`encodeMZ`) keeps the second set (Legacy Computing code
  points, from Wikipedia's Sharp MZ character set, European tables). Pictures 320x200, no border.
- **Glue**: init() in each file (fonts, palettes before Custom, pageModes, modeOrder, modeNames, openAs,
  modeNoStream, editorOwn, formats after cpcdsk); editor.go: Tab/Snap fields, cellW/H, cell, brushCell,
  setPalette, printable, charToByte, runeOf, snapshot/restore, maxRows, pick, setFg/setBg, nColours,
  defaultColours, swapColours, reparse/load/encode chains, setMode, makeTab, setRows/setCols, play, applyLook's
  glyph0; view.go (drawCell, labels, Screen gadget, palette list, status line), panel.go, uni_ui.go
  (previewCell, charLabel), anim.go (openMSX's 320x240 border), cpc.go's borderPx, frames, rowscols, strip,
  effects, shapes (MSX box lines 17 16 18 19 1A 1B), fontfile/fontedit, streams, menus, start (tiles), main,
  batch, autosave (`msxAutosave`), prefs, editorformats (psci), dialogs (help), fuzz_test.
- **Tests** (`msx_test.go`, `mz_test.go`, helpers `msxF*`/`mzF*`): fonts, runes, palettes, typing, colours and
  undo, every program (3 modes x own font x .rom/.bin) run in a small Z80 (`msxFZ80`) with a model of the BIOS
  and compared pixel for pixel with the page; editor round trips (.rom, .bin, .psci); Playscii charset;
  Open as text; picture sizes; autosave; ROM font detection; MZ: quarter-block bit order, second set,
  pick, .mzf run in the Z80 (video RAM byte for byte), round trip, other programs (memcpy and LDIR forms;
  `CHARCOAL_MZ_FILES=dir` opens real .mzf files and writes PNGs), text and UTF-8, ROM fonts.
  **Emulators** (CHARCOAL_EMU=1): `TestEmuMSX` runs the cartridges in MAME's hb10p (MSX1) and phc23 (MSX2) with
  C-BIOS as their BIOS (`tools/emucheck/get-msx-roms.sh`; MAME warns "WRONG CHECKSUMS" and runs it): SCREEN 0,
  SCREEN 1, own fonts and WIDTH 80 are CharEdit's PNG pixel for pixel, and MAME's RGB equals msx1Palette.
  `TestEmuMZ` runs the .mzf in MAME's mz700 with a stand-in monitor ROM written by the test (copies the .mzf to
  $1200 and jumps; Sharp's monitor may not be shipped) and a CG ROM of CharEdit's font: pixel for pixel, RGB too.
  `TestMSXMZSamples` (`CHARCOAL_MSXMZ_SAMPLES=dir`) writes sample files.
- **Not done**: other people's MSX .rom/.bin/.dsk/.cas (no known text-art format; the 2025 MSX ASCII piece was
  not found as a file), a .dsk with the .bin, an MSX BASIC listing, MSX2 blink attributes, sprites; the
  monitor's own loading of the .mzf on a real MZ-700 (MAME ran it through the stand-in monitor), the MZ-700
  "Character Graphics Editor"'s files (a Japanese Windows tool seen in a Shadow Party 2024 entry's work stages;
  not found), Playscii's "sharp" charset (another layout), flips of second-set characters (they use the first
  set's mirror table). (The 2nd set's look without Sharp's ROM: done 2026-10-16.)

## Character sets: MZ-700, Plus/4, DirArt (2026-10-16, branch r5-charsets)

- **The user's decisions (2026-10-16):** the MZ-700 second-set look-alike (drawn by eye from a picture of the
  table, nothing extracted) ships as CC0, as Russell Marks's first set; the DirArt header line is not part of the
  art (yet: roadmap); saving DirArt over a disk keeps its real files listed; the Plus/4 BASIC export with an own
  set needs a 64 KB Plus/4.

- **MZ-700 second set** (`mz.go`, `fonts/mz700-set2.font`, `tools/mz700-set2.txt` + `tools/mz700-set2.py`):
  `mzFreeSet2` replaces the hex boxes (`mzHexFont` is gone) when Sharp's ROM is not found; the ROM still wins
  (`mzSet2Font`). Drawn by hand for CharEdit from each character's meaning (the Unicode names `mzRunes` gives:
  rockets, snake heads, chess quarters, faces...) with sharpmz.net's "Display code table part II" (a GIF of the
  ROM's glyphs) as the picture of the look, never extracted from it. Outlined letters/digits: `skel` lines (a
  5x6 middle line; the glyph is its 8-neighbour wall), turned families (`= cw/ccw/180/flipx/flipy`), 2x2 pictures
  (`big`), the four pie circles computed. CC0 (`fonts/mz700-set2-NOTICE.txt`). `TestChsMZFontFile` checks the
  .font is what the script makes of the .txt (needs python3; run `python3 tools/mz700-set2.py` after editing the
  .txt; `--png sheet.png` draws the table, needs Pillow). Princess (Demodulation 2021) looks right with it (its
  83 second-set cells: bushes, bishop/rook quarters, the pie circles). MAME's mz700 runs the .mzf with this font
  as its CG ROM (TestEmuMZ, pixel for pixel).
- **Plus/4 own sets** (`tedchars.go`): the TED reads characters from RAM when $FF12 bit 2 = 0, at $FF13's bits
  7-2 (1 KB steps); $FF07 bit 7 = 1 turns the hardware reverse off: 256 characters, 2 KB aligned (*read* in
  VICE's ted.c / ted-draw.c, *verified* in xplus4). `tedSetBytes`: 1024 bytes when 128-255 are the inverse of
  0-127 (`tedHalfReversed`), else 2048. **CharEdit's Plus/4 .prg** (`tedxPrg`, only when the page has its own
  set; plain pages keep Marq's form): `10 SYS4109`, SEI, colours, Marq's copy loop (codes $1080, attributes
  $1468), $FF12 &= $FB, $FF07 AND #$7F / ORA #$80, $FF13 = $20 | its low bits, JMP *; "CHAREDIT PLUS/4" mark;
  the set at $2000 (file padded; 5121 or 6145 bytes). `parseTEDPrg` falls back to `parseTEDxPrg`, so
  `.prg`/`.d64` detection, `tedScreen` and `d64Art` take it; `tedCharsAfterLoad` (after `c64xAfterLoad`) gives
  the page the set. **TEDSE**: its project byte 0 ("charset changed") = 1 and `NAME.chrs` (PRG from $B800, 128
  characters, as its `savecharset`) written by `tedSaved`; read back in `loadTED` (`tedseChrs`). A 256-character
  set saved there loses its own reversed half (the note says so). `.chrs` is also a font file (load/save,
  `fontExts`). **BASIC** (`writeTEDBasSet`): the set's DATA first (lines renumbered by 1 from 70), POKEd to
  $E000 (RAM under the KERNAL: BASIC's DATA reach far past $2000), $FF12/$FF07/$FF13 POKEd; **ACME**
  (`writeTEDAsmSet`): SEI + the registers after `sta $ff15`, the set at `*=$2000`. `carriesFont` now includes
  "tedse". **Font editor**: on a Plus/4 page a change to a character also sets its partner (c^128) to the
  inverse (`tedFollow` in `FontEd.change`, `tedFollowGlyph` in the drag path), so the page shows what the TED
  shows; a C64 page's characters stay apart.
- **DirArt** (`dirart.go`): machine "dirart" (family c64, 16 columns, mono, palette "dirart" = monoPal light
  blue 6C5EB5 with colour 0 = blue 352879; `newMachineDoc` border 1). Format "dirart" ("c64 (DirArt)", .d64),
  shown only on DirArt pages, which hide prg/d64 (`dirartHides` in `modeFormats`); `editorOwn`. Names:
  Petmate9's `petsciiToScreen`; `screenToName` writes 0-31 +64, 64-126 +128 (192-254, never $A0), 127 as 191,
  reversed control codes 129-159 as $01-$1F and 192-223 as $80-$9F; forbidden (Marq's/Petmate9's list: 34,
  128, 141, 148, 160-191, 205, 224-255) become spaces, counted in the note (`dirartSaved`, appended to
  writeOut's note). `writeDirArt(d, src, name)`: over the source disk when the page came from one (rows map to
  entries in order; a row whose cells equal the entry's decoded name keeps its bytes, $A0s and all; new rows go
  in free slots after the last entry, then new track-18 sectors in the 1541's interleave-3 order, BAM updated;
  entries past the page: DEL/0 blocks are cleared, files stay and are counted), else a fresh disk
  (`writeD64Empty`: writeD64 minus its file). 144 rows at most. Reading (`readDirArt`): every entry with a type
  byte != 0, $A0 as space. `loadDirArt` (before loadTED): a .d64 with no screen program of ours (d64Art fails)
  or Open as "C64 DirArt". Marq's `DIRART` META and Petmate9's "dirart" charset map to the page; `marqName`
  writes `DIRART` without colours.
- **Tests** (`chs_test.go`, helpers `chsF*`): the MZ set (complete, distinct, turned copies, quarters meet, ROM
  wins, .font = script output); the Plus/4 program in a small 6502 (`chsF6502`: registers, $2000, screen) for
  128/256, editor round trips (.prg, .d64, .scrn + .chrs + .proj flag), .chrs as a font file, the font editor's
  reverse (and a C64 page's not), BASIC DATA order and ACME output assembled and run (`CHARCOAL_ACME`);
  DirArt: names table vs Marq's list, disks of 1-144 rows (BAM, 664 blocks free, sectors), over a disk with a
  program (kept byte for byte, $A0 names kept, a ninth row adds sector 18/4), the editor (formats shown, save,
  Ctrl+S over the same disk, a screen's .d64 still a screen, Open as DirArt, Marq's .c). FuzzParsers: "plus4
  own prg", "dirart" (written back over itself). **Emulators** (CHARCOAL_EMU=1): `TestEmuChsPlus4` (xplus4
  3.10: .prg 128/256, .d64, BASIC via petcat, ACME, a readable text page: all pixel for pixel),
  `TestEmuChsDirArt` (x64sc with `-8 disk -keybuf 'load"$",8\nlist\n'`: the listing's lines, built as screen
  codes, found on VICE's screen). Samples: `CHARCOAL_CHS_SAMPLES=dir` (a DirArt disk, a Plus/4 program with a
  set).
- Traps: a reversed T (screen code 148 = DEL) or M (141 = RETURN) cannot be in a name, so reversed words in
  DirArt lose those letters; LIST shows a name's bytes after an $A0 outside the quotes (CharEdit shows them as
  in the name and keeps them when the row is unchanged); CharEdit's PETSCII terminal does not model LIST's
  quote mode, so the emulator test builds the expected listing from screen codes.
- Not done: the disk's header line as part of the art (kept from the source disk, else the file's name), the
  block counts and file types as art (always `0 ... DEL` for new rows), DirArt in lower case (the .d64 keeps
  no case: the note says the listing shows the upper set), a Plus/4 set in Petmate9/Marq files (neither has
  one), MZ second-set flips (they use the first set's mirror table).

## TheDraw fonts: the Logo tool (2026-10-16, branch r5-tdf)

- **The user's decisions (2026-10-16):** keep "Download..." for Roy/SAC's collection (fetched from his site on
  the user's click, never shipped); block/outline glyphs fill their box (as IcyDraw); colour-font spaces on black
  see-through; spacing columns skipped; Return to the run's start column; "Logo", Alt+G, Alt+Shift+G, Ctrl+Shift+G.

- **What**: `tdf.go` (reader, glyphs, layout, TheDraw's 19 outline styles), `tdf_ui.go` (library, download, the
  Logo tool, its options, the font picker `TDFReq`, the line requester `TDFLineReq`). Tool id "logo" (appended to
  `tools` in `init`; hidden like Half on pages that are not PC/Amiga/Unicode, `tdfPage`), actions `tool_logo`
  Alt+G, `logo_font` Alt+Shift+G, `logo_line` Ctrl+Shift+G, Draw menu entries, `prefs.TDF` (paths, font by its
  SHA-1 `sum`, outline, spacing (0 = the font's, n = n-1), the line's text).
- **Format** (read from TheDraw 4.63's manual, its TDFONTS.TDF and THEDRAW.EXE, IcyDraw's retrofont; checked on
  Roy/SAC's 3,536 fonts): header 19 + "TheDraw FONTS file" + 1A; per font 55 AA 00 FF, name length + 12 bytes,
  4 zero bytes, type 0 outline / 1 block / 2 colour, spacing, u16 block size, 94 u16 offsets for '!'..'~'
  (FFFF none), the block; a glyph = width, height, cells to a 0 byte, 13 = new line, colour cells char + DOS
  attribute (BIOS order: `biosANSI`). **Traps**: the height byte leaves out descender lines, which end in '&';
  an '&' that is *not* at the end of a line is a character (with an attribute in colour fonts: the "Dolce" fonts
  use it; reading every '&' as a mark made 40 of their glyphs wider than their width byte, the end-of-line rule
  makes all agree). Fonts in one file can be cut short or followed by junk (one tdfiglet file): the fonts before
  are kept. Width may be less than the longest line (old fonts): the longest line wins.
- **Drawing** (`render`): outline letters A-P through `tdfOutlines` (19 x 16 bytes, read out of THEDRAW.EXE at
  0x2305B; O = 0xFF and P = space are blanks), '@' = TheDraw's solid space (blank, drawn); block 0xFF = solid
  space; block/outline glyphs fill their box (width x lines, not descenders) with spaces in the brush's colours
  first (IcyDraw does this), a space in the glyph leaves the fill. Colour fonts: a space on a black background is
  left out (the page shows through), a space with a background is drawn (1,671 of Roy/SAC's colour fonts draw with
  coloured spaces; IcyDraw drops them all), 0xFF is a space in its colours.
- **Typing** (`logoGlyph`): one `pushUndo` per glyph, cursor set to the glyph's corner *before* the push (undo
  puts it back there); then x += width + spacing. `logo` keeps the run: `left` (Return's column), `x` (may pass
  the page's width), the glyphs' generations `gens`. Backspace (`logoKey`) is `doUndo` only when the page's gen is
  the last glyph's and the cursor is where it left it; else the normal Backspace. `logoRun` also continues after
  Ctrl+Z (the gen is one of the run's). Return = left column, y + `lineHeight` (the tallest glyph without its
  descenders); `moved()` may add rows (its own undo step, counted in gens). A glyph that does not fit wraps to the
  next line of letters. Space = half the letters' usual width; a letter the font lacks: its other case, else a
  message. Typing goes to `logoText` from `textInput`, Backspace/Return from `editorKey` (both only with the tool on
  a `tdfPage`).
- **Pages**: PC = CP437 as is; Unicode = `uniFromRune(cp437)`, colour fonts `styleOf(fg, bg, 0)`; Amiga = the CP437
  character's Latin-1 byte or refused (`tdfMissing`, the whole glyph, with the character named). Only 118 of the
  3,536 fonts fit an Amiga page (`tdfFits`, cached per font): the picker lists only those there unless "Also
  fonts an Amiga page cannot show" is ticked.
- **Fonts**: none built in (art by others, almost none with a licence). Library = `<data dir>/tdf` + the user's
  paths (files, folders walked, `.zip`s read), each font once by `sum`, sorted by name. Download (`FontReq` with
  `fetchTDF`): Roy/SAC's `TDFONTSALL.ZIP` (3,136,213 bytes, one 20 MB TDFONTSALL.TDF, 3,536 fonts: 3,388 colour,
  146 block, 2 outline), from roysac.com, which offers it for free download (his page's FAL 1.3 covers his tools,
  not the fonts); no SHA pin (the zip must hold readable .tdf). A `.tdf` opened (openPath) or dropped joins the
  list and opens the picker. TheDraw's own TDFONTS.TDF (11 fonts, incl. Outline/BigOutline) is shareware-package
  content: not shipped, not fetched.
- **Picker**: rows of name/kind/file + a preview of the Text field in the font (`renderDoc` of the laid-out line,
  `blitScaled`, cached per font+mode+style+spacing+brush+text), the selected font larger below. Line requester:
  preview, size, characters the font lacks; Stamp = `startPaste(tdfBlock(...))`.
- **Tests** (`tdf_test.go`, helpers `tdfF...`; a writer `tdfFFile` makes fonts): parsing (cut, junk, offsets, types,
  dedupe), every outline style, block fill/solid space/descender, colour transparency rules and BIOS colours,
  typing/Backspace/Ctrl+Z/Return/wrap/click, Amiga refusal and fit filter, Unicode cells, the line stamp, library
  (folders, zip, bad files, prefs, open .tdf, find), download (httptest), requesters drawing headless.
  `TestTDFRealFiles` (CHARCOAL_TDF, default ~/.cache/charcoal-tdf/roysac; CHARCOAL_THEDRAW for TDFONTS.TDF):
  every glyph of 3,536 fonts in every style, TheDraw's Outline 'Q' and its descender. FuzzParsers reads "tdf".
- **Not done**: a font editor for .tdf (IcyDraw has one), FIGlet fonts, TheDraw's own fonts menu keys in the
  TheDraw key preset, a ghost of the next letter under the mouse, kerning. Not checked against TheDraw running
  (no DOS emulator here): the box fill and spacing columns follow IcyDraw and the manual.

## Drawing together: Moebius's collaboration protocol (2026-10-16, branch r5-collab)

- **Decided (2026-10-16):** a password is required to host for the network, not for this computer only; web
  viewers are refused when there is a password; frames or a non-PC page end the session with a message; a remote
  resize clears undo (as Moebius). Not yet done: a real Moebius Electron app joining CharEdit (the user was asked
  to try one).

- What: File > **Join a drawing session** (CharEdit as a client of a Moebius or IcyDraw server), **Host a drawing
  session** (CharEdit as the server; Moebius and IcyDraw clients join), **End the drawing session**; the session
  line above the status line (where, who, last chat line, chat field: Ctrl+Return `collab_chat`); the others'
  cursors on the page. `collab.go` (protocol, client, host, the per-pass sync), `collab_ui.go` (requesters, the
  line, cursors). Library: github.com/coder/websocket v1.8.15 (ISC, no dependencies, Go 1.23).
- Protocol (read in blocktronics/moebius app/server.js + app/document/doc.js, IcyDraw's
  icy_engine_edit/src/collaboration): `{"type": n, "data": {...}}` JSON text frames; types 0 CONNECTED ... 21
  SET_BG (consts `cbConnected`...); a cell `{code, fg, bg}` = CP437 code, BIOS colours (`cbCell`/`cbBlockOf`
  through `biosANSI`; a code > 255 from IcyDraw is mapped to CP437 or `?`); the page on joining is
  libtextmode.compress (`cbDoc`: runs `[value, repeats]` per stream, `cbExpand` refuses anything not exactly
  cols*rows), palette 16 x {r,g,b} in 6 bits (IcyDraw may send 8: `cbPalette` guesses by the maximum; the
  standard EGA one keeps the tab's palette, any other becomes the page's own, "file").
- **Sync without hooks** (the design to keep): `collabSession.shadow` is the page as the others have it. Each main
  loop pass (`collabTick`, main.go; the loop waits at most 30 ms while a session is on) first applies what came
  in to the page *and* the shadow (`collabApply`, `collabPut`), then `collabSendChanges` sends a DRAW for every
  cell where page and shadow differ, SET_CANVAS_SIZE when the size differs (the new area marked `cbUnknown`, so
  it is sent in full: IcyDraw's server fills new cells with code 0 on black, Moebius's with 32/7/0), and
  ICE_COLORS / USE_9PX_FONT / CHANGE_FONT / SAUCE when the tab's differ from the last sent, CURSOR when the text
  cursor moves (HIDE_CURSOR when the tab is not the one worked on). So every tool, undo, paste, effect is shared.
  Remote cells are also written into the tab's undo/redo snapshots of the same width, so Ctrl+Z here takes back
  only this user's steps. A remote resize clears undo (as Moebius).
- The session's tab: joining makes a new PC tab named after the session's path (`collabFileName`, "art.ans"), with
  `source` nil (so setMode/setCols never reparse it); an empty untouched start tab gives way. One session at a time
  (`collab` global). It ends (`collabEnd`) when the tab is closed, gets frames or stops being a PC page, the
  connection drops, or the user ends it; a joined page then stays, dirty.
- Hosting (`collabHost`): `net.Listen` on 127.0.0.1:port, or every address only when the user picks "Others on
  the network" (then a password is required by the requester). Any path is accepted (Moebius serves one path per
  file). Ids from `nextID`, the host is 0. Per Moebius's Joint.message: CONNECTED -> password check (REFUSED and
  close), CONNECTED {id, doc, users, chat_history, status} (lists never null: Moebius's chat.welcome iterates
  them and IcyDraw refuses a null list), JOIN to the others; DRAW/SAUCE/ICE/9PX/FONT/CANVAS_SIZE applied then
  passed on rebuilt from the checked values with the guest's real id (never the id it sent); CURSOR/SELECTION/
  OPERATION/HIDE/ROTATE/FLIP/STATUS passed on; PASTE_AS_SELECTION checked cell by cell; CHAT cleaned, kept 32.
  Web viewers (no nick) are refused when there is a password (Moebius lets them watch) and never draw.
- Untrusted input (the knowledge base, go/untrusted-input-traps.md): read limits 48 MB (the page on joining,
  2M cells of runs that never repeat) and 16 MB per guest message; `collabMaxCells` 2M, 1000 columns; strings
  through `clean` (no control characters, nicks 32 runes, chat 1000, SAUCE fields 35/20/20 trimmed of the record's
  space padding: `sauceField`); 32 guests; a guest that does not log in within 30 s is closed; a connection that
  cannot take its queue (65536 messages) is closed; up to 20000 messages a pass. Passwords are never saved
  (prefs keep server, nick, group, port, who) or logged.
- Traps found: Moebius's server sends SAUCE fields padded with spaces; its server reads UTF-8 comment lines by
  characters where the record has bytes (the corpus' sauce-utf8.ans comment loses its first letter) and **writes
  the page back over the file it serves when closed** (tools/collab/moebius-server.js never closes it; the tests
  serve copies); Moebius's client swaps author and group of a SAUCE message it receives (doc.js emits
  title, group, author); a null chat_history stops Moebius's client and IcyDraw's.
- Tests (`collab_test.go`, helpers `collabF*`): runs, cells, page limits, palettes, addresses, names; hosting with
  raw clients (password, two users, false ids, hostile messages, host edits/resize/SAUCE/iCE/chat reach both,
  remote cursor, undo keeps the other's cell, resize from a guest, leave, web viewer, only 127.0.0.1 listens,
  pages that cannot be shared, the port closed afterwards); joining a fake server built from app/server.js
  (scenario `collabFJoinScenario`: page, raw client, both draw, resize, chat, cursor, a late joiner gets the same
  page); refusals; hostile servers; `FuzzCollab` (messages to a joined and a hosted session) and FuzzParsers'
  "collab" (the page on joining). **Real servers** (tools/collab/README.md): `CHARCOAL_COLLAB_MOEBIUS` (node +
  Moebius's app/ + ws 8.15 + two stubs) runs `TestCollabMoebiusServer` (42 corpus files served by Moebius's
  server, each page = the corpus' Moebius reading, then the scenario) and `TestCollabMoebiusHost`
  (`moebius-client.js`: libtextmode.uncompress + doc.js's DRAW handling joins CharEdit's host);
  `CHARCOAL_COLLAB_ICY` / `CHARCOAL_COLLAB_ICYCLIENT` (icy_engine_edit examples `icydraw-collabsrv.rs`,
  `icydraw-collabcli.rs`, built in the bwrap sandbox) run `TestCollabIcyDrawServer` and `TestCollabIcyDrawHost`.
  All pass (2026-10-16). Screenshots: /tmp/charcoal-collab/*.png.
- Not done: the real Moebius Electron app was not run (its protocol code was); no TLS when hosting (Moebius has
  none either; `wss://` works when joining); Moebius's web viewer page is not served; Moebius's live view of a
  guest's floating block (PASTE_AS_SELECTION, ROTATE, FLIP) is passed on but not shown in CharEdit; idle/away
  status is shown but CharEdit never sends its own; SET_BG (C64 pages) ignored; no reconnect.

## Layers (2026-10-16, branch r6-layers; design: docs/layers-design.md)

- **Decided (2026-10-16, the user trusts these calls):** "black shows below" means the background below (not
  IcyDraw's lower-half-of-the-cell rule); CharEdit writes the background as seen into .icy, so IcyDraw shows the
  same picture. The side file is accepted on SHA-1 + page type + width (no cell compare: .ata keeps no colours).
  Playscii layers kept on PC/Amiga/ST only (PETSCII/Atari Playscii files have one screen background). Clicking
  the current layer's name renames it. Keys Alt+Shift+N/M/E/Up/Down, Ctrl+Shift+PgUp/PgDn.

- **The user's decisions (2026-10-16):** empty cells (blankAt) show the layers below, a black full block covers;
  "black shows below" per layer; after a flat save the layers are kept beside the file (SHA-1 of the bytes);
  layers and frames together later; 16M cells for all layers together, 64 layers at most; Pick = what is seen,
  Copy = the current layer, Edit > Copy what is seen = the picture; hidden layers left out of flat saves and
  REXPaint, with a message.
- Model (`layers.go`): `Tab.layers []Layer` (doc, name, hidden, clear 0/1, keep: the file's IcyDraw flags and
  colour, Playscii z), `Tab.lay`, `Tab.others` (view: 0 shown, 1 dim, 2 hidden). **nil = one layer = t.doc**, and
  a list always has 2+ (`oneLayer` after delete/merge), so a page of one layer is the old page byte for byte.
  **t.doc is always the live current layer**, `layers[lay].doc` stale (as frames); `goLayer` clones; layers not
  current are never changed in place. Page fields (bg, border, ink, glyph0, uni) live on t.doc (`layerFields`).
  The current layer is the page's height; the others may be shorter (missing rows are clear; `shrinkLayer` cuts
  an upper layer's empty rows when it stops being current, never the bottom one's).
- `flatten(ls, cur, d)` is the one merge; `t.seen()` = t.doc itself without layers. **`asSeen()`** puts the
  flattened page in t.doc for a while (the live layer waits in layers[lay], `t.seeing`, `layerDoc` knows) and is
  deferred at the top of `encodeFor` (so every writer and export), `playBytes`, `playPauses`, batch's animate (after
  its load); `seen()` in Animate's previews, the draw order, party check, Save for the party (`encodeStage` drops
  layers), work stages (`recordStage` flattens with the step's layer, `stageCand.lays` lazily). `shownAt` draws a
  cell (`drawLayersRow` in drawPage; dim = `fb.blend` 150 toward `layerPaper`), the status line, the hover, Pick.
- Editing: `newLayer(copied)`, `deleteLayer`, `moveLayer`, `mergeDown`, `mergeAll`, `toggleLayer`,
  `toggleBlackClear`, `renameLayer`, `cycleOthers`; `layerRefused` (page type, frames, a drawing session, 64,
  cells). Undo: `Snap.lay`, `Snap.lwhole` + `Snap.layers` (`pushLayersUndo`), `restore` goes to the edited layer,
  `snapshotLike` makes the redo of the same kind. Structure: `withEveryFrame` calls `withEveryLayer` on a layered
  page (W, machine width, crop, Unicode/TED conversions); `setRows` cuts every layer; rowscols' four ops run on
  every layer through `everyLayerDo` (`Tab.layering` makes their own pushUndo a no-op). `maxRows` = what the other
  layers leave of maxPageCells. `uniCompact` counts every layer and snapshot layer. No frames, Split, Animate as
  frames, drawing sessions or page types without layers on a layered page (`layersNoFrames`).
- UI (`layers_ui.go`): the panel section in Studio (`drawLayers`: rows top first, check box, name / rename field
  `fieldLayer`, buttons, the others choice), Page menu group, Edit > Copy what is seen (`copySeen`), keys block
  "Layers" in keys.go (Alt+Shift+N, Ctrl+Shift+PgUp/PgDn, Alt+Shift+Up/Down, Alt+Shift+M, Alt+Shift+E; the rest
  menu only; `TestDefaultKeysHaveNoClashes` checks all presets).
- Keeping (`layers_keep.go`): autosave `Lay`/`Layers` (cells one layer after another; a meta without them is one
  layer). The side file `<data dir>/layers/<sha1 of the path>.json` (`writeLayersFile` after an own-format save in
  a format that does not keep layers; `restoreLayersFile` in openPath): taken back only if the file has the SHA-1
  written, the same page type and width. Cells are not compared: an .ata keeps no colours in cells, so the page
  read differs from the layers' in bits that do not show.
- Files (`layers_formats.go`): `layersKeptBy(fmt, mode)`: .icy (PC, Amiga), .xp (PC), .psci (PC, Amiga, ST; not
  PETSCII/Atari: their one screen background makes a lone layer read back differently). Readers are unchanged;
  `fileLayers` (from reparseEditor, never in CharView) reads each layer as the file's only one with the format's
  own reader and keeps them only when `flatten` looks as the format's page (`layersKept`, `sameLook`), else the
  page opens flat as before. IcyDraw: invisible cell = empty, a layer with transparent backgrounds = black shows
  below; groups, chars/attributes modes, picture layers fall back. **IcyDraw's transparent background is not
  "the background below"**: icy_engine gives it the colour of the lower half of the cell below (HalfBlock::from_char:
  a space's bg, a full or lower half block's fg, an upper half's bg, fg when fg == bg, else 0), so the writer
  writes the background as seen where that differs (`icyLowerColour`). CharEdit's own reader (`icyMerge`) takes the
  cell's background below; left as it is (readers unchanged). REXPaint: pink = empty; hidden layers left out,
  black-shows-below cells get the colour below, more than 9 layers: the lowest merged. Playscii: one frame,
  empty = tile 0/0/0, black shows below = bg 0.
- Tests (`layers_test.go`, helpers `layF*`): flatten per page type and mode, one layer = the doc, short layers,
  every format of a two-layer page = its flat page's bytes, refusals, stages, edit/undo/redo across layers,
  structure, limits, short layers cost, Unicode styles kept, drawing (pixels = the flat page; dim/hidden differ),
  Pick and Copy what is seen, the panel, autosave (PC/Unicode/PETSCII), the side file (and a changed file opens
  flat), .icy/.xp/.psci keep layers, REXPaint's leftovers, fallbacks, CharView, an Atari side file; FuzzParsers
  "layers" (and seeds); env-gated `TestLayIcyDrawReads` (IcyDraw's engine dumps a layered .icy cell for cell as
  seen: CHARCOAL_ICYMACH/CHARCOAL_ICYDIR as TestOthIcyExternal) and `TestLayRealFiles` (CHARCOAL_LAYER_FILES).
  The big check reads pages as seen and counts files that keep their layers (none of its .icy/.xp/.psci have
  more than one layer). Screenshots: /tmp/charcoal-layers/*.png.
- Not done: layers with frames; lock (IcyDraw's lock bits are only written back); groups, offsets, text layers,
  opacity; layers on Spectrum/CPC/MSX/MZ pages (their empty cell is not blank); .icy layers of PETSCII/Atari/ST
  pages (flattened, kept beside the file); Playscii's PETSCII/Atari layers; a drawing session with layers.

## Keys and safety (2026-10-13, evening)

- `keys.go`: one table of actions (id, default keys, help line, what it does) drives every shortcut, the help's
  KEYS list (`keysHelp`) and the keys file (`~/.config/charcoal/keys` on Linux) (written with the defaults the first time; bad lines are
  skipped with a status-line message; a key the file gives an action is taken from the others' defaults). Status
  hints name keys with `keyName(id)` so they follow the file. Only the left Alt is Alt (AltGr types).
- Defaults follow Moebius/PabloDraw/TheDraw (survey: the knowledge base, bbs/text-art-editor-keys.md): Ctrl+arrows
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
- Moebius facts (verified by running its code): the knowledge base (bbs/moebius-file-format.md).

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
- Menu bar (step 2, 2026-10-14): `menus.go`. `menuBar` lists File, Edit, View, Page, Draw, Animate, Help; an item
  names an action id from `keys.go` (its keys shown are `keysNow`, so presets and the keys file show), or has
  its own `run` (machines, layouts, Settings); `tick` draws a tick. A wrong action id panics at start (tests
  catch it). The open menu is a `Popup` with `menu` set: "" items are group lines, `hot` is moved by the mouse
  or Up/Down, Return picks, Left/Right open the next menu, Esc closes. The mouse switches menus only when it
  moves onto another title (`p.mouse`); a click on the titles goes through (`ui.closedMenu`: the open one's
  title closes it). Drawn above the gadget bar (`drawBar(u, top)`), hidden with the gadgets (Ctrl+H).
- Layouts (step 3, 2026-10-14): `layout.go`, `panel.go`. Studio (default, `prefs.UILayout` ""): menu bar, one
  toolbar row (`drawBar`: New Open Save Save-as, Undo Redo, zoom Grid Ref pages Sync, Animate... speed Play), tabs,
  then a tool rail (`drawRail`), the page, and a side panel (`drawPanel`, `panelW`, scrolled by the wheel,
  `panelScroll`): Page (`drawPageGadgets(u, fl, compact)`, labelled rows), Colours (8 to a row) and the brush,
  Characters (F-key set switch in the heading, 32 or 16 columns as the font's width allows), the tool's options or
  the block's buttons (wrapping, `drawToolOptions(u, x, y, right)`). Focus (`"focus"`): the old arrangement, the
  page gadgets in the toolbar and the drawing gadgets under the page (`drawPane`). Both draw the same pieces
  (`drawTools`, `drawCharset(cols)`, `drawFsetSwitch`, `drawFkeyRow`, `drawColours(perRow)`, `drawBrush`,
  `drawBlockOrToolOptions`). View > "Studio layout" / "Focus layout" switches. `u.hover`/`u.take` ignore what
  is outside `fb.clip`, so gadgets scrolled out of the panel take no clicks. Info, Settings and ? left the
  toolbar for the menus. The status line shows the file's name, the title bar its folder.
- Themes and the interface font (step 4, 2026-10-14): `themes` in `theme.go` has Dark (the old look, the
  default), Light and Workbench; `setTheme` (View menu, Settings) stores `prefs.Theme`. `Faint` is for marks
  that are off (Animate's unlit cost bars, menu group lines): EdgeLt is white in the light themes. The text
  on `On` (cBlue) is cText/cHi, so each theme's On is light enough for its dark text or the other way round.
  `uifont.go`: `ttFace` (golang.org/x/image opentype, glyph alpha masks cached per rune, advances summed
  without kerning so `width` and `draw` agree) for `prefs.UIFont` "" = the system's sans (fc-match on
  Linux/BSD, Helvetica/SF on a Mac, Segoe UI on Windows), "bundled" = the Go font (gofont/goregular, in
  x/image, BSD), "topaz", "file" (`prefs.UIFontFile`, a FileReq from Settings). `applyUIFont` falls back to
  the Go font, then Topaz, with a message. Size `prefs.UIFontSize` (13 default); `lineH` = ascent + descent + 1
  (Noto Sans 13: 19). `centre` makes requesters bigger when lineH > 16. The art window is drawn at logical
  pixels and scaled by the Window size setting, so at 2x the interface text is pixel-doubled, not rendered at
  the screen's resolution. New module: golang.org/x/text (x/image's sfnt needs it).
- Requesters (step 5, 2026-10-14): explanations go in the gadget's hint, not in paragraphs (the BBS
  "safe choice" text in Animate and Settings); secondary notes in cDim. While a requester is open the hint
  shows as a tooltip by the mouse (`drawTooltip`, main.go): a big requester covers the status line.
- Rough edges fixed (2026-10-14): the Focus strip grows when the tool options wrap (`paneNeed`; `pass` lays the
  window out a second time at once when it changed); the tool rail has no gap where the Half tool is missing;
  gadgets after a label start past the label's measured width (`colAfter`, `settingsCol`), Animate's pattern
  list is as wide as "Checkerboard" needs, and its cost list is clipped to its column (whole rows only).
  Checked at 20 pixels on Animate, Animate a folder, Open, Info, the font editor and the menus.
- README screenshots in the new look: hero, start, amiga (Workbench theme, Animate menu), light, focus. Still in
  the old look: c64, teletext, atari, split, viewer, frames, reference, fonteditor, animate-cost, animate-folder,
  animate.gif. Headless recipe with the prefs written first: an absolute XDG_CONFIG_HOME with
  charcoal/prefs.json holding `"c64asked": true` (no first-run requester), `width`/`height`, `theme`, `ui_layout`.
  An 80-column page at 2x needs about 1720 pixels of window in the Studio layout.
- Sources: 16colo.rs packs and tags, asciiarena.se (sitemap; files at /collections/<name>/<file>),
  a folder of real BBS art (412 files), the PETSCII folders on the NAS (with the PETSCII editor's
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

- The knowledge base, amiga/ansi-art-real-world-traps.md: sources, robots rules, all the format traps
  (0x1A in art, .bin width, BIOS colour order, page height, Amiga vs PC, ATASCII, teletext, Petmate .prg).
- The knowledge base, bbs/petscii-seq.md: PETSCII/.seq, C64 screens, ATASCII and ST notes.
- The knowledge base, go/sdl2-desktop-gui.md: the GUI approach and headless testing.

Last updated 2026-10-14 (0.1b7 released; "Now" section at the top).
