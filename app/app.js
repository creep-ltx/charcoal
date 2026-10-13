'use strict';
// amiansi: the editor. Needs fonts.js and core.js first.

const $ = id => document.getElementById(id);

// ---------------------------------------------------------------- fonts and palettes

const FONTS = FONT_DATA.map(f => {
  const raw = atob(f.b64), bits = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bits[i] = raw.charCodeAt(i);
  return { ...f, bits };
});
const fontById = id => FONTS.find(f => f.id === id);
const CP437_REV = new Map();
for (let b = 255; b >= 0; b--) CP437_REV.set(CP437_UNICODE[b], b);

const BRIGHT = ['#555555', '#ff5555', '#55ff55', '#ffff55', '#5555ff', '#ff55ff', '#55ffff', '#ffffff'];
const PALETTES = {
  amiga: { name: 'Amiga ANSI', colors: ['#000000', '#aa0000', '#00aa00', '#aaaa00', '#0000aa', '#aa00aa', '#00aaaa', '#aaaaaa', ...BRIGHT] },
  vga: { name: 'PC VGA', colors: ['#000000', '#aa0000', '#00aa00', '#aa5500', '#0000aa', '#aa00aa', '#00aaaa', '#aaaaaa', ...BRIGHT] },
  wb: { name: 'Workbench 2/3', colors: ['#000000', '#bb0000', '#00bb00', '#eeaa00', '#6688bb', '#bb66bb', '#55aaaa', '#aaaaaa',
    '#666666', '#ff6666', '#66ff66', '#ffee55', '#88aaff', '#ff88ff', '#88ffff', '#ffffff'] },
  custom: { name: 'Custom', colors: null },
};

// The F-key sets (bytes). Amiga sets are Latin-1, PC sets CP437.
const FKEYS_DEFAULT = {
  amiga: [
    [0x5F, 0xAF, 0x2F, 0x5C, 0x7C, 0x3A, 0x2E, 0xB8, 0xB7, 0x27],
    [0xB4, 0x60, 0xA8, 0xB0, 0xA6, 0x2D, 0x2C, 0x3B, 0x28, 0x29],
    [0xAB, 0xBB, 0x3C, 0x3E, 0x3D, 0x2B, 0x2A, 0x5E, 0x7E, 0x22],
    [0xD7, 0xF7, 0xA4, 0xF8, 0xD8, 0xDF, 0xB5, 0xB6, 0xA7, 0xA9],
    [0x5B, 0x5D, 0x7B, 0x7D, 0x23, 0x40, 0x24, 0x25, 0x26, 0x21],
  ],
  pc: [
    [0xB0, 0xB1, 0xB2, 0xDB, 0xDF, 0xDC, 0xDD, 0xDE, 0xFE, 0xFA],
    [0xDA, 0xBF, 0xC0, 0xD9, 0xC4, 0xB3, 0xC3, 0xB4, 0xC2, 0xC1],
    [0xC9, 0xBB, 0xC8, 0xBC, 0xCD, 0xBA, 0xCC, 0xB9, 0xCB, 0xCA],
    [0xC5, 0xCE, 0xD5, 0xB8, 0xD4, 0xBE, 0xD6, 0xB7, 0xD3, 0xBD],
    [0xF9, 0xF8, 0xFD, 0xF7, 0xFB, 0xEF, 0xA9, 0xAA, 0x10, 0x11],
  ],
};

// ---------------------------------------------------------------- preferences

function loadPrefs() {
  try { return JSON.parse(localStorage.getItem('amiansi') || '{}'); } catch (e) { return {}; }
}
function savePrefs() {
  const p = {
    mode: S.mode, font: S.fontFor, palette: S.paletteFor, zoom: S.zoom, ch8: S.ch8, smear: S.smear,
    fkeys: S.fkeys, custom: PALETTES.custom.colors, dir: S.lastDir, baud: $('baud').value,
  };
  try { localStorage.setItem('amiansi', JSON.stringify(p)); } catch (e) { /* private window */ }
}
const prefs = loadPrefs();
PALETTES.custom.colors = Array.isArray(prefs.custom) && prefs.custom.length === 16 ? prefs.custom : PALETTES.amiga.colors.slice();

// ---------------------------------------------------------------- state

const S = {
  doc: new Doc(80, 25),
  mode: prefs.mode === 'pc' ? 'pc' : 'amiga',
  fontFor: Object.assign({ amiga: 'topaz1p', pc: 'vga' }, prefs.font),
  paletteFor: Object.assign({ amiga: 'amiga', pc: 'vga' }, prefs.palette),
  zoom: prefs.zoom || 2, ch8: !!prefs.ch8, smear: !!prefs.smear, ice: false,
  tool: 'draw', fg: 7, bg: 0, brush: 0x2F, italic: false, under: false, insert: false,
  cx: 0, cy: 0,
  path: null, name: 'untitled.ans', dirty: false, source: null, sauce: null, saveFmt: 'ansi',
  info: { title: '', author: '', group: '', writeSauce: true, clear: false },
  sel: null, selecting: false, float: null, clip: null, clipText: '',
  undo: [], redo: [],
  fset: 0, factive: 0, fkeys: prefs.fkeys && prefs.fkeys.amiga ? prefs.fkeys : JSON.parse(JSON.stringify(FKEYS_DEFAULT)),
  hover: null, last: null, button: -1, playing: null, blink: true,
  server: null, cwd: '', lastDir: prefs.dir || '',
  dirtyView: true,
};
const amiga = () => S.mode === 'amiga';
const font = () => fontById(S.fontFor[S.mode]) || FONTS[amiga() ? 0 : FONTS.length - 1];
const cellH = () => (S.ch8 && amiga() ? 8 : 16);
const styleFlags = () => (amiga() ? (S.italic ? ITALIC : 0) | (S.under ? UNDERLINE : 0) : 0);
const brushCell = () => makeCell(S.brush, S.fg, S.bg, styleFlags());
let PAL32 = new Uint32Array(16), PALCSS = [];

function setPalette() {
  PALCSS = PALETTES[S.paletteFor[S.mode]]?.colors || PALETTES.amiga.colors;
  PALCSS.forEach((c, i) => {
    const n = parseInt(c.slice(1), 16);
    PAL32[i] = (0xFF000000 | ((n & 255) << 16) | (n & 0xFF00) | (n >>> 16)) >>> 0;
  });
}

// Is this byte a glyph on the current page?
const printable = c => (amiga() ? !(c < 0x20 || (c >= 0x7F && c < 0xA0)) : c !== 0);

