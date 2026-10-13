'use strict';
// amiansi core: the document, SAUCE, the ANSI parser and the ANSI writer.
// No DOM here, so it runs under Node for the tests as well.
//
// A cell is one 32-bit number:
//   bits 0-7   the byte (Latin-1 on an Amiga page, CP437 on a PC page)
//   bits 8-11  foreground 0-15 (8-15 = bold / bright)
//   bits 12-15 background 0-15 (8-15 = blink, shown as bright with iCE colours)
//   bit 16     italic (Amiga), bit 17 underline (Amiga)

const ITALIC = 1 << 16, UNDERLINE = 1 << 17;
const cellCh = v => v & 255;
const cellFg = v => (v >>> 8) & 15;
const cellBg = v => (v >>> 12) & 15;
const cellFlags = v => v & (ITALIC | UNDERLINE);
const makeCell = (ch, fg, bg, flags = 0) => (ch & 255) | ((fg & 15) << 8) | ((bg & 15) << 12) | flags;
const BLANK = makeCell(32, 7, 0);

class Doc {
  constructor(cols = 80, rows = 25) {
    this.cols = cols;
    this.rows = rows;
    this.cells = new Uint32Array(cols * Math.max(rows, 1)).fill(BLANK);
  }
  get(x, y) {
    if (x < 0 || y < 0 || x >= this.cols || y >= this.rows) return BLANK;
    return this.cells[y * this.cols + x];
  }
  set(x, y, v) {
    if (x < 0 || y < 0 || x >= this.cols) return;
    if (y >= this.rows) this.ensureRows(y + 1);
    this.cells[y * this.cols + x] = v;
  }
  ensureRows(n) {
    if (n <= this.rows) return;
    const need = n * this.cols;
    if (need > this.cells.length) {
      const grown = new Uint32Array(Math.max(need, this.cells.length * 2)).fill(BLANK);
      grown.set(this.cells.subarray(0, this.rows * this.cols));
      this.cells = grown;
    }
    this.rows = n;
  }
  clone() {
    const d = new Doc(this.cols, 1);
    d.rows = this.rows;
    d.cells = this.cells.slice(0, this.rows * this.cols);
    return d;
  }
  isBlank(v) {
    const ch = v & 255;
    return (ch === 32 || ch === 0) && cellBg(v) === 0 && !(v & UNDERLINE);
  }
  lastCol(y) { // last non-blank column of a row, -1 if empty
    for (let x = this.cols - 1; x >= 0; x--) if (!this.isBlank(this.get(x, y))) return x;
    return -1;
  }
  usedRows() { // rows up to the last non-blank one
    for (let y = this.rows - 1; y >= 0; y--) if (this.lastCol(y) >= 0) return y + 1;
    return 0;
  }
  trim(minRows = 25) { // drop blank rows at the bottom
    this.rows = Math.max(this.usedRows(), minRows);
    this.ensureRows(this.rows);
  }
  withCols(cols) {
    const d = new Doc(cols, this.rows);
    const w = Math.min(cols, this.cols);
    for (let y = 0; y < this.rows; y++)
      for (let x = 0; x < w; x++) d.cells[y * cols + x] = this.cells[y * this.cols + x];
    return d;
  }
  copyRect(x0, y0, w, h) {
    const out = new Uint32Array(w * h);
    for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) out[y * w + x] = this.get(x0 + x, y0 + y);
    return { w, h, cells: out };
  }
}

// ---------------------------------------------------------------- SAUCE

const SAUCE_FONTS_PC = ['IBM VGA', 'IBM VGA50', 'IBM EGA', 'IBM EGA43', 'IBM VGA25G'];

function latin1(bytes, a, b) {
  let s = '';
  for (let i = a; i < b; i++) s += String.fromCharCode(bytes[i]);
  return s.replace(/[\0 ]+$/, '');
}

