[Charcoal](../../README.md) · [Manual](README.md)

# Drawing and editing

## The window

A native program (Go + SDL2, one binary; needs only the SDL2 library that every Linux desktop has). Every
command is in the menu bar (File, Edit, View, Page, Draw, Animate, Help) with its keys beside it; one toolbar
row holds the everyday ones. Two layouts (View menu or Settings): **Studio**, the tools in a rail on the left and
the page's settings, colours, characters and the tool's options in a panel on the right, and **Focus**, the page
as wide as the window with the gadgets under it. Three themes: **Dark**, **Light** and **Workbench**. The
interface text is the system's font by default, or the Go font, Topaz, or any .ttf / .otf, in sizes 11 to 20
(Settings); the art always keeps its own fonts. **Ctrl+H** hides every gadget. **Help > Keys and mouse** (F12)
lists every key.

## Tools and the bottom strip

- Bottom strip: tools, all 256 characters (click = brush), 5 F-key sets (F1–F10), 16 colours (left fg, right bg,
  Shift+click edits the colour).
- Tools: Type, Draw (Shift+click = line), Colour, Fill, Pick (or middle click), Select (copy/cut/paste/stamp), Shape, Half.
- Undo/redo, play at modem speed; dropping a file on the window opens it.

## Pages, tabs and the grid

- A page has a real height (H): a file opens as tall as it is, its empty lines at the end included; the page
  ends where the dark area starts; every row is saved. Enter or Down past the last row adds a row.
- Pages for a machine's screen keep its width: PETSCII the machine's, Atari 38 or 40; only Amiga, PC and Unicode
  pages can be any width (W).

- **Tabs and two pages at once**: every file opens in a tab; show one page, two side by side or one over the other
  (Ctrl+L), optionally scrolling together.
- **Grid** (Ctrl+G, brighter every 10 columns and every 25 rows), **zoom** 1x-8x, fit to width (Ctrl+9),
  Ctrl+wheel zooms around the mouse.
- **Flip** a block left-right (Alt+H) or upside down (Alt+V): characters become their mirror images (known
  pairs first, then by comparing the glyphs' pixels, so it works in every font).

## Safety

- **Safety**: pages with changes are kept aside every two minutes and offered back after a crash; saving over a
  file keeps the old one as `name.bak` (Settings); the Open requester lists the last 10 files.

## Reference picture, shapes and brushes

![A PC page traced over a reference picture: the sun and tree drawn in half blocks, the hill shaded](../screenshots/reference.png)

- **Reference picture** to trace over (the **Ref** gadget, Ctrl+Shift+I, or drop a PNG, JPEG, GIF, BMP or WebP on
  the window): it shows in the empty cells, or faintly over everything; fainter / stronger (Ctrl+[ / Ctrl+]),
  hide (Ctrl+Shift+R), greys, move (Ctrl+Shift+M then drag or arrows, or Ctrl+drag), size (fit width, fit page,
  1:1, bigger, smaller). Only shown: never turned into characters, never saved or exported. It is remembered with
  the art file (`prefs.json` `references`) and in the autosave.
- **Shape** tool: line, box or ellipse, dragged, outlined or filled (Shift at the press, or Filled); in the brush,
  in box-drawing lines (PC single or double, PETSCII and Atari lines) or in half blocks (PC; sixels on teletext).
  Right button erases. One undo step each.
- **Shade** brush (Draw): left steps a cell up the shades, right down (PC: empty, the three shade blocks, the full
  block; PETSCII: the checkerboard, then reversed; other pages a short ASCII ramp). Each page type's own sequence
  can go in `prefs.json`, e.g. `"shades": {"amiga": [32, 46, 58, 43, 35]}`.
- **Mirror**: what Draw, Colour, Half and Shape put down is mirrored left-right and/or top-bottom around the page's
  centre, characters as their mirror twins.
- **Colour only / Char only**: Draw and Shape paint just the colours or just the characters.
- These options sit under the brush while a drawing tool is in use.
