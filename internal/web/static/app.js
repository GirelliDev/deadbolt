'use strict';
const $ = (id) => document.getElementById(id);
const el = (tag, text, cls) => { const e = document.createElement(tag); if (text != null) e.textContent = text; if (cls) e.className = cls; return e; };

async function api(path, body) {
  const opt = { headers: { 'X-Dbolt': '1' } };
  if (body !== undefined) { opt.method = 'POST'; opt.headers['Content-Type'] = 'application/json'; opt.body = JSON.stringify(body); }
  const r = await fetch(path, opt);
  const data = await r.json().catch(() => ({}));
  if (r.status === 401 && path !== '/api/login') { showLogin(); throw new Error('not logged in'); }
  if (!r.ok) throw new Error(data.error || ('HTTP ' + r.status));
  return data;
}

let toastTimer;
function toast(msg) { const t = $('toast'); t.textContent = msg; t.classList.remove('hidden'); clearTimeout(toastTimer); toastTimer = setTimeout(() => t.classList.add('hidden'), 4500); }
async function run(fn) { try { await fn(); } catch (e) { toast(e.message); } }

function showLogin() { $('app').classList.add('hidden'); $('logout').classList.add('hidden'); $('login').classList.remove('hidden'); stopPolling(); }
function showApp() { $('login').classList.add('hidden'); $('app').classList.remove('hidden'); $('logout').classList.remove('hidden'); refreshAll(); startPolling(); }

$('loginForm').addEventListener('submit', async (ev) => {
  ev.preventDefault(); $('loginErr').textContent = '';
  try { await api('/api/login', { password: $('pw').value }); $('pw').value = ''; showApp(); }
  catch (e) { $('loginErr').textContent = e.message; }
});
$('logout').addEventListener('click', () => run(async () => { await api('/api/logout', {}); showLogin(); }));

async function loadStatus() {
  const s = await api('/api/status');
  const line = $('statusLine'); line.textContent = '';
  line.append(el('span', '', 'dot ' + (s.state === 'active' ? 'on' : 'off')), document.createTextNode(
    `${s.publicName || s.server} · ${s.state} · port ${s.port || '?'}/udp · ${s.mods} mods (${s.items} Workshop items)`));
}

async function loadPlayers() {
  const p = await api('/api/players');
  $('playersErr').textContent = p.online ? '' : 'RCON unavailable: ' + p.error;
  $('pcount').textContent = p.online ? `(${p.players.length} online)` : '';
  const tb = $('players').tBodies[0]; tb.textContent = '';
  for (const name of p.players) {
    const tr = el('tr'); tr.append(el('td', name));
    const td = el('td', null, 'actions');
    const btn = (label, cls, fn) => { const b = el('button', label, 'small ' + (cls || 'ghost')); b.addEventListener('click', () => run(fn)); td.append(b); };
    btn('Give item', '', async () => {
      const item = prompt(`Item for ${name} (e.g. Base.Axe)`); if (!item) return;
      const count = parseInt(prompt('How many?', '1'), 10); if (!count) return;
      await act('additem', { user: name, item: item.trim(), count }); toast(`Gave ${count}× ${item}`);
    });
    btn('Teleport to…', '', async () => { const t = prompt(`Teleport ${name} to which player?`); if (t) { await act('teleport', { user: name, target: t.trim() }); toast('Teleported'); } });
    btn('God mode', '', async () => { await act('godmod', { user: name }); toast('Toggled god mode'); });
    btn('Kick', 'danger', async () => { const r = prompt(`Kick ${name}? Reason (optional):`); if (r === null) return; await act('kick', { user: name, reason: r.trim() }); toast('Kicked'); await loadPlayers(); });
    btn('Ban', 'danger', async () => { const r = prompt(`BAN ${name}? Reason (optional):`); if (r === null) return; await act('ban', { user: name, reason: r.trim() }); toast('Banned'); await loadPlayers(); });
    tr.append(td); tb.append(tr);
  }
}
const act = (action, params) => api('/api/action', Object.assign({ action }, params));

$('msgBtn').addEventListener('click', () => run(async () => { const m = $('msg').value.trim(); if (!m) return; await act('servermsg', { message: m }); $('msg').value = ''; toast('Message sent'); }));
$('saveBtn').addEventListener('click', () => run(async () => { await act('save', {}); toast('World saved'); }));
$('unbanBtn').addEventListener('click', () => run(async () => { const u = $('offUser').value.trim(); if (!u) return; await act('unban', { user: u }); toast('Unbanned ' + u); }));
document.querySelectorAll('[data-ctl]').forEach((b) => b.addEventListener('click', () => run(async () => {
  const a = b.dataset.ctl;
  if (a !== 'start' && !confirm(`${a[0].toUpperCase() + a.slice(1)} the server? Players will be disconnected.`)) return;
  await api('/api/server', { action: a }); toast(`Server: ${a} requested`); setTimeout(() => run(loadStatus), 2000);
})));

async function loadUpdates(refresh) {
  const u = await api('/api/updates' + (refresh ? '?refresh=1' : ''));
  const box = $('updates'); box.textContent = '';
  box.append(el('div', u.game ? 'Game: ' + u.game : 'Game: up to date'));
  box.append(el('div', u.mods.length ? `Mods to update: ${u.mods.length}` : 'Mods: all up to date'));
  for (const m of u.mods) box.append(el('div', '• ' + m, 'muted'));
  if (u.note) box.append(el('div', u.note, 'muted'));
}
$('updRefresh').addEventListener('click', () => run(() => loadUpdates(true)));

async function loadBackups() {
  const rows = await api('/api/backups'); const tb = $('backups').tBodies[0]; tb.textContent = '';
  for (const b of rows) { const tr = el('tr'); tr.append(el('td', b.name), el('td', (b.size / 1048576).toFixed(1) + ' MiB'), el('td', b.reason)); tb.append(tr); }
}
$('bkBtn').addEventListener('click', () => run(async () => { $('bkBtn').disabled = true; try { const r = await api('/api/backups', {}); toast('Backup created: ' + r.name); await loadBackups(); } finally { $('bkBtn').disabled = false; } }));

async function loadMods() {
  const rows = await api('/api/mods'); const tb = $('mods').tBodies[0]; tb.textContent = '';
  for (const m of rows) {
    const tr = el('tr'); tr.append(el('td', m.id));
    tr.append(el('td', m.mods.length ? m.mods.map((x) => (m.enabled.includes(x) ? '+' : '–') + x).join('  ') : '(not downloaded)'));
    tb.append(tr);
  }
}

function refreshAll() { run(loadStatus); run(loadPlayers); run(() => loadUpdates(false)); run(loadBackups); run(loadMods); }
let poll;
function startPolling() { stopPolling(); poll = setInterval(() => { run(loadStatus); run(loadPlayers); }, 15000); }
function stopPolling() { clearInterval(poll); }

(async () => { try { await api('/api/status'); showApp(); } catch { showLogin(); } })();