// Returns {sauce, end}: the SAUCE record (or null) and where the art ends.
function readSauce(bytes) {
  let end = bytes.length, sauce = null;
  const n = bytes.length;
  if (n >= 128 && latin1(bytes, n - 128, n - 121) === 'SAUCE00') {
    const o = n - 128;
    const u16 = i => bytes[o + i] | (bytes[o + i + 1] << 8);
    const u32 = i => (u16(i) | (u16(i + 2) << 16)) >>> 0;
    sauce = {
      title: latin1(bytes, o + 7, o + 42), author: latin1(bytes, o + 42, o + 62),
      group: latin1(bytes, o + 62, o + 82), date: latin1(bytes, o + 82, o + 90),
      fileSize: u32(90), dataType: bytes[o + 94], fileType: bytes[o + 95],
      tinfo1: u16(96), tinfo2: u16(98), comments: bytes[o + 104], flags: bytes[o + 105],
      font: latin1(bytes, o + 106, o + 128),
    };
    end = o;
    if (sauce.comments) {
      const c = o - 5 - sauce.comments * 64;
      if (c >= 0 && latin1(bytes, c, c + 5) === 'COMNT') {
        sauce.notes = [];
        for (let i = 0; i < sauce.comments; i++) sauce.notes.push(latin1(bytes, c + 5 + i * 64, c + 69 + i * 64));
        end = c;
      }
    }
  }
  // The art ends at the first EOF (Ctrl+Z) byte.
  const eof = bytes.subarray(0, end).indexOf(0x1A);
  if (eof >= 0) end = eof;
  return { sauce, end };
}

function u16le(a, i, v) { a[i] = v & 255; a[i + 1] = (v >> 8) & 255; }

// The EOF byte and a 128-byte SAUCE record, to append after the art.
function makeSauce(info, artSize) {
  const out = new Uint8Array(129).fill(0);
  out[0] = 0x1A;
  const put = (i, s, len, pad = 32) => {
    for (let k = 0; k < len; k++) out[1 + i + k] = k < s.length ? (s.charCodeAt(k) & 255) : pad;
  };
  put(0, 'SAUCE00', 7);
  put(7, info.title || '', 35);
  put(42, info.author || '', 20);
  put(62, info.group || '', 20);
  const d = new Date();
  put(82, `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}${String(d.getDate()).padStart(2, '0')}`, 8);
  const b = out.subarray(1);
  u16le(b, 90, artSize & 0xFFFF); u16le(b, 92, artSize >>> 16);
  b[94] = 1;                       // DataType: Character
  b[95] = info.fileType ?? 1;      // 0 ASCII, 1 ANSi
  u16le(b, 96, info.cols);
  u16le(b, 98, info.rows);
  b[104] = 0;
  b[105] = info.flags || 0;
  put(106, info.font || '', 22, 0);
  return out;
}

// ---------------------------------------------------------------- the parser

// Terminal emulator that writes into a Doc. The page is endless downwards:
// rows are counted from the top of the art, the way ANSI viewers do.
class Term {
  constructor(doc, amiga) {
    this.doc = doc;
    this.amiga = amiga;
    this.x = 0; this.y = 0;
    this.saved = [0, 0];
    this.state = 0;           // 0 text, 1 ESC, 2 CSI, 3 ESC + one byte to skip
    this.params = ''; this.inter = '';
    this.fontRequest = null;  // a SyncTERM font number asked for by CSI 0;n SP D
    this.resetAttrs();
  }
  resetAttrs() {
    this.fg = 7; this.bg = 0;
    this.bold = this.blink = this.italic = this.under = this.reverse = this.conceal = false;
  }
  attr() {
    let f = this.reverse ? this.bg : this.fg, b = this.reverse ? this.fg : this.bg;
    f += this.bold ? 8 : 0;
    b += this.blink ? 8 : 0;
    if (this.conceal) f = b & 7;
    return (f << 8) | (b << 12) | (this.italic ? ITALIC : 0) | (this.under ? UNDERLINE : 0);
  }
  blankCell() { return makeCell(32, 7, this.reverse ? this.fg : this.bg); }

