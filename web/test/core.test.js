// Run: node test/core.test.js
const assert = require('assert');
const C = require('../app/core.js');
const enc = s => Uint8Array.from(s, c => c.charCodeAt(0));
const row = (d, y) => Array.from({ length: d.cols }, (_, x) => String.fromCharCode(C.cellCh(d.get(x, y)))).join('').trimEnd();

// Amiga 8-bit CSI, form feed, LF-only lines, bold.
let { doc } = C.parseArt(enc('junk\x0c\x9b1;33mHi\x9b0m\nthere\x9b3;4mX'), { amiga: true });
assert.strictEqual(row(doc, 0), 'Hi');
assert.strictEqual(C.cellFg(doc.get(0, 0)), 11);
assert.strictEqual(row(doc, 1), 'thereX');
assert.ok(doc.get(5, 1) & C.ITALIC && doc.get(5, 1) & C.UNDERLINE);
// On a PC page 0x9B is a glyph and form feed too.
({ doc } = C.parseArt(enc('a\x9bb'), { amiga: false }));
assert.strictEqual(C.cellCh(doc.get(1, 0)), 0x9b);
// Positioning, CRLF, wrap at once at 80 columns.
({ doc } = C.parseArt(enc('\x1b[2J\x1b[3;5HX\r\n' + 'y'.repeat(80) + 'z'), { amiga: false }));
assert.strictEqual(C.cellCh(doc.get(4, 2)), 88);
assert.strictEqual(row(doc, 4), 'z');
// Round trip.
const d = new C.Doc(80, 3);
d.set(0, 0, C.makeCell(65, 12, 1)); d.set(79, 0, C.makeCell(66, 3, 9));
for (let x = 0; x < 80; x++) d.set(x, 1, C.makeCell(0xB0, 7, 0));
d.set(2, 2, C.makeCell(67, 7, 0, C.ITALIC));
for (const amiga of [true, false]) {
  const bytes = C.writeAnsi(d, { amiga });
  const back = C.parseArt(bytes, { amiga }).doc;
  for (let y = 0; y < 3; y++) for (let x = 0; x < 80; x++) assert.strictEqual(back.get(x, y), d.get(x, y), `${amiga} ${x},${y}`);
}
// SAUCE.
const art = C.writeAnsi(d, { amiga: false });
const s = C.makeSauce({ title: 'Test', author: 'me', cols: 132, rows: 3, font: 'Amiga Topaz 1+' }, art.length);
const file = new Uint8Array(art.length + s.length); file.set(art); file.set(s, art.length);
const r = C.readSauce(file);
assert.strictEqual(r.end, art.length);
assert.strictEqual(r.sauce.title, 'Test');
assert.strictEqual(r.sauce.font, 'Amiga Topaz 1+');
assert.strictEqual(C.parseArt(file, { amiga: true }).doc.cols, 132);
// Insert / delete lines.
({ doc } = C.parseArt(enc('a\nb\nc\x1b[2;1H\x1b[L'), { amiga: true }));
assert.deepStrictEqual([0, 1, 2, 3].map(y => row(doc, y)), ['a', '', 'b', 'c']);
({ doc } = C.parseArt(enc('a\nb\nc\x1b[1;1H\x1b[M'), { amiga: true }));
assert.deepStrictEqual([0, 1].map(y => row(doc, y)), ['b', 'c']);
// SyncTERM font switch.
assert.strictEqual(C.parseArt(enc('\x1b[0;41 D'), { amiga: true }).fontRequest, 41);
// Guess.
assert.ok(C.guessAmiga(enc('x\n\xaf\xaf\xb8\ny\nz\nw\nv\n')));
assert.ok(!C.guessAmiga(enc('\xdb\xdb\xb0\xb1\r\n\xdc\xdf\r\n')));
console.log('core: all tests pass');
