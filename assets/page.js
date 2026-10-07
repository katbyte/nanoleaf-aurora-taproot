'use strict';

// the theme, before anything is painted
(function () {
  let theme = 'dark';
  try { theme = localStorage.getItem('taproot-theme') || 'dark'; } catch (e) { /* storage is off: dark it is */ }
  document.documentElement.dataset.theme = theme;
})();

// The six plugins every Light Panels controller has, by the ids the
// documentation gives them. What each does to a palette is drawn in sampler().
const PLUGINS = {
  '6970681a-20b5-4c5e-8813-bdaebc4ee4fa': 'wheel',
  '027842e4-e1d6-4a4c-a731-be74a1ebd4cf': 'flow',
  '713518c1-d560-47db-8991-de780af71d1e': 'explode',
  'b3fd723a-aae8-4c99-bf2b-087159e0ef53': 'fade',
  'ba632d3e-9c2b-4413-a965-510c839b3f71': 'random',
  '70b7c636-6bf8-491f-89c1-f4103508d642': 'highlight',
};

// shapes that are part of a layout but give no light: the rhythm module, controllers, connectors
const UNLIT = new Set([1, 12, 16, 19, 20]);
// what a shape's side is when the controller reports none (firmware from 5.0.0)
const SIDES = { 0: 150, 2: 100, 3: 100, 4: 100, 7: 67, 8: 134, 9: 67, 14: 134, 15: 58, 17: 154, 18: 77, 29: 50, 30: 180, 31: 180, 32: 180 };

let state = { controllers: [], pairing: [], dryRun: false };
const cards = new Map(); // controller name -> its card
let copying = null; // the scene the copy dialog is about

// ---- small helpers ---------------------------------------------------------

const $ = (id) => document.getElementById(id);

// el builds an element. Text only ever goes in as text: names come from
// controllers and are never treated as markup.
function el(tag, props, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key === 'style') node.style.cssText = value; // the page's security policy drops a style attribute; this way is allowed
    else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else if (key === 'dataset') Object.assign(node.dataset, value);
    else node.setAttribute(key, value === true ? '' : value);
  }
  for (const child of children) {
    if (child === null || child === undefined || child === false) continue;
    node.append(child);
  }
  return node;
}

function svg(tag, attrs) {
  const node = document.createElementNS('http://www.w3.org/2000/svg', tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (key.startsWith('on') && typeof value === 'function') node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value);
  }
  return node;
}

function toast(message, bad) {
  const node = el('div', { class: bad ? 'toast bad' : 'toast', text: message });
  $('toasts').append(node);
  setTimeout(() => node.remove(), bad ? 12000 : 6000);
}

function store(key, value) {
  try {
    if (value === undefined) return JSON.parse(localStorage.getItem(key));
    localStorage.setItem(key, JSON.stringify(value));
  } catch (e) { /* storage is off: nothing is remembered */ }
  return null;
}

// api talks to taproot. Every request that changes something carries the
// header the server insists on, which another site's page cannot send.
async function api(method, path, body) {
  const options = { method, headers: { 'X-Taproot': 'page' } };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }
  const res = await fetch(path, options);
  if (res.status === 204) return null;
  let data = null;
  try { data = await res.json(); } catch (e) { /* an answer with no body */ }
  if (!res.ok) throw new Error((data && data.error) || `${res.status} ${res.statusText}`);
  return data;
}

// run does something to a controller and says how it went.
async function run(what, work) {
  try {
    const out = await work();
    if (what) toast(what);
    return out;
  } catch (err) {
    toast(err.message, true);
    return undefined;
  } finally {
    soon(150);
  }
}

// ---- reading the state -----------------------------------------------------

let pending = null;
let timer = null;

async function refresh() {
  if (pending) return pending;
  pending = (async () => {
    try {
      state = await api('GET', '/api/state');
      $('problem').hidden = true;
      render();
    } catch (err) {
      $('problem').textContent = `cannot reach taproot: ${err.message}`;
      $('problem').hidden = false;
    } finally {
      pending = null;
      schedule();
    }
  })();
  return pending;
}

// soon asks for a fresh reading shortly, once, however many things ask.
function soon(ms) {
  clearTimeout(timer);
  timer = setTimeout(refresh, ms === undefined ? 250 : ms);
}

// schedule keeps the page fresh without being asked: quickly while a
// controller is being connected, slowly otherwise, since the event stream
// reports changes as they happen.
function schedule() {
  const waiting = state.pairing.some((p) => p.state === 'waiting');
  clearTimeout(timer);
  timer = setTimeout(refresh, waiting ? 2000 : 15000);
}

function listen() {
  const events = new EventSource('/api/events');
  events.onmessage = () => soon(200);
  // a browser reconnects by itself; the poll covers the gap
}

// ---- colour ----------------------------------------------------------------

// hsb is a palette colour as [r, g, b], each 0 to 255.
function hsb(h, s, b) {
  const sat = Math.max(0, Math.min(100, s)) / 100;
  const val = Math.max(0, Math.min(100, b)) / 100;
  const hue = (((h % 360) + 360) % 360) / 60;
  const c = val * sat;
  const x = c * (1 - Math.abs((hue % 2) - 1));
  const m = val - c;
  const [r, g, bl] = hue < 1 ? [c, x, 0] : hue < 2 ? [x, c, 0] : hue < 3 ? [0, c, x] : hue < 4 ? [0, x, c] : hue < 5 ? [x, 0, c] : [c, 0, x];
  return [(r + m) * 255, (g + m) * 255, (bl + m) * 255];
}

// kelvin is a white of that temperature as [r, g, b], near enough for a preview.
function kelvin(k) {
  const t = Math.max(1000, Math.min(12000, k)) / 100;
  const r = t <= 66 ? 255 : 329.7 * Math.pow(t - 60, -0.1332);
  const g = t <= 66 ? 99.47 * Math.log(t) - 161.12 : 288.12 * Math.pow(t - 60, -0.0755);
  const b = t >= 66 ? 255 : t <= 19 ? 0 : 138.52 * Math.log(t - 10) - 305.04;
  return [r, g, b].map((v) => Math.max(0, Math.min(255, v)));
}

const mix = (a, b, t) => [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t];
const css = (c) => `rgb(${Math.round(c[0])},${Math.round(c[1])},${Math.round(c[2])})`;
const clamp01 = (v) => Math.max(0, Math.min(1, v));
const smooth = (v) => { const t = clamp01(v); return t * t * (3 - 2 * t); };
const num = (v, fallback) => (typeof v === 'number' && isFinite(v) ? v : fallback);

// rnd is a number from 0 to 1 that is always the same for the same two
// integers: each panel's own steady stream of dice.
function rnd(a, b) {
  let x = Math.imul(a ^ 0x9e3779b9, 0x85ebca6b) ^ Math.imul((b | 0) + 0x7f4a7c15, 0xc2b2ae35);
  x ^= x >>> 15; x = Math.imul(x, 0x2c1b3c6d); x ^= x >>> 12; x = Math.imul(x, 0x297a2d39); x ^= x >>> 15;
  return (x >>> 0) / 4294967296;
}

