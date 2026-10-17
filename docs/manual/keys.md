[Charcoal](../../README.md) · [Manual](README.md)

# Keys

Every shortcut can be changed in `~/.config/charcoal/keys` (`action = key, key`; written with the defaults and
comments on the first start; a line it cannot read is skipped and named in the status line). Alt is the left Alt
only (the right one is AltGr). Fixed: typing, F1-F10 (the F-key set), the cursor keys, Return, Tab, Backspace,
Delete, Insert, Esc. The defaults, the PC ANSI editors' (Moebius, PabloDraw, TheDraw) where they agree:

| Keys | Action |
|---|---|
| Ctrl+O, Ctrl+Shift+O | open, open the most recent file again |
| Ctrl+S, Ctrl+Shift+S, Ctrl+I | save, save as, info (SAUCE) |
| Ctrl+T / Ctrl+N, Ctrl+W, Ctrl+Tab / Ctrl+Shift+Tab (Ctrl+PgDn / PgUp) | new page, close page, next / previous page |
| Ctrl+L, Ctrl+Q | one page / side by side / top and bottom, quit |
| Ctrl+Z, Ctrl+Y / Ctrl+Shift+Z | undo, redo |
| Ctrl+A, Ctrl+C, Ctrl+X, Ctrl+V | select all, copy, cut, paste |
| Shift+arrows / Home / End / PgUp / PgDn | select from the cursor (any tool); Esc lets go |
| Alt+H, Alt+V, Alt+R, Alt+I | mirror, upside down, turn, invert the block (nothing selected: H/V/R turn the brush character) |
| Alt+F, Ctrl+K | fill the selection with the brush (nothing selected: Fill tool), crop to the selection (Amiga, PC) |
| Ctrl+Home, Ctrl+End | top, bottom of the page |
| Alt+Up / Alt+Down, Alt+Left / Alt+Right | delete / insert a row, delete / insert a column |
| Alt+T, D, K, L, P, S, B | tools: Type, Draw, Colour, Fill, Pick, Select, Half (PC, Unicode) / Sixel (teletext, Viewdata, Minitel) |
| Alt+G, Alt+Shift+G, Ctrl+Shift+G | Logo tool (TheDraw fonts), the font list, a whole line to stamp (Draw > New / Edit a TheDraw font: no keys) |
| Ctrl+Alt+PgUp / PgDn | Unicode page: the character table's previous / next block |
| Ctrl+Up / Ctrl+Down, Ctrl+Left / Ctrl+Right | foreground -/+, background -/+ |
| Ctrl+0-7, Alt+0-7 | foreground, background colour (again: the bright one; Plus/4: the hue, its luminance kept) |
| Alt+U, Ctrl+D, Ctrl+Shift+X, Ctrl+E | take the colours under the cursor, default colours, swap, iCE on/off |
| Ctrl+F1-F10, Ctrl+Shift+Up / Down | F-key set 1-10, next / previous set |
| Ctrl+= / Ctrl++, Ctrl+-, Ctrl+9 | zoom in, out, fit the width |
| Ctrl+G, Ctrl+R, Ctrl+H | grid, reverse (PETSCII, Atari), hide the gadgets |
| Alt+N, Ctrl+Shift+D, Alt+Delete | new frame, copy the frame, delete it |
| Alt+, / Alt+. (Alt+PgUp / PgDn), Alt+Home / End | previous / next frame, first / last |
| Alt+Shift+PgUp / PgDn, Alt+[ / Alt+] | move the frame earlier / later, hold it shorter / longer |
| Ctrl+P, Alt+O, Ctrl+Shift+F | play the frames, onion skin, show / hide the frame strip |
| Alt+Shift+N, Ctrl+Shift+PgUp / PgDn, Alt+Shift+Up / Down | new layer, the layer above / below, move the layer up / down |
| Alt+Shift+M, Alt+Shift+E | merge the layer down, show / hide it |
| Alt+Shift+L, R, O; Alt+Shift+F, B | Shape tool: line, box, ellipse; filled, style (brush / lines / half blocks) |
| Alt+Shift+S, Alt+Shift+H / V, Alt+Shift+K / C | shade brush, mirror left-right / top-bottom, colour only / character only |
| Ctrl+Shift+I, Ctrl+Shift+R, Ctrl+[ / Ctrl+], Ctrl+Shift+M, Ctrl+Alt+= / - | reference: open, show / hide, fainter / stronger, move, bigger / smaller |
| Ctrl+Return | drawing session: chat (Return sends, Esc back to the page) |

