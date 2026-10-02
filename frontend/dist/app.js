'use strict';

const $ = (id) => document.getElementById(id);
const SVGNS = 'http://www.w3.org/2000/svg';
const C = 120, R = 100;           // compass centre and radius (viewBox 240)

let api = null;
let cfg = null;
let st = null;
let logLines = [];
let agSig = '';
const svgParts = {};

// ---------- helpers ----------
function call(method, ...args) {
  return api[method](...args)
    .then((r) => { if (typeof r === 'string' && r) toast(r); return r; })
    .catch((e) => toast(String(e)));
}

let toastTimer = null;
function toast(msg, info) {
  const t = $('toast');
  t.textContent = msg;
  t.className = 'toast' + (info ? ' info' : '');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.add('hidden'), 3500);
}

function setStatus(el, ok, okText, badText) {
  el.classList.toggle('ok', ok);
  const b = el.querySelector('b');
  if (b) b.textContent = ok ? okText : badText;
}

function svg(tag, attrs, parent) {
  const e = document.createElementNS(SVGNS, tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (parent) parent.appendChild(e);
  return e;
}

function polar(deg, r) {
  const a = (deg - 90) * Math.PI / 180;
  return [C + r * Math.cos(a), C + r * Math.sin(a)];
}

function norm(az) { return ((Math.round(az) % 360) + 360) % 360; }

// ---------- compass ----------
function buildCompass() {
  const s = $('compass');
  s.innerHTML = '';
  const defs = svg('defs', {}, s);
  const g = svg('radialGradient', { id: 'globe', cx: '42%', cy: '38%', r: '75%' }, defs);
  svg('stop', { offset: '0%', 'stop-color': '#34424f' }, g);
  svg('stop', { offset: '100%', 'stop-color': '#0b0f13' }, g);

  svg('circle', { cx: C, cy: C, r: R, fill: 'url(#globe)', stroke: '#4a4f55', 'stroke-width': 2 }, s);
  [R * 0.66, R * 0.33].forEach((r) => svg('circle', { cx: C, cy: C, r, fill: 'none', stroke: 'rgba(255,255,255,.07)' }, s));
  for (let d = 0; d < 360; d += 5) {
    const long = d % 30 === 0, mid = d % 10 === 0;
    const [x1, y1] = polar(d, R - (long ? 11 : mid ? 7 : 4));
    const [x2, y2] = polar(d, R - 1);
    svg('line', { x1, y1, x2, y2, stroke: long ? '#e8e8e8' : '#7d838b', 'stroke-width': long ? 2 : 1 }, s);
  }
  for (let d = 30; d < 360; d += 30) {
    if (d % 90 === 0) continue;
    const [x, y] = polar(d, R - 21);
    svg('text', { x, y, class: 'ctick' }, s).textContent = d;
  }
  [['N', 0], ['E', 90], ['S', 180], ['W', 270]].forEach(([l, d]) => {
    const [x, y] = polar(d, R + 11);
    svg('text', { x, y, class: 'ccard' }, s).textContent = l;
  });

  svgParts.beam = svg('path', { class: 'beam' }, s);
  svgParts.target = svg('line', { class: 'tline', x1: C, y1: C, x2: C, y2: C - R + 12 }, s);
  svgParts.needle = svg('line', { class: 'needle', x1: C, y1: C, x2: C, y2: C - R + 12 }, s);
  svgParts.ghost = svg('line', { class: 'ghost hidden', x1: C, y1: C, x2: C, y2: C - R }, s);
  svgParts.ghostTxt = svg('text', { class: 'ghosttxt hidden' }, s);
  svg('circle', { cx: C, cy: C, r: 4, class: 'hub' }, s);

  const angle = (ev) => {
    const b = s.getBoundingClientRect();
    const x = (ev.clientX - b.left) / b.width * 240 - C;
    const y = (ev.clientY - b.top) / b.height * 240 - C;
    return norm(Math.atan2(x, -y) * 180 / Math.PI);
  };
  s.addEventListener('mousemove', (ev) => {
    const a = angle(ev);
    svgParts.ghost.classList.remove('hidden');
    svgParts.ghostTxt.classList.remove('hidden');
    svgParts.ghost.setAttribute('transform', `rotate(${a} ${C} ${C})`);
    const [x, y] = polar(a, R * 0.5);
    svgParts.ghostTxt.setAttribute('x', x);
    svgParts.ghostTxt.setAttribute('y', y);
    svgParts.ghostTxt.textContent = a + '°';
  });
  s.addEventListener('mouseleave', () => {
    svgParts.ghost.classList.add('hidden');
    svgParts.ghostTxt.classList.add('hidden');
  });
  s.addEventListener('click', (ev) => call('RotorGoTo', angle(ev)));
}

function wedge(bw) {
  const r = R - 12;
  const [x1, y1] = polar(-bw / 2, r);
  const [x2, y2] = polar(bw / 2, r);
  return `M${C},${C} L${x1},${y1} A${r},${r} 0 0 1 ${x2},${y2} Z`;
}

// ---------- static parts built from settings ----------
function applyConfig() {
  const p = $('panels');
  p.className = cfg.layout === 'vertical' ? 'vertical' : 'horizontal';
  document.body.style.overflow = cfg.layout === 'vertical' ? 'auto' : 'hidden';
  $('btnTop').classList.toggle('on', !!cfg.alwaysOnTop);
  document.body.classList.toggle('mini', !!cfg.mini);
  $('btnMini').classList.toggle('on', !!cfg.mini);

  svgParts.beam.setAttribute('d', wedge(cfg.rotor.beamWidth || 60));

  const pr = $('presets');
  pr.innerHTML = '';
  (cfg.rotor.presets || []).forEach((ps) => {
    if (!ps.label) return;
    const b = document.createElement('button');
    b.textContent = ps.label;
    b.title = ps.azimuth + '°';
    b.onclick = () => call('RotorGoTo', ps.azimuth);
    pr.appendChild(b);
  });

  const bands = $('stBands');
  bands.innerHTML = '';
  (cfg.steppir.bands || []).filter((b) => b.enabled).forEach((band) => {
    const b = document.createElement('button');
    b.textContent = band.name;
    b.dataset.band = band.name;
    b.title = band.mhz.toFixed(3) + ' MHz';
    b.onclick = () => call('SteppirBand', band.name);
    bands.appendChild(b);
  });

  const step = cfg.steppir.stepKHz;
  $('stDown').textContent = '⌄ ' + step;
  $('stUp').textContent = '⌃ ' + step;
  $('stDown').title = `Down ${step} kHz`;
  $('stUp').title = `Up ${step} kHz`;
  $('dir34').classList.toggle('hidden', !cfg.steppir.showThreeQuarter);

  const sel = $('stThresh');
  const opts = [5, 10, 25, 50, 100, 200];
  if (!opts.includes(cfg.steppir.trackThresholdKHz)) opts.push(cfg.steppir.trackThresholdKHz);
  sel.innerHTML = opts.sort((a, b) => a - b).map((k) => `<option value="${k}">${k} kHz</option>`).join('');
  sel.value = String(cfg.steppir.trackThresholdKHz);
}

// ---------- live rendering ----------
function render() {
  if (!st || !cfg) return;
  renderRadios();
  renderRotor();
  renderSteppir();
  renderAG();
}

function renderRadios() {
  const n = st.n1mm;
  [1, 2].forEach((nr) => {
    const r = n.radios[nr - 1];
    const el = $('r' + nr);
    el.textContent = r.seen ? `R${nr} ${r.mhz.toFixed(3)} ${r.mode}` : `R${nr} —`;
    el.classList.toggle('tx', r.seen && r.tx);
    el.classList.toggle('active', n.activeRadio === nr);
  });
}

function renderRotor() {
  const r = st.rotor;
  $('rotorPanel').classList.toggle('offline', !r.connected);
  $('rotorStatus').classList.toggle('ok', r.connected);
  $('rotorName').textContent = r.name || '';
  const known = r.azimuth >= 0;
  $('heading').textContent = known ? r.azimuth + '°' : '—';
  svgParts.beam.classList.toggle('hidden', !known);
  svgParts.needle.classList.toggle('hidden', !known);
  if (known) {
    svgParts.beam.setAttribute('transform', `rotate(${r.azimuth} ${C} ${C})`);
    svgParts.needle.setAttribute('transform', `rotate(${r.azimuth} ${C} ${C})`);
  }
  const moving = r.moving > 0;
  const mv = $('rotorMove');
  mv.textContent = ['STOP', 'CW', 'CCW'][r.moving] || 'STOP';
  mv.classList.toggle('moving', moving);
  const showT = moving && r.target >= 0;
  svgParts.target.classList.toggle('hidden', !showT);
  if (showT) svgParts.target.setAttribute('transform', `rotate(${r.target} ${C} ${C})`);
  $('target').innerHTML = showT ? `turning to ${r.target}°` : '&nbsp;';

  const sp = st.n1mm.lastAz;
  $('spVal').textContent = sp >= 0 ? sp + '°' : '—';
  $('lpVal').textContent = sp >= 0 ? norm(sp + 180) + '°' : '—';
  $('btnSP').disabled = sp < 0;
  $('btnLP').disabled = sp < 0;
}

function renderSteppir() {
  const s = st.steppir;
  $('stPanel').classList.toggle('offline', !s.connected);
  $('stStatus').classList.toggle('ok', s.connected);
  $('stFreq').textContent = s.mhz > 0 ? s.mhz.toFixed(3) + ' MHz' : '—';
  document.querySelectorAll('#stDirs button').forEach((b) => b.classList.toggle('on', b.dataset.dir === s.direction));
  document.querySelectorAll('#stBands button').forEach((b) => b.classList.toggle('on', b.dataset.band === s.band));
  $('stAuto').classList.toggle('on', s.autotrack);
  const tr = $('stTrack');
  tr.classList.toggle('on', !!cfg.steppir.tracking);
  tr.textContent = cfg.steppir.tracking ? 'Tracking on' : 'Tracking off';

  const info = $('stInfo');
  info.className = 'stinfo';
  if (s.holding) {
    info.textContent = 'Holding — radio transmitting';
    info.classList.add('hold');
  } else if (s.moving) {
    info.textContent = 'Elements moving…';
    info.classList.add('moving');
  } else {
    const f = cfg.steppir.followRadio;
    const who = f === 'active' ? 'active radio' : f === 'ag' ? 'radio via AG' : 'radio ' + f;
    info.textContent = s.connected ? (cfg.steppir.tracking ? `Following ${who}` : 'Ready') : 'Offline';
  }
}

function renderAG() {
  const ag = st.ag;
  $('agPanel').classList.toggle('offline', !ag.connected);
  setStatus($('agStatus'), ag.connected, 'ONLINE', 'OFFLINE');
  const ports = ag.ports || [];
  const pA = ports.find((p) => p.id === 1) || {};
  const pB = ports.find((p) => p.id === 2) || {};
  $('agBandA').textContent = (pA.bandName || '—') + (pA.tx ? ' TX' : '');
  $('agBandB').textContent = (pB.bandName || '—') + (pB.tx ? ' TX' : '');
  $('agBandA').parentElement.classList.toggle('txing', !!pA.tx);
  $('agBandB').parentElement.classList.toggle('txing', !!pB.tx);

  const sig = JSON.stringify([ag.connected, ag.authMsg, ag.antennas, pA, pB]);
  if (sig === agSig) return;   // rebuild only on change so clicks are never lost
  agSig = sig;

  const list = $('agList');
  const ants = ag.antennas || [];
  if (ag.authMsg) {
    list.innerHTML = `<div class="empty warnmsg">${ag.authMsg}</div>`;
    return;
  }
  if (!ants.length) {
    list.innerHTML = `<div class="empty">${ag.connected ? 'Loading antennas…' : 'Not connected'}</div>`;
    return;
  }
  const avail = (ant, p) => !p.band || ((ant.rx >> p.band) & 1) === 1;
  list.innerHTML = ants.map((a) => {
    const isA = pA.rxant === a.id || pA.txant === a.id;
    const isB = pB.rxant === a.id || pB.txant === a.id;
    const okA = avail(a, pA), okB = avail(a, pB);
    const cls = ['agrow', isA && 'isA', isB && 'isB', !okA && !okB && 'na'].filter(Boolean).join(' ');
    const btn = (port, sel, ok, tx) =>
      `<button class="ab${sel ? (port === 1 ? ' selA' : ' selB') : ''}${sel && tx ? ' tx' : ''}" data-port="${port}" data-ant="${a.id}"${ok ? '' : ' disabled'} title="${ok ? '' : 'Not available on this band'}">${port === 1 ? 'A' : 'B'}</button>`;
    const name = a.name.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
    return `<div class="${cls}">${btn(1, isA, okA, pA.tx)}<div class="agname" title="${name}">${name}</div>${btn(2, isB, okB, pB.tx)}</div>`;
  }).join('');
}

// ---------- settings dialog ----------
function getPath(o, path) { return path.split('.').reduce((v, k) => (v == null ? v : v[k]), o); }
function setPath(o, path, val) {
  const ks = path.split('.');
  const last = ks.pop();
  ks.reduce((v, k) => v[k], o)[last] = val;
}

function openSettings() {
  document.querySelectorAll('[data-path]').forEach((el) => {
    const v = getPath(cfg, el.dataset.path);
    if (el.type === 'checkbox') el.checked = !!v;
    else el.value = v == null ? '' : v;
  });

  const ants = (st && st.ag.antennas) || [];
  const sel = $('agAntSelect');
  const cur = cfg.steppir.agAntenna || 0;
  sel.innerHTML = '<option value="0">(none)</option>' + ants.map((a) => `<option value="${a.id}">${a.id}: ${a.name}</option>`).join('');
  if (cur && !ants.some((a) => a.id === cur)) sel.innerHTML += `<option value="${cur}">${cur}</option>`;
  sel.value = String(cur);

  const presets = cfg.rotor.presets || [];
  let html = '';
  for (let i = 0; i < 8; i++) {
    const p = presets[i] || { label: '', azimuth: '' };
    html += `<div><input class="pl" maxlength="4" placeholder="label" value="${p.label}"><input class="pa" type="number" min="0" max="359" placeholder="°" value="${p.azimuth}"></div>`;
  }
  $('presetRows').innerHTML = html;

  const bands = cfg.steppir.bands || [];
  html = '<span class="h">On</span><span class="h">Band</span><span class="h">Tune MHz</span><span class="h">From</span><span class="h">To</span>';
  bands.forEach((b) => {
    html += `<input type="checkbox" class="be"${b.enabled ? ' checked' : ''}><input class="bn" value="${b.name}"><input class="bm" type="number" step="0.001" value="${b.mhz}"><input class="b0" type="number" step="0.001" value="${b.min}"><input class="b1" type="number" step="0.001" value="${b.max}">`;
  });
  $('bandRows').innerHTML = html;

  api.GetAPIURLs().then((urls) => {
    const k = cfg.api.key ? '?key=' + encodeURIComponent(cfg.api.key) : '';
    $('apiExample').textContent = (urls && urls[0] ? urls[0] : '') + 'api/rotor/220' + k;
  });

  showTab('setup');
  $('settings').classList.remove('hidden');
}

function saveSettings() {
  const c = JSON.parse(JSON.stringify(cfg));
  document.querySelectorAll('[data-path]').forEach((el) => {
    let v;
    if (el.type === 'checkbox') v = el.checked;
    else if (el.type === 'number' || el.dataset.type === 'int') v = Number(el.value) || 0;
    else v = el.value.trim();
    setPath(c, el.dataset.path, v);
  });

  c.rotor.presets = [];
  document.querySelectorAll('#presetRows > div').forEach((d) => {
    const label = d.querySelector('.pl').value.trim();
    const az = d.querySelector('.pa').value;
    if (label && az !== '') c.rotor.presets.push({ label, azimuth: norm(Number(az)) });
  });

  const q = (cls) => [...document.querySelectorAll('#bandRows .' + cls)];
  const en = q('be'), nm = q('bn'), mz = q('bm'), lo = q('b0'), hi = q('b1');
  c.steppir.bands = nm.map((_, i) => ({
    enabled: en[i].checked, name: nm[i].value.trim(),
    mhz: Number(mz[i].value) || 0, min: Number(lo[i].value) || 0, max: Number(hi[i].value) || 0,
  })).filter((b) => b.name);

  api.SaveConfig(c).then((err) => {
    if (err) { toast(err); return; }
    $('settings').classList.add('hidden');
    toast('Settings saved', true);
  });
}

function showTab(name) {
  document.querySelectorAll('.tabs [data-tab]').forEach((b) => b.classList.toggle('on', b.dataset.tab === name));
  $('tab-setup').classList.toggle('hidden', name !== 'setup');
  $('tab-log').classList.toggle('hidden', name !== 'log');
  if (name === 'log') renderLog();
}

function renderLog() {
  const v = $('logView');
  v.textContent = logLines.map((l) => `${l.time}  ${l.text}`).join('\n');
  v.parentElement.scrollTop = v.parentElement.scrollHeight;
}

function addLog(line) {
  logLines.push(line);
  if (logLines.length > 500) logLines.shift();
  if (!$('tab-log').classList.contains('hidden') && !$('settings').classList.contains('hidden')) renderLog();
}

// ---------- wiring ----------
function wire() {
  $('btnLayout').onclick = () => call('SetLayout', cfg.layout === 'vertical' ? 'horizontal' : 'vertical');
  $('btnTop').onclick = () => call('SetAlwaysOnTop', !cfg.alwaysOnTop);
  $('btnMini').onclick = () => call('SetMini', !cfg.mini);
  $('btnSettings').onclick = openSettings;
  $('setCancel').onclick = () => $('settings').classList.add('hidden');
  $('setSave').onclick = saveSettings;
  $('apiPage').onclick = () => call('OpenAPIPage');
  document.querySelectorAll('.tabs [data-tab]').forEach((b) => (b.onclick = () => showTab(b.dataset.tab)));

  const go = () => {
    const v = $('azInput').value;
    if (v === '') return;
    call('RotorGoTo', norm(Number(v)));
    $('azInput').select();
  };
  $('btnGo').onclick = go;
  $('azInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') go(); });
  $('btnStop').onclick = () => call('RotorStop');
  $('btnSP').onclick = () => { if (st.n1mm.lastAz >= 0) call('RotorGoTo', st.n1mm.lastAz); };
  $('btnLP').onclick = () => { if (st.n1mm.lastAz >= 0) call('RotorGoTo', norm(st.n1mm.lastAz + 180)); };

  document.querySelectorAll('#stDirs button').forEach((b) => (b.onclick = () => call('SteppirDirection', b.dataset.dir)));
  $('stDown').onclick = () => call('SteppirStep', -1);
  $('stUp').onclick = () => call('SteppirStep', 1);
  $('stTrack').onclick = () => call('SetTracking', !cfg.steppir.tracking);
  $('stThresh').onchange = (e) => call('SetTrackThreshold', Number(e.target.value));
  $('stRetract').onclick = () => {
    if (confirm('Retract all SteppIR elements?')) call('SteppirRetract');
  };
  $('stCal').onclick = () => {
    if (confirm('Calibrate the SteppIR elements?')) call('SteppirCalibrate');
  };
  $('stAuto').onclick = () => call('SteppirAutotrack', !st.steppir.autotrack);

  $('agList').addEventListener('click', (e) => {
    const b = e.target.closest('button[data-ant]');
    if (b && !b.disabled) call('AGSelect', Number(b.dataset.port), Number(b.dataset.ant));
  });

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') $('settings').classList.add('hidden');
  });
}

function boot() {
  if (!window.go || !window.go.main || !window.runtime) {
    setTimeout(boot, 50);
    return;
  }
  api = window.go.main.App;
  buildCompass();
  wire();
  Promise.all([api.GetConfig(), api.GetState(), api.GetLog()]).then(([c, s, logs]) => {
    cfg = c;
    st = s;
    logLines = logs || [];
    applyConfig();
    render();
    window.runtime.EventsOn('state', (s2) => { st = s2; render(); });
    window.runtime.EventsOn('config', (c2) => { cfg = c2; applyConfig(); render(); });
    window.runtime.EventsOn('log', addLog);
    api.UIReady();
    api.GetVersion().then((v) => { $('verLabel').textContent = 'AntennaDeck v' + v; });
  });
}

boot();