function tileOff() {
  const raw = getComputedStyle(document.documentElement).getPropertyValue('--tile-off').trim();
  const m = /^#([0-9a-f]{6})$/i.exec(raw);
  if (!m) return [34, 34, 32];
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

// ---- how a scene moves -----------------------------------------------------

// sampler returns what colour a panel is at a time, for a scene: the six
// built-in plugins as the documentation describes them. It is a likeness, not
// a recording: the controller does not report its panels' colours.
function sampler(scene) {
  const palette = (scene.palette || []).map((c) => hsb(c.hue, c.saturation, c.brightness));
  if (palette.length === 0) return null;
  const n = palette.length;
  const at = (i) => palette[((i % n) + n) % n];
  const o = scene.options || {};
  const trans = Math.max(0.1, num(o.transTime, 20) / 10); // tenths of a second
  const dwell = Math.max(0, num(o.delayTime, 0) / 10);
  const cycle = trans + dwell;
  const direction = o.linDirection || 'right';
  // how far along the motion a panel is: 0 where a colour arrives first
  const along = (p) => (direction === 'left' ? 1 - p.u : direction === 'up' ? 1 - p.v : direction === 'down' ? p.v : p.u);

  let kind = PLUGINS[scene.pluginUuid];
  let dim = 1;
  if (scene.kind === 'rhythm') { kind = 'sound'; dim = 0.6; }

  // one colour handing over to the next across the layout, a panel at a time
  const sweep = (t, place) => {
    const step = Math.floor(t / cycle);
    const f = Math.min(1, (t - step * cycle) / trans);
    return mix(at(step), at(step + 1), smooth(f * 1.6 - place * 0.6));
  };
  // every panel its own colour and its own moment to change
  const scatter = (t, p, period, pick) => {
    const shifted = t / period + rnd(p.id, 7);
    const step = Math.floor(shifted);
    const f = Math.min(1, ((shifted - step) * period) / Math.min(period, trans));
    return mix(pick(p.id, step), pick(p.id, step + 1), smooth(f));
  };

  switch (kind) {
    case 'wheel': {
      // a gradient of the whole palette that never settles, sliding along
      const span = Math.max(1, Math.min(n, num(o.nColorsPerFrame, Math.min(n, 2))));
      return (t, p) => {
        const x = t / trans - along(p) * span;
        const i = Math.floor(x);
        return mix(at(i), at(i + 1), x - i);
      };
    }
    case 'flow':
      return (t, p) => sweep(t, along(p));
    case 'explode':
      return (t, p) => sweep(t, p.r);
    case 'fade':
      return (t) => sweep(t, 0);
    case 'highlight': {
      const main = num(o.mainColorProb, 80) / 100;
      const pick = (id, step) => (n === 1 || rnd(id, step) < main ? palette[0] : at(1 + Math.floor(rnd(id, step + 9173) * (n - 1))));
      return (t, p) => scatter(t, p, cycle, pick);
    }
    case 'sound': {
      // these move to music, which the page cannot hear: the palette, idling
      const pick = (id, step) => mix([0, 0, 0], at(Math.floor(rnd(id, step) * n)), (0.35 + 0.65 * rnd(id, step + 31)) * dim);
      return (t, p) => scatter(t, p, 1.4, pick);
    }
    case 'random':
    default: {
      const pick = (id, step) => mix([0, 0, 0], at(Math.floor(rnd(id, step) * n)), 0.55 + 0.45 * rnd(id, step + 31));
      return (t, p) => scatter(t, p, cycle, pick);
    }
  }
}

// lighting is what colours a controller's panels are, as a function of time
// and panel, or null for panels that are dark.
function lighting(ctl) {
  if (!ctl.reachable || !ctl.on) return null;
  if (ctl.colorMode === 'ct') { const white = kelvin(ctl.ct); return () => white; }
  if (ctl.colorMode === 'hs') { const one = hsb(ctl.hue, ctl.sat, 100); return () => one; }
  const scene = ctl.scenes.find((s) => s.name === ctl.running);
  if (scene) return sampler(scene);
  // *Static*, *Dynamic*: something is showing, and the controller does not say what
  const grey = [150, 150, 145];
  return () => grey;
}

// ---- where the panels are --------------------------------------------------

// corners is a panel's outline around its centre, in the layout's own terms:
// y upwards, turned anticlockwise by its orientation.
function corners(shape, side, o) {
  const s = side || SIDES[shape] || 100;
  let count = 12;
  let radius = s * 0.4;
  let first = 0;
  if (shape === 0 || shape === 8 || shape === 9) { count = 3; radius = s / Math.sqrt(3); first = 90; }
  else if (shape === 2 || shape === 3 || shape === 4 || shape === 30 || shape === 31 || shape === 32) { count = 4; radius = s / Math.sqrt(2); first = 45; }
  else if (shape === 7 || shape === 14 || shape === 15) { count = 6; radius = s; first = 0; }
  const out = [];
  for (let i = 0; i < count; i++) {
    const a = ((first + o + (360 / count) * i) * Math.PI) / 180;
    out.push([radius * Math.cos(a), radius * Math.sin(a)]);
  }
  return out;
}

// place works out where to draw each panel: the layout turned the way the
// controller says its owner turned it, then the way this browser was asked
// to, and scaled into the picture. It also says, for each panel, how far
// across, up and out from the middle it is, which is what a scene's
// direction is measured against.
function place(ctl, view) {
  const layout = ctl.layout;
  if (!layout || !layout.positionData) return null;
  const lit = layout.positionData.filter((p) => !UNLIT.has(p.shapeType));
  if (lit.length === 0) return null;

  const turn = ((ctl.orientation + view.turn) * Math.PI) / 180;
  const cos = Math.cos(turn);
  const sin = Math.sin(turn);
  const move = ([x, y]) => {
    const tx = x * cos - y * sin;
    const ty = x * sin + y * cos;
    return [view.flip ? -tx : tx, ty];
  };

  const panels = lit.map((p) => {
    const side = layout.sideLength || SIDES[p.shapeType];
    const centre = move([p.x, p.y]);
    // each corner pulled in a little, so neighbours show a seam between them
    const points = corners(p.shapeType, side, p.o).map(([dx, dy]) => move([p.x + dx * 0.95, p.y + dy * 0.95]));
    return { id: p.panelId, centre, points };
  });

  const xs = panels.flatMap((p) => p.points.map((q) => q[0]));
  const ys = panels.flatMap((p) => p.points.map((q) => q[1]));
  const box = { minX: Math.min(...xs), maxX: Math.max(...xs), minY: Math.min(...ys), maxY: Math.max(...ys) };
  const cx = panels.reduce((sum, p) => sum + p.centre[0], 0) / panels.length;
  const cy = panels.reduce((sum, p) => sum + p.centre[1], 0) / panels.length;
  const cxs = panels.map((p) => p.centre[0]);
  const cys = panels.map((p) => p.centre[1]);
  const spanX = Math.max(...cxs) - Math.min(...cxs) || 1;
  const spanY = Math.max(...cys) - Math.min(...cys) || 1;
  const far = Math.max(...panels.map((p) => Math.hypot(p.centre[0] - cx, p.centre[1] - cy))) || 1;

  for (const p of panels) {
    p.u = (p.centre[0] - Math.min(...cxs)) / spanX; // 0 at the left, 1 at the right
    p.v = (Math.max(...cys) - p.centre[1]) / spanY; // 0 at the top, 1 at the bottom
    p.r = Math.hypot(p.centre[0] - cx, p.centre[1] - cy) / far;
    // the picture's y runs downwards
    p.path = p.points.map(([x, y]) => `${(x - box.minX).toFixed(1)},${(box.maxY - y).toFixed(1)}`).join(' ');
  }
  const pad = 26;
  return { panels, viewBox: `${-pad} ${-pad} ${box.maxX - box.minX + pad * 2} ${box.maxY - box.minY + pad * 2}` };
}

// ---- a controller's card ---------------------------------------------------

function makeCard(name) {
  // how this browser last had the picture: turned, mirrored, zoomed and moved
  const card = { name, view: Object.assign({ turn: 0, flip: false, zoom: 1, pan: { x: 0, y: 0 } }, store(`taproot-view-${name}`) || {}), tiles: [], signature: {}, dragging: false, sendTimer: null };

  card.title = el('h2', { text: name });
  card.meta = el('div', { class: 'meta' });
  card.glow = svg('g', { class: 'glow' });
  card.tilesGroup = svg('g', { class: 'tiles' });
  card.svg = svg('svg', { role: 'img', preserveAspectRatio: 'xMidYMid meet' });
  card.svg.append(card.glow, card.tilesGroup);
  card.running = el('b');
  card.plugin = el('span');
  card.power = el('input', { type: 'checkbox', 'aria-label': 'power' });
  card.powerLabel = el('span', { text: 'off' });
  card.slider = el('input', { type: 'range', min: 0, max: 100, step: 1, 'aria-label': 'brightness' });
  card.level = el('output');
  card.scenes = el('div', { class: 'scenes' });
  card.note = el('div', { class: 'away-note' });

  const act = (label, title, work) => el('button', { class: 'plain', type: 'button', text: label, title, onclick: work });
  card.root = el('section', { class: 'card', dataset: { name } },
    // the head is the handle: drag a card by it to put the controllers in the order you want
    el('div', { class: 'head', draggable: 'true', title: 'drag to reorder', // the string: draggable="" means not draggable
      ondragstart: (e) => { dragging = card; card.root.classList.add('dragging'); e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', name); },
      ondragend: () => { dragging = null; card.root.classList.remove('dragging'); saveOrder(); } },
      el('div', {}, card.title, card.meta),
      el('div', { class: 'actions' },
        act('flash', 'flash the panels, to tell which controller this is', () => run(`${name} is flashing`, () => api('POST', `/api/controllers/${encodeURIComponent(name)}/identify`))),
        act('back up', 'save every scene of this controller to a dated folder', () => backup(name)),
        act('forget', 'delete its token and remove it from taproot', () => forget(name)))),
    el('div', { class: 'stage' },
      card.svg,
      el('div', { class: 'view' },
        el('button', { type: 'button', text: '↻', title: 'turn the picture, if it does not match your wall', onclick: () => turnView(card, 30) }),
        el('button', { type: 'button', text: '⇋', title: 'mirror the picture', onclick: () => flipView(card) }),
        el('button', { type: 'button', text: '+', title: 'zoom in (drag the picture to move it)', onclick: () => zoomView(card, 1.25) }),
        el('button', { type: 'button', text: '−', title: 'zoom out', onclick: () => zoomView(card, 1 / 1.25) }),
        el('button', { type: 'button', text: '⟲', title: 'fit the picture again', onclick: () => resetView(card) })),
      el('div', { class: 'caption' }, card.running, card.plugin)),
    el('div', { class: 'controls' },
      el('label', { class: 'switch' }, card.power, card.powerLabel),
      card.slider, card.level),
    card.scenes,
    card.note);

  panView(card);
  card.power.addEventListener('change', () => {
    const on = card.power.checked;
    card.ctl.on = on;
    paint(card);
    run(null, () => api('PUT', `/api/controllers/${encodeURIComponent(name)}/power`, { on }));
  });
  // dragging dims the picture at once and tells the controller a few times a second
  card.slider.addEventListener('input', () => {
    card.dragging = true;
    card.ctl.brightness = Number(card.slider.value);
    card.level.textContent = `${card.slider.value}%`;
    if (card.sendTimer) return;
    card.sendTimer = setTimeout(() => { card.sendTimer = null; sendBrightness(card); }, 200);
  });
  card.slider.addEventListener('change', () => {
    clearTimeout(card.sendTimer);
    card.sendTimer = null;
    card.dragging = false;
    sendBrightness(card).then(() => soon(300));
  });

  return card;
}

function sendBrightness(card) {
  return api('PUT', `/api/controllers/${encodeURIComponent(card.name)}/brightness`, { value: Number(card.slider.value) })
    .catch((err) => toast(err.message, true));
}

// applyView shows the part of the picture the zoom and the pan pick out of the whole of it
function applyView(card) {
  if (!card.base) return;
  const [x, y, w, h] = card.base;
  const z = card.view.zoom || 1;
  const vw = w / z, vh = h / z;
  card.svg.setAttribute('viewBox', `${x + (w - vw) / 2 - card.view.pan.x} ${y + (h - vh) / 2 - card.view.pan.y} ${vw} ${vh}`);
}

function zoomView(card, by) {
  card.view.zoom = Math.min(8, Math.max(0.25, (card.view.zoom || 1) * by));
  store(`taproot-view-${card.name}`, card.view);
  applyView(card);
}

function resetView(card) {
  card.view.zoom = 1;
  card.view.pan = { x: 0, y: 0 };
  store(`taproot-view-${card.name}`, card.view);
  applyView(card);
}

// panView lets the picture be dragged about: a pointer's movement in pixels becomes movement in the
// picture's own units, so it follows the pointer whatever the zoom
function panView(card) {
  let from = null;
  card.svg.addEventListener('pointerdown', (e) => {
    if (e.button !== 0) return;
    from = { x: e.clientX, y: e.clientY, pan: { ...card.view.pan } };
    card.svg.setPointerCapture(e.pointerId);
    card.svg.classList.add('panning');
  });
  card.svg.addEventListener('pointermove', (e) => {
    if (!from || !card.base) return;
    const box = card.svg.getBoundingClientRect();
    const [, , w, h] = card.base;
    const z = card.view.zoom || 1;
    // the picture is drawn to fit: the scale is whichever axis is the tighter fit
    const scale = Math.max(w / z / box.width, h / z / box.height);
    card.view.pan = { x: from.pan.x + (e.clientX - from.x) * scale, y: from.pan.y + (e.clientY - from.y) * scale };
    applyView(card);
  });
  const done = () => {
    if (!from) return;
    from = null;
    card.svg.classList.remove('panning');
    store(`taproot-view-${card.name}`, card.view);
  };
  card.svg.addEventListener('pointerup', done);
  card.svg.addEventListener('pointercancel', done);
}

function turnView(card, by) {
  card.view.turn = (card.view.turn + by) % 360;
  store(`taproot-view-${card.name}`, card.view);
  card.signature.layout = null;
  update(card, card.ctl);
}

function flipView(card) {
  card.view.flip = !card.view.flip;
  store(`taproot-view-${card.name}`, card.view);
  card.signature.layout = null;
  update(card, card.ctl);
}

// update brings a card in line with a reading of its controller, touching
// only what changed, so a slider being dragged or a list being scrolled is
// left where it is.
function update(card, ctl) {
  card.ctl = ctl;
  card.root.classList.toggle('away', !ctl.reachable);
  // each part on one line, whatever the width; an update the controller says is waiting is said beside the firmware
  const parts = [ctl.device, ctl.host, ctl.model].filter(Boolean).map((text) => el('span', { class: 'part', text }));
  if (ctl.firmware) {
    // the firmware, and an update with its trigger, stay together on one line
    const firmware = el('span', { class: 'part' }, `firmware ${ctl.firmware}`);
    if (ctl.firmwareUpdate) {
      firmware.append(' · ', el('span', { class: 'update', text: `update to ${ctl.firmwareUpdate} available` }), ' · ',
        el('button', { class: 'plain trigger', type: 'button', text: 'trigger', title: 'have the controller fetch and install it, after a backup', onclick: () => triggerFirmware(ctl.name, ctl.firmware, ctl.firmwareUpdate) }));
    }
    parts.push(firmware);
  }
  card.meta.replaceChildren(...parts.flatMap((p, i) => (i ? [' · ', p] : [p])));
  const upgrading = upgradeNote(card, ctl);
  card.note.textContent = upgrading || (ctl.reachable ? '' : `cannot be reached: ${ctl.error || 'no answer'}`);
  if (!ctl.reachable) { card.light = null; return; }

  // zoom and pan only move the window on the picture: they do not redraw it
  const layoutSig = JSON.stringify([ctl.layout, ctl.orientation, card.view.turn, card.view.flip]);
  if (card.signature.layout !== layoutSig) {
    card.signature.layout = layoutSig;
    card.glow.replaceChildren();
    card.tilesGroup.replaceChildren();
    card.tiles = [];
    const placed = place(ctl, card.view);
    if (placed) {
      card.base = placed.viewBox.split(' ').map(Number);
      applyView(card);
      for (const p of placed.panels) {
        const halo = svg('polygon', { points: p.path });
        const tile = svg('polygon', { points: p.path });
        card.glow.append(halo);
        card.tilesGroup.append(tile);
        card.tiles.push({ panel: p, halo, tile });
      }
    }
  }

  card.power.checked = ctl.on;
  card.powerLabel.textContent = ctl.on ? 'on' : 'off';
  if (!card.dragging) {
    card.slider.value = ctl.brightness;
    card.level.textContent = `${ctl.brightness}%`;
  }

  const scene = ctl.scenes.find((s) => s.name === ctl.running);
  card.running.textContent = ctl.on ? ctl.running : 'off';
  card.plugin.textContent = !ctl.on ? '' : scene ? `· ${scene.plugin || 'unknown plugin'}${scene.kind === 'rhythm' ? ' · moves to sound' : ''}` : '· not a saved scene';

  const scenesSig = JSON.stringify([ctl.scenes, ctl.running, state.controllers.length]);
  if (card.signature.scenes !== scenesSig) {
    card.signature.scenes = scenesSig;
    card.scenes.replaceChildren(...ctl.scenes.map((s) => sceneRow(card, ctl, s)));
  }
  paint(card);
}

function sceneRow(card, ctl, scene) {
  const swatch = el('span', { class: 'swatch' });
  for (const c of (scene.palette || []).slice(0, 12)) {
    const chip = el('i');
    chip.style.background = css(hsb(c.hue, c.saturation, c.brightness));
    swatch.append(chip);
  }
  const kind = [scene.plugin, scene.kind === 'rhythm' ? '♪' : ''].filter(Boolean).join(' ');
  return el('div', { class: scene.name === ctl.running ? 'scene running' : 'scene' },
    el('button', { class: 'pick', type: 'button', title: `start ${scene.name}`, onclick: () => select(card, scene.name) },
      swatch, el('span', { class: 'name', text: scene.name }), el('span', { class: 'kind', text: kind })),
    sceneMenu(ctl, scene));
}

// sceneMenu is the … at the end of a scene's row: what can be done with the scene, one click away
function sceneMenu(ctl, scene) {
  const running = scene.name === ctl.running;
  const item = (text, title, onclick, disabled) => el('button', { class: 'item', type: 'button', text, title, disabled: disabled || null, onclick: () => { closeMenus(); onclick(); } });
  const menu = el('div', { class: 'menu', hidden: true },
    item('copy…', state.controllers.length > 1 ? 'copy this scene to another controller' : 'there is no other controller to copy it to', () => openCopy(ctl.name, scene.name), state.controllers.length < 2),
    item('delete…', running ? 'the scene that is running cannot be deleted: start another first' : 'take this scene off the controller (it is backed up first)', () => deleteScene(ctl.name, scene.name), running),
    item('edit…', 'open a copy of this scene in the editor', () => openFromController(ctl.name, scene.name)));
  const more = el('button', { class: 'plain more', type: 'button', text: '…', title: 'copy, delete, edit', 'aria-haspopup': 'menu',
    onclick: (e) => { e.stopPropagation(); const open = menu.hidden; closeMenus(); menu.hidden = !open; } });
  return el('span', { class: 'actions' }, more, menu);
}

// ---- reordering the cards ----------------------------------------------------

let dragging = null; // the card being dragged, by its head

// as a card is dragged over another, it moves before or after that one, by which half the pointer is in
function dragOver(e) {
  if (!dragging) return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  const over = e.target.closest('.card');
  if (!over || over === dragging.root) return;
  const box = over.getBoundingClientRect();
  const columns = getComputedStyle($('controllers')).gridTemplateColumns.split(' ').length;
  const after = columns > 1 ? e.clientX > box.left + box.width / 2 : e.clientY > box.top + box.height / 2;
  over.parentNode.insertBefore(dragging.root, after ? over.nextSibling : over);
}

function saveOrder() {
  store('taproot-order', [...$('controllers').children].map((c) => c.dataset.name));
}

function closeMenus() {
  for (const m of document.querySelectorAll('.scene .menu')) m.hidden = true;
}
document.addEventListener('click', closeMenus);
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeMenus(); });

