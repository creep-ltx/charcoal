# Layers for CharEdit: design

Based on the code at c8d9ec8 (0.1b11). Artists ask for layers (Moebius #148: "I waste the most time on
integrating things"); IcyDraw has layers, groups and text layers, Playscii layers shown / dim / hidden, REXPaint
up to 9 layers. CharEdit already *reads* .icy, .xp and .psci layers by putting them together
(editorformats.go: `icyMerge`, `parseXP`, `parsePsci`).

The plan copies what frames did (docs/frames-design.md, frames.go), because that worked: **`t.doc` stays the
thing every tool edits, `nil` means "one", and the one case runs exactly the old code.**

## 1. The model

```go
// layers.go
type Layer struct {
    doc    *Doc   // never changed in place while it is not the current layer (as frames)
    name   string
    hidden bool
    clear  int    // which cells let the layers below show: 0 blank cells, 1 also black backgrounds
    keep   *layerKeep // what the file's layer had that CharEdit does not use (IcyDraw title colour,
                      // flags, Playscii z), written back unchanged
}
```

`Tab` gets `layers []Layer` (nil: one layer, which is `t.doc`), `lay int` (the current one, 0 = bottom) and
`others int` (how the other layers are shown: 0 as they are, 1 dim, 2 hidden; view only, not saved).

- **Rule for `t.doc`:** it is always the live current layer; `layers[lay].doc` is stale while it is current.
  `goLayer(i)` stores `a.doc` back and makes a *clone* of layer i current, exactly like `goFrame`. So layers
  that are not current are never changed in place, and undo snapshots can share their pointers.
- **`layers != nil` means at least two.** Deleting or merging down to one sets `layers = nil` again. So a page
  of one layer is, in every byte, today's page.
- **Page fields** (`bg`, `border`, `ink`, `glyph0`, `uni`, `fullest`) live on `a.doc`, as today, so the panel,
  GTIA, TED and CTM code that writes `a.doc.border` etc. stays right. `goLayer` and `flatten` copy them from
  the current doc (`layerFields(dst, src)`); the other layers' copies are stale and never read.
- **Height.** The current layer is always the page's full height: `goLayer` and `restore` grow the clone to
  the tallest layer (`ensureRows`, free on a clone). Other layers may be shorter; their missing rows are clear
  (`Doc.get` past the last row already returns `blank`). So every existing `a.doc.rows`/`a.doc.cols` (view,
  status line, select all, the H and W fields, drag limits) still means the page. When a layer stops being
  current it is cut to its used rows if that halves it (`usedRows`, a fresh slice), so a short text layer
  over a tall picture costs little. Width is the same for every layer.

### Which cells are clear (see-through)

**The rule: on a layer above the bottom, a cell is clear when `d.blankAt(v)` is true.** That is the empty
cell every page type already has (core.go `blankAt`: a space or NUL on background 0 without underline; on
PETSCII and Atari pages NUL is a glyph and so not clear; on the Unicode page `uniBlank`). It is the same idea
Shift+click stamping uses today (`stamp`: "without blanks") and the onion skin (`onionBlank`). The bottom layer
covers everything, blank cells too.

Why a rule and not an extra "nothing here" bit:
- A Unicode cell uses all 32 bits (uni.go), so there is no spare bit for every page type; a parallel bit mask
  per layer would have to be kept by every write, and every eraser, Delete, cut, move-a-block, delete-row and
  effect writes `blank`/`emptyCell()`. With the rule **every existing tool is already right on a layer**:
  erasing makes a cell clear.
- The price: a layer cannot cover what is below with a plain black space. Black full block (219, fg 0) covers
  and looks the same. (Question 1.)

**`clear = 1` ("black shows below")**, per layer, for letters over a picture: a non-blank cell whose background
is 0 takes the background of what is below; its character, foreground and italic/underline are the layer's.
That is Playscii's model (palette 0 is transparent, per cell) and IcyDraw's transparent background tag. Offered
on pages whose cells have their own background (PC, Amiga, Unicode, ST); on PETSCII and Atari pages it is the
same as 0 and not offered. On the Unicode page the merged cell needs a new style (`styleOf`), which may add to
`uniStyles`.

### Which page types get layers

| Page | Layers | Why |
|---|---|---|
| PC, Amiga, Unicode | yes | free cells; blank = space on background 0 |
| PETSCII (all machines, VIC modes), Atari, Atari ST | yes | every cell stands alone; `blankAt` is right (NUL is a glyph where it should be); screen colour, border, ink are page fields |
| Teletext, Viewdata, Minitel | no | serial attributes: a colour or mosaic code acts on the rest of the row (`ttDecode`, `mtDecode`), so a cell on one layer would change cells on others; "clear" has no meaning in a row that is decoded left to right |
| ZX Spectrum, CPC, MSX, MZ-700 | not now | fixed screens of 24-25 rows; their empty cell is not `blank` (white paper, `cpcBlank`, `msxBlank`, `mzCell(32, fg, bg)`), so clear needs a rule per machine. Possible later: `layerClear(v)` = `blankAt(v) or v == emptyCell()` |
| IGS and SkyPix pages | no | they already have a picture layer under the cells with its own saving (igs_page.go, skypix.go) |

`a.layered()` (page type allows it) gates every menu item, key and the panel section. Changing the page type
to one without layers (`setMode`, `setMachine` to teletext...) asks to merge first: "Merge the layers first".

## 2. One function puts the layers together

```go
// flatten: the page as it is seen and saved. Hidden layers are left out; a hidden bottom layer leaves blank cells.
func flatten(ls []Layer, cur int, d *Doc /* the live current layer */) *Doc
func (t *Tab) page() *Doc  // t.doc when t.layers == nil (the same pointer: nothing copied), else flatten
func (a *App) shownAt(x, y int) (v uint32, from int) // one cell, for drawing and the pick tool
```

`page()` is the only way out: every place that reads the *whole page* for output calls it. The audit of
`a.doc` / `t.doc` (about 350 uses) sorts them into three kinds:

| Kind | Where | With layers |
|---|---|---|
| **edit** (tools) | editor.go tools, shapes.go, blockjobs.go, tdf_ui.go (Logo), minitel_ui/msx (no layers), typing, paste/stamp, fill, shade | `a.doc` = the current layer, unchanged |
| **page out** | `encodeFor` (editor.go: every writer), export.go:371 (PNG/HTML/GIF), compo.go:223/543 (party check, Save for the party), stages.go:246 + `recordStage` (work stages), transitions_ui.go:199 / transitions.go:1185 (Animate's draw order from the page), animcost*.go, batch.go:524-547, start.go thumbnails, refimage.go:285, collab | `a.page()` |
| **structure** (every layer) | `setCols`, `setRows`, rowscols.go insert/delete row and column, crop (Ctrl+K), `setMachine`/`setSTRes` width, PC/Amiga <-> Unicode conversion (`setMode`), `withEveryFrame` | `withEveryLayer(fn)`, like `withEveryFrame`: new docs for every layer, one undo step |

The pick tool reads what is seen (`shownAt`), not the current layer: an artist picks the colour they see.
Flood fill, shade and the Select tool work on the current layer only (as in IcyDraw and Playscii).

### Readers and writers

- **Every reader stays as it is** (it gives one flat page). The three layered formats get a second reader
  that returns layers: `icyLayers(doc *icyDoc)`, `xpLayers(b)`, `psciLayers(f, frame)`.
- **Opening a file never changes what it looks like.** The layers are kept only when
  `flatten(layers) == the format's own merge` (today's `icyPage` / `parseXP` / `parsePsci` page) cell for cell;
  otherwise the page opens flat, as today, with a note ("its layers use transparency CharEdit's layers do not
  have: put together"). Cases that fall back: IcyDraw groups, characters-only / colours-only layer modes,
  layers with an offset or another size than the page, a visible black space covering something, a
  transparent foreground; a REXPaint black cell covering something; Playscii files of several frames (see §4).
- **Writers that keep layers**, chosen in `encodeEditor` when `a.layers != nil`:
  - `.icy` (PC/Amiga page; IcyDraw's other machines through `writeIcyMachine` later): one LAYER record per
    layer, title = name, flags bit 0 = visible (and the file's other bits from `keep`), offset 0,0, the page's
    size; a clear cell is written invisible (attr 0x8000); on a `clear = 1` layer a background 0 is the
    transparent tag (0).
  - `.xp`: one REXPaint layer per visible layer (REXPaint keeps no visibility: hidden layers are left out, with
    a message); clear cells get the pink background 255,0,255. A `clear = 1` cell's background is written as
    what is below it (REXPaint has no see-through background), with a message. More than 9 layers: the lowest
    are merged (REXPaint shows 9).
  - `.psci` (one frame): every layer with z = its place, `visible`, `name`; clear cells are char 0 fg 0 bg 0
    (transparent), a `clear = 1` layer's background 0 is Playscii's index 0.
  - **Every other format** gets `a.page()`. Hidden layers are not in it; the message says so once.

### Saving a layered page as .ans (or any flat format)

Like the work stages (`writeStages` / `restoreStages`): after an own-format save of a layered page in a flat
format, the layers go to `<data dir>/layers/<sha1 of the absolute path>` (JSON head + `packDoc` pictures) with
the SHA-1 of the bytes written. `openPath` takes them back only if the file still has those bytes, the page
type matches **and** `flatten(layers)` equals the page the file reads as. So an ANSI artist keeps working in
.ans with layers, Ctrl+S marks the page saved, and nothing is lost if the file changed elsewhere (then it opens
flat). (Question 2: the alternative is the frames rule, "page stays dirty".)

## 3. Undo, frames, stages, autosave, collaboration, Animate, CharView

- **Undo** (one stack, as now). `pushUndo()` (25 call sites) clones `a.doc`, the current layer: a cell step
  costs one layer's cells, exactly as today. `Snap` gets `lay int`, `lwhole bool`, `layers []Layer`:
  `pushLayersUndo(extra)` for add, copy, delete, move, merge, hide/show, rename, clear mode, and every
  `withEveryLayer` change (shallow copy of the list with `[lay].doc` = a clone; `cost` = that clone + 8 per
  layer + the cells of replaced docs). `restore` of a cell step of another layer stores `a.doc` back and goes
  to that layer (you see what was undone), then re-grows to the page height. `snapshotLike` makes the redo
  step of the same kind. `undoBudget` and the 300-step cap are unchanged (they sum `cost`).
- **Frames: not together in this round.** A page has frames or layers: New frame / Copy frame / Split /
  Animate's "as frames" on a layered page say "Merge the layers first (Page > Merge all layers)"; New layer on
  a page with frames says "Layers are for pages of one picture". Playscii files of several frames open as
  frames, layers put together per frame, as today. (Question 3. Later: `Frame.layers`, or IcyDraw-style
  layers shared by all frames, the useful case for animation: a still background under moving letters.)
- **Work stages:** `recordStage` packs `flatten` of the step's layers (the step's current layer = `s.doc`, the
  others as they are now; they cannot have changed in a cell step), so stages are what was seen.
  `stageCands` flattens the same way. Party check and Save for the party use `page()`.
- **Autosave:** `autosaveMeta` gets `Lay int` and `Layers []autosaveLayer{Name, Hidden, Clear, Rows, Keep}`;
  the cells of every layer follow one another in `.cells` (as frames do); the top-level Cols/Rows stay the
  current layer's. A meta without `Layers` reads as one layer, as today. `restoreAutosave` checks the length.
- **Unicode styles:** `uniCompact` (uni.go:748 `addDocs`) must count every layer's cells and every snapshot's
  layers, or a compaction renumbers styles a hidden layer still uses. One line each; a test for it.
- **Collaboration:** a layered page is not shared (`collabHost`/join refuse it like frames: "Moebius pages
  are one picture: merge the layers first"), and New layer is refused during a session. Sharing `page()` is
  not enough: a remote DRAW would have to land in some layer, and a cell covered by a higher layer would
  differ from the shadow forever.
- **Animate:** the draw order (`order`) animates `page()` and Save writes it; making frames is refused (above).
- **CharView** shows `page()`; it never keeps layers (it calls the flat readers, as today).
- **The big check:** readers are unchanged, so its report must not change. One new pass: for every .icy, .xp
  and .psci file, read layers, check `flatten(layers) == the flat reading` (the invariant above) and that a
  layered write and read gives the same layers; count how many files keep their layers.

## 4. The interface (quiet and small)

- **Panel (Studio layout):** a "Layers" section under Colours, only on pages that allow layers. With one layer
  it is one heading line with a small `+` (New layer). With two or more, one row per layer, the top layer
  first (as every editor shows them): an eye box (show / hide), the name (click: work on it; double click: a
  `Field` to rename), a small `b` mark on a `clear = 1` layer. Under them one row of small buttons: `+`, Copy,
  Up, Down, Merge down, Delete, and Others: All / Dim / Hide (Playscii's three, for the layers that are not
  current). The current layer's row is `cBlue`.
- **Focus layout (no panel), and Ctrl+H:** the status line shows `Layer 2/3 Letters` after the cursor position
  when the page has layers; the menu and keys do the rest.
- **Menu:** Page > Layers (a group): New layer, Copy layer, Delete layer, Merge down, Merge all layers, Layer
  up, Layer down, Show / hide layer, Black shows below (tick), Other layers: as they are / dim / hidden,
  Rename layer. Edit > Copy what is seen (copies `page()` cells of the selection).
- **Drawing:** `drawPage` draws `shownAt(x, y)` for each visible cell (top layer down until a cell that is not
  clear: about 16000 visible cells times the layers, no cache needed). Others = Dim: a cell that comes from a
  layer other than the current one is drawn, then `fb.blend` toward the paper (as the onion skin does, about
  50%). Others = Hide: only the current layer, its clear cells as paper.
- **The current layer for every tool:** every tool, typing, paste, stamp, the Logo tool and the effects write
  `a.doc`, so they draw on the current layer with no change. A selection and a floating block belong to the
  current layer; lifting a block leaves clear cells, so what is below shows (as artists expect).
- **Keys** (a "Layers" block in keys.go's `actions`, so help, presets and the keys file follow; checked free
  in the defaults and the Moebius/PabloDraw/TheDraw/AcidDraw presets; the implementer re-runs the clash test):
  Ctrl+Shift+PageUp / Ctrl+Shift+PageDown layer above / below, Alt+Shift+N new layer, Alt+Shift+M merge down,
  Alt+Shift+Up / Alt+Shift+Down move the layer up / down, Alt+Shift+E show / hide. The rest are menu only.

## 5. Memory and limits

- **All layers of a page together stay under `maxPageCells` (16M cells, 64 MB)**, the rule frames already
  have. New / Copy layer is refused past it with a message; `a.maxRows()` on a layered page is
  `(maxPageCells - cells of the other layers) / cols`, so drawing downwards cannot pass it either.
  Examples: 80 x 100000 (8M cells): the picture and one more full-height layer, or the picture and any number
  of short layers (a 30-row text layer is 2400 cells, cut when it stops being current). 80 x 1000: 200 layers.
- **At most 64 layers** (REXPaint 9, Playscii and IcyDraw have no limit; 64 rows already need scrolling).
- Undo: a cell step keeps one layer, as today; a structural step keeps only replaced docs. Flatten for saving
  an 80 x 100000 page of 2 layers: 16M cells read once, tens of milliseconds.
- Readers keep their limits (`icyMaxCells`, the REXPaint 64 MB unpacked, Playscii's cells); the layered
  readers add the 64-layer cap and fall back to flat past it.

## 6. Keeping every existing test green

- `layers == nil` is the only state any existing page, file, test or autosave can be in; `page()` returns
  the same `*Doc`. Every new branch is behind `a.layers != nil`. So bytes written and PNGs drawn are the old
  ones by construction.
- New tests prove it: for a set of pages (PC, Amiga, Unicode, PETSCII, Atari, ST) a two-layer page whose top
  layer is all clear writes the same bytes in every format as the one-layer page; merging all gives the page
  back; `go test ./...` and the big check unchanged after each commit.

## 7. The work, in small commits (each with its tests, prefix `lay`)

1. `layers.go`: `Layer`, `Tab.layers/lay/others`, `flatten`, `page`, `shownAt`, `layerFields`, the nil rule,
   `a.layered()`. Tests: flatten per page type and both clear modes, hidden layers, short layers, one-layer
   identity (`page() == doc`).
2. Every "page out" use goes through `page()`; refusals (frames, collab, page types). Tests: two-layer pages
   save every format as their flattened one-layer page does; party check and stages see the flat page.
3. Editing: `goLayer`, new / copy / delete / move / merge down / merge all / hide / rename / clear mode, undo
   (`Snap.lay/lwhole/layers`, `restore`, `snapshotLike`), `withEveryLayer` for W, H, rows/columns, crop,
   machine, conversion; `maxRows` share; `uniCompact`. Tests: undo/redo across layers and structure, budget,
   page height rules, Unicode styles kept.
4. Drawing: `drawPage` through `shownAt`, others dim / hidden, pick what is seen, Copy what is seen.
   Headless screenshots in /tmp/charcoal-layers/.
5. Interface: panel section, Page > Layers menu, keys block, status line. Tests: menu action ids, keys.
6. Autosave of layers; the layers kept beside a flat file (`<data dir>/layers/`). Tests: restart brings them
   back; an outside change or another look opens flat.
7. Layered .icy (read + write), then .xp, then .psci (one frame). Tests: the invariant on real files
   (testdata, the corpus), round trips keep layers, IcyDraw's own reader on our files where the tools exist,
   `FuzzParsers` runs the layered readers and checks the invariant. Big check pass.
8. README (formats table: the three keep layers; keys), HANDOFF section, roadmap tick, `kb/layers.md`.

**If it grows, cut in this order:** the .xp and .psci layered writers (keep .icy); the sidecar for flat files
(then flat saves keep the page dirty, as frames do); `clear = 1`; others = dim (keep hide); Copy what is seen.
Not in this round at all: lock (IcyDraw's lock flags are kept in `keep` and written back), groups, layer
offsets, text layers, layer opacity, layers with frames.

## 8. The user's answers (2026-10-16)

1. Clear cells by the `blankAt` rule, as proposed (a black full block covers); `clear = 1` per layer as designed.
2. After a flat save the layers are kept beside the file, keyed by the SHA-1 of the bytes written.
3. Layers together with frames: later.
4. 16M cells for all layers together.
5. Pick = what is seen; Copy = the current layer; Edit > Copy what is seen = the picture.
6. Hidden layers are left out of flat saves and REXPaint files, with a message.
