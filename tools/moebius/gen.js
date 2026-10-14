// Builds documents and writes them with Moebius's own encoders (app/libtextmode),
// then reads each file back with Moebius's own decoder: that page is the reference.
// Usage: node gen.js <moebius app dir> <out dir>
"use strict";
const fs = require("fs");
const path = require("path");
const app = path.resolve(process.argv[2]);
const out = path.resolve(process.argv[3]);
const lib = (m) => require(path.join(app, "libtextmode", m));
const {encode_as_ansi, Ansi} = lib("ansi.js");
const {encode_as_bin, BinaryText} = lib("binary_text.js");
const {encode_as_xbin, XBin} = lib("xbin.js");
const {ega} = lib("palette.js");
const {unicode_to_cp437} = lib("encodings.js");

// A fixed date: Moebius writes today's date, which would change the files every run.
const RealDate = Date;
global.Date = class extends RealDate { constructor(...a) { if (a.length) super(...a); else super(2026, 9, 14); } };

let seed = 12345;
function rnd(n) { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return (seed >>> 12) % n; } // high bits: the low ones repeat soon

function fontFile(name) {
    const map = {"IBM VGA": "ibm/CP437.F16", "IBM VGA50": "ibm/CP437.F08", "IBM EGA": "ibm/CP437.F14", "IBM EGA43": "ibm/CP437.F08",
        "IBM VGA25G": "ibm/CP437.F19", "IBM VGA 850": "ibm/CP850.F16", "IBM VGA50 866": "ibm/CP866.F08",
        "Amiga Topaz 1+": "amiga/TopazPlus_a500.F16", "Amiga Topaz 2": "amiga/Topaz_a1200.F16", "Amiga MicroKnight": "amiga/MicroKnight.F16",
        "Amiga P0T-NOoDLE": "amiga/P0T-NOoDLE.F16", "Amiga mOsOul": "amiga/mO'sOul.F16"};
    return new Uint8Array(fs.readFileSync(path.join(app, "fonts", map[name] || "ibm/CP437.F16")));
}

function doc(o) {
    const d = Object.assign({columns: 80, rows: 25, title: "", author: "", group: "", date: "20261014", palette: ega,
        font_name: "IBM VGA", ice_colors: false, use_9px_font: false, comments: "", c64_background: undefined}, o);
    if (!d.data) {
        d.data = [];
        for (let i = 0; i < d.columns * d.rows; i++) d.data.push({code: 32, fg: 7, bg: 0});
    }
    if (!d.font_bytes) d.font_bytes = fontFile(d.font_name);
    d.font_height = d.font_bytes.length / 256;
    return d;
}

function randomCells(d, {codes, maxBg = 16, blanks = 0.3}) {
    for (let i = 0; i < d.data.length; i++) {
        if (rnd(100) < blanks * 100) {
            d.data[i] = {code: 32, fg: rnd(16), bg: rnd(4) == 0 ? rnd(maxBg) : 0};
        } else {
            d.data[i] = {code: codes[rnd(codes.length)], fg: rnd(16), bg: rnd(maxBg)};
        }
    }
    return d;
}

const range = (a, b) => Array.from({length: b - a + 1}, (_, i) => a + i);
// 0x9B left out of random text: before digits and a letter it is the Amiga CSI, which makes a file
// without SAUCE read as Amiga (the "controls" pages have it).
const printable = range(33, 126).concat(range(128, 254)).filter(c => c != 0x9B);
const blocks = [176, 177, 178, 219, 220, 221, 222, 223, 254, 196, 179, 218, 191, 192, 217, 205, 186];
// What PC art is drawn with: ASCII and the block and line characters (without a record
// CharEdit tells PC from Amiga by such bytes; evenly random high bytes look like neither).
const pcArt = range(33, 126).concat(blocks, blocks, blocks);
const latin1 = range(33, 126).concat(range(0xA0, 0xFF));

