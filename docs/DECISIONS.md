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

## Status and disable (M2)

- **"Commits since baseline"** counts commits newer than the `Baseline snapshot before agent work`
  commit, excluding init's own `Add agentlog commit rules…` commit, split by `DetectAgent`
  (`""` → human, `auto` → auto, anything else → agent). With no baseline commit (e.g. `init` on a
  clean repo) every commit counts. At most 5000 commits are scanned.
- **`status --json` fields:** `repo, enabled, enabled_via ("repo"|"enforce_everywhere"|""), branch,
  has_baseline, commits_since_baseline{agent,human,auto}, uncommitted_files, ui_url`.
- **`runningUIURL` and `stopRunningUI`** in `cli/status.go` are placeholders for `uiproc`; they are
  wired in T3.4 (status shows the URL, disable stops the server).

## Standalone mode and hook dedupe (M2)

- **Lock key includes `stop_hook_active`:** `.git/agentlog/stop-<session>-<0|1>`. The spec's literal
  `stop-<session_id>` with a 5 s TTL would swallow Claude's genuine second Stop attempt when it
  arrives within 5 s of the first (a fast "ok" reply), skipping the auto-commit safety net.
  Identical duplicate invocations (plugin + standalone) share the key; the real second attempt does
  not. Tested both ways.
- **SessionStart is deduped too** (`start-<session>`), so the rules aren't injected twice when both
  the plugin and the standalone hooks are present. Locks are only created after the enabled check,
  so repos that never opted in never get a `.git/agentlog` directory.
- **Session ids are sanitised** (`[A-Za-z0-9_-]`, max 64 chars) before becoming file names. Locks are
  created with `O_EXCL`; stale `stop-*`/`start-*` files (>1 min) are swept, never other files.
- **Settings merge re-serialises JSON** (2-space indent, keys sorted), so formatting and key order of
  `settings.local.json` may change. Values are preserved exactly (numbers use `json.Number`); all
  other keys and hooks are kept. Invalid JSON is never touched; `init` prints a ready-to-paste snippet.
- **`uninstall` deletes `settings.local.json` if it ends up `{}`** (and the `.claude` dir if empty).
- **`gitx.Repo.GitPath`** resolves paths inside the git dir with `rev-parse --git-path`, so linked
  worktrees put `agentlog/` state in the shared git dir.

## Watch (M2)

- **`WorkingSignature` adds size and mtime of each dirty file** (first 500) to the spec's
  `status` + `diff --stat`, so re-editing a file to an identical diff stat still counts as activity.
- `--quiet` and `--interval` accept fractional seconds (`Float64`), which keeps tests fast.
- `streams` carries a `ctx` (cancelled on SIGINT/SIGTERM); `watch` returns 0 on cancel.

## Build incident: tests ran against this repo (fixed)

From T2.6 until this fix, `TestStubsDispatch` ran every subcommand with no arguments before
skipping the implemented ones, so `init`/`disable`/`uninstall` (and later `watch`, which never
returns) operated on the agentlog repo itself. Effects:

- Stray commits `1d8a10f`, `13087b5`, `425bfce`, `d957f53`, `48ddcca`, `a8621eb`, `e5f89de`
  ("Baseline snapshot before agent work" / "Add agentlog commit rules…") were created by tests, not
  by a task. The baseline commits swept up the uncommitted work of T2.6-T2.9, so code for those tasks
  lives in them rather than in the like-named task commits (`115a470`, `a8e4e24`, `52be86c`).
- `AGENTS.md` briefly gained a rules block (restored from the spec pack) and `.gitignore` gained
  the `.claude/settings.local.json`, `.env`, `.env.*` lines (kept; harmless).
- No work was lost and nothing was pushed. History was not rewritten (CLAUDE.md forbids it).

Fix: the stub test only runs commands that are still stubs; the `cli` and `agent` test binaries
`chdir` into a throwaway directory in `TestMain`; `TestWorkingDirectoryIsNotAGitRepo` fails if tests
ever run inside a repo. Verified: a full `go test ./...` leaves HEAD, status and git config unchanged.
**Rule for new tests:** never run a command whose default path is `.` without an explicit temp dir.

## HTTP server (M3)

- **Handler chain:** recover → Host check → GET-only → idle bookkeeping → routes. Host is checked
  before the method (a POST with a bad Host is 403), and idle time is only refreshed by requests that
  pass the Host check, so a hostile page cannot keep the server alive.
- **Host check is exact:** `127.0.0.1:<port>` or `localhost:<port>` (case-insensitive). `[::1]`, a
  missing port and look-alikes (`127.0.0.1.evil.com`) are 403, as the spec says.
