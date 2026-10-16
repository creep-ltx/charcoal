# Work stages for compos: design

Some compos want the stages of the work handed in with the entry, to show it was drawn by hand: Revision's
oldskool graphics exactly 10, Zoo's PETSCII at least 10, Deadline 6 (sources in compo.go's rule table).
CharEdit should write them from the page's own history, so an artist does not have to remember to save copies.

## What CharEdit keeps today (read from the code at 0cad3ce)

- **Undo** (editor.go `pushSnap`): a stack of whole-page snapshots, one per step, at most **300 steps** or
  `undoBudget` (16M cells) per page; the oldest are dropped first. A page of 80x25 drawn by hand takes thousands
  of steps (every stroke, every run of typing, every fill is one), so after an hour the start of the work is gone.
- Opening a file empties it (`loadBytes`, `finishLoad`: `a.undo, a.redo = nil, nil`); quitting loses it.
- **Autosave** (autosave.go) keeps only the page as it is now, not its history.
- Saving does not touch the undo stack, so within one session the last 300 steps survive saves.

So the undo stack alone can give stages only for a short session. A work that takes evenings needs a
history of its own: small, spread over the whole work, kept with the file.

## The design: a thinned stage log per page

- `Tab.steps` is the page's **depth**: how many steps led to the page as it is. `pushSnap` adds one, undo
  and redo set it to the step's own (`Snap.step`). After a file is opened with its log, it goes on from the
  log's depth.
- `Tab.stages *stageLog`: pictures of the page at depths that are multiples of `every` (1 at first).
  `pushSnap` gets the snapshot of the page before the step, at depth n: samples deeper than n are dropped (an
  undone branch the work left), and if n is a multiple of `every` the picture is kept.
- **Thinning**: past 64 pictures, or 24 MB, `every` doubles and the pictures off the new multiples go. The log
  is then always <= 64 pictures spread evenly over the whole work (32-64 of them), whatever its length:
  enough for 10 stages (Revision), 6 (Deadline), "at least 10" (Zoo).
- **Memory**: a picture is its cells (4 bytes each, little-endian) packed with flate (BestSpeed), plus the
  Doc's size and colours. Text art packs very well (measured in `compo_test.go` on real pages: see HANDOFF).
  A Unicode page's styles past 256 are renumbered into the picture's own small table (so `uniCompact` and
  another session's table do not matter).
- **Kept with the file**: when the page is saved in one of its own formats (writeOut's `own`), the log goes
  to `<data dir>/stages/<sha1 of the file's absolute path>.json`, with the SHA-1 of the bytes written. Opening
  the file again takes the log back only if the file's bytes still have that SHA-1 (edited elsewhere: the
  work there is unknown, so the stages start again from the file) and the page type is the same.
- **Autosave**: the log goes to `<id>.stages` beside the autosave and comes back with it.
- Exports (PNG ...) do not write the log: they are not the page's file.

## Export work stages...

- File menu (and an action without a default key): a small requester with the number of stages (default
  from the chosen party, else 10), PNG size (1x / 2x), "also in the page's format" (its saveFmt: the .prg,
  .ans ... of each stage), the folder and the base name. Files: `name-stage01.png` ... `name-stage10.png`.
- Which pictures: every candidate with its depth (the log's pictures, the undo stack's snapshots of the same
  page type, and the page now). Stage k of N wants depth k x depth / N; the nearest candidate deeper than the
  stage before is taken (so stages never repeat and stage N is the page now). With fewer different
  pictures than N, as many as there are are written and the message says so (never padded with copies).
- Stage 1 is not the empty page: it is the first tenth of the work. The empty page is no stage.
- A picture is written by swapping it into a copy of the tab (`stageTab`): the page's font, colours, palette
  and machine as they are now (a log does not keep fonts or CPC inks: drawing stages are about the cells).

## Not done (on purpose)

- No per-cell diff log (smaller, but a bug there loses work; whole packed pictures are simple and small).
- Frames: the log follows the frame being worked on (`a.doc`); an animation's stages show whichever frame was
  being drawn at the time.
- No log kept for files saved only as exports, or for pages never saved (the autosave keeps it until then).

Written 2026-10-16 (proposed, then built: compo.go, stages.go).
