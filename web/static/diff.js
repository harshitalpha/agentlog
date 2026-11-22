// diff.js: pure unified-diff parser (no DOM, no globals) so Node can test it.
//
//   parsePatch(text) -> File[]
//   File = { path, oldPath, status: 'added'|'deleted'|'renamed'|'modified', binary, add, del,
//            hunks: [{ header, lines: [{ type: 'add'|'del'|'ctx', old: number|null, new: number|null, text }] }] }
//
// It understands the output of `git show` / `git diff` with a/ and b/ prefixes: new/deleted
// file mode, rename from/to, similarity index, binary files, "\ No newline at end of file",
// /dev/null on either side, hunk headers without counts, paths with spaces and C-style
// quoted paths ("a/tab\there.txt").

const HUNK_RE = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/** Decodes a git C-style quoted path ("a\tb", octal escapes for raw bytes). Unquoted input is returned as is. */
export function unquotePath(s) {
  if (s.length < 2 || s[0] !== '"' || s[s.length - 1] !== '"') return s;
  const body = s.slice(1, -1);
  const bytes = [];
  const encoder = new TextEncoder();
  const simple = { a: 7, b: 8, f: 12, n: 10, r: 13, t: 9, v: 11, '\\': 92, '"': 34 };
  for (let i = 0; i < body.length; ) {
    const ch = body[i];
    if (ch !== '\\') {
      const cp = body.codePointAt(i);
      const str = String.fromCodePoint(cp);
      bytes.push(...encoder.encode(str));
      i += str.length;
      continue;
    }
    const next = body[i + 1];
    if (next in simple) {
      bytes.push(simple[next]);
      i += 2;
    } else if (/^[0-7]{3}$/.test(body.slice(i + 1, i + 4))) {
      bytes.push(parseInt(body.slice(i + 1, i + 4), 8) & 0xff);
      i += 4;
    } else { // unknown escape: keep it literally
      bytes.push(92);
      i += 1;
    }
  }
  return new TextDecoder().decode(new Uint8Array(bytes));
}

/** Reads one possibly-quoted token from the start of s. Returns [token, rest]. */
function readQuoted(s) {
  let i = 1;
  while (i < s.length) {
    if (s[i] === '\\') i += 2;
    else if (s[i] === '"') break;
    else i += 1;
  }
  return [s.slice(0, i + 1), s.slice(i + 1)];
}

/** Paths from "diff --git <a> <b>" (the part after "diff --git "). Rename lines refine them later. */
function headerPaths(rest) {
  const strip = (p, prefix) => (p.startsWith(prefix) ? p.slice(prefix.length) : p);
  if (rest.startsWith('"')) {
    const [first, after] = readQuoted(rest);
    const second = after.replace(/^ /, '');
    return [strip(unquotePath(first), 'a/'), strip(unquotePath(second), 'b/')];
  }
  const quotedSecond = rest.indexOf(' "b/');
  if (quotedSecond > 0) {
    return [strip(rest.slice(0, quotedSecond), 'a/'), strip(unquotePath(rest.slice(quotedSecond + 1)), 'b/')];
  }
  // Both unquoted. Equal paths ("a/P b/P") can be split exactly even with spaces in P.
  if (rest.startsWith('a/')) {
    const len = (rest.length - 5) / 2;
    if (Number.isInteger(len) && len > 0 && rest.slice(2 + len, 5 + len) === ' b/' &&
        rest.slice(2, 2 + len) === rest.slice(5 + len)) {
      return [rest.slice(2, 2 + len), rest.slice(2, 2 + len)];
    }
  }
  const m = rest.match(/^a\/(.+?) b\/(.+)$/);
  return m ? [m[1], m[2]] : [rest, rest];
}

/** Path from a "--- " / "+++ " line. null means /dev/null. */
function markerPath(s, prefix) {
  const raw = s.replace(/\t$/, ''); // git appends a tab after paths that contain spaces
  if (raw === '/dev/null') return null;
  const p = unquotePath(raw);
  return p.startsWith(prefix) ? p.slice(prefix.length) : p;
}

