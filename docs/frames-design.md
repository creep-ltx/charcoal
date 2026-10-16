# Frames and animation for CharEdit: design

This plan is based on the code as it is at ce29c21 (0.1b2), plus the roadmap, HANDOFF and the two Knowledge notes.

## 0. A data-loss bug to fix first

**Saving a multi-screen `.c` or `.petmate` file today wipes every screen but the first.** `loadBytes` (editor.go, the `kind != ""` branch) opens screen 1 in a tab that has the file's path. Screens 2 to n open in their own tabs with `path ""`. Saving the first tab runs `encode("marq")`, which calls `writeMarq(d)` with one array, or `writePetmate(d)` with one framebuf and `customFonts` emptied. That one screen goes over the file and the other screens are gone from disk.

There are two ways to fix it:
- **Fold it into Step 2 below** (the frames model fixes it by design).
- **Ship a stopgap now.** Like `writeTTISubpage`: `writeMarqScreens(src []byte, i int, d *Doc)` re-reads all screens from `a.source`, puts back screen `i` and writes them all. Give the other tabs the path too. Test: open a 2-screen `.c`, save tab 1 unchanged, and both screens read back.

## 1. Data model

New file `frames.go`:

```go
type Frame struct {
    doc   *Doc   // never changed in place while it is not the current frame (see the rule below)
    hold  int    // ms shown when played; 0 means holdDefault(mode): 100, teletext 8000
    name  string // Petmate screen name
    lower int    // PETSCII case of this screen: -1 not said, 0 upper, 1 lower
    seg   []byte // .tti: the subpage text it came from (or was duplicated from); nil for a new frame
}
```

**Changes to `Tab`:**
- Add `frames []Frame` and `cur int`.
- Add `onion bool` and `strip int` (0 automatic, 1 on, -1 off).
- `frames == nil` means one frame, which is `t.doc`. Single-picture code paths therefore stay as they are.
- Structural operations turn nil into `[]Frame{{doc: t.doc, lower: -1}}`.
- `reparse()` sets `frames = nil, cur = 0` before its switch. The multi-screen kinds then fill `frames`.

**Rule for `t.doc`:** it is always the live current frame. `frames[cur].doc` is stale while that frame is current.

**Helpers:**
- `(t *Tab) nFrames()`
- `frameDoc(i)` returns `t.doc` when `i == cur`
- `allDocs() []*Doc`, used by save, export, autosave and play
- `(a *App) goFrame(i)`: does `frames[cur].doc = a.doc`, then `a.doc = frames[i].doc.clone()`, sets `cur`, and applies `frames[i].lower` through `setCase`. It also clears `sel`, `float` and `inRun`, keeps `cx`/`cy`, and calls `applyLook`.
- `newFrame()`: a blank frame after `cur`, the same size and colours as the current one (`bg`, `border`, `ink`, `glyph0`).
- `dupFrame()`, `deleteFrame()` (refused on the last frame), `moveFrame(d)`, `setHold(ms)`.

**Why the clone in `goFrame`:** frames that are not current are never changed in place. Any change to all frames replaces their docs with new ones (`withCols` already returns a new Doc). So snapshots can share those pointers safely.

### Undo

There is still one stack per tab, with two kinds of snapshot:

```go
type Snap struct {
    doc    *Doc
    cx, cy int
    gen    int
    cur    int      // the frame it belongs to
    frames []Frame  // nil: a cell edit of frame cur; non-nil: the whole list (structural step)
    cost   int      // cells this step holds for the budget
}
```

- **`pushUndo()`** (the 25 call sites stay as they are) stores `cur` plus a clone of the current doc, exactly as today. `cost = len(cells)`.
- **`pushFramesUndo(extra int)`** is new. It is used for new, duplicate, delete, move, hold, split and all-frames width changes. It stores a shallow copy of `frames` with `[cur].doc` set to the clone. `cost` = the clone's cells + 8 per frame + `extra` (the cells of docs that were replaced or deleted).
- **`restore(s)`:**
  - Cell edit: if `s.cur != a.cur`, store `a.doc` back into `frames[a.cur]` first. Then set `cur = s.cur` and `doc = s.doc`.
  - Structural: `frames = slices.Clone(s.frames)`, then `cur` and `doc` from the snapshot.
