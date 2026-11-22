// detail.js: DOM builders for the right-hand column (commit header, file cards, diff table).

import { h, stat } from './render.js';
import { stripTrailers, baseName, plural } from './util.js';

/** Cards with more changed lines than this start collapsed. */
export const BIG_FILE_LINES = 400;

export function numberColumns(...extra) {
  const cols = [];
  for (let i = 0; i < 2; i++) {
    const col = h('col');
    col.style.width = '52px'; // CSSOM, allowed by the CSP (style attributes are not)
    cols.push(col);
  }
  return h('colgroup', {}, cols, extra);
}

function diffTable(file) {
  const rows = [];
  for (const hunk of file.hunks) {
    rows.push(h('tr', { class: 'hunk' }, h('td', { colspan: 3, text: hunk.header })));
    for (const l of hunk.lines) {
      rows.push(h('tr', { class: l.type },
        h('td', { class: 'n', text: l.old ?? '' }),
        h('td', { class: 'n', text: l.new ?? '' }),
        h('td', { class: 'c', text: l.text })));
    }
  }
  return h('table', { class: 'diff' }, numberColumns(h('col')), h('tbody', {}, rows));
}

function fileBody(file) {
  if (file.binary) return h('div', { class: 'bin', text: 'Binary file' });
  if (!file.hunks.length) return h('div', { class: 'bin', text: `No content changes (${file.status})` });
  return diffTable(file);
}

function fileCard(file, i, hash) {
  const closed = file.add + file.del > BIG_FILE_LINES;
  const canView = file.status !== 'deleted' && !file.binary;
  return h('div', { class: `file${closed ? ' closed' : ''}`, id: `f${i}` },
    h('div', { class: 'f-head' },
      h('button', { type: 'button', class: 'tog', 'data-tog': i, 'aria-label': 'Collapse or expand file', text: '▾' }),
      h('span', { class: 'f-path', title: file.path, text: file.path }),
      h('span', { class: `f-status ${file.status}`, text: file.status }),
      stat(file.add, file.del, 'stat f-stat'),
      canView ? h('button', { type: 'button', class: 'f-btn', 'data-view': file.path, 'data-hash': hash, text: 'View file' }) : null),
    fileBody(file));
}

function metaRow(row, total, files) {
  const add = files.reduce((s, f) => s + f.add, 0);
  const del = files.reduce((s, f) => s + f.del, 0);
  const summary = h('span', {}, `${plural(files.length, 'file')} · `, stat(add, del));
  if (row.work) return h('div', { class: 'd-meta' }, h('span', { text: 'Not committed yet, the agent is probably still working' }), summary);
  const date = new Date(row.date);
  return h('div', { class: 'd-meta' },
    h('span', { text: `#${row.n} of ${total}` }),
    h('span', {}, h('b', { text: row.author }), row.agent && row.agent !== row.author ? ` · ${row.agent}` : ''),
    h('span', { title: row.date, text: date.toLocaleString() }),
    h('button', { type: 'button', class: 'copy', 'data-copy': row.hash, title: 'Copy full hash', text: `${row.short} ⧉` }),
    summary);
}

export function renderLoading(el) {
  el.replaceChildren(h('div', { class: 'detail' }, h('div', { class: 'placeholder short', text: 'Loading…' })));
}

export function renderError(el, message) {
  el.replaceChildren(h('div', { class: 'detail' }, h('div', { class: 'bin', text: message })));
}

export function renderPlaceholder(el, text = 'Pick a commit') {
  el.replaceChildren(h('div', { class: 'placeholder', text }));
}

/** row: a timeline row; files: parsePatch() output; total: number of commits. */
export function renderDetail(el, row, files, { truncated = false, total = 0 } = {}) {
  const body = row.work ? '' : stripTrailers(row.body);
  el.replaceChildren(h('div', { class: 'detail' },
    truncated ? h('div', { class: 'banner', role: 'status', text: 'Diff truncated (very large commit)' }) : null,
    h('div', { class: 'd-head' },
      h('h1', { text: row.subject }),
      body ? h('p', { class: 'd-body', text: body }) : null),
    metaRow(row, total, files),
    h('div', { class: 'files' }, files.map((f, i) => h('button', { type: 'button', class: 'chip', 'data-jump': i, title: f.path },
      baseName(f.path), h('i', { class: 'a', text: `+${f.add}` }), h('i', { class: 'd', text: `−${f.del}` })))),
    files.length ? files.map((f, i) => fileCard(f, i, row.hash)) : h('div', { class: 'bin', text: 'No file changes.' })));
}