function triggerFirmware(name, from, to) {
  confirmThen(`update ${name} from ${from} to ${to}?`,
    'The controller fetches the firmware from Nanoleaf\'s cloud and installs it itself, and is off the network while it does. Every scene is backed up first; firmware can change how scenes are stored, so keep that backup.',
    'update it',
    async () => {
      const out = await run(null, () => api('POST', `/api/controllers/${encodeURIComponent(name)}/firmware`));
      if (!out) return;
      if (state.dryRun) { toast(`dry run: ${name} would be backed up to ${out.backup} and told to update`); return; }
      toast(`${name}: backed up to ${out.backup}, and told to fetch and install ${to}; watching`);
      const card = cards.get(name);
      const ctl = card && card.ctl;
      // the card follows it through: quiet while it installs, then what came back against what was there
      if (card) card.updating = { since: Date.now(), from, to, quietSince: null, scenes: ctl ? ctl.scenes.map((s) => s.name) : [], running: ctl ? ctl.running : '' };
      soon(0);
    });
}

// upgradeNote is what a card says while its controller is being upgraded, and once when it is back
function upgradeNote(card, ctl) {
  const u = card.updating;
  if (!u) return null;
  const secs = (ms) => `${Math.round(ms / 1000)}s`;
  if (!ctl.reachable) {
    u.quietSince = u.quietSince || Date.now();
    return `updating to ${u.to}: gone quiet, installing… (${secs(Date.now() - u.quietSince)})`;
  }
  if (!u.quietSince) {
    if (Date.now() - u.since > 90000) { card.updating = null; toast(`${ctl.name}: still on ${ctl.firmware} after 90s: it did not start an upgrade`, true); return null; }
    return `updating to ${u.to}: asked ${secs(Date.now() - u.since)} ago, still answering on ${ctl.firmware}`;
  }
  // back
  card.updating = null;
  const lost = u.scenes.filter((n) => !ctl.scenes.some((s) => s.name === n));
  const took = secs(Date.now() - u.since);
  if (ctl.firmware === u.from) toast(`${ctl.name}: back after ${took}, still on ${ctl.firmware}`, true);
  else toast(`${ctl.name}: back on ${ctl.firmware} after ${took}; ${ctl.scenes.length} scenes (had ${u.scenes.length})${lost.length ? `; gone: ${lost.join(', ')} — in the backup taken first` : ''}`, lost.length > 0);
  return null;
}

