// Reads CharEdit's saves of the Moebius corpus with Moebius's own decoder and
// compares each page with the reference (testdata/moebius/<file>.json).
// Usage: node check.js <moebius app dir> <testdata/moebius> <dir of CharEdit's saves>
"use strict";
const fs = require("fs");
const path = require("path");
const [app, refs, saves] = process.argv.slice(2).map(p => path.resolve(p));
const lib = (m) => require(path.join(app, "libtextmode", m));
const {Ansi} = lib("ansi.js");
const {BinaryText} = lib("binary_text.js");
const {XBin} = lib("xbin.js");

// Characters CharEdit writes as spaces in ANSI (a BBS terminal would take them as controls);
// 0, 7, 8 and 9 too unless the file it read had them as characters (with SAUCE).
const asSpace = new Set([10, 13, 27]);
const asSpaceNoSauce = new Set([0, 7, 8, 9, 10, 13, 27]);
let bad = 0;
for (const name of fs.readFileSync(path.join(refs, "INDEX"), "utf8").split("\n").filter(Boolean)) {
    const ref = JSON.parse(fs.readFileSync(path.join(refs, name + ".json"), "utf8"));
    const bytes = fs.readFileSync(path.join(saves, name));
    const ext = path.extname(name).toLowerCase();
    let doc;
    try {
        doc = ext == ".bin" ? new BinaryText(bytes) : ext == ".xb" ? new XBin(bytes) : new Ansi(bytes);
    } catch (e) {
        console.log(`${name}: Moebius cannot read it: ${e}`);
        bad++;
        continue;
    }
    const cells = Buffer.from(ref.cells, "base64");
    const problems = [];
    if (doc.columns != ref.columns) problems.push(`columns ${doc.columns}, page ${ref.columns}`);
    if (doc.rows != ref.rows && !(ref.nosauce && doc.rows >= ref.rows)) problems.push(`rows ${doc.rows}, page ${ref.rows}`);
    const ansi = ext != ".bin" && ext != ".xb";
    // Without a record CharEdit reads 0x00, 0x07, 0x08, 0x09 as a terminal does
    // (moebius_test.go): from the first row holding one, the rows are not compared.
    let stop = cells.length / 2;
    if (ref.nosauce && !ref.utf8) {
        for (let i = 0; i < cells.length / 2; i++) {
            if ([0, 7, 8, 9].includes(cells[2 * i])) { stop = i - i % ref.columns; break; }
        }
    }
    let diffs = 0, first = "";
    for (let i = 0; i < stop && i < doc.data.length; i++) {
        let code = cells[2 * i], attr = cells[2 * i + 1];
        if (ansi && (ref.nosauce ? asSpaceNoSauce : asSpace).has(code)) code = 32;
        const b = doc.data[i];
        let fg = attr & 15, bg = attr >> 4;
        if (ansi && code == 32 && bg == 0) fg = b.fg; // a blank's foreground is not written
        if (b.code != code || b.fg != fg || b.bg != bg) {
            if (!diffs) first = `cell ${i % ref.columns},${Math.floor(i / ref.columns)}: ${b.code}/${b.fg}/${b.bg}, page ${code}/${fg}/${bg}`;
            diffs++;
        }
    }
    if (diffs) problems.push(`${diffs} cells differ (${first})`);
    if (!ref.nosauce && ext != ".xb") {
        if (!!doc.ice_colors != ref.ice) problems.push(`iCE ${doc.ice_colors}`);
        if (!!doc.use_9px_font != ref.ninepx) problems.push(`9px ${doc.use_9px_font}`);
        if (doc.font_name != ref.font && !(ref.font == "IBM EGA43" && doc.font_name == "IBM VGA50")) problems.push(`font ${doc.font_name}, page ${ref.font}`);
    }
    if (!ref.nosauce && (doc.title || "").trimEnd() != ref.title) problems.push(`title "${doc.title}"`);
    if (problems.length) {
        bad++;
        console.log(`${name}: ${problems.join("; ")}`);
    }
}
console.log(bad ? `${bad} files differ` : "every save reads back in Moebius as the page");
process.exit(bad ? 1 : 0);