function charToByte(ch) {
  const u = ch.codePointAt(0);
  if (amiga()) return u <= 0xFF && printable(u) ? u : null;
  if (u >= 0x20 && u < 0x7F) return u;
  return CP437_REV.has(ch) ? CP437_REV.get(ch) : null;
}
const byteToChar = c => (amiga() ? String.fromCharCode(c) : CP437_UNICODE[c]);

// ---------------------------------------------------------------- drawing cells

// Draws a cell into a 32-bit pixel buffer at pixel (px, py).
function drawCell(buf, stride, px, py, v) {
  const f = font(), H = cellH(), skip = H === 8 ? 2 : 1;
  let ch = v & 255, fg = (v >>> 8) & 15, bg = (v >>> 12) & 15;
  const it = v & ITALIC, ul = v & UNDERLINE;
  let fgc, bgc, smear = false;
  if (S.ice) bgc = PAL32[bg];
  else {
    bgc = PAL32[bg & 7];
    if (bg > 7 && !amiga() && !S.blink) ch = 32; // blinking text, off phase
  }
  if (amiga() && S.smear && fg > 7) { fgc = PAL32[fg & 7]; smear = true; } else fgc = PAL32[fg];
  if (!printable(ch)) ch = 32;
  const base = ch * 16;
  for (let r = 0; r < H; r++) {
    let bits = f.bits[base + r * skip];
    const r8 = H === 8 ? r : r >> 1;
    if (it) bits = bits >> Math.max(0, (6 - r8) >> 1);
    if (smear) bits |= bits >> 1;
    if (ul && r8 === 7) bits = 0xFF;
    let o = (py + r) * stride + px;
    for (let m = 0x80; m; m >>= 1) buf[o++] = bits & m ? fgc : bgc;
  }
}

function paintCells(ctx, cells, w, h, x0 = 0, y0 = 0) { // a block of cells at cell (x0, y0) of ctx
  const H = cellH(), img = ctx.createImageData(w * 8, h * H), buf = new Uint32Array(img.data.buffer);
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) drawCell(buf, w * 8, x * 8, y * H, cells[y * w + x]);
  ctx.putImageData(img, x0 * 8, y0 * H);
}

// ---------------------------------------------------------------- the page view

const viewport = $('viewport'), sizer = $('sizer'), screen = $('screen'), sctx = screen.getContext('2d');
let image = null, imageBuf = null;

function viewRows() {
  const fit = Math.ceil(viewport.clientHeight / (cellH() * S.zoom));
  return Math.max(S.doc.rows + 25, fit);
}
function layout() {
  const H = cellH(), z = S.zoom;
  sizer.style.width = S.doc.cols * 8 * z + 'px';
  sizer.style.height = viewRows() * H * z + 'px';
  S.dirtyView = true;
}

function render() {
  const H = cellH(), z = S.zoom, rowPx = H * z, d = S.doc, cols = d.cols;
  const first = Math.max(0, Math.floor(viewport.scrollTop / rowPx));
  const n = Math.max(1, Math.min(Math.ceil(viewport.clientHeight / rowPx) + 1, viewRows() - first));
  const w = cols * 8, h = n * H;
  if (screen.width !== w || screen.height !== h) {
    screen.width = w; screen.height = h;
    image = sctx.createImageData(w, h); imageBuf = new Uint32Array(image.data.buffer);
  }
  screen.style.width = w * z + 'px';
  screen.style.height = h * z + 'px';
  screen.style.top = first * rowPx + 'px';
  for (let y = 0; y < n; y++)
    for (let x = 0; x < cols; x++) drawCell(imageBuf, w, x * 8, y * H, d.get(x, first + y));
  sctx.putImageData(image, 0, 0);
  overlay(first);
}

function overlay(first) {
  const H = cellH(), c = sctx, oy = -first * H;
  c.save();
  c.translate(0, oy);
  if (S.float && S.hover) {
    const f = S.float;
    paintCells({ createImageData: (a, b) => sctx.createImageData(a, b), putImageData: (img, x, y) => sctx.putImageData(img, x, y + oy) },
      f.cells, f.w, f.h, S.hover.x, S.hover.y);
    c.strokeStyle = '#ff8a00'; c.setLineDash([3, 2]);
    c.strokeRect(S.hover.x * 8 + 0.5, S.hover.y * H + 0.5, f.w * 8 - 1, f.h * H - 1);
  }
  if (S.sel) {
    const r = selRect();
    c.fillStyle = 'rgba(255,138,0,0.18)';
    c.fillRect(r.x * 8, r.y * H, r.w * 8, r.h * H);
    c.strokeStyle = '#ff8a00'; c.setLineDash([3, 2]);
    c.strokeRect(r.x * 8 + 0.5, r.y * H + 0.5, r.w * 8 - 1, r.h * H - 1);
  }
  c.setLineDash([]);
  if (S.hover && !S.float && S.tool !== 'type' && S.tool !== 'select') {
    c.strokeStyle = 'rgba(255,255,255,0.7)';
    if (S.tool === 'half') c.strokeRect(S.hover.x * 8 + 0.5, S.hover.y * H + S.hover.half * H / 2 + 0.5, 7, H / 2 - 1);
    else c.strokeRect(S.hover.x * 8 + 0.5, S.hover.y * H + 0.5, 7, H - 1);
  }
  if (S.blink || S.playing) { // the cursor, Amiga style: an orange block
    c.fillStyle = 'rgba(255,138,0,0.65)';
    if (S.insert) c.fillRect(S.cx * 8, S.cy * H + H - Math.max(2, H / 4), 8, Math.max(2, H / 4));
    else c.fillRect(S.cx * 8, S.cy * H, 8, H);
  }
  c.restore();
}

function frame() {
  if (S.playing) playStep();
  if (S.dirtyView) { S.dirtyView = false; render(); }
  requestAnimationFrame(frame);
}
const redraw = () => { S.dirtyView = true; };

setInterval(() => { S.blink = !S.blink; redraw(); }, 500);
viewport.addEventListener('scroll', redraw);
window.addEventListener('resize', () => { layout(); });

function ensureVisible(x, y) {
  const H = cellH() * S.zoom, W = 8 * S.zoom;
  const top = y * H, bottom = top + H, left = x * W;
  if (top < viewport.scrollTop) viewport.scrollTop = top - H;
  else if (bottom > viewport.scrollTop + viewport.clientHeight) viewport.scrollTop = bottom - viewport.clientHeight + H;
  if (left < viewport.scrollLeft) viewport.scrollLeft = left - W;
  else if (left + W > viewport.scrollLeft + viewport.clientWidth) viewport.scrollLeft = left + W * 2 - viewport.clientWidth;
}

// ---------------------------------------------------------------- editing

