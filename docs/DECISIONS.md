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

## Hooks (M2)

- **Hook output shapes verified against https://code.claude.com/docs/en/hooks (2026-11-22):**
  SessionStart context goes in `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"…"}}`
  (also supports `sessionTitle`; plain stdout works too, but we print JSON as the spec says).
  Stop input has `stop_hook_active` (true when Claude is already continuing because of a stop hook)
  and blocks with `{"decision":"block","reason":"…"}`. No change to the spec was needed.
- **Docs mention an 8-consecutive-continuation cap** on Stop hooks (resets when Claude calls a
  tool). Our design (block once, auto-commit on the second try) stays far below it.
- **Output limit:** hook `additionalContext` is capped at 10,000 characters; the rules text is
  ~1.3 KB and a test guards the cap.
- **Not-enabled hint is on by default.** In a git repo where the plugin is installed but not
  opted in, SessionStart adds the one-line `NotEnabledHint` (spec §2 says "may"). It never blocks.
  If it proves noisy, drop it or gate it behind a plugin option.
- **Malformed hook stdin → no output, exit 0.** Empty stdin is treated as `{}` and falls back to
  `$CLAUDE_PROJECT_DIR`, then the working directory.
- **Hook errors** go to stderr and `${CLAUDE_PLUGIN_DATA}/agentlog.log`; `runHook` also recovers
  from panics so a hook can never exit non-zero.

## Init (M2)

- **Init logic lives in `internal/agent/init.go`** (`agent.Init`), with `cli/init.go` only parsing
  flags. This keeps the "cli holds no business logic" rule; the file is an addition to the layout in
  `03-project-structure.md`, inside an existing package.
- **Baseline need is decided before `.gitignore` is edited.** Spec order is: `.gitignore`, baseline,
  enable, rules, commit. If we evaluated "is the tree dirty" after step 4, every existing clean repo
  would get a "baseline" containing only our `.gitignore` edit. So: decide first, but still write
  `.gitignore` before the baseline's `git add -A` so `.env` files are never committed (tested).
- **Step 9 commits only the files init touched** (`.gitignore`, `AGENTS.md`, `CLAUDE.md`) through a
  new `gitx.CommitPaths`, so with `--no-baseline` the user's other uncommitted edits stay uncommitted.
- **init commits are human commits** (user's identity, no `Agent:` trailer), so they show without a
  badge in the UI and count as "human" in `status`.
- **Refuses to `git init` in `$HOME` or `/`.** The baseline runs `git add -A`; doing that over a home
  directory would be a disaster. Subfolders of home are fine, and existing repos are never refused.
- **Flags may follow the path** (`init <dir> --claude-md`); Go's `flag` stops at the first positional
  so `cli/args.go` re-parses around it.
- **Init commits are signed per the user's own git config** (they are the user's commits). Only
  auto-commits force signing off.
- `--standalone-hook` returns "not implemented yet" until T2.8.

## Status and disable (M2)

- **"Commits since baseline"** counts commits newer than the `Baseline snapshot before agent work`
  commit, excluding init's own `Add agentlog commit rules…` commit, split by `DetectAgent`
  (`""` → human, `auto` → auto, anything else → agent). With no baseline commit (e.g. `init` on a
  clean repo) every commit counts. At most 5000 commits are scanned.
- **`status --json` fields:** `repo, enabled, enabled_via ("repo"|"enforce_everywhere"|""), branch,
  has_baseline, commits_since_baseline{agent,human,auto}, uncommitted_files, ui_url`.
- **`runningUIURL` and `stopRunningUI`** in `cli/status.go` are placeholders for `uiproc`; they are
  wired in T3.4 (status shows the URL, disable stops the server).

## Later

- `WorkingPatch` spawns one `git diff --no-index` per untracked file (up to 200). Fine on
  normal machines; if `/api/patch?hash=WORKING` feels slow with many new files, run them in a
  small worker pool.
