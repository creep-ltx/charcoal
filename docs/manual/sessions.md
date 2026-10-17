[Charcoal](../../README.md) · [Manual](README.md)

# Drawing together (Moebius's sessions)

CharEdit speaks **Moebius's collaboration protocol**, which IcyDraw speaks too, so CharEdit users, Moebius users
and IcyDraw users can draw on one PC page at the same time.

- **File > Join a drawing session...**: the server as Moebius writes it (`host:port/path`, port 8000 when none;
  or a `ws://` / `wss://` address), your nick and group, the password. The page comes in a new tab; what you draw
  goes to the others as you draw it (every tool, paste, undo, Animate's frames: anything that changes the page),
  and theirs comes to you. The page's size, iCE colours, 9-pixel cells, font and SAUCE text travel too.
  Undo takes back only your own steps (the others' cells stay). Leaving keeps the page as a page of its own
  (not saved).
- **File > Host a drawing session...** shares the page being worked on (a PC page of one picture): pick the port
  (8000 as Moebius), who may join (**this computer only**, the default: 127.0.0.1; or **others on the network**,
  which needs a password) and a password. Moebius (Network > Connect to server) and IcyDraw join it. Nothing
  listens before you press Host, and while it does the line above the status line says **HOSTING** in orange.
- The **session line** above the status line says where you draw and who else is there, shows the last chat
  line, and holds the chat field: **Ctrl+Return** (or a click) types into it, Return sends, Esc goes back to the
  page; while it has the keys the last lines show above it. The others' cursors show on the page as small frames
  with their nicks.
- **File > End the drawing session** leaves, or stops hosting. Closing the page ends it too.
- What Moebius cannot have: Moebius sends no fonts or palettes after joining (the page's own font shows as IBM VGA
  to the others), and pages with frames, Amiga pages and pages of other machines cannot be shared. Moebius's own
  client swaps the author and group of a SAUCE change it receives (its bug, not CharEdit's). Checked against
  Moebius's own server and client code and IcyDraw's (`tools/collab/README.md`).
