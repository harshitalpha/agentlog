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