function snapshot() { return { doc: S.doc.clone(), cx: S.cx, cy: S.cy }; }
function pushUndo() {
  S.undo.push(snapshot());
  if (S.undo.length > 300) S.undo.shift();
  S.redo.length = 0;
  setDirty(true);
}
function restore(s) {
  S.doc = s.doc; S.cx = s.cx; S.cy = s.cy;
  layout(); updateStatus();
}
function undo() { if (!S.undo.length) return msg('Nothing to undo'); S.redo.push(snapshot()); restore(S.undo.pop()); setDirty(true); }
function redo() { if (!S.redo.length) return msg('Nothing to redo'); S.undo.push(snapshot()); restore(S.redo.pop()); setDirty(true); }

function setDirty(d) {
  S.dirty = d;
  document.title = `${d ? '* ' : ''}${S.name} - amiansi`;
  updateStatus();
}

function setCell(x, y, v) {
  if (x < 0 || x >= S.doc.cols || y < 0) return;
  const grow = y >= S.doc.rows;
  S.doc.set(x, y, v);
  if (grow) layout();
  redraw();
}

function typeByte(b) {
  pushUndo();
  const d = S.doc;
  if (S.cx >= d.cols) { S.cx = 0; S.cy++; }
  if (S.insert) for (let x = d.cols - 1; x > S.cx; x--) d.set(x, S.cy, d.get(x - 1, S.cy));
  setCell(S.cx, S.cy, makeCell(b, S.fg, S.bg, styleFlags()));
  S.cx++;
  if (S.cx >= d.cols) { S.cx = 0; S.cy++; }
  moved();
}

function moved() {
  S.cx = Math.max(0, Math.min(S.doc.cols - 1, S.cx));
  S.cy = Math.max(0, S.cy);
  if (S.cy >= S.doc.rows - 1) { S.doc.ensureRows(S.cy + 2); layout(); }
  S.blink = true;
  ensureVisible(S.cx, S.cy);
  redraw(); updateStatus();
}

function deleteAt(x, y) { // remove the cell, pull the rest of the row left
  const d = S.doc;
  for (let i = x; i < d.cols - 1; i++) d.set(i, y, d.get(i + 1, y));
  d.set(d.cols - 1, y, BLANK);
  redraw();
}

function line(x0, y0, x1, y1, fn) {
  const dx = Math.abs(x1 - x0), dy = -Math.abs(y1 - y0), sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1;
  let err = dx + dy;
  for (;;) {
    fn(x0, y0);
    if (x0 === x1 && y0 === y1) break;
    const e2 = 2 * err;
    if (e2 >= dy) { err += dy; x0 += sx; }
    if (e2 <= dx) { err += dx; y0 += sy; }
  }
}

function paintAt(x, y, button) {
  const old = S.doc.get(x, y);
  switch (S.tool) {
    case 'draw': setCell(x, y, button === 2 ? BLANK : brushCell()); break;
    case 'colour':
      if (button === 2) setCell(x, y, (old & ~0xF000) | (S.bg << 12));
      else setCell(x, y, (old & ~0xF00) | (S.fg << 8));
      break;
  }
}

// Half blocks: a cell holds two pixels, top and bottom.
function paintHalf(x, hy, colour) {
  const y = hy >> 1, bottom = hy & 1, v = S.doc.get(x, y);
  const ch = cellCh(v), fg = cellFg(v), bg = cellBg(v);
  let top, bot;
  if (ch === 0xDB) top = bot = fg;
  else if (ch === 0xDF) { top = fg; bot = bg; }
  else if (ch === 0xDC) { top = bg; bot = fg; }
  else top = bot = bg;
  if (bottom) bot = colour; else top = colour;
  const lim = S.ice ? 15 : 7;
  let cell;
  if (top === bot) cell = top <= lim ? makeCell(32, 7, top) : makeCell(0xDB, top, 0);
  else if (bot <= lim) cell = makeCell(0xDF, top, bot);
  else if (top <= lim) cell = makeCell(0xDC, bot, top);
  else cell = makeCell(0xDF, top, bot & 7);
  setCell(x, y, cell);
}

function floodFill(x, y) {
  const d = S.doc, target = d.get(x, y), want = brushCell();
  if (target === want || y >= d.rows) return;
  const stack = [[x, y]];
  while (stack.length) {
    const [px, py] = stack.pop();
    if (px < 0 || py < 0 || px >= d.cols || py >= d.rows || d.get(px, py) !== target) continue;
    d.set(px, py, want);
    stack.push([px + 1, py], [px - 1, py], [px, py + 1], [px, py - 1]);
  }
  redraw();
}

function pick(x, y) {
  const v = S.doc.get(x, y);
  S.brush = cellCh(v); S.fg = cellFg(v); S.bg = cellBg(v);
  S.italic = !!(v & ITALIC); S.under = !!(v & UNDERLINE);
  refreshBrush();
}

// ---------------------------------------------------------------- selection and clipboard

function selRect() {
  const s = S.sel, x = Math.min(s.x0, s.x1), y = Math.min(s.y0, s.y1);
  return { x, y, w: Math.abs(s.x1 - s.x0) + 1, h: Math.abs(s.y1 - s.y0) + 1 };
}
function copySel(cut) {
  if (!S.sel) return msg('Select a block first (Select tool)');
  const r = selRect();
  S.clip = S.doc.copyRect(r.x, r.y, r.w, r.h);
  S.clipText = docText(S.doc, amiga(), CP437_UNICODE, r).join('\n');
  navigator.clipboard?.writeText(S.clipText).catch(() => {});
  if (cut) clearSel();
  msg(`${cut ? 'Cut' : 'Copied'} ${r.w}×${r.h}`);
}
function clearSel() {
  const r = selRect();
  pushUndo();
  for (let y = r.y; y < r.y + r.h; y++) for (let x = r.x; x < r.x + r.w; x++) S.doc.set(x, y, BLANK);
  redraw();
}
function startPaste(block) {
  if (!block) return msg('The clipboard is empty');
  S.float = block;
  S.sel = null;
  if (!S.hover) S.hover = { x: S.cx, y: S.cy, half: 0 };
  msg('Click to stamp the block (Shift: without blanks), Esc to stop');
  redraw();
}
function stamp(x0, y0, transparent) {
  const f = S.float;
  pushUndo();
  for (let y = 0; y < f.h; y++) for (let x = 0; x < f.w; x++) {
    const v = f.cells[y * f.w + x];
    if (transparent && S.doc.isBlank(v)) continue;
    if (x0 + x < S.doc.cols) S.doc.set(x0 + x, y0 + y, v);
  }
  layout();
}
function textBlock(text) {
  const lines = text.replace(/\r\n?/g, '\n').replace(/\n$/, '').split('\n');
  const w = Math.min(S.doc.cols, Math.max(1, ...lines.map(l => [...l].length))), h = lines.length;
  const cells = new Uint32Array(w * h).fill(BLANK);
  lines.forEach((l, y) => [...l].slice(0, w).forEach((ch, x) => {
    const b = ch === '\t' ? 32 : charToByte(ch);
    cells[y * w + x] = makeCell(b ?? 0x3F, S.fg, S.bg, styleFlags());
  }));
  return { w, h, cells };
}

