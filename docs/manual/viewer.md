[Charcoal](../../README.md) · [Manual](README.md)

# CharView, the viewer

    charview FILE|FOLDER...   # just looks: n / Right / Space next, p / Left previous, q / Esc quits

**CharView** is the same program started as `charview` (a link to `charedit`, or `charedit --view`): the art
fills the window, fitted to its width, with one line at the bottom (i hides it). Up/Down/PgUp/PgDn/Home/End
and the wheel scroll (Shift+Left/Right sideways), +/- zoom (0 fits again), g grid, m plays the file at modem
speed, e opens it in CharEdit. **a** scrolls by itself (tall art down, wide art sideways; at the end it waits two
seconds and goes to the next file), **s** switches smooth / a text row at a time, **[ ]** set the speed (0.5-48
rows a second); these are remembered. Art wider than the window is shown smaller (1/2, 1/3 ...; - and + step it), drawn once so scrolling stays
smooth. A folder gives every art file in it (.ans .asc .nfo .diz .txt .bin .seq
.pet .ice ...). It never changes the editor's other settings.