const docs = {};
// Every colour pair, iCE on and off.
for (const ice of [true, false]) {
    const d = doc({rows: 4, ice_colors: ice, title: ice ? "iCE colours" : "Blink"});
    for (let i = 0; i < d.data.length; i++) d.data[i] = {code: blocks[i % blocks.length], fg: i % 16, bg: (i >> 4) % 16};
    docs[ice ? "colours-ice" : "colours-blink"] = d;
}
// 9-pixel letter spacing: the line characters 0xC0-0xDF.
docs["ninepx"] = randomCells(doc({use_9px_font: true, ice_colors: true, rows: 10}), {codes: range(0xC0, 0xDF)});
// Odd and wide widths.
for (const w of [1, 2, 3, 7, 8, 9, 40, 79, 80, 81, 132, 160, 199, 200]) {
    docs["width-" + w] = randomCells(doc({columns: w, rows: 6 + rnd(6), ice_colors: true}), {codes: pcArt});
}
// A tall page, mostly empty, with empty rows at the end.
{
    const d = doc({rows: 150, ice_colors: true});
    for (let k = 0; k < 200; k++) d.data[rnd(80 * 140)] = {code: blocks[rnd(blocks.length)], fg: rnd(16), bg: rnd(16)};
    docs["tall"] = d;
}
// Fonts named in SAUCE.
for (const f of ["IBM VGA50", "IBM EGA", "IBM EGA43", "IBM VGA25G", "IBM VGA 850", "IBM VGA50 866", "Amiga Topaz 1+", "Amiga Topaz 2",
    "Amiga MicroKnight", "Amiga P0T-NOoDLE", "Amiga mOsOul"]) {
    const amiga = f.startsWith("Amiga");
    const d = randomCells(doc({font_name: f, rows: 8, ice_colors: !amiga}), {codes: amiga ? latin1 : printable, maxBg: amiga ? 8 : 16});
    docs["font-" + f.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/-$/, "")] = d;
}
// Every byte 0-255 as a character, on colours.
{
    const d = doc({rows: 8, ice_colors: true});
    for (let c = 0; c < 256; c++) d.data[Math.floor(c / 64) * 80 * 2 + (c % 64)] = {code: c, fg: (c % 15) + 1, bg: c % 3};
    docs["controls"] = d;
}
// The same bytes on a plain page (no colours but the default).
{
    const d = doc({rows: 4});
    for (let c = 0; c < 256; c++) d.data[Math.floor(c / 64) * 80 + (c % 64)] = {code: c, fg: 7, bg: 0};
    docs["controls-plain"] = d;
}
// Blank lines, trailing blanks with and without a background, spaces on bright backgrounds.
{
    const d = doc({rows: 20, ice_colors: true});
    const put = (x, y, c) => { d.data[y * 80 + x] = c; };
    for (let x = 0; x < 10; x++) put(x, 1, {code: 65 + x, fg: 14, bg: 1});
    for (let x = 10; x < 80; x++) put(x, 2, {code: 32, fg: 7, bg: 4}); // trailing blanks with a background
    for (let x = 0; x < 80; x++) put(x, 3, {code: 219, fg: x % 16, bg: 0}); // a full row
    put(0, 5, {code: 32, fg: 7, bg: 8}); // a space on a bright background, then blanks to the end
    put(5, 6, {code: 32, fg: 7, bg: 9}); put(6, 6, {code: 32, fg: 7, bg: 0}); put(7, 6, {code: 32, fg: 7, bg: 0}); put(8, 6, {code: 88, fg: 7, bg: 0});
    put(79, 8, {code: 88, fg: 12, bg: 0}); // the last column only
    put(40, 10, {code: 32, fg: 3, bg: 0}); // a blank with another foreground
    put(0, 12, {code: 0, fg: 7, bg: 0}); put(1, 12, {code: 255, fg: 7, bg: 0});
    docs["blanks"] = d;
}
// SAUCE text: title, author, group, comments (a long line, an empty line), non-ASCII.
docs["sauce-text"] = randomCells(doc({rows: 5, title: "Moebius title", author: "An Author", group: "A Group",
    comments: "First comment line\nA second line that is longer than sixty-four characters, so Moebius splits it\n\nLast line"}), {codes: printable});
docs["sauce-utf8"] = randomCells(doc({rows: 3, title: "Smörgåsbord", author: "Åsa", group: "Grüne", comments: "Kommentar på svenska"}), {codes: printable});
// An empty page.
docs["empty"] = doc({});
// XBin extras: a page's own palette and font.
{
    const pal = ega.map((c, i) => ({r: (c.r + i * 3) % 64, g: (c.g + i * 5) % 64, b: (c.b + i * 7) % 64}));
    const font = new Uint8Array(256 * 16);
    for (let i = 0; i < font.length; i++) font[i] = rnd(256);
    docs["own-palette-font"] = randomCells(doc({rows: 6, palette: pal, font_bytes: font, ice_colors: true}), {codes: printable});
    const font8 = new Uint8Array(256 * 8);
    for (let i = 0; i < font8.length; i++) font8[i] = rnd(256);
    docs["own-font-8"] = randomCells(doc({rows: 6, font_bytes: font8, ice_colors: false}), {codes: printable});
}

