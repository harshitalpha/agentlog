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