  put(ch) {
    const d = this.doc;
    if (this.x >= d.cols) { this.x = 0; this.y++; }
    d.set(this.x, this.y, ch | this.attr());
    // Wrap at once, like ANSI.SYS and the Amiga console.
    if (++this.x >= d.cols) { this.x = 0; this.y++; }
  }

  write(bytes, from = 0, to = bytes.length) {
    for (let i = from; i < to; i++) this.byte(bytes[i]);
  }

  byte(c) {
    switch (this.state) {
      case 1: return this.esc(c);
      case 2: return this.csiByte(c);
      case 3: this.state = 0; return;
    }
    if (c === 0x1B) { this.state = 1; return; }
    if (this.amiga) {
      if (c === 0x9B) { this.startCsi(); return; }       // the Amiga's 8-bit CSI
      if (c === 0x84) { this.y++; return; }              // IND
      if (c === 0x85) { this.x = 0; this.y++; return; }  // NEL
      if (c === 0x8D) { if (this.y > 0) this.y--; return; } // RI
      if (c === 0x0C) { this.clearAll(); return; }       // form feed: clear screen and home
    }
    switch (c) {
      case 0x0A: this.x = 0; this.y++; return;
      case 0x0D: this.x = 0; return;
      case 0x08: if (this.x > 0) this.x--; return;
      case 0x09: this.x = Math.min(this.doc.cols - 1, (this.x & ~7) + 8); return;
      case 0x07: case 0x00: return;
    }
    if (this.amiga && (c < 0x20 || (c >= 0x7F && c < 0xA0))) return; // no glyphs there in Latin-1
    this.put(c);
  }

  esc(c) {
    this.state = 0;
    switch (c) {
      case 0x5B: this.startCsi(); return;                      // [
      case 0x63: this.resetAttrs(); this.clearAll(); return;   // c: reset
      case 0x44: this.y++; return;                             // D: index
      case 0x45: this.x = 0; this.y++; return;                 // E: next line
      case 0x4D: if (this.y > 0) this.y--; return;             // M: reverse index
      case 0x37: this.saved = [this.x, this.y]; return;        // 7
      case 0x38: [this.x, this.y] = this.saved; return;        // 8
      case 0x28: case 0x29: case 0x23: case 0x25: this.state = 3; return; // charset etc.: skip one byte
    }
  }

  startCsi() { this.state = 2; this.params = ''; this.inter = ''; }

  csiByte(c) {
    if (c >= 0x30 && c <= 0x3F) { if (this.params.length < 64) this.params += String.fromCharCode(c); return; }
    if (c >= 0x20 && c <= 0x2F) { this.inter += String.fromCharCode(c); return; }
    this.state = 0;
    if (c >= 0x40 && c <= 0x7E) this.csi(String.fromCharCode(c));
    else if (c === 0x1B) this.state = 1;
  }