function deleteScene(name, scene) {
  confirmThen(`delete ${scene} from ${name}?`,
    'A controller has no bin and no undo. The whole controller is backed up first, and taproot restore can put the scene back from that backup.',
    'delete',
    async () => {
      const report = await run(null, () => api('DELETE', `/api/controllers/${encodeURIComponent(name)}/scenes/${encodeURIComponent(scene)}`));
      if (!report) return;
      const res = report.results && report.results[0];
      if (res && res.outcome === 'deleted') toast(state.dryRun ? `dry run: ${scene} would be deleted from ${name}` : `${name}: deleted ${scene}; backed up first to ${report.backup}`);
      else if (res) toast(`${name}: ${scene} ${res.outcome}: ${res.reason || ''}`);
      refresh();
    });
}

function select(card, name) {
  // show it at once; the next reading confirms or corrects it
  card.ctl.running = name;
  card.ctl.colorMode = 'effect';
  card.signature.scenes = null;
  update(card, card.ctl);
  run(null, () => api('PUT', `/api/controllers/${encodeURIComponent(card.name)}/scene`, { name }));
}

// paint works out afresh how the card's panels are lit, and draws them once
// straight away: a tab in the background gets no animation frames, and must
// not sit there unpainted until it is looked at.
function paint(card) {
  card.light = lighting(card.ctl);
  draw(card, performance.now() / 1000, tileOff());
}

// ---- the picture, moving ---------------------------------------------------

// draw colours a card's panels as they are at a time.
function draw(card, t, off) {
  if (!card.ctl || !card.ctl.reachable) return;
  // brightness dims every panel alike; a panel at nothing is still drawn, as glass
  const level = card.light ? 0.12 + 0.88 * (card.ctl.brightness / 100) : 0;
  for (const { panel, halo, tile } of card.tiles) {
    const colour = card.light ? css(mix(off, card.light(t, panel), level)) : css(off);
    if (tile.dataset.fill === colour) continue;
    tile.dataset.fill = colour;
    tile.setAttribute('fill', colour);
    halo.setAttribute('fill', card.light ? colour : 'none');
  }
}

let lastFrame = 0;

function frame(now) {
  requestAnimationFrame(frame);
  if (document.hidden || now - lastFrame < 33) return; // thirty a second is plenty
  lastFrame = now;
  const off = tileOff();
  for (const card of cards.values()) draw(card, now / 1000, off);
  editorFrame(now);
}

// ---- the whole page --------------------------------------------------------

function render() {
  const names = new Set(state.controllers.map((c) => c.name));
  for (const [name, card] of cards) {
    if (!names.has(name)) { card.root.remove(); cards.delete(name); }
  }
  // in the order this browser last dragged them into; one it has not seen goes after those, by name
  const order = store('taproot-order') || [];
  const rank = (name) => { const i = order.indexOf(name); return i < 0 ? order.length : i; };
  const controllers = [...state.controllers].sort((a, b) => rank(a.name) - rank(b.name) || a.name.localeCompare(b.name));
  for (const ctl of controllers) {
    let card = cards.get(ctl.name);
    if (!card) {
      card = makeCard(ctl.name);
      cards.set(ctl.name, card);
    }
    $('controllers').append(card.root);
    update(card, ctl);
  }

  const away = state.controllers.filter((c) => !c.reachable).length;
  $('count').textContent = state.controllers.length === 0 ? '' : `· ${state.controllers.length} controller${state.controllers.length === 1 ? '' : 's'}${away ? `, ${away} unreachable` : ''}`;
  $('empty').hidden = state.controllers.length > 0 || state.pairing.length > 0 || $('view').value !== 'controllers';
  viewCounts();
  if (editor.effect) { renderPluginsFrom(); renderLayoutPick(); }
  $('backup-all').hidden = state.controllers.length === 0;
  $('dryrun').hidden = !state.dryRun;
  renderPairing();
  if (copying && $('copybox').open) renderTargets();
}

// renderPairing shows each controller being connected: what to do, how long
// is left, and how it ended.
function renderPairing() {
  $('pairing').replaceChildren(...state.pairing.map((p) => {
    if (p.state === 'waiting') {
      const left = Math.max(0, Math.round((new Date(p.until) - Date.now()) / 1000));
      return el('div', { class: 'strip warn' },
        el('span', { class: 'pulse' }),
        el('span', { class: 'grow' }, 'connecting to ', el('b', { text: p.address }),
          ` — hold its power button for 5 to 7 seconds, until the light flashes. asking for another ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`),
        el('button', { class: 'plain', type: 'button', text: 'stop', onclick: () => run(null, () => api('DELETE', `/api/connect?address=${encodeURIComponent(p.address)}`)) }));
    }
    if (p.state === 'connected') {
      return el('div', { class: 'strip good' }, el('span', { class: 'grow' }, 'connected ', el('b', { text: p.name }), ` (${p.address}): ${p.message}`));
    }
    return el('div', { class: 'strip bad' }, el('span', { class: 'grow' }, `${p.address}: ${p.message}`),
      el('button', { class: 'plain', type: 'button', text: 'dismiss', onclick: () => run(null, () => api('DELETE', `/api/connect?address=${encodeURIComponent(p.address)}`)) }));
  }));
}

// ---- things the buttons do -------------------------------------------------

async function backup(name) {
  const out = await run(null, () => api('POST', `/api/controllers/${encodeURIComponent(name)}/backup`));
  if (out) toast(state.dryRun ? `dry run: ${name} would be backed up to ${out.dir}` : `${name}: ${out.scenes} scenes backed up to ${out.dir}`);
}

async function backupAll() {
  for (const ctl of state.controllers.filter((c) => c.reachable)) await backup(ctl.name);
}

function confirmThen(title, text, label, work) {
  $('confirm-title').textContent = title;
  $('confirm-text').textContent = text;
  $('confirm-go').textContent = label;
  $('confirm-go').onclick = () => { $('confirmbox').close(); work(); };
  $('confirmbox').showModal();
}

function forget(name) {
  const ctl = state.controllers.find((c) => c.name === name);
  const away = ctl && !ctl.reachable;
  confirmThen(`forget ${name}?`,
    away
      ? 'It cannot be reached, so its token is only removed from taproot and stays valid on the controller until that is reset. Its scenes and backups are not touched.'
      : 'The controller deletes the token and taproot removes it. Its scenes and backups are not touched. Connecting again needs the power button.',
    'forget',
    () => run(`forgot ${name}`, () => api('DELETE', `/api/controllers/${encodeURIComponent(name)}${away ? '?local=1' : ''}`)));
}

// ---- connecting ------------------------------------------------------------

function openConnect() {
  $('found').replaceChildren();
  $('address').value = '';
  $('newname').value = '';
  $('connectbox').showModal();
  $('address').focus();
}

