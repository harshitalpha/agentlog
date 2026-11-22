# Decisions

Choices made where the spec was ambiguous or the real world disagreed. Newest last.

## Build setup

- **Module path is a placeholder:** `github.com/OWNER/agentlog`. Replace `OWNER` in `go.mod`,
  Go imports, `plugin.json` and `marketplace.json` when the repo is published
  (see `RELEASING.md`).
- **Build repo identity:** this repo's commits use a repo-local `local-dev <local-dev@localhost>`
  with `commit.gpgsign=false`, because the machine had no global identity. Authorship can be
  rewritten before publishing if wanted.
- **`agentlog-build-docs/`** (the original spec pack) is gitignored; its contents were copied
  into `docs/` and `reference/`.
- **Version:** `main.version` defaults to `0.1.0`. The Makefile only overrides it when
  `VERSION=` is passed (used by `make dist`), so `make build` prints `0.1.0`.

## Git layer (M1)

- **`DirtyFiles` returns `[]DirtyFile{Path, Status}`** (not bare strings) so callers can
  distinguish untracked from modified without a second call.
- **`core.quotePath=false` on every git call.** Without it, git writes non-ASCII paths in diff
  headers as `"a/\303\274.txt"`. Paths containing a tab, `"` or `\` are still quoted by git, so
  `web/static/diff.js` must handle quoted `diff --git` / `---` / `+++` paths (C-style escapes).
- **`FileAt` uses `cat-file -s` + `cat-file blob`** instead of `show <h>:<p>`: it checks the size
  before reading (2 MB cap) and fails cleanly on directories. Adds `ErrTooLarge` and `ErrNotFound`
  alongside `ErrBinary`. The server maps `ErrTooLarge` to a 413-style JSON error.
- **Revisions are passed after `--end-of-options`** so a value starting with `-` can never be
  read as a git flag (defence in depth on top of the HTTP hash validation).
- **Git commit message cleanup** collapses runs of blank lines, so `Commit.Body` never contains
  more than one consecutive blank line.

- **`Commit` takes `CommitOpts{Identity, NoVerify, NoSign}`** instead of a bare `*Identity`, so
  options are explicit. `AutoCommit` sets all three: reserved `agentlog` identity via `git -c`,
  `--no-verify`, and `commit.gpgsign=false`. Signing is forced off because many users sign
  commits globally; signing under the reserved identity could fail or prompt, and the Stop-hook
  safety net must never lose work.
- **Auto-commit subject lists files in `git status` path order** (not edit order), truncated to
  100 runes.

## Later

- `WorkingPatch` spawns one `git diff --no-index` per untracked file (up to 200). Fine on
  normal machines; if `/api/patch?hash=WORKING` feels slow with many new files, run them in a
  small worker pool.