  csi(f) {
    const raw = this.params;
    const priv = /^[<=>?]/.test(raw);
    const p = (priv ? raw.slice(1) : raw).split(';').map(s => (s === '' ? NaN : parseInt(s, 10)));
    const n = (i, def = 1) => (Number.isNaN(p[i]) || p[i] === undefined || (p[i] === 0 && def === 1) ? def : p[i]);
    const d = this.doc;
    if (this.inter === ' ') {
      if (f === 'D' && !priv) this.fontRequest = n(1, 0); // SyncTERM font select
      return;                                             // Amiga SP p (cursor) and the rest
    }
    if (this.inter) return;
    if (priv) return; // ?7h, >1m and friends: nothing to draw
    switch (f) {
      case 'A': this.y = Math.max(0, this.y - n(0)); break;
      case 'B': this.y += n(0); break;
      case 'C': this.x = Math.min(d.cols - 1, this.x + n(0)); break;
      case 'D': this.x = Math.max(0, Math.min(d.cols - 1, this.x) - n(0)); break;
      case 'E': this.x = 0; this.y += n(0); break;
      case 'F': this.x = 0; this.y = Math.max(0, this.y - n(0)); break;
      case 'G': this.x = Math.min(d.cols - 1, n(0) - 1); break;
      case 'd': this.y = n(0) - 1; break;
      case 'H': case 'f':
        this.y = n(0) - 1;
        this.x = Math.min(d.cols - 1, n(1) - 1);
        break;
      case 'J': {
        const m = n(0, 0);
        if (m === 2 || m === 3) { this.clearAll(this.blankCell()); break; }
        if (m === 0) this.fill(this.x, this.y, d.cols * Math.max(d.rows, this.y + 1));
        else this.fill(0, 0, this.y * d.cols + this.x + 1);
        break;
      }
      case 'K': {
        const m = n(0, 0);
        const a = m === 0 ? this.x : 0, b = m === 1 ? this.x + 1 : d.cols;
        for (let x = a; x < b; x++) d.set(x, this.y, this.blankCell());
        break;
      }
      case 'X': for (let k = 0; k < n(0); k++) d.set(this.x + k, this.y, this.blankCell()); break;
      case '@': this.shiftRow(n(0)); break;
      case 'P': this.shiftRow(-n(0)); break;
      case 'L': this.shiftRows(n(0)); break;
      case 'M': this.shiftRows(-n(0)); break;
      case 's': this.saved = [this.x, this.y]; break;
      case 'u': if (raw === '') [this.x, this.y] = this.saved; break; // Amiga: CSI n u = line length
      case 'm': this.sgr(p); break;
      // t, x, y (Amiga page length / offsets), h, l, n, r, S, T: nothing to draw.
    }
  }

  sgr(p) {
    for (let i = 0; i < p.length; i++) {
      const v = Number.isNaN(p[i]) ? 0 : p[i];
      if (v === 0) this.resetAttrs();
      else if (v === 1) this.bold = true;
      else if (v === 3) this.italic = true;
      else if (v === 4) this.under = true;
      else if (v === 5 || v === 6) this.blink = true;
      else if (v === 7) this.reverse = true;
      else if (v === 8) this.conceal = true;
      else if (v === 22 || v === 21) this.bold = false;
      else if (v === 23) this.italic = false;
      else if (v === 24) this.under = false;
      else if (v === 25) this.blink = false;
      else if (v === 27) this.reverse = false;
      else if (v === 28) this.conceal = false;
      else if (v >= 30 && v <= 37) this.fg = v - 30;
      else if (v === 39) this.fg = 7;
      else if (v >= 40 && v <= 47) this.bg = v - 40;
      else if (v === 49) this.bg = 0;
      else if (v >= 90 && v <= 97) { this.fg = v - 90; this.bold = true; }
      else if (v >= 100 && v <= 107) { this.bg = v - 100; this.blink = true; }
      else if (v === 38 || v === 48) i += p[i + 1] === 5 ? 2 : p[i + 1] === 2 ? 4 : 0;
    }
  }

  fill(from, y0, to) { // linear cell range starting at (from, y0)
    const d = this.doc, start = y0 * d.cols + from;
    for (let k = start; k < to && k < d.rows * d.cols; k++) d.cells[k] = this.blankCell();
  }
  clearAll(blank = BLANK) {
    this.doc.cells.fill(blank, 0, this.doc.rows * this.doc.cols);
    this.x = 0; this.y = 0;
  }
  shiftRow(n) {
    const d = this.doc, row = [];
    for (let x = 0; x < d.cols; x++) row.push(d.get(x, this.y));
    for (let x = this.x; x < d.cols; x++) {
      const src = x - n;
      d.set(x, this.y, src >= this.x && src < d.cols ? row[src] : this.blankCell());
    }
  }
  shiftRows(n) {
    const d = this.doc, c = d.cols, top = this.y;
    d.ensureRows(top + 1);
    const rows = d.rows;
    if (n > 0) {
      d.ensureRows(rows + n);
      d.cells.copyWithin((top + n) * c, top * c, rows * c);
      d.cells.fill(BLANK, top * c, (top + n) * c);
    } else {
      const k = Math.min(-n, rows - top);
      d.cells.copyWithin(top * c, (top + k) * c, rows * c);
      d.cells.fill(BLANK, (rows - k) * c, rows * c);
    }
  }
}