async function find() {
  $('find').disabled = true;
  $('find').textContent = 'searching…';
  $('found').replaceChildren();
  try {
    const scan = $('scan').value.trim();
    const found = await api('GET', `/api/find${scan ? `?scan=${encodeURIComponent(scan)}` : ''}`);
    if (found.length === 0) {
      $('found').append(el('div', { class: 'hit' }, el('span', { class: 'where', text: 'none answered. Controllers announce themselves, but often not from one network to another: type the address below, or give a subnet to knock on.' })));
    }
    for (const c of found) {
      $('found').append(el('div', { class: 'hit' },
        el('span', { text: c.name || 'a controller' }),
        el('span', { class: 'where grow', text: [c.address, c.model, c.firmware && `firmware ${c.firmware}`].filter(Boolean).join(' · ') }),
        c.connected
          ? el('span', { class: 'where', text: `connected as ${c.connected}` })
          : c.address
            ? el('button', { class: 'plain', type: 'button', text: 'connect', onclick: () => connect(c.address, '') })
            : el('span', { class: 'where', text: 'its address did not come back: search again' })));
    }
  } catch (err) {
    toast(err.message, true);
  } finally {
    $('find').disabled = false;
    $('find').textContent = 'search the network';
  }
}

async function connect(address, name) {
  const started = await run(null, () => api('POST', '/api/connect', { address, name }));
  if (started) $('connectbox').close();
}

// ---- copying ---------------------------------------------------------------

function openCopy(from, scene) {
  copying = { from, scene };
  $('copy-what').textContent = `${scene}, from ${from}`;
  $('copy-result').replaceChildren();
  $('copy-force').checked = false;
  $('copy-select').checked = false;
  renderTargets(true);
  $('copybox').showModal();
}

// renderTargets lists the controllers a scene can go to and whether each has
// a scene of that name already. Whether that one is the same scene is the
// server's to say, when it is asked to copy.
function renderTargets(fresh) {
  const ticked = new Set([...document.querySelectorAll('#copy-targets input:checked')].map((i) => i.value));
  const others = state.controllers.filter((c) => c.name !== copying.from);
  $('copy-targets').replaceChildren(...others.map((c) => {
    const has = c.scenes.some((s) => s.name === copying.scene);
    const box = el('input', { type: 'checkbox', value: c.name, disabled: !c.reachable });
    // to begin with, the ones that lack it are the ones it is for
    box.checked = c.reachable && (fresh ? !has : ticked.has(c.name));
    return el('label', { class: c.reachable ? 'target' : 'target off' }, box,
      el('span', { class: 'grow', text: c.name }),
      el('span', { class: 'has', text: !c.reachable ? 'cannot be reached' : has ? 'has a scene of this name' : 'does not have it' }));
  }));
}

async function copy() {
  const to = [...document.querySelectorAll('#copy-targets input:checked')].map((i) => i.value);
  if (to.length === 0) { toast('tick at least one controller to copy to', true); return; }
  $('copy-go').disabled = true;
  $('copy-result').replaceChildren(el('div', { class: 'line same' }, el('span', { class: 'why', text: 'backing up and copying…' })));
  try {
    const reports = await api('POST', '/api/copy', { scene: copying.scene, from: copying.from, to, force: $('copy-force').checked, select: $('copy-select').checked });
    $('copy-result').replaceChildren(...reports.flatMap((r) => {
      if (r.error) return [el('div', { class: 'line bad' }, el('span', { text: r.controller }), el('span', { class: 'what', text: 'failed' }), el('span', { class: 'why', text: r.error }))];
      return (r.results || []).map((res) => {
        const good = res.outcome === 'added' || res.outcome === 'replaced';
        const cls = good ? 'line good' : res.outcome === 'unchanged' ? 'line same' : 'line bad';
        const why = res.reason
          || (res.readsBack && res.readsBack.length ? `the controller changed it as it stored it: it reads back with a different ${res.readsBack.join(', ')}` : '')
          || (res.outcome === 'unchanged' ? 'the same scene was already there' : '')
          || (r.backup ? `backed up first, to ${r.backup}` : '');
        return el('div', { class: cls }, el('span', { text: r.controller }),
          el('span', { class: 'what', text: r.dryRun && good ? `would be ${res.outcome}` : res.outcome }), el('span', { class: 'why', text: why }));
      });
    }));
  } catch (err) {
    $('copy-result').replaceChildren(el('div', { class: 'line bad' }, el('span', { class: 'what', text: 'failed' }), el('span', { class: 'why', text: err.message })));
  } finally {
    $('copy-go').disabled = false;
    soon(100);
  }
}

// ---- start -----------------------------------------------------------------

document.addEventListener('DOMContentLoaded', () => {
  wireEditor();
  $('theme').addEventListener('click', () => {
    const root = document.documentElement;
    root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem('taproot-theme', root.dataset.theme); } catch (e) { /* not remembered */ }
    for (const card of cards.values()) for (const t of card.tiles) t.tile.dataset.fill = ''; // repaint against the new wall
  });
  $('controllers').addEventListener('dragover', dragOver);
  $('controllers').addEventListener('drop', (e) => e.preventDefault());
  // how big each card is: three sizes, cycled by the button, remembered by this browser
  const sizes = ['small', 'medium', 'large'];
  const setSize = (size) => { $('controllers').dataset.size = size; $('size').textContent = `cards: ${size}`; };
  setSize(sizes.includes(store('taproot-size')) ? store('taproot-size') : 'medium');
  $('size').addEventListener('click', () => {
    const next = sizes[(sizes.indexOf($('controllers').dataset.size) + 1) % sizes.length];
    setSize(next);
    store('taproot-size', next);
  });
  $('add').addEventListener('click', openConnect);
  $('add-first').addEventListener('click', openConnect);
  $('backup-all').addEventListener('click', backupAll);
  $('find').addEventListener('click', find);
  $('scan').addEventListener('keydown', (e) => { if (e.key === 'Enter') { e.preventDefault(); find(); } });
  $('connectform').addEventListener('submit', (e) => { e.preventDefault(); connect($('address').value.trim(), $('newname').value.trim()); });
  $('copy-go').addEventListener('click', copy);
  for (const button of document.querySelectorAll('[data-close]')) button.addEventListener('click', () => button.closest('dialog').close());
  document.addEventListener('visibilitychange', () => { if (!document.hidden) soon(0); });

  refresh();
  listen();
  requestAnimationFrame(frame);
  // the countdown on a connect in progress ticks without asking the server
  setInterval(() => { if (state.pairing.some((p) => p.state === 'waiting')) renderPairing(); }, 1000);
});

// ---- the scene library and the editor ----------------------------------------
//
// A scene is edited as the document the controller holds it as: only the
// fields the form knows are touched, and whatever else the firmware put in
// it goes back as it came. The preview is the same likeness the cards show.

let library = []; // what the library holds, as the page lists it
const plugins = new Map(); // controller name -> the motions it has
const editor = {
  effect: null, // the scene being edited, as its JSON object
  from: null, // { kind: 'library' | 'controller' | 'new', name, controller }
  dirty: false,
  colour: 0, // which palette entry is being edited
  stage: null, // a card-like thing the preview is drawn on
  time: 0, // the preview's own clock, which the speed control runs
  speed: 1,
  paused: false,
  layout: 'wall', // a controller's name, or 'wall' for the built one
};

const SIDE = 150; // a Light Panels triangle's side
const BUILD = { rows: 7, cols: 14 };

// the six motions every Light Panels controller has, for when no controller is there to ask
const BUILTIN = [
  { uuid: '6970681a-20b5-4c5e-8813-bdaebc4ee4fa', name: 'Wheel', type: 'color', description: 'the palette slides across the panels as a gradient', config: ['transTime', 'delayTime', 'linDirection', 'nColorsPerFrame', 'loop'] },
  { uuid: '027842e4-e1d6-4a4c-a731-be74a1ebd4cf', name: 'Flow', type: 'color', description: 'each colour sweeps across, one after another', config: ['transTime', 'delayTime', 'linDirection', 'loop'] },
  { uuid: '713518c1-d560-47db-8991-de780af71d1e', name: 'Burst', type: 'color', description: 'each colour bursts out from the middle', config: ['transTime', 'delayTime', 'loop'] },
  { uuid: 'b3fd723a-aae8-4c99-bf2b-087159e0ef53', name: 'Fade', type: 'color', description: 'every panel fades from one colour to the next together', config: ['transTime', 'delayTime', 'loop'] },
  { uuid: 'ba632d3e-9c2b-4413-a965-510c839b3f71', name: 'Random', type: 'color', description: 'each panel picks its own colour, in its own time', config: ['transTime', 'delayTime', 'loop'] },
  { uuid: '70b7c636-6bf8-491f-89c1-f4103508d642', name: 'Highlight', type: 'color', description: 'mostly the first colour, with the others flashing through', config: ['transTime', 'delayTime', 'mainColorProb', 'loop'] },
];
const OPTION_DEFAULTS = {
  transTime: { type: 'int', defaultValue: 20, minValue: 1, maxValue: 600, hint: 'tenths of a second a change takes' },
  delayTime: { type: 'int', defaultValue: 10, minValue: 0, maxValue: 600, hint: 'tenths of a second a colour stays' },
  linDirection: { type: 'string', defaultValue: 'right', strings: ['left', 'right', 'up', 'down'], hint: 'which way it moves' },
  nColorsPerFrame: { type: 'int', defaultValue: 2, minValue: 1, maxValue: 12, hint: 'how many colours are showing at once' },
  mainColorProb: { type: 'int', defaultValue: 80, minValue: 0, maxValue: 100, hint: 'how often the first colour shows, in percent' },
  loop: { type: 'bool', defaultValue: true, hint: 'start again at the end' },
};