- **`limit` query:** non-integer or `< 1` is 400; above 5000 is clamped.
- **`/api/file` errors:** unknown path 404, directory 404, over 2 MB 413 `file too large`, binary
  `200 {"content":"","binary":true}`. `WORKING` reads from disk only after `EvalSymlinks` proves the
  file is inside the repo; symlinks escaping the repo are 404 (a symlink that stays inside works).
- **`/api/patch` for an unknown commit is 404** `commit not found`; `truncated` is omitted when false.
- **Non-canonical static paths** (`/../x`, `//x`) get ServeMux's 307 to the cleaned path, whose
  answer is 404. Only files in the embedded FS are ever served, with `nosniff`.
- **`/api/repo` runs its three git calls concurrently** to stay well under 100 ms on slow git.
- **`Serve` returns `ErrIdleTimeout`** after 8 h without requests (treated as a normal exit by the
  CLI) and nil when cancelled. Ports: try `port..port+10`, `0` means any free port.

## UI process management (M3)

- **A pid is never trusted on its own.** `Stop` only signals a process after `GET /api/repo` on the
  recorded URL answers with this repo's path (pids get reused; `ui.json` is a plain file anyone can
  corrupt). A live pid that is not our server is left alone (`NotOurs`: stale file removed, nothing
  killed). Pids `<= 1` are never signalled: `kill(0)` and `kill(-1)` address whole process groups.
  `Running` applies the same check, so repo B can never adopt repo A's server.
- **`--detach` prints just the URL** (one line) so the `/agentlog:ui` skill and scripts can use it
  directly. It opens the browser only when it starts a *new* server (not on reuse) and never with
  `--no-open`.
- **The parent learns the real port from `ui.log`** (the child's `→ http://127.0.0.1:<port>` banner),
  because the child may fall back across ports. The log is truncated on every spawn so a stale line
  can't be mistaken for the new server. A child that exits at once, or doesn't answer within 3 s, is
  reported with the last log line and cleaned up.
- **The child is re-executed as `ui --no-open --port N <repo-root>` in its own session** (`Setsid`
  on unix; `CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS` on Windows, compile-checked only). The spawner
  keeps a reaper goroutine so long-lived callers don't accumulate zombies (a zombie still answers
  `kill(pid, 0)`).
- **`DetachOptions.Exe` / `cli.uiExecutable` are injectable** because under `go test`
  `os.Executable()` is the test binary, not agentlog.
- **Stop output:** `✓ stopped`, `agentlog UI is not running` (exit 0), or a stderr note for `NotOurs`.
  `disable` and `uninstall` stop the server silently; `status` shows the URL only for a verified server.
