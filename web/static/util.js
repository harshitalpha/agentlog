// util.js: small pure helpers shared by the UI (no DOM access, easy to test in Node).

const SECOND = 1000, MINUTE = 60 * SECOND, HOUR = 60 * MINUTE, DAY = 24 * HOUR;

/** "just now", "5m ago", "3h ago", "2d ago". `now` is injectable for tests. */
export function timeAgo(iso, now = Date.now()) {
  const ms = now - new Date(iso).getTime();
  if (!(ms >= MINUTE)) return 'just now'; // also covers clock skew and bad dates
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)}m ago`;
  if (ms < DAY) return `${Math.floor(ms / HOUR)}h ago`;
  return `${Math.floor(ms / DAY)}d ago`;
}

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** Day header: "Today", "Yesterday", otherwise "Sat, 21 Nov 2026". `now` is injectable. */
export function dayLabel(iso, now = new Date()) {
  const d = new Date(iso);
  if (sameDay(d, now)) return 'Today';
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (sameDay(d, yesterday)) return 'Yesterday';
  return d.toLocaleDateString('en-GB', { weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' });
}

const TRAILER_RE = /^(Co-Authored-By|Agent|Claude-Session|Signed-off-by):/i;

/** A commit body without its trailer lines (Agent:, Co-Authored-By:, ...), trimmed. */
export function stripTrailers(body) {
  return (body || '')
    .split('\n')
    .filter((line) => !TRAILER_RE.test(line))
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}

/** Last path segment. */
export function baseName(path) {
  const i = path.lastIndexOf('/');
  return i < 0 ? path : path.slice(i + 1);
}

/** "1 file", "2 files". */
export function plural(n, word) {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}

/**
 * Timeline rows in display order. `commits` is newest first (the API order).
 * n is the position counted from the oldest commit (1 = first). The working-tree
 * pseudo-row (hash "WORKING", work: true) exists only when dirty > 0; it sits
 * first in "new" order and last in "old" order.
 * filesByHash maps a hash to the paths of its already-loaded patch (filter input).
 */
export function buildRows(commits, dirty, { query = '', order = 'new', filesByHash = new Map(), now = new Date() } = {}) {
  let rows = commits.map((c, i) => ({ ...c, n: commits.length - i }));
  if (dirty > 0) {
    rows.unshift({
      hash: 'WORKING', short: '', subject: `Uncommitted changes (${plural(dirty, 'file')})`, body: '',
      author: '', agent: '', date: now.toISOString(), ins: 0, dels: 0, n: 0, work: true,
    });
  }
  const q = (query || '').trim().toLowerCase();
  if (q) {
    rows = rows.filter((r) => {
      const text = [r.subject, r.body, r.short, r.agent].join('\n').toLowerCase();
      return text.includes(q) || (filesByHash.get(r.hash) || []).some((p) => p.toLowerCase().includes(q));
    });
  }
  if (order === 'old') rows.reverse();
  return rows;
}
