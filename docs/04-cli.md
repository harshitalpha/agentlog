# 04: CLI specification

Binary name: `agentlog`. Inside the plugin it's invoked through
`${CLAUDE_PLUGIN_ROOT}/scripts/agentlog` (launcher). Standalone users call it directly.

Every command takes an optional `[path]` (default `.`), which can be any folder inside the
repo. Commands resolve the repo root with `git rev-parse --show-toplevel` unless stated otherwise.

Global: `agentlog --help`, `agentlog <cmd> --help`, `agentlog --version`.
Exit codes: `0` success, `1` runtime error, `2` usage error.
Human output goes to **stdout**. Errors go to **stderr**, prefixed `agentlog: `.
Exception: `hook-*` commands follow `05-agent-integration.md` (always exit 0, JSON-only stdout).

| Command | Used by |
|---|---|
| `init` | `/agentlog:init` skill, standalone users |
| `disable` | `/agentlog:disable` skill |
| `status` | `/agentlog:status` skill |
| `ui` | `/agentlog:ui` skill, users in a terminal |
| `watch` | users of agents without hooks |
| `hook-session-start`, `hook-stop` | plugin hooks (`hooks/hooks.json`) |
| `uninstall` | standalone users |

---

## `agentlog init [path] [--no-agents-md] [--claude-md] [--no-baseline] [--standalone-hook]`

Prepares a project and **enables enforcement** for it. **Idempotent**: running it twice changes nothing the second time.

1. If `path` isn't a directory → error, exit 1.
2. If not inside a git repo → `git init` in `path`. Print `✓ initialised local git repo`.
3. **Identity:** if `user.name` or `user.email` is unset (any scope), set repo-local values
   `local-dev` / `local-dev@localhost` and print a note. Never touch the global config.
4. **.gitignore:** ensure these lines exist (append only the missing ones):
   `.claude/settings.local.json`, `.env`, `.env.*`
5. **Baseline:** unless `--no-baseline`, if the repo has no commits **or** the tree is dirty,
   `git add -A` and commit `Baseline snapshot before agent work` (body: `Created by agentlog init
   so later agent commits diff against this state.`). Skip if nothing is staged.
6. **Enable:** `git config --local agentlog.enabled true`.
7. **Rules files:** upsert the rules block into `AGENTS.md` unless `--no-agents-md`. Upsert it
   into `CLAUDE.md` only with `--claude-md`. (In plugin mode, Claude gets the rules from the
   SessionStart hook, so CLAUDE.md is optional.)
8. **Standalone hook:** only with `--standalone-hook`, install the hooks into
   `.claude/settings.local.json` (`05-agent-integration.md` §8).
9. If steps 4 and 7 changed tracked files, commit them as `Add agentlog commit rules for AI agents`.
10. Print:
    ```
    ✓ agentlog enabled in /path/to/repo
      Every change your agent makes will now be committed locally.
      View history: /agentlog:ui   (or: agentlog ui)
    ```

## `agentlog disable [path]`

`git config --local --unset agentlog.enabled` (ignore "not set"). Stops the UI server if
it's running for this repo. Leaves rules files and commits as they are. Prints `✓ agentlog disabled
in <repo> (commits untouched)`.

## `agentlog status [path] [--json]`

Prints: enabled (yes/no, and whether that's because of `enforce_everywhere`), branch,
commits since baseline split by agent / human / auto, uncommitted file count, and UI URL if
running. `--json` prints the same as a JSON object (used by the status skill).

## `agentlog ui [path] [--port N] [--no-open] [--detach] [--stop]`

- Errors if not inside a git repo (`run agentlog init first`), exit 1.
- Port: `--port`, else env `CLAUDE_PLUGIN_OPTION_UI_PORT`, else `7777`. If the port is busy, try
  the next 10 ports. Binds **only** `127.0.0.1`.
- **Foreground (default):** opens the browser unless `--no-open`, prints
  `agentlog UI for <repo> → http://127.0.0.1:<port>  (Ctrl+C to stop)`, and shuts down cleanly on SIGINT/SIGTERM.
- **`--detach`:** if `.git/agentlog/ui.json` names a live process serving this repo
  (check the PID is alive **and** `GET /api/repo` returns this repo path), print its URL and exit 0.
  Otherwise re-exec itself in the background (`ui --no-open --port …`, detached from the
  terminal and session: `Setsid` on Unix, `CREATE_NEW_PROCESS_GROUP` on Windows, stdio
  redirected to `.git/agentlog/ui.log`), wait up to 3 s for it to answer, write
  `{"pid":…, "port":…, "url":…, "started":…}` to `.git/agentlog/ui.json`, and print the URL. Open the browser unless `--no-open`.
- **`--stop`:** kill the PID from `ui.json` if it's alive, delete the file, and print `✓ stopped`.
- The server stops itself after 8 hours with no HTTP requests (idle timeout), so orphans don't pile up.

## `agentlog watch [path] [--quiet 20] [--interval 2]`

Fallback for agents without hooks. Ignores the enabled flag (running it is the opt-in).

- Every `--interval` s, compute the signature `git status --porcelain --untracked-files=all` + `git diff --stat`.
- When the signature is non-empty and unchanged for `--quiet` s, auto-commit
  (`05-agent-integration.md` §7) and print `HH:MM:SS [auto] edits: a.go, b.go (+3 more)`.
- Ctrl+C exits 0.

## `agentlog hook-session-start` / `agentlog hook-stop`

For plugin hooks only. Behaviour is in `05-agent-integration.md` §4–5. Both must:
- **Always exit 0.** Never write anything except a single JSON object to stdout.
- Finish fast: session-start < 1 s, stop < 2 s on a normal repo.

## `agentlog uninstall [path]`

For standalone users. Removes agentlog hooks from `.claude/settings.local.json` (keeps
everything else), removes the marker blocks from `AGENTS.md` / `CLAUDE.md` (deleting a file
that becomes empty), and runs `disable`. Never touches commits or `.gitignore`.
Prints `✓ removed agentlog from <repo> (your commits are untouched)`.
Plugin users uninstall the plugin via `/plugin` and run `/agentlog:disable` in repos they enabled.
