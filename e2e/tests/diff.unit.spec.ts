import { test, expect } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
// @ts-ignore plain JS module shared with the browser
import { parsePatch, unquotePath } from '../../web/static/diff.js';

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../testdata/patches');
const load = (n: string) => parsePatch(fs.readFileSync(path.join(dir, n + '.patch'), 'utf8'));
const brief = (fs_: any[]) => fs_.map((f) => [f.path, f.oldPath, f.status, f.binary, f.add, f.del, f.hunks.length]);

// path, oldPath, status, binary, add, del, hunks
const expected: Record<string, any[][]> = {
  'add': [['hello.txt', '', 'added', false, 2, 0, 1]],
  'binary': [['img.bin', '', 'added', true, 0, 0, 0]],
  'binary-modify': [['img.bin', 'img.bin', 'modified', true, 0, 0, 0]],
  'dash-lines': [['dash.txt', 'dash.txt', 'modified', false, 1, 1, 1]],
  'delete': [['hello.txt', 'hello.txt', 'deleted', false, 0, 2, 1]],
  'empty-file': [['empty.txt', '', 'added', false, 0, 0, 0]],
  'mode-change': [['run.sh', 'run.sh', 'modified', false, 0, 0, 0]],
  'multi-file': [
    ['moved.txt', 'after.txt', 'renamed', false, 0, 0, 0],
    ['multi-new.txt', '', 'added', false, 1, 0, 1],
    ['multi.bin', '', 'added', true, 0, 0, 0],
    ['new-name.txt', 'new-name.txt', 'deleted', false, 0, 4, 1],
    ['one.txt', 'one.txt', 'modified', false, 1, 0, 1],
  ],
  'multi-hunk': [['long.txt', 'long.txt', 'modified', false, 3, 3, 3]],
  'no-count-hunk': [['one.txt', 'one.txt', 'modified', false, 1, 1, 1]],
  'no-newline': [
    ['nonl.txt', 'nonl.txt', 'modified', false, 1, 1, 1],
    ['nonl2.txt', 'nonl2.txt', 'modified', false, 1, 1, 1],
  ],
  'quoted-path': [
    ['quote"name.txt', '', 'added', false, 1, 0, 1],
    ['tab\there.txt', '', 'added', false, 1, 0, 1],
  ],
  'rename-edit': [['after.txt', 'before.txt', 'renamed', false, 1, 1, 1]],
  'rename-pure': [['new-name.txt', 'old-name.txt', 'renamed', false, 0, 0, 0]],
  'root-commit': [
    ['README.md', '', 'added', false, 1, 0, 1],
    ['main.go', '', 'added', false, 3, 0, 1],
  ],
  'spaces-unicode': [['dir with space/héllo wörld.txt', '', 'added', false, 1, 0, 1]],
  'untracked': [['sub/draft.txt', '', 'added', false, 2, 0, 1]],
};

for (const [name, want] of Object.entries(expected)) {
  test(`parsePatch ${name}`, () => {
    expect(brief(load(name))).toEqual(want);
  });
}

test('every patch in testdata/patches has an expectation', () => {
  const have = fs.readdirSync(dir).filter((f) => f.endsWith('.patch')).map((f) => f.replace(/\.patch$/, '')).sort();
  expect(have).toEqual(Object.keys(expected).sort());
});

test('line numbers: multi-hunk', () => {
  const [f] = load('multi-hunk');
  for (const h of f.hunks) {
    const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(h.header)!;
    const first = h.lines[0];
    expect(first.old ?? first.new).toBe(first.type === 'add' ? +m[2] : +m[1]);
    // numbers increase by one per side
    let o = +m[1], n = +m[2];
    for (const l of h.lines) {
      if (l.type !== 'add') expect(l.old).toBe(o++); else expect(l.old).toBeNull();
      if (l.type !== 'del') expect(l.new).toBe(n++); else expect(l.new).toBeNull();
    }
  }
});

test('in-hunk lines that look like headers are content', () => {
  const [f] = load('dash-lines');
  const texts = f.hunks[0].lines.map((l: any) => [l.type, l.text]);
  expect(texts.some(([t, x]: string[]) => t === 'del' && x.startsWith('-- sql comment'))).toBe(true);
  expect(texts.some(([t, x]: string[]) => t === 'add' && x.startsWith('++ counter'))).toBe(true);
});

test('"No newline" marker is not a line', () => {
  for (const f of load('no-newline')) {
    for (const h of f.hunks) for (const l of h.lines) expect(l.text.startsWith('\\')).toBe(false);
  }
});

test('trailing tab after +++ path is stripped', () => {
  expect(load('spaces-unicode')[0].path).not.toMatch(/\t/);
});

test('degenerate input', () => {
  expect(parsePatch('')).toEqual([]);
  expect(parsePatch(undefined as any)).toEqual([]);
  expect(parsePatch('garbage\nnot a diff\n')).toEqual([]);
});

test('unquotePath decodes escapes and UTF-8 octal', () => {
  expect(unquotePath('"a\\tb"')).toBe('a\tb');
  expect(unquotePath('"q\\"x"')).toBe('q"x');
  expect(unquotePath('"\\303\\251"')).toBe('é');
  expect(unquotePath('plain')).toBe('plain');
});