Dragging inside a selection with the Select tool moves it (one undo step). Alt+9 switches a PC page to
9-pixel letter spacing (VGA's 9-dot cells; kept in the SAUCE record).

## Coming from Moebius, PabloDraw, TheDraw or ACiDDraw

Settings > **Keys like** puts CharEdit's actions on that editor's keys. It writes `preset = moebius` (or
`pablodraw`, `thedraw`, `aciddraw`) at the top of the keys file and keeps the file as it was in `keys.bak`; your
own lines in the file still win over the preset. The help (F12) lists the keys in use.

| Action | CharEdit | Moebius | PabloDraw | TheDraw | ACiDDraw |
|---|---|---|---|---|---|
| F-key set 1-10 | Ctrl+F1-F10 | Alt+F1-F10 | Alt+F1-F10 | Alt+F1-F10 | Alt+F1-F10 |
| next / previous set | Ctrl+Shift+Up / Down | Ctrl+. / Ctrl+, | same as CharEdit | same | same |
| foreground +1 / -1 | Ctrl+Down / Ctrl+Up | Ctrl+Down / Ctrl+Up | Ctrl+Down / Ctrl+Up | **Ctrl+Up / Ctrl+Down** | **Ctrl+Up / Ctrl+Down** |
| insert / delete a row | Alt+Down / Alt+Up | Alt+Down / Alt+Up | Alt+Down / Alt+Up | Alt+I / Alt+Y | Ctrl+I / Ctrl+Y |
| open, save, quit, help | Ctrl+O, Ctrl+S, Ctrl+Q, F12 | the same | the same | Alt+L, Alt+S, Alt+X, Alt+H (Ctrl ones too) | as TheDraw |
| undo | Ctrl+Z | Ctrl+Z | Ctrl+Z | Alt+R, Ctrl+Z | Alt+R, Ctrl+Z |
| select | Alt+S (tool) | Alt+B | Alt+E | Alt+B | Alt+B |
| draw, pick, fill tools | Alt+D, Alt+P, Alt+L | Alt+D, Alt+P, Alt+L | Alt+P / Alt+B, Alt+I, - | Alt+D, -, Alt+P | Alt+D, Alt+P, - |
| line, box, ellipse | Alt+Shift+L, R, O | as CharEdit | Alt+L, Alt+R, Alt+O | as CharEdit | as CharEdit |
| 9-pixel letter spacing | Alt+9 | Ctrl+F | Alt+9 | Alt+9 | Alt+9 |
| reference picture on / off | Ctrl+Shift+R | Ctrl+Tab | Ctrl+Shift+R | Ctrl+Shift+R | Ctrl+Shift+R |

The colour keys (Ctrl+0-7, Alt+0-7, Alt+U, Ctrl+D, Ctrl+Shift+X, Ctrl+E), Ctrl+arrows for the background, Alt+Left /
Right for columns, Ctrl+K crop and Ctrl+I info are the same in all of them. An action the other editor does not
have keeps CharEdit's key unless the preset gave that key to another action; then it has no key until the keys
file gives it one: Moebius: Half; PabloDraw: Turn, Invert, Onion, Fill, Half, shape style; TheDraw: Mirror, Turn,
Invert, Pick, Half; ACiDDraw: Info (File > Info stays), Mirror, Turn, Fill, Half. What cannot be matched: the plain-letter block keys of Moebius and PabloDraw (M move, C
copy, F fill, R rotate, X / Y flip, S stamp ... while a block is up) and Moebius's tool letters (K B I P), because
plain letters type in CharEdit (use the block buttons or Alt+H / V / R); Alt+F4 stays the window manager's, so
F-key set 4 is Ctrl+F4 in every preset.