function newFile(headerRest) {
  const [oldHeader, newHeader] = headerPaths(headerRest);
  return {
    // working state, folded into the public shape by finish()
    _oldHeader: oldHeader, _newHeader: newHeader,
    _isNew: false, _isDeleted: false, _renameFrom: null, _renameTo: null,
    _minus: undefined, _plus: undefined,
    path: newHeader, oldPath: oldHeader, status: 'modified', binary: false, add: 0, del: 0, hunks: [],
  };
}

function finish(f) {
  const isRename = f._renameFrom !== null || f._renameTo !== null;
  f.status = isRename ? 'renamed' : f._isNew ? 'added' : f._isDeleted ? 'deleted' : 'modified';
  const oldSide = f._renameFrom ?? (f._minus || f._oldHeader);
  const newSide = f._renameTo ?? (f._plus || f._newHeader);
  f.path = f.status === 'deleted' ? oldSide : newSide;
  f.oldPath = f.status === 'added' ? '' : oldSide;
  for (const k of Object.keys(f)) if (k[0] === '_') delete f[k];
  return f;
}

export function parsePatch(text) {
  const files = [];
  if (!text) return files;

  let file = null;
  let hunk = null;
  let remOld = 0, remNew = 0, oldNo = 0, newNo = 0;

  for (const line of text.split('\n')) {
    if (line.startsWith('diff --git ')) {
      file = newFile(line.slice('diff --git '.length));
      files.push(file);
      hunk = null;
      continue;
    }
    if (!file) continue;

    if (line.startsWith('@@')) {
      const m = HUNK_RE.exec(line);
      hunk = { header: line, lines: [] };
      file.hunks.push(hunk);
      if (m) {
        oldNo = +m[1]; newNo = +m[3];
        remOld = m[2] === undefined ? 1 : +m[2];
        remNew = m[4] === undefined ? 1 : +m[4];
      } else { // unreadable header: accept everything until the next hunk
        oldNo = newNo = 0;
        remOld = remNew = Infinity;
      }
      continue;
    }

    if (hunk) {
      // Inside a hunk a line is content, whatever it looks like: "--- x" is a
      // removed "-- x". The counts from the header say when the hunk is over.
      if (line.startsWith('\\')) continue; // "\ No newline at end of file"
      if (remOld <= 0 && remNew <= 0) continue;
      const c = line[0];
      if (c === '+') {
        hunk.lines.push({ type: 'add', old: null, new: newNo++, text: line.slice(1) });
        file.add++; remNew--;
      } else if (c === '-') {
        hunk.lines.push({ type: 'del', old: oldNo++, new: null, text: line.slice(1) });
        file.del++; remOld--;
      } else if (c === ' ' || (line === '' && remOld > 0 && remNew > 0)) {
        hunk.lines.push({ type: 'ctx', old: oldNo++, new: newNo++, text: line.slice(1) });
        remOld--; remNew--;
      }
      continue;
    }

    // File header lines (before the first hunk).
    if (line.startsWith('new file mode')) file._isNew = true;
    else if (line.startsWith('deleted file mode')) file._isDeleted = true;
    else if (line.startsWith('rename from ')) file._renameFrom = unquotePath(line.slice(12));
    else if (line.startsWith('rename to ')) file._renameTo = unquotePath(line.slice(10));
    else if (line.startsWith('copy from ')) { file._isNew = true; file._renameFrom = null; }
    else if (line.startsWith('Binary files ') || line.startsWith('GIT binary patch')) {
      file.binary = true;
      if (line.startsWith('Binary files /dev/null and ')) file._isNew = true;
      else if (line.endsWith(' and /dev/null differ')) file._isDeleted = true;
    } else if (line.startsWith('--- ')) {
      const p = markerPath(line.slice(4), 'a/');
      if (p === null) file._isNew = true; else file._minus = p;
    } else if (line.startsWith('+++ ')) {
      const p = markerPath(line.slice(4), 'b/');
      if (p === null) file._isDeleted = true; else file._plus = p;
    }
  }
  return files.map(finish);
}
