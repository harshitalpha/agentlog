# 10: Tasks

Work top to bottom. Tick each box only when its acceptance criteria pass. Commit after each task.

## M0: Scaffold

- [x] **T0.1 Project skeleton.** The layout from `03-project-structure.md` (empty packages with doc
  comments, empty `plugin/` folders), `go.mod` (Go 1.22), `.gitignore`, and the Makefile with `build`, `check`, `install`.
  *Accept:* `make build && make check` pass. `bin/agentlog --version` prints `0.1.0`.
- [x] **T0.2 CLI dispatch.** `cli.Run` with every subcommand from `04-cli.md` as a stub, `--help` text, and exit codes.
  *Accept:* unknown command → usage on stderr, exit 2. `cli_test.go` covers dispatch.

## M1: Git layer

- [x] **T1.1 Runner + repo basics.** `run.go`, `repo.go`, `identity.go`, `config.go` (local get/set/unset).
- [x] **T1.2 Status.** `DirtyFiles` with `-z` parsing and renames.
- [x] **T1.3 Log.** Format + numstat parser, limit handling.
- [x] **T1.4 Patches and files.** `CommitPatch`, `WorkingPatch` (untracked, empty-tree base), `FileAt`.
- [x] **T1.5 Commit helpers.** `AddAll`, `StagedEmpty`, `Commit`, `AutoCommit` (`05-agent-integration.md` §7).
  *Accept (M1):* all 10 edge-case tests in `06-git-layer.md` pass.

## M2: Agent logic + CLI commands

- [x] **T2.1 Rules.** Embed `rules.md`. `UpsertRules` / `RemoveRules` with tests.
- [x] **T2.2 Enabled check.** `IsEnabled(repo, env)`: git config or `enforce_everywhere`.
- [x] **T2.3 SessionStart hook.** `HandleSessionStart` + `agentlog hook-session-start`.
- [x] **T2.4 Stop hook.** `HandleStop` + `agentlog hook-stop`. Enabled check first; never non-zero; JSON-only stdout.
- [x] **T2.5 Detection.** `DetectAgent` table tests.
- [x] **T2.6 `agentlog init`.** All 10 steps, idempotent.
- [x] **T2.7 `agentlog disable` and `agentlog status`** (incl. `--json`).
- [x] **T2.8 Standalone mode.** `init --standalone-hook`, `uninstall`, settings merge, session-lock dedupe.
- [x] **T2.9 `agentlog watch`.**
- [x] **T2.10 Scripts.** `fake-claude-stop.sh`, `fake-session-start.sh`, `make-demo-repo.sh` (`09-testing.md` §3).
  *Accept (M2):* the demo repo has exactly the expected history, and the "not enabled + dirty → silent" test passes.

## M3: HTTP server + UI process

- [x] **T3.1 Server + middleware.** Localhost bind, Host check, recover, GET-only, port fallback, idle timeout.
- [x] **T3.2 Endpoints.** `/api/repo`, `/api/commits`, `/api/patch`, `/api/file`, embedded static.
- [x] **T3.3 `agentlog ui` foreground.** Browser open, clean shutdown, port from flag/env/default.
- [x] **T3.4 `uiproc`: `--detach` / `--stop`.** Reuse a live server, spawn detached, `ui.json`, log file.
  *Accept (M3):* all tests in `07-http-api.md` pass. `ui --detach` twice gives the same PID and URL, and `--stop` kills it.

## M4: Web UI

- [x] **T4.1 Shell + tokens.** `index.html`, `styles.css`, header, two-column layout.
- [x] **T4.2 diff.js.** `parsePatch`, `testdata/patches/`, Node unit tests.
- [x] **T4.3 Timeline.** Day groups, dots, badges, numbering, working-tree item, both orders, empty states.
- [x] **T4.4 Detail.** Header, trailer stripping, meta, copy hash, chips, file cards, colgroup diff table, collapse, large-file default.
- [x] **T4.5 Modal.** View file at a commit or in the working tree.
- [x] **T4.6 Behaviour.** Polling, live dot, filter, keyboard, URL-hash deep links, stale-response guard, persisted order.
  *Accept (M4):* compared with `reference/screenshots/`, it looks at least as good, and the diff number columns are narrow.

## M5: Plugin packaging

- [x] **T5.1 Re-read the plugin docs.** Check `11-plugin-packaging.md` against the current Claude Code plugin docs
  (manifest reference, components, hooks reference for the SessionStart output, marketplace reference for
  `git-subdir` + `ref`). Record any differences in `DECISIONS.md` and follow the docs.
- [x] **T5.2 Plugin shell.** `plugin.json`, `hooks/hooks.json`, launcher (executable bit), `assets/icon.png`
  (simple and original, a timeline/commit motif; don't use Anthropic or Claude branding), `plugin/README.md`, `LICENSE`.
- [x] **T5.3 Skills.** `init`, `ui`, `disable`, `status` SKILL.md files per `11-plugin-packaging.md` §7.
- [x] **T5.4 Build targets.** `make dist`, `make dist-local`, `make validate`. The launcher test from `09-testing.md` §5.
- [x] **T5.5 Marketplace file.** Root `.claude-plugin/marketplace.json`.
- [x] **T5.6 Local plugin run.** `make dist-local && claude --plugin-dir ./plugin`: walk through the plugin items of the manual checklist.
  *Accept (M5):* `make validate` passes (`--strict`). All 4 skills work in a real Claude Code session. The Stop hook
  blocks in the enabled repo and stays silent in a non-enabled repo.

## M6: Tests, docs, release

- [x] **T6.1 Playwright e2e.** Every scenario in `09-testing.md` §4. `make e2e` green.
- [x] **T6.2 CI.** `.github/workflows/ci.yml` (check + e2e) and `release.yml` (tag → `make dist` → push the `release` branch).
- [x] **T6.3 README.md.** Install via `/plugin marketplace add` + `/plugin install`, quickstart (`/agentlog:init`, work, `/agentlog:ui`),
  how enforcement and opt-in work, the `enforce_everywhere` option, standalone install, other agents (+ `watch`),
  Windows note, uninstall and disable, FAQ ("Will this push my code?" No. "Will it block Claude in my other repos?" No, unless you enable them), screenshot.
- [x] **T6.4 RELEASING.md.** The checklist from `11-plugin-packaging.md` §9.
- [x] **T6.5 Manual checklist.** Walk through `09-testing.md` §6 and record the results in `DECISIONS.md`.
- [x] **T6.6 Final review.** No TODOs, `make check e2e validate` green, tree clean.

**Stop here.** Publishing is done by the human (it needs their GitHub and claude.ai accounts):
tag `v0.1.0`, let CI build `release`, test the install from the marketplace, then submit at
`claude.ai/directory/manage` (`11-plugin-packaging.md` §9b). Write the exact steps into `RELEASING.md`.

## Later (note ideas in DECISIONS.md; don't build in v1)

- Group the timeline by Claude session (`Agent-Session:` trailer from the hook's `session_id`).
- Split diff view, syntax highlighting, light theme.
- `agentlog squash <range>` to tidy history before a real push.
- Branch-per-task mode (`/agentlog:task start "name"` → `agent/<name>` branch or worktree).
- Native Windows launcher (PowerShell) or exec-form hooks per OS.
- A monitor (plugin `experimental.monitors`) that auto-starts the UI server.