let pasteTimer = null;
document.addEventListener('paste', e => {
  if (e.target.closest('input, textarea, dialog')) return;
  clearTimeout(pasteTimer);
  const text = e.clipboardData?.getData('text/plain') || '';
  e.preventDefault();
  if (S.clip && (text === S.clipText || !text)) startPaste(S.clip);
  else if (text) startPaste(textBlock(text));
});

// ---------------------------------------------------------------- mouse on the page

function cellAt(e) {
  const r = sizer.getBoundingClientRect(), H = cellH() * S.zoom, W = 8 * S.zoom;
  const fx = (e.clientX - r.left) / W, fy = (e.clientY - r.top) / H;
  const x = Math.floor(fx), y = Math.floor(fy);
  return { x: Math.max(0, Math.min(S.doc.cols - 1, x)), y: Math.max(0, y), half: fy - y >= 0.5 ? 1 : 0, inside: fx >= 0 && fx < S.doc.cols && fy >= 0 };
}

sizer.addEventListener('contextmenu', e => e.preventDefault());
sizer.addEventListener('mousedown', e => {
  if (S.playing) return;
  e.preventDefault();
  viewport.focus({ preventScroll: true });
  const p = cellAt(e);
  if (!p.inside) return;
  S.hover = p;
  if (S.float) {
    if (e.button === 0) stamp(p.x, p.y, e.shiftKey);
    else if (e.button === 2) { S.float = null; redraw(); }
    return;
  }
  if (e.button === 1 || S.tool === 'pick') { pick(p.x, p.y); return; }
  S.button = e.button;
  switch (S.tool) {
    case 'type': S.cx = p.x; S.cy = p.y; S.sel = null; moved(); break;
    case 'select':
      S.sel = { x0: p.x, y0: p.y, x1: p.x, y1: p.y }; S.selecting = true; redraw(); break;
    case 'fill': if (e.button === 0) { pushUndo(); floodFill(p.x, p.y); } break;
    case 'half': {
      pushUndo();
      const hy = p.y * 2 + p.half, col = e.button === 2 ? S.bg : S.fg;
      if (e.shiftKey && S.last) line(S.last.x, S.last.y, p.x, hy, (x, y) => paintHalf(x, y, col));
      else paintHalf(p.x, hy, col);
      S.last = { x: p.x, y: hy };
      break;
    }
    default: // draw, colour
      pushUndo();
      if (e.shiftKey && S.last) line(S.last.x, S.last.y, p.x, p.y, (x, y) => paintAt(x, y, e.button));
      else paintAt(p.x, p.y, e.button);
      S.last = { x: p.x, y: p.y };
  }
});

window.addEventListener('mousemove', e => {
  if (S.playing) return;
  const p = cellAt(e);
  const over = e.target === screen || e.target === sizer;
  const prev = S.hover;
  S.hover = over || S.button >= 0 ? p : (S.float ? S.hover : null);
  if (S.button >= 0) {
    if (S.selecting) { S.sel.x1 = p.x; S.sel.y1 = p.y; }
    else if (S.tool === 'half') {
      const hy = p.y * 2 + p.half, col = S.button === 2 ? S.bg : S.fg;
      if (S.last) line(S.last.x, S.last.y, p.x, hy, (x, y) => paintHalf(x, y, col));
      S.last = { x: p.x, y: hy };
    } else if (S.tool === 'draw' || S.tool === 'colour') {
      if (S.last) line(S.last.x, S.last.y, p.x, p.y, (x, y) => paintAt(x, y, S.button));
      S.last = { x: p.x, y: p.y };
    }
  }
  if (!prev || !S.hover || prev.x !== S.hover.x || prev.y !== S.hover.y || prev.half !== S.hover.half) { redraw(); updateStatus(); }
});
window.addEventListener('mouseup', () => {
  if (S.selecting) {
    S.selecting = false;
    const r = selRect();
    msg(`${r.w}×${r.h} selected: Ctrl+C copy, Ctrl+X cut, Del clear`);
  }
  S.button = -1;
});

// ---------------------------------------------------------------- keys

document.addEventListener('keydown', e => {
  const t = e.target;
  if (t.closest('dialog') || (t.matches('input, select, textarea') && !t.matches('[type=checkbox]'))) return;
  if (S.playing) { if (e.key === 'Escape') stopPlay(true); e.preventDefault(); return; }
  const ctrl = e.ctrlKey || e.metaKey, k = e.key;
  if (ctrl && !e.altKey) {
    const lk = k.toLowerCase();
    const act = {
      z: () => (e.shiftKey ? redo() : undo()), y: redo, s: () => save(e.shiftKey), o: openDialog,
      a: () => { S.sel = { x0: 0, y0: 0, x1: S.doc.cols - 1, y1: Math.max(0, S.doc.usedRows() - 1) }; redraw(); },
      c: () => copySel(false), x: () => copySel(true),
      arrowup: () => setFset(S.fset + 1), arrowdown: () => setFset(S.fset - 1),
      arrowleft: () => (e.shiftKey ? setBg((S.bg + 15) % 16) : setFg((S.fg + 15) % 16)),
      arrowright: () => (e.shiftKey ? setBg((S.bg + 1) % 16) : setFg((S.fg + 1) % 16)),
    }[lk];
    if (lk === 'v') { // the paste event follows when the system clipboard has text
      pasteTimer = setTimeout(() => startPaste(S.clip), 80);
      return;
    }
    if (act) { e.preventDefault(); act(); }
    return;
  }
  const fk = /^F(\d+)$/.exec(k);
  if (fk && +fk[1] >= 1 && +fk[1] <= 10) {
    e.preventDefault();
    typeByte(S.fkeys[S.mode][S.fset][+fk[1] - 1]);
    return;
  }
  const visRows = Math.floor(viewport.clientHeight / (cellH() * S.zoom));
  const nav = {
    ArrowLeft: () => { S.cx--; }, ArrowRight: () => { S.cx++; }, ArrowUp: () => { S.cy--; }, ArrowDown: () => { S.cy++; },
    Home: () => { S.cx = 0; }, End: () => { S.cx = Math.min(S.doc.cols - 1, S.doc.lastCol(S.cy) + 1); },
    PageUp: () => { S.cy -= visRows; }, PageDown: () => { S.cy += visRows; },
    Enter: () => { S.cx = 0; S.cy++; },
    Tab: () => { S.cx = Math.min(S.doc.cols - 1, (S.cx & ~7) + 8); },
  }[k];
  if (nav) { e.preventDefault(); nav(); moved(); return; }
  switch (k) {
    case 'Escape':
      e.preventDefault();
      if (S.float) S.float = null; else S.sel = null;
      redraw(); return;
    case 'Backspace':
      e.preventDefault();
      if (S.cx > 0) {
        pushUndo(); S.cx--;
        if (S.insert) deleteAt(S.cx, S.cy); else setCell(S.cx, S.cy, makeCell(32, S.fg, S.bg));
        moved();
      }
      return;
    case 'Delete':
      e.preventDefault();
      if (S.sel) clearSel(); else { pushUndo(); deleteAt(S.cx, S.cy); }
      return;
    case 'Insert':
      e.preventDefault(); S.insert = !S.insert; $('insMode').checked = S.insert; updateStatus(); redraw(); return;
  }
  if (e.altKey && !e.getModifierState('AltGraph')) return;
  if ([...k].length === 1) {
    e.preventDefault();
    const b = charToByte(k);
    if (b === null) return msg(`"${k}" is not in the ${amiga() ? 'Latin-1' : 'CP437'} set`);
    typeByte(b);
  }
});