// motionName is a built-in motion's name from its id, for a scene no controller has named the motion of
function motionName(uuid) {
  const p = BUILTIN.find((b) => b.uuid === uuid);
  return p ? p.name : '';
}

function pluginList(controller) {
  const list = controller && plugins.get(controller);
  if (list && list.length) return list.map((p) => ({ uuid: p.uuid, name: p.name, type: p.type, description: p.description, config: p.pluginConfig || [] }));
  return BUILTIN.map((p) => ({ ...p, config: p.config.map((name) => ({ name, ...OPTION_DEFAULTS[name] })) }));
}

async function loadPlugins(controller) {
  if (!controller || plugins.has(controller)) return;
  try {
    plugins.set(controller, await api('GET', `/api/controllers/${encodeURIComponent(controller)}/plugins`));
  } catch (e) { plugins.set(controller, []); }
}

// ---- the view: controllers, or scenes

function showView(which) {
  $('view').value = which;
  $('controllers').hidden = which !== 'controllers';
  $('empty').hidden = which !== 'controllers' || state.controllers.length > 0;
  $('library').hidden = which !== 'scenes';
  store('taproot-view', which);
  if (which === 'scenes') loadLibrary();
}

function viewCounts() {
  const [c, s] = $('view').options;
  c.textContent = `controllers (${state.controllers.length})`;
  s.textContent = `scenes (${library.length})`;
}

// ---- the library

async function loadLibrary() {
  try {
    library = await api('GET', '/api/scenes');
  } catch (e) { toast(e.message, true); return; }
  viewCounts();
  $('lib-where').textContent = library.length ? `${library.length} kept` : 'nothing kept yet';
  $('lib-list').replaceChildren(...library.map((s) => {
    const swatch = el('span', { class: 'swatch' });
    for (const c of (s.palette || []).slice(0, 12)) swatch.append(el('i', { style: `background:${css(hsb(c.hue, c.saturation, c.brightness))}` }));
    const current = editor.from && editor.from.kind === 'library' && editor.from.name === s.name;
    return el('div', { class: current ? 'scene current' : 'scene' },
      el('button', { class: 'pick', type: 'button', title: `edit ${s.name}`, onclick: () => openFromLibrary(s.name) },
        swatch, el('span', { class: 'name', text: s.name }), el('span', { class: 'kind', text: [s.builtIn ? 'built in' : '', s.plugin || motionName(s.pluginUuid)].filter(Boolean).join(' · ') })));
  }));
  if (library.length === 0) $('lib-list').append(el('p', { class: 'note', text: 'import a scene off a controller, or make a new one' }));
}

async function openFromLibrary(name) {
  if (!(await leaveEditor())) return;
  try {
    const effect = await api('GET', `/api/scenes/${encodeURIComponent(name)}`);
    const entry = library.find((l) => l.name === name);
    loadEditor(effect, { kind: 'library', name, builtIn: Boolean(entry && entry.builtIn) });
  } catch (e) { toast(e.message, true); }
}

async function openFromController(controller, name) {
  if (!(await leaveEditor())) return;
  try {
    const effect = await api('GET', `/api/controllers/${encodeURIComponent(controller)}/scenes/${encodeURIComponent(name)}`);
    showView('scenes');
    await loadPlugins(controller);
    loadEditor(effect, { kind: 'controller', name, controller });
  } catch (e) { toast(e.message, true); }
}

function newScene() {
  leaveEditor().then((ok) => {
    if (!ok) return;
    const effect = {
      version: '2.0', animName: 'New scene', animType: 'plugin', colorType: 'HSB',
      palette: [{ hue: 200, saturation: 100, brightness: 100 }, { hue: 280, saturation: 100, brightness: 100 }, { hue: 330, saturation: 80, brightness: 100 }],
      pluginType: 'color', pluginUuid: BUILTIN[0].uuid,
      pluginOptions: [{ name: 'transTime', value: 20 }, { name: 'delayTime', value: 10 }, { name: 'linDirection', value: 'right' }, { name: 'nColorsPerFrame', value: 2 }, { name: 'loop', value: true }],
    };
    loadEditor(effect, { kind: 'new', name: '' });
    $('ed-name').focus();
    $('ed-name').select();
  });
}

// leaveEditor asks before unsaved work is thrown away
function leaveEditor() {
  if (!editor.effect || !editor.dirty) return Promise.resolve(true);
  return new Promise((resolve) => {
    confirmThen(`leave ${editor.effect.animName || 'this scene'} unsaved?`, 'What you changed here is not in the library and not on any controller.', 'leave it',
      () => resolve(true));
    $('confirmbox').addEventListener('close', () => resolve(false), { once: true });
  });
}

function openImport() {
  const rows = [];
  for (const c of state.controllers) {
    if (!c.reachable) continue;
    for (const s of c.scenes) {
      const kept = library.some((l) => l.name === s.name);
      rows.push(el('div', { class: 'line' },
        el('span', { class: 'what grow', text: `${s.name}` }), el('span', { class: 'why', text: `on ${c.name}${kept ? ' · already in the library' : ''}` }),
        el('button', { class: 'plain', type: 'button', text: kept ? 'import again' : 'import', onclick: async () => {
          try {
            await api('POST', '/api/scenes/import', { controller: c.name, scene: s.name });
            toast(`${s.name} imported from ${c.name}`);
            $('importbox').close();
            await loadLibrary();
            openFromLibrary(s.name);
          } catch (e) { toast(e.message, true); }
        } })));
    }
  }
  $('import-list').replaceChildren(...(rows.length ? rows : [el('p', { class: 'note', text: 'no controller can be reached to import from' })]));
  $('importbox').showModal();
}

// ---- the editor

function option(name) {
  const o = (editor.effect.pluginOptions || []).find((x) => x.name === name);
  return o ? o.value : undefined;
}
function setOption(name, value) {
  editor.effect.pluginOptions = editor.effect.pluginOptions || [];
  const o = editor.effect.pluginOptions.find((x) => x.name === name);
  if (o) o.value = value; else editor.effect.pluginOptions.push({ name, value });
}

// asSceneView is the scene as the cards see one: what sampler takes
function asSceneView() {
  const e = editor.effect;
  const options = {};
  for (const o of e.pluginOptions || []) options[o.name] = o.value;
  return { name: e.animName, kind: e.pluginType, pluginUuid: e.pluginUuid, palette: e.palette || [], options };
}

function loadEditor(effect, from) {
  editor.effect = effect;
  editor.from = from;
  editor.dirty = from.kind === 'new';
  editor.colour = 0;
  $('ed-empty').hidden = true;
  $('ed-body').hidden = false;
  $('ed-name').value = effect.animName || '';
  renderPluginsFrom();
  renderPlugin();
  renderPalette();
  renderOptions();
  renderLayoutPick();
  renderStatus();
  loadLibrary();
  previewAgain();
}

function touched() {
  editor.dirty = true;
  renderStatus();
  previewAgain();
}

function renderStatus() {
  const f = editor.from;
  const where = f.kind === 'library' ? (f.builtIn ? 'built into taproot' : 'in the library') : f.kind === 'controller' ? `a copy of ${f.name} on ${f.controller}` : 'not saved anywhere yet';
  $('ed-status').textContent = editor.dirty ? `${where} · changed` : where;
  $('ed-delete-lib').disabled = f.kind !== 'library' || f.builtIn;
  $('ed-delete-lib').title = f.builtIn ? 'it ships with taproot: saving a changed copy stands in for it, and deleting that copy brings it back' : 'take it out of the library';
}

function renderPluginsFrom() {
  const sel = $('ed-plugins-from');
  const current = sel.value;
  const reachable = state.controllers.filter((c) => c.reachable);
  sel.replaceChildren(el('option', { value: '', text: 'the built-in six' }), ...reachable.map((c) => el('option', { value: c.name, text: c.name })));
  const want = editor.from && editor.from.controller ? editor.from.controller : current;
  sel.value = reachable.some((c) => c.name === want) ? want : '';
  if (sel.value) loadPlugins(sel.value).then(renderPlugin);
}

function renderPlugin() {
  const list = pluginList($('ed-plugins-from').value);
  const sel = $('ed-plugin');
  const uuid = editor.effect.pluginUuid;
  const known = list.some((p) => p.uuid === uuid);
  sel.replaceChildren(...list.map((p) => el('option', { value: p.uuid, text: `${p.name}${p.type === 'rhythm' ? ' ♪' : ''}` })));
  if (!known && uuid) sel.append(el('option', { value: uuid, text: `(as it is: ${uuid.slice(0, 8)}…)` }));
  sel.value = uuid || list[0].uuid;
  const p = list.find((x) => x.uuid === sel.value);
  $('ed-plugin-desc').textContent = p ? (p.description || '') + (p.type === 'rhythm' ? ' · moves to sound, which the preview cannot hear' : '') : 'a motion this list does not know; its options are kept as they are';
}

function choosePlugin(uuid) {
  const list = pluginList($('ed-plugins-from').value);
  const p = list.find((x) => x.uuid === uuid);
  editor.effect.pluginUuid = uuid;
  if (p) {
    editor.effect.pluginType = p.type || 'color';
    // the options this motion takes, at their defaults where the scene has no value yet; the rest are dropped
    const kept = [];
    for (const c of p.config) {
      const had = option(c.name);
      kept.push({ name: c.name, value: had !== undefined ? had : c.defaultValue });
    }
    editor.effect.pluginOptions = kept;
  }
  renderPlugin();
  renderOptions();
  touched();
}

