# 09: Testing

## 1. Unit tests (Go, `go test ./...`)

- **gitx:** all 10 edge cases in `06-git-layer.md`. Use a helper that creates a temp
  repo with `t.TempDir()`, sets a repo-local identity, and writes and commits files.
- **agent/rules:** upsert into a missing file, an empty file, a file with user content,
  and a file already holding an old block (replaced, user content intact). Remove restores the original.
- **agent/standalone:** install into a missing file, `{}`, a file with other hooks
  (preserved), and a file with an old agentlog hook (replaced, not duplicated). Invalid JSON
  is left untouched with a warning. Uninstall preserves everything else.
- **agent/enabled:** not enabled → false; `git config agentlog.enabled true` → true;
  `CLAUDE_PLUGIN_OPTION_ENFORCE_EVERYWHERE=true` → true in any repo; non-repo → false.
- **agent/sessionstart:** enabled → JSON with the rules in `additionalContext`; not enabled
  inside a git repo → the one-line hint; non-repo → no output; malformed stdin → no output, exit 0.
- **agent/stophook:** **not enabled + dirty → no output (critical)**; clean → no output; dirty + `stop_hook_active:false` → valid block
  JSON listing the files; dirty + `true` → an auto-commit is created and there's no output;
  non-repo cwd → no output and no error; malformed stdin → no output, exit 0; >40 files → list capped.
- **agent/detect:** table test covering every rule in `05-agent-integration.md` §6,
  including author "Claude Dev" with no trailers → `""`.
- **server:** everything in `07-http-api.md` "Tests".

## 2. diff.js unit tests (Node)

`e2e/tests/diff.unit.spec.ts` imports `../../web/static/diff.js` and runs `parsePatch` on
each file in `testdata/patches/`. It asserts file count, statuses, add/del counts and line numbers.

## 3. Integration: init → agent session → UI

`scripts/make-demo-repo.sh <dir>` must:
1. Create `<dir>` with a small Go project and run `agentlog init`.
2. Make 4 agent-style commits with `Agent: Claude` trailers (controller layer, repo
   interface + impl, query by date, then a **fix commit** that modifies an earlier file).
3. Make 1 human commit (no trailer, author "Claude Dev", to check pitfall #1).
4. Run `scripts/fake-claude-stop.sh` with a dirty file twice (false, then true) to produce one `[auto]` commit.
5. Leave 2 uncommitted changes (one modified, one untracked).

## 4. Browser e2e (Playwright, `make e2e`)

Start `bin/agentlog ui /tmp/agentlog-e2e --no-open --port 7799` as a test fixture, then:
- Timeline shows the working-tree item and every commit. The human commit has no badge; the auto commit has the `auto` badge.
- Clicking "Add GetProductsByDate…" shows its subject in the detail and a file card with a `+` line.
- **Layout check:** the diff's first number column is ≤ 70px wide (catches the colgroup bug).
- Story order puts "Baseline snapshot…" first and the working tree last.
- Filter "product" narrows the list. Clearing it restores the list.
- `j`/`k` changes the selection, and the URL hash updates.
- "View file" opens the modal with line numbers. `Esc` closes it.
- Live update: append a commit to the repo from the test, and within 5 s it appears at the top without a reload.
- No console errors during the run.

## 5. Plugin checks

- `claude plugin validate --strict ./plugin` and `claude plugin validate .` pass (`make validate`).
- Launcher test (`scripts/` shell test or a Go test that runs it): with a fake `dist/` holding only
  `agentlog-linux-amd64`, it execs that binary on linux/amd64. With no matching binary, `hook-stop` exits 0
  with empty stdout and `ui` exits 1 with the install hint.
- `plugin/` contains no path referencing `..`, has no top-level `bin/` directory, and has no `CLAUDE.md`.
- `hooks.json` parses, has the top-level `"hooks"` key, and every `${CLAUDE_PLUGIN_ROOT}` is quoted.

## 6. Manual acceptance checklist (run on a fresh machine or folder)

- [ ] `make build` produces one binary. `bin/agentlog --version` works.
- [ ] `make dist-local` then `claude --plugin-dir ./plugin`: `/agentlog:init`, `/agentlog:ui`, `/agentlog:status`, `/agentlog:disable` all appear and work.
- [ ] In a folder with code and no git: `/agentlog:init` → repo, baseline commit, `.gitignore` lines, `AGENTS.md`, `git config agentlog.enabled` = true.
- [ ] Running `/agentlog:init` a second time creates no new commit.
- [ ] Ask Claude for a multi-part change. It ends with several descriptive commits and a clean tree.
- [ ] **In a different git repo where init was never run**, Claude edits files and finishes normally: no block, no auto-commit.
- [ ] `/agentlog:ui` twice → the same URL both times (the server is reused, not duplicated).
- [ ] Ask for a fix on top. It lands as a new commit, not an amend.
- [ ] The UI opens. The session reads clearly in Story order.
- [ ] While Claude is working, "Uncommitted changes" appears and updates live.
- [ ] `curl -H 'Host: evil.com' 127.0.0.1:7777/api/repo` → 403.
- [ ] `/agentlog:disable` stops enforcement. Commits remain.
- [ ] Standalone: `go install` + `agentlog init --standalone-hook` works without the plugin. `agentlog uninstall` removes it and other settings stay.
- [ ] Install from the marketplace (release branch) on a clean machine works end to end.
- [ ] `git remote -v` is still empty. Nothing was pushed.