// ---------------------------------------------------------------- the bottom pane

const charmap = $('charmap'), cmctx = charmap.getContext('2d');
const CM_COLS = 32;

function drawCharmap() {
  const H = cellH(), rows = 256 / CM_COLS;
  charmap.width = CM_COLS * 8; charmap.height = rows * H;
  charmap.style.width = CM_COLS * 16 + 'px'; charmap.style.height = rows * H * 2 + 'px';
  const fg = 7, bg = 0; // grey on black reads best whatever the brush is
  const cells = new Uint32Array(256);
  for (let c = 0; c < 256; c++) cells[c] = printable(c) ? makeCell(c, fg, bg) : makeCell(32, 7, 0);
  paintCells(cmctx, cells, CM_COLS, rows);
  for (let c = 0; c < 256; c++) if (!printable(c)) { // marks the bytes that are no glyphs here
    cmctx.fillStyle = '#26282d';
    cmctx.fillRect((c % CM_COLS) * 8 + 1, Math.floor(c / CM_COLS) * H + 1, 6, H - 2);
  }
  cmctx.strokeStyle = '#ff8a00';
  cmctx.strokeRect((S.brush % CM_COLS) * 8 + 0.5, Math.floor(S.brush / CM_COLS) * H + 0.5, 7, H - 1);
}
function charmapCode(e) {
  const r = charmap.getBoundingClientRect(), H = cellH() * 2;
  const x = Math.floor((e.clientX - r.left) / 16), y = Math.floor((e.clientY - r.top) / H);
  return x >= 0 && x < CM_COLS && y >= 0 && y < 256 / CM_COLS ? y * CM_COLS + x : -1;
}
charmap.addEventListener('contextmenu', e => e.preventDefault());
charmap.addEventListener('mousedown', e => {
  e.preventDefault();
  const c = charmapCode(e);
  if (c < 0 || !printable(c)) return;
  if (e.button === 2) { S.fkeys[S.mode][S.fset][S.factive] = c; savePrefs(); drawFkeys(); return; }
  if (S.tool === 'type' && !e.shiftKey) { typeByte(c); }
  else {
    S.brush = c;
    if (!['draw', 'fill', 'type'].includes(S.tool)) setTool('draw');
  }
  refreshBrush();
});
charmap.addEventListener('mousemove', e => {
  const c = charmapCode(e);
  $('charInfo').textContent = c < 0 ? '' : `${c} · 0x${c.toString(16).toUpperCase().padStart(2, '0')}${printable(c) ? ` · ${byteToChar(c)}` : ' · (none)'}`;
});
charmap.addEventListener('mouseleave', () => { $('charInfo').textContent = ''; });

function glyphCanvas(cv, c, fg, bg) {
  const H = cellH();
  cv.width = 8; cv.height = H;
  paintCells(cv.getContext('2d'), new Uint32Array([makeCell(c, fg, bg, styleFlags())]), 1, 1);
}

function drawFkeys() {
  const box = $('fslots');
  box.innerHTML = '';
  $('fSet').textContent = `${S.fset + 1}/${S.fkeys[S.mode].length}`;
  S.fkeys[S.mode][S.fset].forEach((c, i) => {
    const d = document.createElement('div');
    d.className = 'fslot' + (i === S.factive ? ' on' : '');
    d.title = `F${i + 1}: ${byteToChar(c)} (${c})`;
    const cv = document.createElement('canvas');
    glyphCanvas(cv, c, 7, 0);
    cv.style.height = cellH() * 3 + 'px';
    const l = document.createElement('small'); l.textContent = 'F' + (i + 1);
    d.append(cv, l);
    d.addEventListener('mousedown', ev => {
      ev.preventDefault();
      S.factive = i; S.brush = c;
      if (!['draw', 'fill', 'type'].includes(S.tool)) setTool('draw');
      refreshBrush();
    });
    box.append(d);
  });
}
function setFset(n) {
  const sets = S.fkeys[S.mode];
  S.fset = (n + sets.length) % sets.length;
  drawFkeys();
}
$('fPrev').onclick = () => setFset(S.fset - 1);
$('fNext').onclick = () => setFset(S.fset + 1);

function drawSwatches() {
  const box = $('swatches');
  box.innerHTML = '';
  PALCSS.forEach((c, i) => {
    const d = document.createElement('div');
    d.className = 'sw' + (i === S.fg ? ' fg' : '') + (i === S.bg ? ' bg' : '');
    d.style.background = c;
    d.title = `${i}: ${c}${i > 7 ? (i === S.bg ? '' : ' (as background: needs iCE)') : ''}`;
    if (i === S.fg || i === S.bg) {
      const t = document.createElement('span'); t.className = 'tag';
      t.textContent = (i === S.fg ? 'F' : '') + (i === S.bg ? 'B' : '');
      d.append(t);
    }
    d.addEventListener('contextmenu', ev => ev.preventDefault());
    d.addEventListener('mousedown', ev => {
      ev.preventDefault();
      if (ev.shiftKey) return editColour(i);
      if (ev.button === 2) setBg(i); else setFg(i);
    });
    box.append(d);
  });
}
function editColour(i) {
  const inp = $('colorEdit');
  inp.value = PALCSS[i];
  inp.oninput = () => {
    if (S.paletteFor[S.mode] !== 'custom') {
      PALETTES.custom.colors = PALCSS.slice();
      S.paletteFor[S.mode] = 'custom';
      $('palette').value = 'custom';
    }
    PALETTES.custom.colors[i] = inp.value;
    setPalette(); savePrefs(); refreshBrush(); redraw();
  };
  inp.click();
}
function setFg(i) { S.fg = i; refreshBrush(); }
function setBg(i) {
  S.bg = i;
  if (i > 7 && !S.ice) msg('Backgrounds 8-15 need iCE colours (otherwise they blink on PC)');
  refreshBrush();
}
$('bSwap').onclick = () => { [S.fg, S.bg] = [S.bg, S.fg]; refreshBrush(); };