function renderPalette() {
  const palette = editor.effect.palette || [];
  $('ed-palette').replaceChildren(...palette.map((c, i) => el('button', {
    class: i === editor.colour ? 'chip current' : 'chip', type: 'button', title: `colour ${i + 1}: hue ${Math.round(c.hue)}, ${Math.round(c.saturation)}% ${Math.round(c.brightness)}%`,
    style: `background:${css(hsb(c.hue, c.saturation, c.brightness))}`, onclick: () => { editor.colour = i; renderPalette(); },
  })));
  const c = palette[editor.colour];
  $('ed-colour').hidden = !c;
  if (!c) return;
  $('ed-hue').value = c.hue; $('ed-hue-out').value = `${Math.round(c.hue)}°`;
  $('ed-sat').value = c.saturation; $('ed-sat-out').value = `${Math.round(c.saturation)}%`;
  $('ed-bri').value = c.brightness; $('ed-bri-out').value = `${Math.round(c.brightness)}%`;
  $('ed-hex').value = hex(hsb(c.hue, c.saturation, c.brightness));
  $('ed-colour-left').disabled = editor.colour === 0;
  $('ed-colour-right').disabled = editor.colour === palette.length - 1;
  $('ed-colour-remove').disabled = palette.length <= 1;
}

const hex = ([r, g, b]) => '#' + [r, g, b].map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');

// toHSB is a colour's hue, saturation and brightness from red, green and blue, as the API counts them
function toHSB(r, g, b) {
  const max = Math.max(r, g, b) / 255, min = Math.min(r, g, b) / 255, d = max - min;
  let h = 0;
  if (d) {
    if (max === r / 255) h = ((g - b) / 255 / d) % 6;
    else if (max === g / 255) h = (b - r) / 255 / d + 2;
    else h = (r - g) / 255 / d + 4;
  }
  return { hue: Math.round(((h * 60) + 360) % 360), saturation: Math.round(max ? (d / max) * 100 : 0), brightness: Math.round(max * 100) };
}

function colourChanged(part, value) {
  const c = editor.effect.palette[editor.colour];
  if (!c) return;
  c[part] = Number(value);
  renderPalette();
  touched();
}

function renderOptions() {
  const list = pluginList($('ed-plugins-from').value);
  const p = list.find((x) => x.uuid === editor.effect.pluginUuid);
  const config = p ? p.config : [];
  const rows = [];
  // the motion's own options first, as its list has them; then any the scene carries that the list does not mention
  const seen = new Set();
  for (const c of config) {
    seen.add(c.name);
    rows.push(optionRow(c, option(c.name)));
  }
  for (const o of editor.effect.pluginOptions || []) {
    if (!seen.has(o.name)) rows.push(optionRow({ name: o.name, type: typeof o.value === 'boolean' ? 'bool' : typeof o.value === 'number' ? 'double' : 'string' }, o.value));
  }
  $('ed-options').replaceChildren(...(rows.length ? rows : [el('p', { class: 'note', text: 'this motion takes no options' })]));
}

function optionRow(c, value) {
  const title = c.hint || '';
  const name = el('span', { class: 'name', text: c.name, title });
  const current = value !== undefined ? value : c.defaultValue;
  if (c.type === 'bool') {
    const box = el('input', { type: 'checkbox' });
    box.checked = Boolean(current);
    box.addEventListener('change', () => { setOption(c.name, box.checked); touched(); });
    return el('label', { class: 'opt' }, name, box, el('span'));
  }
  if (c.type === 'string' && c.strings && c.strings.length) {
    const sel = el('select', {}, ...c.strings.map((s) => el('option', { value: s, text: s })));
    sel.value = current;
    sel.addEventListener('change', () => { setOption(c.name, sel.value); touched(); });
    return el('label', { class: 'opt' }, name, sel, el('span'));
  }
  if (c.type === 'int' || c.type === 'double') {
    const min = num(c.minValue, 0), max = num(c.maxValue, 1000);
    const step = c.type === 'int' ? 1 : (max - min) / 200;
    const range = el('input', { type: 'range', min, max, step, value: num(current, min) });
    const number = el('input', { type: 'number', min, max, step, value: num(current, min) });
    const set = (v) => { const n = c.type === 'int' ? Math.round(Number(v)) : Number(v); range.value = n; number.value = n; setOption(c.name, n); touched(); };
    range.addEventListener('input', () => set(range.value));
    number.addEventListener('change', () => set(number.value));
    return el('label', { class: 'opt' }, name, range, number);
  }
  const text = el('input', { type: 'text', value: current == null ? '' : String(current) });
  text.addEventListener('change', () => { setOption(c.name, text.value); touched(); });
  return el('label', { class: 'opt' }, name, text, el('span'));
}

// ---- the preview

function renderLayoutPick() {
  const sel = $('ed-layout');
  const current = sel.value || editor.layout;
  sel.replaceChildren(el('option', { value: 'wall', text: 'the wall you built' }),
    ...state.controllers.filter((c) => c.reachable && c.layout).map((c) => el('option', { value: c.name, text: `${c.name}'s panels` })));
  sel.value = [...sel.options].some((o) => o.value === current) ? current : 'wall';
  editor.layout = sel.value;
  const live = $('ed-target-live');
  const was = live.value;
  live.replaceChildren(...state.controllers.filter((c) => c.reachable).map((c) => el('option', { value: c.name, text: c.name })));
  if ([...live.options].some((o) => o.value === was)) live.value = was;
  $('ed-show').disabled = live.options.length === 0;
}

// stageCtl is what the preview draws on, in the shape a card's controller has
function stageCtl() {
  const sv = editor.effect ? asSceneView() : null;
  let layout, orientation = 0;
  if (editor.layout !== 'wall') {
    const c = state.controllers.find((x) => x.name === editor.layout);
    if (c && c.layout) { layout = c.layout; orientation = c.orientation; }
  }
  if (!layout) layout = wallLayout();
  return { reachable: true, on: true, brightness: 100, colorMode: 'effect', layout, orientation, scenes: sv ? [sv] : [], running: sv ? sv.name : '' };
}

function previewAgain() {
  if (!editor.stage) {
    const s = { name: 'editor', view: { turn: 0, flip: false, zoom: 1, pan: { x: 0, y: 0 } }, tiles: [], signature: {} };
    s.svg = $('ed-svg');
    s.glow = svg('g', { class: 'glow' });
    s.tilesGroup = svg('g', { class: 'tiles' });
    s.svg.append(s.glow, s.tilesGroup);
    panView(s);
    editor.stage = s;
  }
  const s = editor.stage;
  s.ctl = stageCtl();
  const sig = JSON.stringify([s.ctl.layout, s.ctl.orientation]);
  if (s.signature.layout !== sig) {
    s.signature.layout = sig;
    s.glow.replaceChildren();
    s.tilesGroup.replaceChildren();
    s.tiles = [];
    const placed = place(s.ctl, s.view);
    if (placed) {
      s.base = placed.viewBox.split(' ').map(Number);
      applyView(s);
      for (const p of placed.panels) {
        const halo = svg('polygon', { points: p.path });
        const tile = svg('polygon', { points: p.path });
        s.glow.append(halo);
        s.tilesGroup.append(tile);
        s.tiles.push({ panel: p, halo, tile });
      }
    }
  }
  s.light = lighting(s.ctl);
  for (const t of s.tiles) t.tile.dataset.fill = '';
  draw(s, editor.time, tileOff());
}

// the preview's clock, run by the frame loop at the chosen speed
let editorLast = 0;
function editorFrame(now) {
  if (!editor.stage || $('library').hidden) { editorLast = now; return; }
  const dt = editorLast ? (now - editorLast) / 1000 : 0;
  editorLast = now;
  if (!editor.paused) editor.time += dt * editor.speed;
  draw(editor.stage, editor.time, tileOff());
}

// ---- building a wall of triangles

// the wall is cells of a triangular grid, each a panel; cells that share an edge make a shape
function wallCells() {
  const kept = store('taproot-wall');
  if (Array.isArray(kept) && kept.length) return kept;
  return [[3, 5], [3, 6], [3, 7], [2, 6]]; // a small shape to start from
}

// cellCentre is where a cell's panel sits, y upwards as the layout counts, and whether it points up
function cellCentre([r, c]) {
  const h = (SIDE * Math.sqrt(3)) / 2;
  const up = (r + c) % 2 === 0;
  return { x: c * (SIDE / 2), y: r * h + (up ? h / 3 : (2 * h) / 3), up };
}

function wallLayout() {
  const cells = wallCells();
  return {
    numPanels: cells.length, sideLength: SIDE,
    positionData: cells.map((cell, i) => { const p = cellCentre(cell); return { panelId: i + 1, x: Math.round(p.x), y: Math.round(p.y), o: p.up ? 0 : 180, shapeType: 0 }; }),
  };
}

