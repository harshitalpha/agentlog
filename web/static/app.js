// agentlog UI entry point: state, polling and events. Vanilla ES modules, no build step.

import { buildRows } from './util.js';
import { parsePatch } from './diff.js';
import { renderTimeline } from './render.js';
import { renderDetail, renderLoading, renderError, renderPlaceholder, renderFileView, renderModalMessage } from './detail.js';

const $ = (selector) => document.querySelector(selector);

const state = {
  repo: { name: '', branch: '', head: '', dirty: 0 },
  commits: [],
  selected: null,
  order: 'new',
  query: '',
  filesByHash: new Map(),
  patches: new Map(), // hash -> {files, truncated}; never holds WORKING
};

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
  // Unfiltered: the filter box may hide the selected commit from the timeline.
  const row = buildRows(state.commits, state.repo.dirty).find((r) => r.hash === hash);
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
    if (state.selected === hash) renderError(el, `Could not load this commit: ${err.message}`);
  }
}

function select(hash) {
  state.selected = hash;
  drawTimeline();
  showDetail();
}

async function refresh(first) {
  const repo = await getJSON('/api/repo');
  const changed = first || repo.head !== state.repo.head || repo.dirty !== state.repo.dirty;
  state.repo = repo;
  $('#repoName').textContent = repo.name;
  $('#branch').textContent = repo.branch;
  document.title = `agentlog · ${repo.name}`;
  if (!changed) return;
  state.commits = await getJSON('/api/commits');
  const prev = state.selected;
  const list = rows();
  if (!list.some((r) => r.hash === state.selected)) {
    const top = state.order === 'new' ? list[0] : list[list.length - 1];
    state.selected = top ? top.hash : null;
  }
  drawTimeline();
  if (first || state.selected !== prev) showDetail();
  else if (state.selected === 'WORKING') showDetail({ keepScroll: true });
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
    if (token === modalToken) renderModalMessage($('#modalBody'), `Could not load the file: ${err.message}`);
  }
}

function closeModal() {
  if ($('#modal').hidden) return false;
  modalToken++; // drop any response still in flight
  $('#modal').hidden = true;
  if (modalOpener && document.contains(modalOpener)) modalOpener.focus();
  return true;
}

$('#modalClose').addEventListener('click', closeModal);
$('#modal').addEventListener('click', (e) => { if (e.target.id === 'modal') closeModal(); });
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeModal(); });

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
  drawOrder();
  drawTimeline();
});
$('#filter').addEventListener('input', (e) => {
  state.query = e.target.value;
  drawTimeline();
});

refresh(true).catch((err) => {
  $('#live').classList.add('off');
  $('#live').title = 'disconnected';
  console.error(err);
});