- **Worktrees share `.git/agentlog/`** (git's common dir), so two worktrees of one repo share one
  `ui.json`. The path check means the second won't reuse the first's server, but it overwrites the
  record, so the first becomes an orphan that exits via the 8 h idle timeout. Acceptable for v1.
- **Stale `stop-*`/`start-*` hook locks** are swept lazily by the next hook run (>1 min old).

## Web UI (M4)

- **No `innerHTML` anywhere; DOM is built with `createElement`/`textContent`.** Commit subjects, bodies
  and file paths are untrusted text. This removes the XSS class instead of relying on escaping, so
  `util.js` has no `esc` (the spec lists one).
- **Strict CSP on the UI:** `default-src 'none'; script-src 'self'; style-src 'self'; connect-src
  'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`. No inline
  scripts or `style=""` attributes (JS sets styles through the CSSOM, which CSP allows).
- **Test tooling:** `npm` is broken on this machine (invalid auth entry for an internal registry in the
  user's npmrc, which was not touched) and pnpm's offline mirror lacks Playwright. `e2e/node_modules` is
  a local, git-ignored symlink to an existing Playwright 1.65.0 install in another project on this
  machine, and the tests are run with `node e2e/node_modules/@playwright/test/cli.js` instead of `npx`.
  On a normal machine `npm install` in `e2e/` produces the same layout.
- **Patch flags are pinned (T4.2):** `CommitPatch`, `WorkingPatch` and the untracked `diff --no-index`
  call pass `--no-ext-diff --no-textconv --src-prefix=a/ --dst-prefix=b/`. Without them a user's
  `diff.noprefix`, `diff.mnemonicPrefix` or `diff.external` changes the patch shape and breaks the parser.
  `TestPatchesIgnoreUserDiffConfig` covers it.
- **Parser fixtures are real git output:** `testdata/patches/generate.sh` builds a scratch repo and writes
  the 17 `.patch` files, so they can be regenerated. `e2e/tests/diff.unit.spec.ts` fails if a patch has no
  expectation. `parsePatch` (in `web/static/diff.js`, no DOM) uses the hunk header counts to decide when
  a hunk ends, so in-hunk lines like `--- sql comment` stay content. Paths come from `rename from/to`
  and `---`/`+++` (trailing tab stripped), with the `diff --git` line only as fallback.
- **`stripTrailers` is case-insensitive** (like the prototype) and only removes `Agent:`,
  `Co-Authored-By:`, `Claude-Session:` and `Signed-off-by:` lines.

- **UI module split (T4.3-T4.6):** `docs/03` lists `app.js`, `diff.js`, `render.js`, `util.js`. The
  detail/diff/modal builders live in an extra `detail.js` so no file passes ~400 lines. `buildRows`
  (numbering, working-tree row, filter, order) is in `util.js` so Node can unit test it.
- **Deep links:** `#<hash>` accepts a full hash or a prefix of at least 4 characters. An unknown or
  too-short hash falls back to the top item. Selection uses `history.replaceState` (no history entry per
  click), and the working tree is never written to the URL because it is transient.
- **Selection survives the filter:** "does the selection still exist" and the detail lookup use the
  unfiltered rows, so typing in the filter never swaps the commit being read.
- **Polling:** one request chain at a time (a slow server never gets stacked polls). The live dot goes grey
  after 2 consecutive failures, back to green on the next success.

## Plugin packaging (T5.1, checked against code.claude.com on 2026-11-22)

Compared `docs/11-plugin-packaging.md` and `docs/05-agent-integration.md` §3 with the current manifest
reference and marketplace reference. Everything in the spec still holds except these points:

- **`${CLAUDE_PROJECT_DIR}` in skills:** the docs say the path variables are *not* in the environment of
  commands Claude runs through the Bash tool, but are substituted inline in skill bodies. Skills therefore
  write the literal `${CLAUDE_PROJECT_DIR}` (quoted) instead of `$CLAUDE_PROJECT_DIR`. Verified in T5.6.
- **`git-subdir` takes `ref`** (and `sha`, a full 40-char commit). `url` accepts the `owner/repo` shorthand,
  so the marketplace entry in the spec is valid as written. No fallback to a separate plugin repo needed.
- **`userConfig`:** every option needs `type`, `title` and `description` (the spec has them) and unknown keys
  inside an option are an error, so none are added. Keys become `CLAUDE_PLUGIN_OPTION_<KEY>` uppercased:
  `ENFORCE_EVERYWHERE` and `UI_PORT`, which the binary already reads.
- **Extra directory-listing fields exist** (`privacyPolicyUrl`, `termsOfServiceUrl`). The plugin sets
  `privacyPolicyUrl` as the spec recommends, in `plugin.json` only (a marketplace entry would warn).
- **`engines` is not set:** it needs Claude Code v2.1.323+ and older versions reject the manifest.
- **Names:** `agentlog` is neither a reserved plugin nor marketplace name.
- **Hooks:** the file keeps the `"hooks"` wrapper and the quoted `"${CLAUDE_PLUGIN_ROOT}"` shell form.
- **Windows:** PowerShell-only machines are unsupported in v1 (launcher is POSIX sh; Git Bash works).

## Local plugin run (T5.6, claude 2.1.323, `claude -p --plugin-dir ./plugin`, scratch dirs under /tmp)

- `/agentlog:init` on a folder with no git: repo, baseline commit (`.env` stayed untracked), rules commit,
  `agentlog.enabled=true`. Run again: HEAD unchanged. This confirms `${CLAUDE_PROJECT_DIR}` is substituted
  inline in skill bodies as the docs say.
- `/agentlog:status` reported the repo correctly. `/agentlog:ui` twice printed the same URL and left one
  server process; `/agentlog:ui stop` removed it.
- Stop hook, enabled repo: told "create a file, do not run git", Claude was blocked once and then the second
  attempt auto-committed (`agentlog` author, `[auto]` subject, `Agent: auto`), tree clean. Claude does
  follow an explicit "don't run git" over the hook, so the `stop_hook_active` auto-commit is what guarantees
  the result.
- Stop hook, repo where init never ran: one turn, no block, no commit, no `.git/agentlog/` created.
- `/agentlog:disable`: commits untouched, enforcement off, a following edit finished without a block.
- Live checks on the UI: `Host: evil.com` gives 403, POST gives 405.
- `make validate` (strict plugin validation and marketplace validation) passes, and `make dist` builds all
  five platforms (this also compiles the Windows spawn code for real).

## Later

- `WorkingPatch` spawns one `git diff --no-index` per untracked file (up to 200). Fine on
  normal machines; if `/api/patch?hash=WORKING` feels slow with many new files, run them in a
  small worker pool.
