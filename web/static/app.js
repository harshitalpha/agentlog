// agentlog UI entry point: state, polling and events. Vanilla ES modules, no build step.

import { buildRows } from './util.js';
import { parsePatch } from './diff.js';
import { renderTimeline } from './render.js';
import { renderDetail, renderLoading, renderError, renderPlaceholder, renderFileView, renderModalMessage } from './detail.js';

const $ = (selector) => document.querySelector(selector);

const ORDER_KEY = 'agentlog.order';
const POLL_MS = 3000;

// localStorage can throw (privacy modes, disabled storage): treat it as optional.
function readOrder() {
  try { return localStorage.getItem(ORDER_KEY) === 'old' ? 'old' : 'new'; } catch { return 'new'; }
}
function saveOrder(order) {
  try { localStorage.setItem(ORDER_KEY, order); } catch { /* ignore */ }
}

const state = {
  repo: { name: '', branch: '', head: '', dirty: 0 },
  commits: [],
  selected: null,
  order: readOrder(),
  query: '',
  filesByHash: new Map(),
  patches: new Map(), // hash -> {files, truncated}; never holds WORKING
};

const UNREACHABLE = "Can't reach the agentlog server. It may have been stopped: run /agentlog:ui (or `agentlog ui`) to start it again.";

// A failed fetch() is a TypeError ("Failed to fetch"): say what it means instead.
async function getJSON(url) {
  const res = await fetch(url, { cache: 'no-store' });
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.json();
}

const rows = () => buildRows(state.commits, state.repo.dirty, {
  query: state.query, order: state.order, filesByHash: state.filesByHash,
});

function drawTimeline() {
  renderTimeline($('#timeline'), rows(), { selected: state.selected, hasCommits: state.commits.length > 0 });
}

function drawOrder() {
  for (const b of document.querySelectorAll('#order button')) {
    b.setAttribute('aria-pressed', String(b.dataset.order === state.order));
  }
}

function setLive(ok) {
  const dot = $('#live');
  dot.classList.toggle('off', !ok);
  dot.title = ok ? 'live: refreshes as commits land' : 'disconnected';
}

/** Rows with no filter and no order games: used to look commits up by hash. */
const allRows = () => buildRows(state.commits, state.repo.dirty);

async function loadPatch(hash) {
  const cached = state.patches.get(hash);
  if (cached) return cached;
  const res = await getJSON(`/api/patch?hash=${encodeURIComponent(hash)}`);
  const parsed = { files: parsePatch(res.patch || ''), truncated: Boolean(res.truncated) };
  if (hash !== 'WORKING') state.patches.set(hash, parsed);
  state.filesByHash.set(hash, parsed.files.map((f) => f.path));
  return parsed;
}

/** Shows the selected commit. keepScroll is for background refreshes of the same item. */
async function showDetail({ keepScroll = false } = {}) {
  const hash = state.selected;
  const el = $('#detail');
  const row = allRows().find((r) => r.hash === hash); // the filter may hide it from the timeline
  if (!row) return renderPlaceholder(el, state.commits.length ? 'Pick a commit' : 'Nothing to show yet');
  if (!keepScroll) renderLoading(el);
  try {
    const { files, truncated } = await loadPatch(hash);
    if (state.selected !== hash) return; // the selection moved on while we were fetching
    const top = el.scrollTop;
    renderDetail(el, row, files, { truncated, total: state.commits.length });
    el.scrollTop = keepScroll ? top : 0;
    if (state.query) drawTimeline(); // the filter can match this commit's files now
  } catch (err) {
    if (state.selected === hash) renderError(el, err instanceof TypeError ? UNREACHABLE : `Could not load this commit: ${err.message}`);
  }
}

function setHash(hash) {
  // Working-tree state is transient, so it is not a deep link.
  const want = hash && hash !== 'WORKING' ? `#${hash}` : '';
  if (location.hash !== want) history.replaceState(null, '', want || location.pathname + location.search);
}

function select(hash, { scroll = false } = {}) {
  state.selected = hash;
  setHash(hash);
  drawTimeline();
  if (scroll) $('#timeline .item.sel')?.scrollIntoView({ block: 'nearest' });
  showDetail();
}

/** A commit matching location.hash (full hash or a prefix of at least 4 chars), or null. */
function hashTarget() {
  const want = location.hash.slice(1).toLowerCase();
  if (want.length < 4) return null;
  return state.commits.find((c) => c.hash.startsWith(want)) || null;
}