function refreshBrush() {
  glyphCanvas($('brush'), S.brush, S.fg, S.bg);
  $('brush').style.height = cellH() * 3 + 'px';
  $('brushInfo').textContent = `${byteToChar(S.brush)} ${S.brush} · fg ${S.fg} · bg ${S.bg}`;
  $('stItalic').checked = S.italic; $('stUnder').checked = S.under;
  drawSwatches(); drawCharmap(); drawFkeys(); updateStatus();
}

function setTool(t) {
  if (t === 'half' && amiga()) t = 'draw';
  S.tool = t;
  document.querySelectorAll('[data-tool]').forEach(b => b.classList.toggle('on', b.dataset.tool === t));
  if (t !== 'select') { S.sel = S.selecting ? S.sel : (t === 'type' ? S.sel : null); }
  redraw();
}
document.querySelectorAll('[data-tool]').forEach(b => { b.onclick = () => { setTool(b.dataset.tool); viewport.focus(); }; });
$('stItalic').onchange = e => { S.italic = e.target.checked; refreshBrush(); };
$('stUnder').onchange = e => { S.under = e.target.checked; refreshBrush(); };
$('insMode').onchange = e => { S.insert = e.target.checked; redraw(); updateStatus(); };

// ---------------------------------------------------------------- toolbar

function fillFontSelect() {
  const sel = $('font');
  sel.innerHTML = '';
  FONTS.filter(f => (f.kind === 'amiga') === amiga()).forEach(f => sel.add(new Option(f.name, f.id)));
  sel.value = font().id;
  const pal = $('palette');
  pal.innerHTML = '';
  for (const [id, p] of Object.entries(PALETTES)) pal.add(new Option(p.name, id));
  pal.value = S.paletteFor[S.mode];
}

function applyMode() {
  document.querySelectorAll('#modeSeg button').forEach(b => b.classList.toggle('on', b.dataset.mode === S.mode));
  $('toolHalf').hidden = amiga();
  $('lbl8x8').hidden = !amiga();
  $('lblSmear').hidden = !amiga();
  $('stItalic').parentElement.hidden = $('stUnder').parentElement.hidden = !amiga();
  if (S.tool === 'half' && amiga()) setTool('draw');
  if (S.fset >= S.fkeys[S.mode].length) S.fset = 0;
  fillFontSelect(); setPalette(); layout(); refreshBrush();
}

function setMode(m) {
  if (m === S.mode) return;
  S.mode = m;
  if (S.source && !S.dirty) reparse();
  else if (S.dirty) msg('Shown with the ' + (amiga() ? 'Amiga' : 'PC') + ' font and colours; the bytes are unchanged');
  applyMode(); savePrefs();
}
document.querySelectorAll('#modeSeg button').forEach(b => { b.onclick = () => setMode(b.dataset.mode); });

function blurAfter(el, fn) { el.addEventListener('change', e => { fn(e); el.blur(); viewport.focus({ preventScroll: true }); }); }
blurAfter($('font'), e => { S.fontFor[S.mode] = e.target.value; savePrefs(); refreshBrush(); redraw(); });
blurAfter($('palette'), e => { S.paletteFor[S.mode] = e.target.value; setPalette(); savePrefs(); refreshBrush(); redraw(); });
blurAfter($('zoom'), e => {
  const H = cellH() * S.zoom, row = viewport.scrollTop / H;
  S.zoom = +e.target.value; layout();
  viewport.scrollTop = row * cellH() * S.zoom;
  savePrefs();
});
blurAfter($('ch8'), e => { S.ch8 = e.target.checked; layout(); refreshBrush(); savePrefs(); });
blurAfter($('ice'), e => { S.ice = e.target.checked; redraw(); refreshBrush(); });
blurAfter($('smear'), e => { S.smear = e.target.checked; redraw(); refreshBrush(); savePrefs(); });
blurAfter($('baud'), () => savePrefs());
blurAfter($('cols'), e => {
  const n = Math.max(8, Math.min(1000, parseInt(e.target.value, 10) || 80));
  e.target.value = n;
  if (n === S.doc.cols) return;
  if (S.source && !S.dirty) { S.forceCols = n; reparse(); }
  else { pushUndo(); S.doc = S.doc.withCols(n); }
  S.cx = Math.min(S.cx, n - 1);
  layout(); updateStatus();
});

// ---------------------------------------------------------------- status

let msgTimer = null;
function msg(text) {
  $('stMsg').textContent = text;
  clearTimeout(msgTimer);
  msgTimer = setTimeout(() => { $('stMsg').textContent = ''; }, 4000);
}
function updateStatus() {
  $('stPos').textContent = `Cursor ${S.cx + 1},${S.cy + 1}${S.insert ? ' INS' : ''}`;
  if (S.hover) {
    const v = S.doc.get(S.hover.x, S.hover.y), c = cellCh(v);
    $('stCell').textContent = `Mouse ${S.hover.x + 1},${S.hover.y + 1}: ${printable(c) ? byteToChar(c) : ' '} ${c} fg ${cellFg(v)} bg ${cellBg(v)}`;
  } else $('stCell').textContent = '';
  $('stMode').textContent = `${amiga() ? 'Amiga Latin-1' : 'PC CP437'} · ${font().name} · ${S.doc.cols}×${Math.max(S.doc.usedRows(), 1)}`;
  $('stFile').textContent = (S.dirty ? '* ' : '') + (S.path || S.name);
  $('cols').value = S.doc.cols;
}

// ---------------------------------------------------------------- loading

function reparse() {
  const r = parseArt(S.source.bytes, { amiga: amiga(), binary: S.source.binary, forceCols: S.forceCols || 0 });
  S.doc = r.doc;
  S.doc.ensureRows(Math.max(S.doc.rows, 25));
  if (r.fontRequest !== null) {
    const f = FONTS.find(f => f.syncterm === r.fontRequest && (f.kind === 'amiga') === amiga());
    if (f) S.fontFor[S.mode] = f.id;
  }
  S.cx = 0; S.cy = 0; S.sel = null; S.float = null;
  layout(); updateStatus();
}

