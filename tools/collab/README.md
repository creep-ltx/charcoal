# Checking drawing sessions against the real servers

`collab_test.go` runs CharEdit's drawing sessions (collab.go) against a server
built from Moebius's source in the test itself, and, when these are set, against
the real ones:

- `CHARCOAL_COLLAB_MOEBIUS=<dir>`: Moebius's own server and client code.
  `<dir>/moebius-app` is the `app/` folder of github.com/blocktronics/moebius
  (checked with commit 1f624d6), `<dir>/node_modules` holds `ws` (Moebius uses
  8.15: `npm pack ws@8.15.1`, unpack into `node_modules/ws`; no install scripts
  run) and two stubs the server only names: `discord.js/index.js` =
  `module.exports = {WebhookClient: class {}};` and `upng-js/index.js` =
  `module.exports = {};`. Never `npm install` Moebius itself (Electron).
  - `TestCollabMoebiusServer`: `moebius-server.js` serves each file of
    `testdata/moebius` (Moebius's reader opens it); CharEdit joins, the page must be
    the one Moebius's reader made (the corpus' `.json`), then a raw client and
    CharEdit draw, resize and chat through the server.
  - `TestCollabMoebiusHost`: CharEdit hosts; `moebius-client.js` (Moebius's
    `libtextmode.uncompress` and its DRAW / SET_CANVAS_SIZE handling) joins, draws,
    chats and writes the page it ends with.
- `CHARCOAL_COLLAB_ICY=<program>`: IcyDraw's server (icy_engine_edit's
  `run_server`). `icydraw-collabsrv.rs` is an example program for it: copy it to
  `crates/icy_engine_edit/examples/collabsrv.rs` in an icy_tools checkout, then
  `cargo fetch` and `cargo build --offline --release -p icy_engine_edit --example
  collabsrv` (in a sandbox: ~/Projects/Knowledge/amiga/text-art-editor-formats.md).
  `TestCollabIcyDrawServer` joins it.

Everything listens on 127.0.0.1 only. Run under a memory cap:

    systemd-run --user --scope -q -p MemoryMax=4G env CHARCOAL_COLLAB_MOEBIUS=~/.cache/charcoal-collab \
      CHARCOAL_COLLAB_ICY=<icy_tools>/target/release/examples/collabsrv go test -run TestCollab -v .