// ---------------------------------------------------------------- loading

// Amiga or PC? Scores the bytes: CP437 shading and box lines against Latin-1
// accents and lone line feeds.
function guessAmiga(bytes) {
  let pc = 0, am = 0, cr = 0, lf = 0;
  for (let i = 0; i < bytes.length; i++) {
    const c = bytes[i];
    if ((c >= 0xB0 && c <= 0xB2) || (c >= 0xC0 && c <= 0xDF) || c === 0xFE || c === 0xF9 || c === 0xFA) pc++;
    else if (c === 0xAF || c === 0xB8 || c === 0xB7 || c === 0xB4 || c === 0xA8 || c === 0xA6 || c === 0xAB || c === 0xBB) am++;
    else if (c === 0x9B && i + 1 < bytes.length && /[0-9;mHJK]/.test(String.fromCharCode(bytes[i + 1]))) am += 20;
    else if (c === 0x0A) { lf++; if (i > 0 && bytes[i - 1] === 0x0D) cr++; }
    else if (c === 0x0C) am += 5;
  }
  if (lf > 4) { if (cr < lf / 4) am += lf / 2; else pc += lf / 4; }
  return am >= pc;
}

// A UTF-8 file of art (as found on the web today): decode it to bytes of the
// page's character set. Returns null if the bytes are not UTF-8 with
// at least one character outside ASCII.
function fromUtf8(bytes, amiga, cp437Table) {
  let text;
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes); } catch (e) { return null; }
  if (!/[\u0080-￿]/.test(text)) return null;
  const rev = new Map();
  for (let b = 0; b < 256; b++) rev.set(cp437Table[b], b);
  const out = [];
  for (const ch of text) {
    const u = ch.codePointAt(0);
    if (u < 0x80) out.push(u);
    else if (amiga) out.push(u <= 0xFF ? u : 0x3F);
    else out.push(rev.has(ch) ? rev.get(ch) : (u <= 0xFF ? u : 0x3F));
  }
  return new Uint8Array(out);
}

// Which page a UTF-8 text suits: box lines and blocks mean PC.
function utf8LooksPc(bytes) {
  let text;
  try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes); } catch (e) { return false; }
  const pc = (text.match(/[─-▟■]/g) || []).length;
  const am = (text.match(/[ -ÿ]/g) || []).length;
  return pc > am;
}

// Parse art into a Doc. bytes = the whole file (with or without SAUCE).
function parseArt(bytes, { amiga, cols = 80, forceCols = 0, binary = false } = {}) {
  const { sauce, end } = readSauce(bytes);
  if (sauce && sauce.dataType === 1 && sauce.tinfo1 >= 8 && sauce.tinfo1 <= 1000) cols = sauce.tinfo1;
  if (sauce && sauce.dataType === 5 && sauce.fileType > 0) { binary = true; cols = sauce.fileType * 2; }
  if (forceCols) cols = forceCols;
  const art = bytes.subarray(0, binary ? (sauce ? sauce.fileSize || end : bytes.length) : end);
  const doc = new Doc(cols, 25);
  let fontRequest = null;
  if (binary) { // PC .BIN: a byte and an attribute byte per cell
    const cells = art.length >> 1;
    doc.ensureRows(Math.ceil(cells / cols));
    for (let i = 0; i < cells; i++) {
      const a = art[i * 2 + 1];
      doc.cells[i] = makeCell(art[i * 2], a & 15, a >> 4);
    }
  } else {
    const t = new Term(doc, amiga);
    t.write(art);
    fontRequest = t.fontRequest;
  }
  doc.trim(1);
  return { doc, sauce, fontRequest };
}

// ---------------------------------------------------------------- saving