- **`doUndo`/`doRedo`** push a redo/undo snapshot of the same kind as the step they pop (`snapshotLike(s)`).
- Undoing an edit made in another frame jumps there, so you see what was undone.
- **Budget:** `undoBudget` (16M cells) and the 300-step cap stay. The loop sums `s.cost` instead of `len(s.doc.cells)`.
- **All frames together** stay under `maxPageCells`; new or duplicate frames are refused past it with a message. `parsePetmate` already caps total cells this way.

### What applies to all frames, and what to one

- **All frames** (one structural step, new docs): W (`setCols`), `setMachine`/`setSTRes` width changes, crop (Ctrl+K).
- **The current frame only:** H (`setRows`; each frame is a page with its own real height, as Petmate screens are), the screen colour, and all drawing.
- `setMode` on a clean source re-reads, and `reparse` rebuilds the frames.

### Saving a multi-frame page in a one-picture format

This covers `.scr`, `.ep1`, `.url`, `.bin`, `.xb`, `.pcb`, `.avt`, `.prg`, `.txt`, `.png` and `.html`. `writeOut` writes the current frame as an export: no `markSaved` and no change of `path`/`saveFmt`. The message reads: "… holds one picture: frame 3 written; the other 11 are only here (save as .ans/.seq/.c/.petmate/.tti/.gif to keep them)". So nothing is marked clean that isn't on disk.

### Autosave (`autosave.go`)

- `<id>.cells` holds every frame's cells one after the other.
- `autosaveMeta` gets `Cur int` and `Frames []autosaveFrame{Cols, Rows, Bg, Border, Ink, Hold, Name, Lower, Seg []byte}`. `Seg` is base64 through JSON, so a carousel autosave doesn't depend on the file staying the same.
- The top-level `Cols`/`Rows` stay as frame 0's.
- `restoreAutosave` checks `len(b) == sum 4*c*r`.
- A meta without `Frames` reads as one frame, as today.
- An old meta with `Subpage = k`: re-read the file's subpages as frames, put the autosaved doc in frame k-1, and make it current.
- `autosaveNow` should use `t.playing.saved` while a tab is playing. Today it would autosave the screen being played; that is a small existing bug.

## 2. UI

### The frame strip

- `drawStrip(u, r)` in view.go. In `drawWindow`, one `gadH` row is taken between the page area and the bottom pane: `strip := Rect{0, pane.y-gadH, W, gadH}`. It goes before `drawPages` so the page shrinks.
- It is shown when `!a.bare && (nFrames()>1 && strip != -1 || strip == 1)`.
- `cDark` background like `drawTabs`, so it stays quiet:
  - Left: numbered boxes `1 2 3 [4] 5`, no thumbnails. The current one is `cBlue`. Overflow arrows work as in `drawTabs`.
  - Hint: "Frame 4 of 12, 100 ms, screen_004".
  - Click goes to a frame. Drag with `u.drag = "frame"` and release over another box to move it.
- Right group, in small buttons:
  - Hold field: `Hold [100] ms`, a `Field` like `fieldRows`; in seconds on teletext.
  - `New`, `Copy`, `Delete`, `Onion`, `Play`/`Stop`, `Loop`.
  - A speed choice: ½× 1× 2× 4×.
- **Split offer:** on a one-frame stream page whose file splits into 2+ pictures (§3), the strip offers `Split: 12 pictures (clear screen)` and `… (cursor home: N)`.
- **Status line** (`drawStatus`): add `Frame 4/12` after the cursor position when `nFrames()>1`.
- **Hold steps:** 20, 40, 60, 80, 100, 150, 200, 300, 500, 750, 1000, 1500, 2000, 3000, 5000, 8000 ms. Teletext uses whole seconds, 1-99, the `.tti` CT range.

### Onion skin (in `drawPage`, after the cells, before grid and selection)

- Runs when `a.onion && cur > 0 && a.playing == nil`.
- Take `prev := frames[cur-1].doc`. For each visible cell where `a.doc.blankAt(cur)` is true and `prev` is not blank:
  1. `drawCell(prev cell)` into the cell rect;
  2. `fb.blend(rect, paper, 190)`, where paper is `a.pal[a.doc.bg]` on screen pages and `cPaper` elsewhere, so it shows at about 25%.