async function refresh(first) {
  const repo = await getJSON('/api/repo');
  const changed = first || repo.head !== state.repo.head || repo.dirty !== state.repo.dirty;
  $('#repoName').textContent = repo.name;
  $('#branch').textContent = repo.branch;
  document.title = `agentlog · ${repo.name}`;
  if (!changed) { state.repo = repo; return; }
  const commits = await getJSON('/api/commits');
  state.repo = repo;
  state.commits = commits;

  const prev = state.selected;
  const linked = first ? hashTarget() : null;
  if (linked) state.selected = linked.hash;
  else if (!allRows().some((r) => r.hash === state.selected)) {
    // Selection is gone (or first run): take the top item of the current order, ignoring the filter.
    const top = buildRows(state.commits, state.repo.dirty, { order: state.order })[0];
    state.selected = top ? top.hash : null;
  }
  setHash(state.selected);
  drawTimeline();
  if (linked) $('#timeline .item.sel')?.scrollIntoView({ block: 'nearest' });
  if (first || state.selected !== prev) showDetail();
  else if (state.selected === 'WORKING') showDetail({ keepScroll: true });
}

let polling = false;
let failures = 0;

async function poll(first = false) {
  if (polling) return; // never stack requests on a slow server
  polling = true;
  try {
    await refresh(first);
    failures = 0;
    setLive(true);
  } catch (err) {
    failures++;
    if (failures >= 2) setLive(false); // one blip is not a disconnect
    console.error(err);
  } finally {
    polling = false;
  }
}

let modalToken = 0;
let modalOpener = null;

async function openModal(hash, path) {
  const token = ++modalToken;
  modalOpener = document.activeElement;
  $('#modalTitle').textContent = `${path} @ ${hash === 'WORKING' ? 'working tree' : hash.slice(0, 8)}`;
  renderModalMessage($('#modalBody'), 'Loading…');
  $('#modal').hidden = false;
  $('#modalClose').focus();
  try {
    const res = await getJSON(`/api/file?hash=${encodeURIComponent(hash)}&path=${encodeURIComponent(path)}`);
    if (token === modalToken) renderFileView($('#modalBody'), res);
  } catch (err) {
    if (token === modalToken) renderModalMessage($('#modalBody'), err instanceof TypeError ? UNREACHABLE : `Could not load the file: ${err.message}`);
  }
}

function closeModal() {
  if ($('#modal').hidden) return false;
  modalToken++; // drop any response still in flight
  $('#modal').hidden = true;
  if (modalOpener && document.contains(modalOpener)) modalOpener.focus();
  return true;
}

function setQuery(value) {
  state.query = value;
  drawTimeline();
}

$('#modalClose').addEventListener('click', closeModal);
$('#modal').addEventListener('click', (e) => { if (e.target.id === 'modal') closeModal(); });

document.addEventListener('keydown', (e) => {
  const filter = $('#filter');
  if (e.key === 'Escape') {
    if (closeModal()) return;
    if (e.target === filter) { filter.value = ''; setQuery(''); filter.blur(); }
    return;
  }
  const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || e.target.isContentEditable;
  if (typing || e.ctrlKey || e.metaKey || e.altKey || !$('#modal').hidden) return;
  if (e.key === '/') {
    e.preventDefault();
    filter.focus();
  } else if (e.key === 'j' || e.key === 'k') {
    const list = rows();
    if (!list.length) return;
    const i = list.findIndex((r) => r.hash === state.selected);
    const next = list[Math.max(0, Math.min(list.length - 1, i + (e.key === 'j' ? 1 : -1)))];
    if (next) select(next.hash, { scroll: true });
  }
});

$('#detail').addEventListener('click', (e) => {
  const t = e.target.closest('[data-tog],[data-jump],[data-copy],[data-view]');
  if (!t) return;
  if (t.dataset.tog !== undefined) $(`#f${t.dataset.tog}`).classList.toggle('closed');
  else if (t.dataset.jump !== undefined) {
    const card = $(`#f${t.dataset.jump}`);
    card.classList.remove('closed');
    card.scrollIntoView({ behavior: 'smooth', block: 'start' });
  } else if (t.dataset.view) {
    openModal(t.dataset.hash, t.dataset.view);
  } else if (t.dataset.copy) {
    navigator.clipboard?.writeText(t.dataset.copy).catch(() => {});
    const label = t.textContent;
    t.textContent = 'copied ✓';
    setTimeout(() => { t.textContent = label; }, 1500);
  }
});

$('#timeline').addEventListener('click', (e) => {
  const btn = e.target.closest('[data-hash]');
  if (btn) select(btn.dataset.hash);
});
$('#order').addEventListener('click', (e) => {
  const btn = e.target.closest('[data-order]');
  if (!btn || btn.dataset.order === state.order) return;
  state.order = btn.dataset.order;
  saveOrder(state.order);
  drawOrder();
  drawTimeline();
});
$('#filter').addEventListener('input', (e) => setQuery(e.target.value));
window.addEventListener('hashchange', () => {
  const target = hashTarget();
  if (target && target.hash !== state.selected) select(target.hash, { scroll: true });
});

drawOrder();
poll(true);
setInterval(poll, POLL_MS);