function loadBytes(bytes, { name, path = null, as = 'auto' }) {
  const { sauce } = readSauce(bytes);
  const binary = /\.bin$/i.test(name) || (sauce && sauce.dataType === 5);
  const utf8 = !sauce && !binary && fromUtf8(bytes, true, CP437_UNICODE) !== null;
  let isAmiga;
  if (as !== 'auto') isAmiga = as === 'amiga';
  else if (sauce && /^amiga/i.test(sauce.font)) isAmiga = true;
  else if (sauce && /^(ibm|cp437|vga|ega|mda)/i.test(sauce.font)) isAmiga = false;
  else if (binary) isAmiga = false;
  else if (utf8) isAmiga = !utf8LooksPc(bytes);
  else isAmiga = guessAmiga(bytes);
  S.mode = isAmiga ? 'amiga' : 'pc';
  let src = bytes;
  if (utf8) { src = fromUtf8(bytes, isAmiga, CP437_UNICODE); msg('Read as UTF-8 text'); }
  if (sauce) {
    const f = FONTS.find(f => f.sauce.toLowerCase() === sauce.font.toLowerCase());
    if (f && (f.kind === 'amiga') === isAmiga) S.fontFor[S.mode] = f.id;
    S.ice = !!(sauce.flags & 1);
  } else S.ice = false;
  $('ice').checked = S.ice;
  S.source = { bytes: src, binary };
  S.sauce = sauce;
  S.forceCols = 0;
  S.info.title = sauce?.title || ''; S.info.author = sauce?.author || ''; S.info.group = sauce?.group || '';
  S.info.writeSauce = !!sauce;
  S.path = path; S.name = name;
  S.saveFmt = binary ? 'bin' : /\.(txt|asc|nfo|diz)$/i.test(name) && !/\x1b|\x9b/.test(String.fromCharCode(...bytes.subarray(0, 4096))) ? 'text' : 'ansi';
  S.undo = []; S.redo = [];
  reparse();
  applyMode();
  setDirty(false);
  viewport.scrollTop = 0; viewport.scrollLeft = 0;
  savePrefs();
}

function newDoc() {
  if (S.dirty && !confirm('Throw away the changes?')) return;
  S.doc = new Doc(80, 25);
  S.source = null; S.sauce = null; S.path = null; S.name = 'untitled.ans'; S.saveFmt = 'ansi';
  S.info = { title: '', author: '', group: '', writeSauce: true, clear: false };
  S.undo = []; S.redo = []; S.cx = S.cy = 0; S.sel = null; S.float = null;
  layout(); setDirty(false);
}
$('bNew').onclick = newDoc;

async function api(path, opts = {}) {
  const r = await fetch(path, { ...opts, headers: { 'X-Token': S.server, ...(opts.headers || {}) } });
  if (!r.ok) {
    let m = r.statusText;
    try { m = (await r.json()).error; } catch (e) { /* not JSON */ }
    throw new Error(m);
  }
  return r;
}

async function openPath(path, as = 'auto') {
  try {
    const r = await api('/api/read?path=' + encodeURIComponent(path));
    const bytes = new Uint8Array(await r.arrayBuffer());
    loadBytes(bytes, { name: path.split('/').pop(), path, as });
    S.lastDir = path.replace(/\/[^/]*$/, '') || '/';
    savePrefs();
    msg(`Opened ${path}`);
  } catch (e) { msg(`Could not open: ${e.message}`); }
}

// The open dialog: a small file browser on the server's file system.
let openSel = null;
async function listDir(dir) {
  const ul = $('openList');
  try {
    const r = await (await api('/api/ls?path=' + encodeURIComponent(dir))).json();
    $('openPath').value = r.path + '/';
    ul.innerHTML = '';
    const items = [{ name: '..', dir: true, path: r.parent }, ...r.entries.map(e => ({ ...e, path: r.path.replace(/\/$/, '') + '/' + e.name }))];
    for (const it of items) {
      const li = document.createElement('li');
      li.className = it.dir ? 'dir' : '';
      li.innerHTML = '<span></span><span></span>';
      li.firstChild.textContent = it.dir ? it.name + '/' : it.name;
      li.lastChild.textContent = it.dir ? '' : it.size.toLocaleString();
      li.onclick = () => {
        if (it.dir) return listDir(it.path);
        ul.querySelectorAll('li.sel').forEach(x => x.classList.remove('sel'));
        li.classList.add('sel'); openSel = it.path; $('openPath').value = it.path;
      };
      li.ondblclick = () => { if (!it.dir) { $('dlgOpen').close(); openPath(it.path, $('openAs').value); } };
      ul.append(li);
    }
  } catch (e) { msg(e.message); }
}
function openDialog() {
  if (!S.server) { $('fileInput').click(); return; }
  openSel = null;
  $('dlgOpen').showModal();
  listDir(S.lastDir || S.cwd);
}
$('bOpen').onclick = openDialog;
$('openPath').addEventListener('keydown', e => {
  if (e.key !== 'Enter') return;
  e.preventDefault();
  const p = $('openPath').value.trim();
  if (p.endsWith('/')) listDir(p);
  else { $('dlgOpen').close(); openPath(p, $('openAs').value); }
});
$('openGo').onclick = e => {
  e.preventDefault();
  const p = $('openPath').value.trim();
  if (openSel === p) { $('dlgOpen').close(); openPath(p, $('openAs').value); } else listDir(p);
};
$('openBrowse').onclick = () => { $('dlgOpen').close(); $('fileInput').click(); };
$('fileInput').onchange = async e => {
  const f = e.target.files[0];
  if (!f) return;
  loadBytes(new Uint8Array(await f.arrayBuffer()), { name: f.name, as: $('openAs').value });
  e.target.value = '';
};
// Dropping a file on the window opens it.
window.addEventListener('dragover', e => e.preventDefault());
window.addEventListener('drop', async e => {
  e.preventDefault();
  const f = e.dataTransfer.files[0];
  if (f) loadBytes(new Uint8Array(await f.arrayBuffer()), { name: f.name });
});

// ---------------------------------------------------------------- saving

function concat(a, b) { const o = new Uint8Array(a.length + b.length); o.set(a); o.set(b, a.length); return o; }

async function encode(fmt) {
  const d = S.doc, am = amiga(), info = S.info;
  const sauce = (art, fileType) => (info.writeSauce ? concat(art, makeSauce({
    ...info, cols: d.cols, rows: Math.max(1, d.usedRows()), font: font().sauce, fileType,
    flags: (S.ice ? 1 : 0),
  }, art.length)) : art);
  switch (fmt) {
    case 'ansi': return sauce(writeAnsi(d, { amiga: am, clear: info.clear }), 1);
    case 'text': return sauce(writeAnsi(d, { amiga: am, colours: false }), 0);
    case 'utf8': return new TextEncoder().encode(docText(d, am, CP437_UNICODE).join(am ? '\n' : '\r\n') + (am ? '\n' : '\r\n'));
    case 'bin': return writeBin(d);
    case 'png': {
      const H = cellH(), rows = Math.max(1, Math.min(d.usedRows(), Math.floor(32000 / H)));
      const cv = document.createElement('canvas');
      cv.width = d.cols * 8; cv.height = rows * H;
      paintCells(cv.getContext('2d'), d.cells.subarray(0, rows * d.cols), d.cols, rows);
      const blob = await new Promise(res => cv.toBlob(res, 'image/png'));
      return new Uint8Array(await blob.arrayBuffer());
    }
  }
}

