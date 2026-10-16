# IGS on the Atari ST page: design (2026-10-16)

IGS ("Instant Graphics and Sound", Larry Mears, 1988-2024; spec IG220.TXT) is the Atari ST BBS graphics
language: `G#` followed by one-letter commands (`L` line, `B` box, `F` flood fill, `W` text, `G` blit, `S`
palette, `&` loop ...) mixed into a VT52 text stream. An IGS file draws **pixels** with GEM VDI, at the ST's
screen resolution (320x200 16 colours, 640x200 4, 640x400 2). Charcoal is a cell editor, so the question is
what of it fits.

## What the evidence says

- Real IGS files (IcyDraw's test set: the IGNITE01 art pack, the IG 2.20 examples, 40 files) are almost all
  pure graphics: lines, polygons, fills, patterns, VDI text, blits, palette tricks. VT52 text is rare, and when
  it is there it is a menu or a caption on top of a drawing.
- Many end the same way: draw, a pause (`t>15`), a sound, then wipe it all (`s>4` / `s>5` clear, `I>0` reset
  palette, `R>1,1` back to medium resolution). Some fade the palette to black before the wipe, some draw in a
  black palette and reveal it with `S` at the end, some cycle colours (`S` ... `q>10` ... `S`).
- A pixel drawing editor for IGS would be a second program (IcyDraw has one: a script editor with a live
  canvas). Charcoal's cells cannot hold a VDI ellipse. **So: Charcoal shows IGS files exactly, keeps every
  byte of the drawing, and lets you edit the VT52 text on top of it.** That is what a sysop does with an IGS
  screen (put the board's menu or name over it), and it round-trips.

## The model: a picture under the cells

An IGS page is an Atari ST page (st.go) with a picture: `Tab.igs *igsLayer`.

- The file is split into three parts at command boundaries:
  - **head**: from the start to the end of the last command that changes pixels (a drawing command, a clear,
    a palette reset or a resolution switch that is followed by more drawing or text). It is run by Charcoal's
    IGS interpreter into a picture (register numbers per pixel). VT52 text in the head is drawn into the
    picture too, in the ST's font, as the ST does.
  - **text**: from there to the closing. Its VT52 text (and IGS's own VT52 commands `c p m l d i r v w`)
    goes into the page's **cells**, printed by the ST page's VT52 terminal starting from the head's cursor and
    colours. Its other commands (palette `S` `X 1` `X 8` `X 12`, sounds `b n N`, pauses `t q`, cursor `k`,
    mouse zones ...) are **kept**, in order.
  - **closing**: from the first clear/reset/resolution switch after the last visible thing (drawing or text)
    to the end. Kept as it is; never shown (it wipes the screen after the pause).
- A cell that is empty (a space or 0 on background pen 0, `isBlank`) is **transparent**: the picture shows
  through it. Any other cell is drawn as the ST draws VT52 text (glyph in its pen on its paper, opaque).
  Erasing (right button, Delete, clear block) gives the picture back.
- The page's palette is the IGS palette (registers as `S`/`X` left them, ST levels: 0-7 x 34, STE 0-15 x 17),
  so the VT52 pens of the cells show in the file's colours. Of the palettes the kept commands go through after
  the drawing (a fade-out, a reveal, colour cycling), the page shows the one in which the picture shows the
  most different colours (later wins a tie): the moment the artist meant to be seen.
- The resolution is the one the head ends in: the file's `R` command, or guessed (x past 319: medium, y past
  199: high; else low). The resolution gadget re-reads the file in another resolution.

## Saving (.ig)

- Unchanged cells: the file's own bytes, exactly.
- Changed cells: head + `CR` (ends IGS's command chain) + the cells as VT52 (`ESC q ESC w`, then per run of
  non-transparent cells `ESC Y` row col, `ESC b`/`ESC c` where the pen changes, the characters) + each kept
  command as `G#<command>` + `CR` + the closing (with `G#` in front when it started inside a chain).
  Read again, this gives the same head, the same cells, the same kept commands, the same closing: the same
  page. What the ST shows at the end of the text part is what the page shows.
- Save as `.vt`: the cells only (the picture is lost: VT52 cannot draw it), as on any ST page. PNG/GIF:
  picture + cells, as shown.
- A plain ST page (no picture) saved as `.ig` is its VT52 text, which IG shows as it is.

## Not editable (said in the status line when the file opens)

- The drawing itself (lines, fills, VDI text, blits): kept byte for byte, not changeable in Charcoal.
- The kept commands (palette, sound, pauses, zones) and the closing: kept, not shown or played.
- Sound (`b`, `n`, `N`) is skipped; pauses are not waited for; loops run all their steps at once; `X 9`
  (MIDI buffer), `X 11` loading from the serial port, mouse zones and input are ignored.
- Random parameters (`r`, `R`) use a fixed seed, so a file looks the same every time it opens.

## The interpreter (igs.go, igs_vdi.go)

Ported from IcyDraw's engine (mkrueger/icy_tools, Apache-2.0/MIT: `icy_parser_core/src/igs`,
`icy_engine/src/palette_screen_buffer/igs`; its code is named after GEM VDI's routines and names no other source; line widths and ends, arcs by
the 1800-step sine table, polygon scan fill bottom to top, the fill patterns and hatches, polymarkers,
GEM text effects). Differences: the parser looks at the whole file (a command letter chained after `:` must
be followed by `>`, as IG's spec writes them, or by a space and a value, so `G#p>24,19:We all ...` prints
"We all"); CR and LF right after a chain are swallowed (the spec: a `:` at the line's end keeps CR LF from the
display); W's text runs to its `@` over line ends (IG 2.20); every group of a loop's chain gang runs; blit
results are masked to the resolution's planes; a command cut short by the file's end is closed before text
is written after it (W gets its `@`, an X 11 bitmap its zeros). Fonts: EmuTOS's 6x6, 8x8 and 8x16 (the same glyphs as IcyDraw's
Atari fonts, checked byte for byte), downloaded with the ST page's fonts.
