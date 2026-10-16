// A Moebius client reduced to its protocol code, for CharEdit's tests: it joins a
// session, reads the page with Moebius's own libtextmode.uncompress, handles DRAW
// and SET_CANVAS_SIZE as app/document/doc.js's Connection does, draws one cell,
// waits for the host's cell at 0,0 and writes the page as JSON.
//   node moebius-client.js <moebius app folder> <ws url> <password> <out.json>
const path = require("path");
const fs = require("fs");
const WebSocket = require("ws");
const [app, url, pass, out] = process.argv.slice(2);
const libtextmode = require(path.resolve(app, "libtextmode/libtextmode"));
const actions = {CONNECTED: 0, REFUSED: 1, DRAW: 9, CHAT: 10, SET_CANVAS_SIZE: 16};
let doc, id;
const chat = [];
const ws = new WebSocket(url);
const send = (type, data = {}) => { data.id = id; ws.send(JSON.stringify({type, data})); };
const done = (why) => {
    if (doc) fs.writeFileSync(out, JSON.stringify({columns: doc.columns, rows: doc.rows, data: doc.data, title: doc.title,
        comments: doc.comments, ice_colors: doc.ice_colors, chat}));
    console.log(why);
    ws.close();
    process.exit(0);
};
setTimeout(() => done("timed out"), 15000);
ws.on("open", () => ws.send(JSON.stringify({type: actions.CONNECTED, data: {nick: "moebius", group: "test", pass}})));
ws.on("message", (m) => {
    const {type, data} = JSON.parse(m);
    switch (type) {
    case actions.CONNECTED:
        id = data.id;
        doc = libtextmode.uncompress(data.doc);
        const block = {code: 219, fg: 4, bg: 1};
        const x = doc.columns - 1, y = doc.rows - 1;
        doc.data[y * doc.columns + x] = block;
        send(actions.DRAW, {x, y, block});
        send(actions.CHAT, {nick: "moebius", group: "test", text: "hello from Moebius"});
        break;
    case actions.REFUSED:
        done("refused");
        break;
    case actions.DRAW:
        doc.data[data.y * doc.columns + data.x] = Object.assign(data.block);
        if (data.x == 0 && data.y == 0 && data.block.code == 77) done("got the host's cell");
        break;
    case actions.CHAT:
        chat.push(data.text);
        break;
    case actions.SET_CANVAS_SIZE:
        libtextmode.resize_canvas(doc, data.columns, data.rows);
        break;
    }
});
ws.on("error", (e) => done("error " + e.message));
