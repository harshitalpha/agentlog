// agentlog UI entry point: state, polling and events. Vanilla ES modules, no build step.

import { buildRows } from './util.js';
import { renderTimeline } from './render.js';

const $ = (selector) => document.querySelector(selector);

const state = {
  repo: { name: '', branch: '', head: '', dirty: 0 },
  commits: [],
  selected: null,
  order: 'new',
  query: '',
  filesByHash: new Map(),
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

function select(hash) {
  state.selected = hash;
  drawTimeline();
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
  const list = rows();
  if (!list.some((r) => r.hash === state.selected)) {
    const top = state.order === 'new' ? list[0] : list[list.length - 1];
    state.selected = top ? top.hash : null;
  }
  drawTimeline();
}

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
