// render.js: DOM builders. Everything is built with createElement/textContent,
// never innerHTML, so commit text and file paths cannot inject markup.

import { timeAgo, dayLabel } from './util.js';

/** h('div', {class: 'x', 'data-a': 1}, child, 'text') */
export function h(tag, props, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') el.className = v;
    else if (k === 'text') el.textContent = v;
    else el.setAttribute(k, v === true ? '' : String(v));
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) el.append(kid);
  return el;
}

/** "+3 −1" as two coloured spans. */
export function stat(add, del, cls = 'stat') {
  return h('span', { class: cls }, h('span', { class: 'a', text: `+${add}` }), ' ', h('span', { class: 'd', text: `−${del}` }));
}

function badge(row) {
  if (row.work) return h('span', { class: 'badge work', text: 'working tree' });
  if (!row.agent) return null; // human commits get no badge
  return h('span', { class: row.agent === 'auto' ? 'badge auto' : 'badge', text: row.agent });
}

function item(row, selected, now) {
  const kind = row.work ? 'work' : row.agent === 'auto' ? 'auto' : row.agent ? 'agent' : 'human';
  const meta = row.work
    ? [badge(row)]
    : [h('span', { class: 'num', text: `#${row.n}` }), badge(row), h('span', { text: timeAgo(row.date, now) }), stat(row.ins, row.dels)];
  return h('button', {
    type: 'button', class: `item ${kind}${row.hash === selected ? ' sel' : ''}`,
    'data-hash': row.hash, 'aria-current': row.hash === selected ? 'true' : null,
  }, h('span', { class: 'dot' }), h('div', { class: 'subj', text: row.subject }), h('div', { class: 'meta' }, meta));
}

/**
 * Fills the timeline element. `rows` come from buildRows; hasCommits tells
 * "nothing yet" apart from "filter matched nothing".
 */
export function renderTimeline(el, rows, { selected, hasCommits, now = new Date() }) {
  if (!rows.length) {
    el.replaceChildren(hasCommits
      ? h('div', { class: 'empty', text: 'No matches.' })
      : h('div', { class: 'empty' }, 'No commits yet.', h('br'), 'Ask your agent for a change and it will show up here.'));
    return;
  }
  const nodes = [];
  let lastDay = null;
  for (const row of rows) {
    const day = row.work ? 'Now' : dayLabel(row.date, now);
    if (day !== lastDay) {
      nodes.push(h('div', { class: 'day', text: day }));
      lastDay = day;
    }
    nodes.push(item(row, selected, now));
  }
  el.replaceChildren(...nodes);
}