function cellsHex(data) {
    return Buffer.from(data.flatMap(c => [c.code, (c.bg << 4) | c.fg])).toString("base64");
}

// The comment lines as Moebius writes them (add_comments_bytes): each line cut into
// 64-character pieces, trimmed, empty lines left out. Its decoder reads them back
// wrong when a line holds letters outside ASCII (it counts 64 bytes, not characters).
function commentLines(raw) {
    const out = [];
    for (const l of (raw || "").split("\n")) {
        for (let s = 0; l.length > 0; s++) {
            const piece = l.substr(s * 64, 64).trim();
            if (piece.length == 0) break;
            out.push(piece);
        }
    }
    return out.join("\n");
}

function reference(r, extra) {
    return Object.assign({columns: r.columns, rows: r.rows, cells: cellsHex(r.data.slice(0, r.columns * r.rows)), ice: !!r.ice_colors,
        ninepx: !!r.use_9px_font, font: r.font_name || "", title: (r.title || "").trimEnd(), author: (r.author || "").trimEnd(),
        group: (r.group || "").trimEnd(), comments: r.comments || ""}, extra);
}

function refWithComments(r, d, extra) {
    return Object.assign(reference(r, extra), {comments: commentLines(d.comments)});
}

fs.mkdirSync(out, {recursive: true});
const index = [];
function write(name, bytes, ref) {
    fs.writeFileSync(path.join(out, name), Buffer.from(bytes));
    fs.writeFileSync(path.join(out, name + ".json"), JSON.stringify(ref) + "\n");
    index.push(name);
}

for (const [base, d] of Object.entries(docs)) {
    const isXbinDoc = base.startsWith("own-");
    if (!isXbinDoc) {
        // .ans with SAUCE, read back by Moebius's decoder.
        let b = encode_as_ansi(d, false);
        write(base + ".ans", b, refWithComments(new Ansi(Buffer.from(b)), d));
    }
    if (["colours-ice", "width-132", "blanks", "controls"].includes(base)) {
        let b = encode_as_ansi(d, true);
        write(base + "-nosauce.ans", b, reference(new Ansi(Buffer.from(b)), {nosauce: true}));
    }
    if (["width-40", "sauce-text", "font-amiga-topaz-1"].includes(base)) {
        for (const ext of [".asc", ".txt", ".nfo", ".diz"]) {
            if (base != "width-40" && ext != ".asc") continue;
            let b = encode_as_ansi(d, false);
            write(base + ext, b, refWithComments(new Ansi(Buffer.from(b)), d));
        }
    }
    if (d.columns % 2 == 0 && ["colours-ice", "colours-blink", "width-2", "width-160", "width-200", "controls", "tall", "ninepx", "font-ibm-vga50"].includes(base)) {
        let b = encode_as_bin(d, false);
        write(base + ".bin", b, refWithComments(new BinaryText(Buffer.from(b)), d));
    }
    if (isXbinDoc || ["colours-ice", "colours-blink", "width-2", "width-200", "controls", "font-ibm-vga50", "font-ibm-ega", "font-amiga-topaz-1"].includes(base)) {
        let b = encode_as_xbin(d, false);
        const r = new XBin(Buffer.from(b));
        write(base + ".xb", b, refWithComments(r, d, {palette: r.palette.flatMap(c => [c.r, c.g, c.b]), fontHeight: r.font_height,
            fontBytes: r.font_bytes ? Buffer.from(r.font_bytes).toString("base64") : ""}));
    }
    if (["colours-ice", "colours-blink", "width-132", "controls", "blanks"].includes(base)) {
        // Export as UTF-8: Moebius has no reader for it, so the reference is the page as written
        // (its stand-ins for control characters, a blank's foreground not kept, U+00A0 read as 255).
        const b = encode_as_ansi(d, false, {utf8: true});
        const sub = {10: 9, 13: 14, 26: 16, 27: 17, 0: 255};
        const data = d.data.map(c => {
            const code = sub[c.code] !== undefined ? sub[c.code] : c.code;
            return {code, fg: c.fg, bg: c.bg};
        });
        // It starts every row with a line end, the first too: the file shows an empty line on top.
        const top = Array.from({length: d.columns}, () => ({code: 32, fg: 7, bg: 0}));
        write(base + ".utf8ans", b, reference({columns: d.columns, rows: d.rows + 1, data: top.concat(data), ice_colors: true}, {utf8: true, nosauce: true}));
    }
}
fs.writeFileSync(path.join(out, "INDEX"), index.join("\n") + "\n");
console.log(index.length + " files");
void unicode_to_cp437;