// The art as ANSI bytes: SGR codes only where the colours change, blank cells
// at the end of a row dropped. A row that fills the width gets no line end
// (the terminal wraps there by itself).
function writeAnsi(doc, { amiga, colours = true, clear = false } = {}) {
  const out = [];
  const str = s => { for (let i = 0; i < s.length; i++) out.push(s.charCodeAt(i)); };
  const nl = amiga ? '\n' : '\r\n';
  let cur = { fg: 7, bg: 0, bold: false, blink: false, it: false, ul: false };
  if (colours) str('\x1b[0m');
  if (clear) str(amiga ? '\x0c' : '\x1b[2J\x1b[1;1H');
  const rows = doc.usedRows();
  for (let y = 0; y < rows; y++) {
    const last = doc.lastCol(y);
    for (let x = 0; x <= last; x++) {
      const v = doc.get(x, y);
      if (colours) {
        const fg = cellFg(v), bg = cellBg(v);
        const want = { fg: fg & 7, bold: fg > 7, bg: bg & 7, blink: bg > 7, it: !!(v & ITALIC), ul: !!(v & UNDERLINE) };
        const p = [];
        if ((cur.bold && !want.bold) || (cur.blink && !want.blink) || (cur.it && !want.it) || (cur.ul && !want.ul)) {
          p.push(0);
          cur = { fg: 7, bg: 0, bold: false, blink: false, it: false, ul: false };
        }
        if (want.bold && !cur.bold) p.push(1);
        if (want.it && !cur.it) p.push(3);
        if (want.ul && !cur.ul) p.push(4);
        if (want.blink && !cur.blink) p.push(5);
        if (want.fg !== cur.fg) p.push(30 + want.fg);
        if (want.bg !== cur.bg) p.push(40 + want.bg);
        if (p.length) { str(`\x1b[${p.join(';')}m`); cur = want; }
      }
      let ch = cellCh(v);
      // Bytes a terminal would take as controls cannot be stored as glyphs.
      if (ch === 0 || ch === 0x1B || ch === 0x1A || ch === 0x0A || ch === 0x0D || ch === 0x08 || ch === 0x09 || ch === 0x07 ||
          (amiga && (ch < 0x20 || (ch >= 0x7F && ch < 0xA0)))) ch = 32;
      out.push(ch);
    }
    if (last < doc.cols - 1) str(nl);
  }
  if (colours) str('\x1b[0m');
  return new Uint8Array(out);
}

// PC .BIN: two bytes per cell, every row in full.
function writeBin(doc) {
  const rows = Math.max(doc.usedRows(), 1), out = new Uint8Array(rows * doc.cols * 2);
  for (let i = 0; i < rows * doc.cols; i++) {
    const v = doc.cells[i];
    out[i * 2] = cellCh(v);
    out[i * 2 + 1] = cellFg(v) | (cellBg(v) << 4);
  }
  return out;
}

// The characters only, as Unicode text (one string per row, trailing blanks dropped).
function docText(doc, amiga, cp437Table, rect) {
  const x0 = rect ? rect.x : 0, y0 = rect ? rect.y : 0;
  const w = rect ? rect.w : doc.cols, h = rect ? rect.h : doc.usedRows();
  const lines = [];
  for (let y = y0; y < y0 + h; y++) {
    let s = '';
    for (let x = x0; x < x0 + w; x++) {
      const c = cellCh(doc.get(x, y));
      s += amiga ? (c < 32 || (c >= 0x7F && c < 0xA0) ? ' ' : String.fromCharCode(c)) : (c === 0 ? ' ' : cp437Table[c]);
    }
    lines.push(s.replace(/ +$/, ''));
  }
  return lines;
}

if (typeof module !== 'undefined') {
  module.exports = { Doc, Term, BLANK, ITALIC, UNDERLINE, makeCell, cellCh, cellFg, cellBg, cellFlags,
    readSauce, makeSauce, parseArt, writeAnsi, writeBin, docText, guessAmiga, fromUtf8, utf8LooksPc, SAUCE_FONTS_PC };
}
