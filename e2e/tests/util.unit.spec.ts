import { test, expect } from '@playwright/test';
// @ts-ignore plain JS module shared with the browser
import { timeAgo, dayLabel, stripTrailers, baseName, plural } from '../../web/static/util.js';

const now = new Date('2026-11-22T12:00:00');
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();

test('timeAgo', () => {
  expect(timeAgo(ago(5_000), now.getTime())).toBe('just now');
  expect(timeAgo(ago(-60_000), now.getTime())).toBe('just now'); // clock skew
  expect(timeAgo('not a date', now.getTime())).toBe('just now');
  expect(timeAgo(ago(5 * 60_000), now.getTime())).toBe('5m ago');
  expect(timeAgo(ago(3 * 3600_000), now.getTime())).toBe('3h ago');
  expect(timeAgo(ago(49 * 3600_000), now.getTime())).toBe('2d ago');
});

test('dayLabel', () => {
  expect(dayLabel(new Date('2026-11-22T01:00:00').toISOString(), now)).toBe('Today');
  expect(dayLabel(new Date('2026-11-21T23:00:00').toISOString(), now)).toBe('Yesterday');
  expect(dayLabel(new Date('2026-11-10T10:00:00').toISOString(), now)).toMatch(/10 Nov 2026/);
});

test('stripTrailers removes trailer lines only', () => {
  const body = 'Why this change.\n\nAgent: claude-code\nCo-Authored-By: Claude <x@y>\nNote: keep me';
  expect(stripTrailers(body)).toBe('Why this change.\n\nNote: keep me');
  expect(stripTrailers('Body\n\nclaude-session: abc')).toBe('Body'); // case-insensitive
  expect(stripTrailers('Agent: x')).toBe('');
  expect(stripTrailers(undefined as any)).toBe('');
});

test('baseName and plural', () => {
  expect(baseName('a/b/c.txt')).toBe('c.txt');
  expect(baseName('c.txt')).toBe('c.txt');
  expect(plural(1, 'file')).toBe('1 file');
  expect(plural(2, 'file')).toBe('2 files');
});

test('buildRows: numbering, working row, order, filter', async () => {
  // @ts-ignore
  const { buildRows } = await import('../../web/static/util.js');
  const commits = [
    { hash: 'c3', short: 'c3', subject: 'Third', body: '', agent: 'Claude', date: '2026-11-22T10:00:00Z' },
    { hash: 'c2', short: 'c2', subject: 'Second', body: 'mentions widget', agent: '', date: '2026-11-21T10:00:00Z' },
    { hash: 'c1', short: 'c1', subject: 'First', body: '', agent: 'auto', date: '2026-11-20T10:00:00Z' },
  ];
  const hashes = (r: any[]) => r.map((x) => x.hash);
  expect(buildRows([], 0)).toEqual([]);
  const newest = buildRows(commits, 2);
  expect(hashes(newest)).toEqual(['WORKING', 'c3', 'c2', 'c1']);
  expect(newest.map((r: any) => r.n)).toEqual([0, 3, 2, 1]);
  expect(newest[0].subject).toBe('Uncommitted changes (2 files)');
  expect(buildRows(commits, 1)[0].subject).toBe('Uncommitted changes (1 file)');
  expect(hashes(buildRows(commits, 2, { order: 'old' }))).toEqual(['c1', 'c2', 'c3', 'WORKING']);
  expect(hashes(buildRows(commits, 0))).toEqual(['c3', 'c2', 'c1']);
  expect(hashes(buildRows(commits, 0, { query: 'WIDGET' }))).toEqual(['c2']);
  expect(hashes(buildRows(commits, 0, { query: 'auto' }))).toEqual(['c1']);
  expect(hashes(buildRows(commits, 0, { query: 'zzz' }))).toEqual([]);
  const files = new Map([['c3', ['internal/repo/orders.go']]]);
  expect(hashes(buildRows(commits, 0, { query: 'orders.go', filesByHash: files }))).toEqual(['c3']);
  expect(hashes(buildRows(commits, 0, { query: 'orders.go' }))).toEqual([]); // patch not loaded yet
});