function renderBuilder() {
  const on = new Set(wallCells().map(([r, c]) => `${r},${c}`));
  const h = (SIDE * Math.sqrt(3)) / 2;
  const cells = [];
  for (let r = 0; r < BUILD.rows; r++) {
    for (let c = 0; c < BUILD.cols; c++) {
      const p = cellCentre([r, c]);
      const pts = corners(0, SIDE, p.up ? 0 : 180).map(([dx, dy]) => `${(p.x + dx * 0.96).toFixed(1)},${(BUILD.rows * h - (p.y + dy * 0.96)).toFixed(1)}`).join(' ');
      cells.push(svg('polygon', { points: pts, class: on.has(`${r},${c}`) ? 'cell on' : 'cell', onclick: () => toggleCell(r, c) }));
    }
  }
  const s = $('ed-build-svg');
  s.setAttribute('viewBox', `${-SIDE / 2} 0 ${(BUILD.cols + 1) * (SIDE / 2)} ${BUILD.rows * h}`);
  s.replaceChildren(...cells);
}

function toggleCell(r, c) {
  const cells = wallCells();
  const i = cells.findIndex(([a, b]) => a === r && b === c);
  if (i >= 0) cells.splice(i, 1); else cells.push([r, c]);
  store('taproot-wall', cells);
  renderBuilder();
  if (editor.layout === 'wall') previewAgain();
}

// ---- saving, showing, and the rest

async function saveToLibrary() {
  const name = $('ed-name').value.trim();
  if (!name) { toast('give the scene a name first', true); $('ed-name').focus(); return; }
  editor.effect.animName = name;
  try {
    await api('PUT', `/api/scenes/${encodeURIComponent(name)}`, editor.effect);
    editor.from = { kind: 'library', name };
    editor.dirty = false;
    renderStatus();
    toast(`${name} saved to the library`);
    loadLibrary();
  } catch (e) { toast(e.message, true); }
}

function deleteFromLibrary() {
  const name = editor.from.name;
  confirmThen(`delete ${name} from the library?`, 'Only the library\'s copy goes; nothing on any controller is touched.', 'delete', async () => {
    try {
      await api('DELETE', `/api/scenes/${encodeURIComponent(name)}`);
      toast(`${name} deleted from the library`);
      editor.effect = null; editor.from = null; editor.dirty = false;
      $('ed-body').hidden = true; $('ed-empty').hidden = false;
      loadLibrary();
    } catch (e) { toast(e.message, true); }
  });
}

async function showOnPanels() {
  const target = $('ed-target-live').value;
  if (!target) return;
  editor.effect.animName = $('ed-name').value.trim() || editor.effect.animName;
  const out = await run(null, () => api('POST', `/api/controllers/${encodeURIComponent(target)}/preview`, editor.effect));
  if (out !== null) toast(state.dryRun ? `dry run: ${target} would show it` : `${target} is showing it, unsaved: start a scene there to go back`);
}

function openPush() {
  const name = $('ed-name').value.trim();
  if (!name) { toast('give the scene a name first', true); $('ed-name').focus(); return; }
  editor.effect.animName = name;
  $('push-what').textContent = name;
  $('push-result').replaceChildren();
  $('push-targets').replaceChildren(...state.controllers.map((c) => {
    const has = c.scenes.some((s) => s.name === name);
    const box = el('input', { type: 'checkbox', value: c.name, disabled: !c.reachable });
    box.checked = c.reachable && editor.from.kind === 'controller' && editor.from.controller === c.name;
    return el('label', { class: c.reachable ? 'target' : 'target off' }, box,
      el('span', { class: 'grow', text: c.name }),
      el('span', { class: 'has', text: !c.reachable ? 'cannot be reached' : has ? 'has a scene of this name' : 'does not have it' }));
  }));
  $('pushbox').showModal();
}

async function pushScene() {
  const to = [...document.querySelectorAll('#push-targets input:checked')].map((i) => i.value);
  if (to.length === 0) { toast('tick at least one controller', true); return; }
  $('push-go').disabled = true;
  const lines = [];
  for (const name of to) {
    try {
      const report = await api('POST', `/api/controllers/${encodeURIComponent(name)}/scenes`, { scene: editor.effect, force: $('push-force').checked, select: $('push-select').checked });
      const r = report.results[0];
      const bad = r.outcome === 'refused' || r.outcome === 'failed';
      lines.push(el('div', { class: bad ? 'line bad' : 'line' }, el('span', { class: 'what', text: `${name}: ${report.dryRun ? 'would be ' : ''}${r.outcome}` }), el('span', { class: 'why', text: r.reason || (report.backup ? `backed up first to ${report.backup}` : '') })));
    } catch (e) {
      lines.push(el('div', { class: 'line bad' }, el('span', { class: 'what', text: `${name}: failed` }), el('span', { class: 'why', text: e.message })));
    }
  }
  $('push-result').replaceChildren(...lines);
  $('push-go').disabled = false;
  soon(100);
}

function openJSON() {
  editor.effect.animName = $('ed-name').value.trim() || editor.effect.animName;
  $('json-text').value = JSON.stringify(editor.effect, null, 2);
  $('json-note').textContent = '';
  $('jsonbox').showModal();
}

function applyJSON() {
  try {
    const effect = JSON.parse($('json-text').value);
    if (!effect || typeof effect !== 'object') throw new Error('not a scene');
    loadEditor(effect, editor.from);
    editor.dirty = true;
    renderStatus();
    $('jsonbox').close();
  } catch (e) { $('json-note').textContent = e.message; }
}

function wireEditor() {
  $('view').addEventListener('change', () => showView($('view').value));
  $('lib-new').addEventListener('click', newScene);
  $('lib-import').addEventListener('click', openImport);
  $('ed-name').addEventListener('input', () => { editor.effect.animName = $('ed-name').value; editor.dirty = true; renderStatus(); });
  $('ed-plugin').addEventListener('change', () => choosePlugin($('ed-plugin').value));
  $('ed-plugins-from').addEventListener('change', async () => { await loadPlugins($('ed-plugins-from').value); renderPlugin(); renderOptions(); });
  $('ed-add-colour').addEventListener('click', () => {
    editor.effect.palette = editor.effect.palette || [];
    const last = editor.effect.palette[editor.effect.palette.length - 1] || { hue: 0, saturation: 100, brightness: 100 };
    editor.effect.palette.push({ hue: (last.hue + 40) % 360, saturation: last.saturation, brightness: last.brightness });
    editor.colour = editor.effect.palette.length - 1;
    renderPalette(); touched();
  });
  $('ed-hue').addEventListener('input', () => colourChanged('hue', $('ed-hue').value));
  $('ed-sat').addEventListener('input', () => colourChanged('saturation', $('ed-sat').value));
  $('ed-bri').addEventListener('input', () => colourChanged('brightness', $('ed-bri').value));
  $('ed-hex').addEventListener('change', () => {
    const m = /^#?([0-9a-f]{6})$/i.exec($('ed-hex').value.trim());
    if (!m) { toast('a colour is six hex digits', true); return; }
    const n = parseInt(m[1], 16);
    Object.assign(editor.effect.palette[editor.colour], toHSB(n >> 16, (n >> 8) & 255, n & 255));
    renderPalette(); touched();
  });
  const move = (by) => { const p = editor.effect.palette; const i = editor.colour, j = i + by; if (j < 0 || j >= p.length) return; [p[i], p[j]] = [p[j], p[i]]; editor.colour = j; renderPalette(); touched(); };
  $('ed-colour-left').addEventListener('click', () => move(-1));
  $('ed-colour-right').addEventListener('click', () => move(1));
  $('ed-colour-dup').addEventListener('click', () => { const p = editor.effect.palette; p.splice(editor.colour + 1, 0, { ...p[editor.colour] }); editor.colour++; renderPalette(); touched(); });
  $('ed-colour-remove').addEventListener('click', () => { const p = editor.effect.palette; if (p.length <= 1) return; p.splice(editor.colour, 1); editor.colour = Math.min(editor.colour, p.length - 1); renderPalette(); touched(); });
  $('ed-layout').addEventListener('change', () => { editor.layout = $('ed-layout').value; previewAgain(); });
  $('ed-speed').addEventListener('input', () => { editor.speed = Math.pow(2, Number($('ed-speed').value)); $('ed-speed-out').value = `${editor.speed}×`; });
  $('ed-pause').addEventListener('click', () => { editor.paused = !editor.paused; $('ed-pause').textContent = editor.paused ? 'play' : 'pause'; });
  $('ed-build').addEventListener('click', () => { const b = $('ed-builder'); b.hidden = !b.hidden; if (!b.hidden) { renderBuilder(); $('ed-layout').value = 'wall'; editor.layout = 'wall'; previewAgain(); } });
  $('ed-build-clear').addEventListener('click', () => { store('taproot-wall', []); renderBuilder(); previewAgain(); });
  $('ed-build-done').addEventListener('click', () => { $('ed-builder').hidden = true; });
  $('ed-save-lib').addEventListener('click', saveToLibrary);
  $('ed-delete-lib').addEventListener('click', deleteFromLibrary);
  $('ed-show').addEventListener('click', showOnPanels);
  $('ed-save-ctl').addEventListener('click', openPush);
  $('push-go').addEventListener('click', pushScene);
  $('ed-json').addEventListener('click', openJSON);
  $('json-apply').addEventListener('click', applyJSON);
  window.addEventListener('beforeunload', (e) => { if (editor.dirty) { e.preventDefault(); e.returnValue = ''; } });
  showView(store('taproot-view') === 'scenes' ? 'scenes' : 'controllers');
}