- Teletext: decode the previous row with `ttDecode(prev, y, …)` and use the cells whose current code is a space.
- Only the previous frame for now; previous and next together can come later.

### Keys

Checked against every default in keys.go and the fixed keys in `editorKey`. All of these are free. Add them as one "Frames" block in `actions`, so the help and the keys file follow automatically.

| id | default | help |
|---|---|---|
| `frame_next` | `Alt+PageDown, Alt+.` | Next frame |
| `frame_previous` | `Alt+PageUp, Alt+Comma` | Previous frame |
| `frame_first` / `frame_last` | `Alt+Home` / `Alt+End` | First / last frame |
| `frame_new` | `Alt+N` | New empty frame after this one |
| `frame_duplicate` | `Ctrl+Shift+D` | Duplicate the frame (Marq's `d`) |
| `frame_delete` | `Alt+Delete` | Delete the frame |
| `frame_earlier` / `frame_later` | `Alt+Shift+PageUp` / `Alt+Shift+PageDown` | Move the frame earlier / later |
| `frame_hold_less` / `frame_hold_more` | `Alt+[`, `Alt+Keypad -` / `Alt+]`, `Alt+Keypad +` | Hold time shorter / longer (as CharView's `[` `]`) |
| `onion` | `Alt+O` | Onion skin on / off |
| `play` | `Ctrl+P` | Play the frames (one frame: play it as from a modem); again or Esc stops |
| `frames_strip` | `Ctrl+Shift+F` | Show / hide the frame strip |

Things to watch with these keys:
- **Write `Alt+Comma`, not `Alt+,`, in the defaults string.** `applyKeyFile` and `defaultKeys` split on `,`. `keyAliases` already maps `comma`.
- `Alt+O` is PabloDraw's ellipse. When the Step 2.4 shapes come, they need another key.
- `[` and `]` don't exist on Nordic layouts, hence the keypad pair.
- Cmd+P, Cmd+Shift+D and Cmd+Shift+F are fine on a Mac (only Cmd+Q/H/M are reserved).
- `TestDefaultKeysHaveNoClashes` covers this table.

### Playback

Generalise `Play` (editor.go) rather than adding a second mechanism. Add fields `frames []*Doc`, `holds []int`, `fi int`, `next time.Time`, `loop bool` and `speed float64`.

- **`playFrames()`:**
  1. `saved = snapshot()`
  2. `a.doc = frames[0]` (read-only: `keyDown` already blocks everything but Esc while `a.playing != nil`, and `drawPane` goes inert)
- **`playStep`:** when `p.frames != nil`, advance by elapsed time: `fi++`, wrap if `loop`, else stop. Pass `now` in (`playStep(now)`) so tests don't need the clock.
- `stopPlay`/`endPlay` restore as today.
- `main.go` already waits 15 ms while `a.playing != nil`.
- `Esc` and `Ctrl+P` stop. Loop is on by default. The speed comes from the strip choice.
- The existing Play button in the bar (modem, at the baud rate) plays the whole stream of frames, because `playBytes` (§3) returns the joined stream.

## 3. Files

### Read and write, by format

| Format | Frames read | Hold | Write |
|---|---|---|---|
| Marq / Petmate `.c` (`parseMarq` already returns `[]PetScreen`) | each `{…};` block | none in the format | `writeMarqFrames(docs []*Doc, lower bool, machine string)`: `frame%04d` arrays in order, one `// META:` at the end (as Petmate writes); `writeMarq` becomes the n=1 case |
| `.petmate` (`parsePetmate`) | `framebufs`, in `screens` order | none | `writePetmateFrames(src []byte, frames)`: decode the source as `map[string]json.RawMessage` (top level and per framebuf), replace `framebuf`/size/colours/charset/name for each frame, keep unknown fields and `customFonts`, write `screens: [0..n-1]` |
| `.tti` | subpages (`ttiSubpages`); `Frame.seg` = the segment | `CT,n,T` in the subpage, else the header's, else 8 s; `CT,n,C` (cycles) is kept, not converted | `writeTTIFrames(head, frames)`, below |
| `.ans` / Amiga ANSI | split on clear (opt-in) | from baud on split; not saved | `writeAnsiFrames`: clear before every frame after the first (`ESC[2J ESC[1;1H`, FF on Amiga) |
| `.seq` | split on `0x93` / `0x13` (opt-in) | ditto | `writeSeqFrames`: `0x93` between frames |
| `.ata` | split on `0x7D` (opt-in) | ditto | `0x7D` between |
| `.vt` (ST) | split on `ESC E` (needs a `beforeClear` in `STTerm`) | ditto | `ESC E` between |

To check before Step 2:
- Whether Petmate9 (wbochar/petmate9) added a per-screen delay field. If it did, read and write it as `hold`.
- Whether Marq's `petscii.zip` source stores a frame delay (`gh search code`).

**Unchanged saves stay byte-identical.** In `writeOut`/`encode`, if `srcKind` is `marq` or `petmate`, `fmtID == saveFmt` and `gen == savedGen`, write `a.source` as it is. That makes an unchanged `.c`/`.petmate` save byte-identical, which it is not today. Don't apply this to `srcKind ""` ANSI, because `a.source` may be the UTF-8-converted copy.

### `writeTTIFrames`

New helper in teletext.go: `ttiHead(b) (head []byte, subs [][]byte)`. It returns the lines before the first PN separately. Joined back together it gives `b`; test that.

Writing: `head` + for each frame:
- `f.seg != nil`: `ttiReplaceSubpage(f.seg, doc)`, which returns `seg` unchanged when the codes are the same.
- `f.seg == nil`: a new segment, `PN,<the first PN's mpp><ss>` and `SC,<ss as 4 digits>`, then the rows.

Renumbering PN `ss` and `SC` to 1..n happens only when `Tab.framesRestructured` is set (new, delete or move since the load or last save), so an unchanged file keeps its bytes.

A hold that differs from what was read rewrites or inserts that segment's `CT,n,T` line. Only that subpage then changes.

### Splitting a stream into frames (`frames.go`)

`splitStream(b []byte, mode string, cols int, atHome bool) (docs []*Doc, holdsMs []int, homes int)` runs the matching emulator (`newTerm`, `newPetTerm`, `newAtariTerm`, `newSTTerm`).

- Through `beforeClear` (and a new `beforeHome` hook in `Term` for `CSI H` / `CSI 1;1H`, and in `PetTerm` for `0x13`), it clones the screen as a frame when it has ink.
- The last screen is added at the end.
- Hold = bytes since the last split × 10 × 1000 / `a.baud` (minimum 20 ms), so the feel at the chosen speed carries over.
- Limits: 10,000 frames, and total cells under `maxPageCells`.

**Load:** `reparse` keeps today's single picture (fullest or last). If the clear split gives 2 or more frames, the `fullest` message becomes "An animation of 12 pictures: <keyName("frames_strip")> shows its frames".

**`splitFrames(atHome)`** is one structural undo step. It sets the frames and marks the page dirty.

**Splitting at cursor home is never automatic.** Many still ANSIs start with `ESC[1;1H`. It is only offered when `homes >= 2` after ink.

**Saving.** `encode` for `ansi`/`seq`/`ata`/`vt52` with `nFrames()>1` calls the `write*Frames` function. SAUCE gets the first frame's cols and its rows (at least 25). A message says the hold times aren't kept, because the speed of these formats is the baud rate. `playBytes` returns the same joined stream.

**Later:** "only the changes" delta writing (cursor moves + changed runs, smaller and smoother at baud) as an Info option.

### GIF export (stdlib `image/gif`, no new dependency)

- **`renderDoc(d *Doc, z int) *FB`:** factored out of `encode`'s `png` branch (the border for `screen()`/`zx()`, `paintCells`, the 32000-px limit). PNG export uses it too.
- **`writeGIF(frames, holds, z) []byte`** in a new `anim.go`:
  - The canvas is the largest frame, padded with the border/background colour.
  - Each FB is turned into an `*image.Paletted` with a palette built from the pixels (16 colours plus the border, always ≤256, exact).
  - `Delay = round(hold/10)`, minimum 2.
  - `LoopCount 0`; when Loop is off, `-1`.
  - Size savings: crop each frame after the first to the box of changed pixels with `DisposalNone`. A frame identical to the one before has its delay added to that one and is dropped.
- **Rendering rules:**
  - A PETSCII frame with its own `lower` goes through `setCase` and back.
  - Teletext renders with `a.blink = true`.
- **Formats:** add `{"gif", "Animation (.gif)", ".gif", all modes}` and `{"apng", "Animation, APNG (.png)", ".png", all modes}` to `formats`. Both are exports in `writeOut`, not the page's own file.

### APNG export (`apng.go`, written by hand, about 120 lines)

Recommendation: no library. go.mod has only go-sdl2, and the format is small.

For each frame, `png.Encode` the same-palette `*image.Paletted`, walk its chunks and take the concatenated `IDAT` payloads. Then write:

1. the signature
2. the first frame's `IHDR` and `PLTE`
3. `acTL(num_frames, num_plays = 0 for loop, 1 for not)`
4. per frame: `fcTL(seq, w, h, x, y, delay_num = hold ms, delay_den = 1000, dispose 0, blend 0)`; frame 0 takes `IDAT`, the others `fdAT(seq, data)`; one running `seq` across `fcTL` and `fdAT`
5. `IEND`

CRC with `hash/crc32`. All frames are padded to the canvas size, so one `IHDR`/`PLTE` fits all of them.

If a library is wanted instead, kettek/apng (a fork of image/png) is, as I recall, BSD-3-Clause. I could not check that here; there is no copy in the module cache and I did not fetch it. It would need to be confirmed and added to the README's licences.

### Play stream at baud

There is nothing new beyond `playBytes` joining the frames. The `.ans`/`.seq`/`.ata`/`.vt` save is the stream.

## 4. Moving multi-screen tabs to frames

**`loadBytes`, the C64 kinds branch:**
- Delete the `for i := 1; i < len(scr)` tab loop and its "each is in a tab of its own" message.
- `reparse` case `"marq","petmate","prg"`: build `frames` from every `PetScreen` (`doc`, `name`, `lower`). Then `a.doc = frames[0].doc.clone()` and `cur = 0`.
- `screenMachine(scr[0].machine)` as today.
- Message: "name: 4 screens, opened as frames".

**`.tti`:**
- `reparse` case `srcKind == "tti"`: frames from the subpages (`seg`, the `parseTTI(seg)` doc, the CT hold).
- Delete `openTTISubpages` and the `ttiSubs > 1` branch.

**`writeOut`:**
- Remove the `(subpage %d of %d)` naming and the loop that refreshes other tabs' `source`. Also drop the `ttiCount` uses that existed only for those.
- After a save of the page's own format, set every frame's `seg` again from the written file. For `.tti`, take `ttiHead(data)` subpages in order and clear `framesRestructured`.
- Message: "Saved X: 3 subpages, 1 changed".

**`srcIndex`:** only the single-screen `prg` path still uses it; remove it at the end.

**What stays byte-identical:**
- `.tti`: the same as now. Unchanged gives the same bytes. Editing subpage k changes only its OL 0-24 rows. PN, SC, PS, CT, FL, OL 25-29 and unknown lines are kept.
- `.c`/`.petmate`: byte-identical when unchanged (new, through the source rule). When edited, the content round-trips.

## 5. Tests and build order

Each step is one commit and shippable on its own. Tests use `testApp`, with XDG dirs in a temp folder for anything that saves.

1. **Stopgap fix (optional, see §0).** `editor_fixes_test.go`: a 2-screen `.c` saved from tab 1 keeps screen 2.

2. **Frame model and undo.** `frames.go`; `Snap`/`pushFramesUndo`/`restore` changes; keys; `Frame 2/3` in the status line; one-picture formats save as exports. Tests in `frames_test.go`:
   - `TestFrameUndoAcrossFrames`: edit f0, go to f1, edit, two undos, then redo: content and `cur` are right.
   - `TestFrameDocsNotShared`: edit a duplicate and the original stays the same; undo a delete and get the frame's earlier content back.
   - `TestFrameUndoBudget`: many frames and structural steps stay under budget.
   - `TestFramesWidthAllFrames`.
   - `TestOnePictureFormatIsExport`.
   - Existing: `TestDefaultKeysHaveNoClashes` and `TestHelpNamesEveryAction` cover the keys.

3. **`.c`/`.petmate` as frames.** `writeMarqFrames`, `writePetmateFrames`, the unchanged-is-source rule. Tests:
   - A 3-frame `.c` opens as 1 tab with 3 frames; unchanged save is byte-identical; edit frame 2, save, read back 3 frames with the edit.
   - `.petmate` keeps `customFonts` and an unknown field.
   - The two-frame case in `TestMarq` extended.
   - Fuzz: `writeMarqFrames(parseMarq(b))` re-parses.

4. **`.tti` carousel as frames.** `ttiHead`, `writeTTIFrames`, CT holds. Rewrite `tti_subpages_test.go`:
   - one tab, `len(frames)==3`;
   - each frame's row 1 is right;
   - unchanged save is byte-identical;
   - edit frame 2: only its rows change and the header lines are kept (same `want` string as today);
   - edits to frames 2 and 3 both kept in one save;
   - a new frame gets `PN,11104` / `SC,0004`;
   - a move renumbers;
   - a CT hold is read, and a changed hold writes `CT,n,T` only in its subpage;
   - `TestTTIOneSubpage` and `TestTTINewPage` unchanged.

5. **Autosave of frames.** `TestAutosaveRoundTrip` extended to 3 frames with holds and segments. Old single-frame and old `Subpage` metas are still restored.

6. **Strip and onion skin.** `drawStrip`, the onion pass, the hold field. Headless screenshots with `CHARCOAL_SCRIPT` (script.go): `key alt+N; …; shot` under `SDL_VIDEODRIVER=offscreen`, never on the user's display. Unit test: `strip` visibility rules. A pixel check of onion on a 1-cell page (`renderDoc` / `drawPage` into an FB).

7. **Playback.** `Play` generalised, `Ctrl+P`. Tests: `playStep(now)` advances by holds, loops, stops without Loop and restores the page. `TestPlaySavesThePage` still passes.

8. **GIF export.** `renderDoc` refactor (the existing PNG export test must give the same bytes), `writeGIF`. Tests: `gif.DecodeAll` gives the frame count, delays, `LoopCount`, a cell's pixel equal to `a.pal[i]`, and identical frames merged.

9. **APNG export.** Tests:
   - parse the chunks: `acTL` `num_frames`, `seq` strictly increasing over `fcTL` and `fdAT`, every CRC valid;
   - `png.Decode` reads frame 0 (APNG falls back to a plain PNG);
   - `num_plays` follows Loop.

10. **Streams.** `splitStream`, `beforeHome` hooks, the STTerm `beforeClear`, `write{Ansi,Seq,Ata,VT52}Frames`, the split offer in the strip, the new load message. Tests:
    - frames written as `.ans`/`.seq`/`.ata`/`.vt` and split again give the same frames (use `cellsMatch` from `bigcheck_test.go`, moved to a non-tagged file);
    - a still ANSI that starts with `ESC[2J` or `ESC[1;1H` is not offered as an animation;
    - holds from baud;
    - `FuzzParsers` also runs `splitStream`, with frame count and total cells bounded.
    - Big check: report the frame counts of files that split, and round-trip each frame.

11. **Later:**
    - delta stream writing;
    - CharView animating frames with their holds (`m` keeps modem play);
    - onion of the next frame;
    - timed capture of scrolling ANSImations at baud.

After each step, update HANDOFF.md and tick roadmap Step 2.1.

### Critical files for implementation
- editor.go (`Tab`, `Snap`, `pushUndo`/`restore`/`doUndo`, `reparse`, `loadBytes`, `openTTISubpages`, `encode`, `writeOut`, `Play`/`playStep`)
- view.go (`drawWindow` layout for the strip, `drawPage` onion, `drawStatus`)
- export.go (`writeMarq`/`parseMarq`, `writePetmate`/`parsePetmate`, the PNG render to factor out)
- teletext.go (`ttiSubpages`, `ttiReplaceSubpage`, `writeTTISubpage`, new `ttiHead`/`writeTTIFrames`)
- autosave.go (`autosaveMeta`, `writeAutosave`, `restoreAutosave`)
- Also: keys.go (the Frames action block), core.go (`Term` `beforeClear`, a new `beforeHome`), pet.go, atari.go, st.go (split hooks), tti_subpages_test.go (to rewrite)
- New files: frames.go, anim.go (GIF), apng.go, frames_test.go