const EXT = { ansi: '.ans', text: '.asc', utf8: '.txt', bin: '.bin', png: '.png' };

async function writeOut(path, fmt) {
  const bytes = await encode(fmt);
  if (S.server) {
    try {
      await api('/api/write?path=' + encodeURIComponent(path), { method: 'POST', body: bytes });
    } catch (e) { msg(`Could not save: ${e.message}`); return false; }
  } else {
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob([bytes]));
    a.download = path.split('/').pop();
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 5000);
  }
  if (['ansi', 'text', 'bin'].includes(fmt)) { // the page's own file; png and utf8 are exports
    S.path = S.server ? path : null; S.name = path.split('/').pop(); S.saveFmt = fmt;
    S.source = { bytes: fmt === 'bin' ? bytes : bytes, binary: fmt === 'bin' };
    S.forceCols = 0;
    setDirty(false);
  }
  msg(`Saved ${path} (${bytes.length.toLocaleString()} bytes)`);
  return true;
}

function save(as) {
  if (as || !S.path) return saveDialog();
  writeOut(S.path, S.saveFmt);
}
function saveDialog() {
  const dir = S.path ? S.path.replace(/\/[^/]*$/, '') : (S.lastDir || S.cwd);
  $('savePath').value = S.server ? (S.path || `${dir}/${S.name}`) : S.name;
  $('saveFmt').value = S.saveFmt;
  $('dlgSave').showModal();
}
$('saveFmt').onchange = e => {
  const p = $('savePath').value, ext = EXT[e.target.value];
  $('savePath').value = /\.[^./]*$/.test(p) ? p.replace(/\.[^./]*$/, ext) : p + ext;
};
$('dlgSave').addEventListener('close', () => {
  if ($('dlgSave').returnValue === 'save') writeOut($('savePath').value.trim(), $('saveFmt').value);
});
$('bSave').onclick = () => save(false);
$('bSaveAs').onclick = () => save(true);

window.addEventListener('beforeunload', e => { if (S.dirty) e.preventDefault(); });

// ---------------------------------------------------------------- info (SAUCE)

$('bInfo').onclick = () => {
  $('sTitle').value = S.info.title; $('sAuthor').value = S.info.author; $('sGroup').value = S.info.group;
  $('sSauce').checked = S.info.writeSauce; $('sClear').checked = S.info.clear;
  const s = S.sauce;
  $('sOrig').textContent = s ? `The file's SAUCE: ${s.title || '(no title)'} by ${s.author || '?'} / ${s.group || '?'}, ${s.date}, ` +
    `${s.tinfo1}×${s.tinfo2}, font "${s.font || '-'}"${s.flags & 1 ? ', iCE colours' : ''}.` +
    (s.notes ? ' Notes: ' + s.notes.join(' / ') : '') : 'The file had no SAUCE record.';
  $('dlgInfo').showModal();
};
$('dlgInfo').addEventListener('close', () => {
  Object.assign(S.info, {
    title: $('sTitle').value, author: $('sAuthor').value, group: $('sGroup').value,
    writeSauce: $('sSauce').checked, clear: $('sClear').checked,
  });
});
$('bHelp').onclick = () => $('dlgHelp').showModal();

// ---------------------------------------------------------------- playback at modem speed

async function play() {
  if (S.playing) return stopPlay(true);
  let bytes = S.source && !S.dirty ? S.source.bytes : writeAnsi(S.doc, { amiga: amiga() });
  if (S.source?.binary && !S.dirty) return msg('A .bin file has no codes to play');
  bytes = bytes.subarray(0, readSauce(bytes).end);
  const doc = new Doc(S.doc.cols, 25);
  S.playing = { bytes, pos: 0, term: new Term(doc, amiga()), saved: snapshot(), last: performance.now() };
  S.doc = doc; S.sel = null; S.float = null;
  viewport.scrollTop = 0;
  $('bPlay').textContent = '■ Stop';
  layout();
}
function playStep() {
  const p = S.playing, now = performance.now();
  const n = Math.max(1, Math.round(((now - p.last) / 1000) * (+$('baud').value / 10)));
  p.last = now;
  const rows = S.doc.rows;
  p.term.write(p.bytes, p.pos, Math.min(p.bytes.length, p.pos + n));
  p.pos += n;
  S.cx = Math.min(p.term.x, S.doc.cols - 1); S.cy = p.term.y;
  S.doc.ensureRows(S.cy + 1);
  if (S.doc.rows !== rows) layout();
  ensureVisible(S.cx, S.cy);
  redraw();
  if (p.pos >= p.bytes.length) stopPlay(false);
}
function stopPlay(skip) {
  const p = S.playing;
  if (skip) p.term.write(p.bytes, p.pos);
  S.playing = null;
  $('bPlay').textContent = '▶ Play';
  // Playing does not change the page: put back the page as it was.
  restore(p.saved);
  msg('Played ' + p.bytes.length.toLocaleString() + ' bytes');
}
$('bPlay').onclick = play;

// ---------------------------------------------------------------- start

async function start() {
  $('zoom').value = S.zoom; $('ch8').checked = S.ch8; $('smear').checked = S.smear;
  if (prefs.baud) $('baud').value = prefs.baud;
  setTool('draw');
  applyMode();
  setDirty(false);
  requestAnimationFrame(frame);
  const token = new URLSearchParams(location.hash.slice(1)).get('t');
  if (token) {
    try {
      S.server = token;
      const init = await (await api('/api/init')).json();
      S.cwd = init.cwd;
      if (!S.lastDir) S.lastDir = init.cwd;
      if (init.file) {
        try {
          const r = await api('/api/read?path=' + encodeURIComponent(init.file));
          loadBytes(new Uint8Array(await r.arrayBuffer()), { name: init.file.split('/').pop(), path: init.file });
        } catch (e) { // a new file
          S.path = init.file; S.name = init.file.split('/').pop(); setDirty(false);
          msg('New file: ' + init.file);
        }
      }
    } catch (e) {
      S.server = null;
      msg('No server: files open and save through the browser');
    }
  } else msg('No server: files open and save through the browser');
  viewport.focus();
}
start();
