[Charcoal](../../README.md) · [Manual](README.md)

# Frames and layers

## Frames and animation

![Frames of a PETSCII page, the frame before shown faint (onion skin)](../screenshots/frames.png)

A page can hold several frames. Alt+N adds an empty one, Ctrl+Shift+D copies the frame, Alt+, and Alt+. (or
Alt+PgUp / PgDn) go through them, Alt+Delete deletes one. The **strip** under the page numbers them (click to go,
drag to move) with each frame's **hold** time, and New, Copy, Delete, **Onion** (the frame before, faint, under the
empty cells; Alt+O), **Play** (Ctrl+P, at 1/2x to 4x, looping or once). Undo covers all of it.

- A `.c` or `.petmate` with several screens opens as frames and saves them all (Petmate's own fields, fonts and
  screen names kept); a teletext `.tti` carousel opens a frame per subpage and saves every line but the changed
  rows, its CT line as the hold.
- An ANSI, PETSCII, ATASCII or VT52 file that clears the screen between pictures (an ANSImation) opens as its last
  picture and says how many it shows: Ctrl+Shift+F shows the strip with **Split** (at each clear, or at each cursor
  home). Saved, the frames are one stream with a clear between them; their hold times come from the modem speed.
- Save as **Animation (.gif)** or **Animation, APNG (.png)** writes every frame for its hold time. A format that
  holds one picture (`.scr`, `.png`, `.xb` ...) writes the frame on show and says the others are only in CharEdit.

## Layers

![A page of three layers: the picture, letters over it, the Layers panel](../screenshots/layers.png)

PC, Amiga, Unicode, PETSCII, Atari, Atari ST, Spectrum, CPC, MSX and MZ-700 pages can have **layers**: pictures on
top of one another. Every tool draws on the current layer; its empty cells (a space on black, as when nothing was
drawn; on the CPC a space on paper 0) show the layers below, so erasing on a layer shows what is under it. A black
full block covers; on the Spectrum and the MZ-700 the machine's own cleared cell (white paper, white on blue) covers
too, as it does on the screen. A layer can let **black show
below**: its letters keep their colour and the background below shows under them (letters over a picture).

- Alt+Shift+N adds a layer above the current one, Ctrl+Shift+PgUp / PgDn go up and down, Alt+Shift+Up / Down move
  the layer, Alt+Shift+M merges it into the one below, Alt+Shift+E shows or hides it. **Page > Layers** has them
  all (copy, delete, merge all, black shows below, the other layers dim or hidden while you work).
- The **Layers** panel (Studio layout) lists them, the top one first: the box shows or hides, a click works on
  one (on the current one: rename it), buttons for New, Copy, Up, Down, Merge, Delete, Black, and the other
  layers shown, dim or hidden. The status line says which layer you are on.
- **Lock** a layer (the panel's Lock button, Page > Lock the layer) and no tool changes it: drawing, typing, fills,
  pasting and Delete say it is locked. A hidden layer takes no drawing either (as in IcyDraw); Select and Pick
  still work. Width, height and rows still change every layer. IcyDraw's lock is read
  and written.
- Pick takes the colours you see; Copy copies the current layer, **Edit > Copy what is seen** the picture.
- Width, height, rows and columns change every layer; undo covers all of it (undoing an edit of another layer
  goes to that layer).
- Saved as IcyDraw `.icy`, REXPaint `.xp` or Playscii `.psci` the layers are in the file. Any other format gets
  the picture as it is seen (hidden layers left out) and CharEdit keeps the layers beside the file: opening the
  file again brings them back, as long as nothing else changed it. Autosave keeps them too.
- **Layers and frames together**, as in Playscii: every frame has every layer (a new layer goes into every frame,
  each frame draws its own on it; a new frame gets every layer). The strip, the onion skin, playing, GIF/APNG and
  every file of frames show each frame as it is seen. Playscii `.psci` keeps both; any other file of frames
  (.ans, .seq ...) gets the frames as seen and the page stays unsaved (only the autosave keeps the layers).
  Split and Animate's frames want a page of one layer.
- Teletext, Viewdata and Minitel pages have one layer (a colour code acts on the rest of its row). A drawing
  session shares pages of one layer.
