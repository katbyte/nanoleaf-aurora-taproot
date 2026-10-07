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
  for (const [key, value] of Object.entries(attrs || {})) node.setAttribute(key, value);
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
  const card = { name, view: store(`taproot-view-${name}`) || { turn: 0, flip: false }, tiles: [], signature: {}, dragging: false, sendTimer: null };

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
  card.root = el('section', { class: 'card' },
    el('div', { class: 'head' },
      el('div', {}, card.title, card.meta),
      el('div', { class: 'actions' },
        act('flash', 'flash the panels, to tell which controller this is', () => run(`${name} is flashing`, () => api('POST', `/api/controllers/${encodeURIComponent(name)}/identify`))),
        act('back up', 'save every scene of this controller to a dated folder', () => backup(name)),
        act('forget', 'delete its token and remove it from taproot', () => forget(name)))),
    el('div', { class: 'stage' },
      card.svg,
      el('div', { class: 'view' },
        el('button', { type: 'button', text: '↻', title: 'turn the picture, if it does not match your wall', onclick: () => turnView(card, 30) }),
        el('button', { type: 'button', text: '⇋', title: 'mirror the picture', onclick: () => flipView(card) })),
      el('div', { class: 'caption' }, card.running, card.plugin)),
    el('div', { class: 'controls' },
      el('label', { class: 'switch' }, card.power, card.powerLabel),
      card.slider, card.level),
    card.scenes,
    card.note);

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
  card.meta.textContent = [ctl.device, ctl.host, ctl.model, ctl.firmware && `firmware ${ctl.firmware}`].filter(Boolean).join(' · ');
  card.note.textContent = ctl.reachable ? '' : `cannot be reached: ${ctl.error || 'no answer'}`;
  if (!ctl.reachable) { card.light = null; return; }

  const layoutSig = JSON.stringify([ctl.layout, ctl.orientation, card.view]);
  if (card.signature.layout !== layoutSig) {
    card.signature.layout = layoutSig;
    card.glow.replaceChildren();
    card.tilesGroup.replaceChildren();
    card.tiles = [];
    const placed = place(ctl, card.view);
    if (placed) {
      card.svg.setAttribute('viewBox', placed.viewBox);
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
    state.controllers.length > 1 && el('button', { class: 'plain copy', type: 'button', text: 'copy…', title: 'copy this scene to another controller', onclick: () => openCopy(ctl.name, scene.name) }));
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
}

// ---- the whole page --------------------------------------------------------

function render() {
  const names = new Set(state.controllers.map((c) => c.name));
  for (const [name, card] of cards) {
    if (!names.has(name)) { card.root.remove(); cards.delete(name); }
  }
  for (const ctl of state.controllers) {
    let card = cards.get(ctl.name);
    if (!card) {
      card = makeCard(ctl.name);
      cards.set(ctl.name, card);
    }
    $('controllers').append(card.root); // in the order they came, which is by name
    update(card, ctl);
  }

  const away = state.controllers.filter((c) => !c.reachable).length;
  $('count').textContent = state.controllers.length === 0 ? '' : `· ${state.controllers.length} controller${state.controllers.length === 1 ? '' : 's'}${away ? `, ${away} unreachable` : ''}`;
  $('empty').hidden = state.controllers.length > 0 || state.pairing.length > 0;
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
  $('theme').addEventListener('click', () => {
    const root = document.documentElement;
    root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
    try { localStorage.setItem('taproot-theme', root.dataset.theme); } catch (e) { /* not remembered */ }
    for (const card of cards.values()) for (const t of card.tiles) t.tile.dataset.fill = ''; // repaint against the new wall
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
