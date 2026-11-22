# 05: Agent integration

agentlog ships primarily as a **Claude Code plugin**. The plugin bundles the hooks and
slash-command skills. The same Go binary also works standalone for people who don't
use the plugin (see §8). Packaging details are in `docs/11-plugin-packaging.md`.

## 1. Strategy

| Layer | Delivered by | Works for | Message quality | Guarantee |
|---|---|---|---|---|
| Commit rules injected at session start | Plugin `SessionStart` hook | Claude Code | High | Soft: Claude may still forget |
| Stop hook blocks finishing with a dirty tree | Plugin `Stop` hook | Claude Code | High (Claude writes them) | Hard |
| Rules block in `AGENTS.md` (and optionally `CLAUDE.md`) | `agentlog init` | Codex and any agent that reads it | High | Soft |
| `agentlog watch` | CLI | Anything | Low (`[auto]`) | Hard, time-based |

## 2. Opt-in per repo (critical)

A plugin's hooks run in **every** project where the plugin is enabled. agentlog must
only act in repos the user has explicitly turned on. Otherwise it would start blocking
Claude in unrelated work repos.

- **Enabled marker:** the local git config key `agentlog.enabled=true`
  (`git config --local agentlog.enabled true`). It's invisible in the working tree,
  never committed, and per clone.
- `agentlog init` sets it. `agentlog disable` (and `uninstall`) unsets it.
- Every hook first checks: inside a git repo **and** `git config --get agentlog.enabled`
  is `true`. If not, exit 0 silently with no output.
- **Plugin user option** `enforce_everywhere` (boolean, default `false`, declared in
  `plugin.json` `userConfig`): when true, hooks treat every git repo as enabled. Hooks
  read it from the env var `CLAUDE_PLUGIN_OPTION_ENFORCE_EVERYWHERE`.
- When the plugin is enabled but the repo isn't opted in, the SessionStart hook may add
  **one short line** of context: `agentlog is installed but not enabled in this repo. The
  user can run /agentlog:init to enable it.` Only do this inside git repos, and never block.

## 3. Hook configuration (`plugin/hooks/hooks.json`)

```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [ { "type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}\"/scripts/agentlog hook-session-start", "timeout": 10 } ] }
    ],
    "Stop": [
      { "hooks": [ { "type": "command", "command": "\"${CLAUDE_PLUGIN_ROOT}\"/scripts/agentlog hook-stop", "timeout": 30 } ] }
    ]
  }
}
```

- The file **must** have the top-level `"hooks"` wrapper (a hooks file without it fails to load).
- Quote `${CLAUDE_PLUGIN_ROOT}` (shell-form). `claude plugin validate` warns if it's unquoted.
- `scripts/agentlog` is the platform launcher (see `11-plugin-packaging.md` §4).
- Hook processes receive `CLAUDE_PLUGIN_ROOT`, `CLAUDE_PLUGIN_DATA`, `CLAUDE_PROJECT_DIR`
  and `CLAUDE_PLUGIN_OPTION_<KEY>` in their environment.

## 4. SessionStart hook (`agentlog hook-session-start`)

Input: JSON on stdin (`session_id`, `cwd`, `source`, …; ignore unknown fields).

```
repo := open(input.cwd or $CLAUDE_PROJECT_DIR or cwd)   // not a repo → exit 0, no output
if !enabled(repo)  → print the optional one-line hint (§2) as additionalContext, exit 0
else               → print rules as additionalContext, exit 0
```

Output (stdout, single JSON object):

```json
{ "hookSpecificOutput": { "hookEventName": "SessionStart", "additionalContext": "<rules text from §6>" } }
```

> Check this output shape against the current Claude Code hooks reference
> (`https://code.claude.com/docs/en/hooks`) before implementing. If it has changed, follow
> the docs and record the change in `docs/DECISIONS.md`.

Must always exit 0 and finish in < 1 s.

## 5. Stop hook (`agentlog hook-stop`)

Input (stdin JSON): `session_id`, `transcript_path`, `cwd`, `hook_event_name`,
`stop_hook_active`. Ignore unknown fields.

```
repo := open(input.cwd or $CLAUDE_PROJECT_DIR or cwd)   // not a repo → exit 0
if !enabled(repo)            → exit 0, no output
files := repo.DirtyFiles()
if len(files) == 0           → exit 0, no output               (allow stop)
if !input.stop_hook_active   → print BLOCK json, exit 0        (force Claude to commit)
else                         → repo.AutoCommit("uncommitted changes"), exit 0, no output
```

BLOCK output (one line of JSON on stdout):

```json
{"decision":"block","reason":"<reason text>"}
```

The reason text must contain all of the following:

```
agentlog: you have uncommitted changes. Before finishing, commit them locally.
Split them into one commit per logical step if they cover more than one concern
(use `git add <paths>` per commit). Each subject should say what was done in plain
words, e.g. "Add repo interface and impl for product DB". Do NOT push, amend or rebase.
End each commit body with the trailer "Agent: Claude".

Uncommitted files:
  path/one.go
  … and N more            (cap the list at 40)
```

Why the second attempt auto-commits: `stop_hook_active: true` means Claude is already
continuing because of this hook. Blocking again risks a loop. Auto-committing preserves
the work, and the `[auto]` label makes the lapse visible in the UI.

**Never** exit non-zero and never write non-JSON to stdout. On internal errors, log to
stderr and to `${CLAUDE_PLUGIN_DATA}/agentlog.log` (if set), then exit 0.

## 6. Rules text

Stored once in `internal/agent/rules.md` (embedded). Used in three places: SessionStart
`additionalContext`, the `AGENTS.md` block, and the optional `CLAUDE.md` block. File
blocks are wrapped in markers so they can be updated or removed without touching the
user's content:

```
<!-- agentlog:start -->
…rules…
<!-- agentlog:end -->
```

Upsert: if the markers exist, replace everything between them (inclusive). Otherwise append
the block after one blank line. Remove: delete the block and the blank lines around it.

Rules content:

```markdown
## Commit discipline (enforced by agentlog)

This repo is tracked locally with git. Every change you make must end up in a
local commit with a clear message, so the human can review your work step by step.

- Commit after each logical unit of work, not once at the end. One concern per commit.
  Good: "Add controller layer for the orders API",
        "Add ProductRepository interface and Postgres impl",
        "Add GetProductsByDate query to product DB".
  Bad:  "updates", "fix", "wip", "changes from claude".
- Subject: imperative mood, under 72 characters, no trailing period.
- Body (optional, after a blank line): why the change was made, anything non-obvious,
  and follow-ups left undone.
- End the body with a trailer naming yourself, e.g. `Agent: Claude` or `Agent: Codex`.
- When fixing something you wrote earlier, make a NEW commit ("Fix nil check in
  GetProductsByDate") instead of amending, so the history shows the order of events.
- Stage with `git add -A` or specific paths (respects .gitignore). Never commit secrets or .env files.
- NEVER run `git push`, `git commit --amend`, `git rebase`, `git reset --hard`,
  or anything else that rewrites or publishes history.
```

## 7. Auto-commit (shared by the Stop-hook fallback and watch)

- `git add -A`. If nothing is staged afterwards, do nothing.
- Subject: `[auto] <label>: <name1>, <name2>, <name3> (+N more)` using the base names of
  changed files (for renames, the new name), truncated to 100 chars.
- Body: `Committed automatically by agentlog because the agent left changes uncommitted.` + blank line + `Agent: auto`
- Identity: `git -c user.name=agentlog -c user.email=agentlog@localhost commit --no-verify -q -m <subject> -m <body>`

## 8. Standalone mode (no plugin)

For users who installed only the binary (`go install`), `agentlog init --standalone-hook`
also writes the Stop and SessionStart hooks into `<repo>/.claude/settings.local.json`,
pointing at the absolute binary path (`os.Executable()` + `EvalSymlinks`, quoted). That
file is gitignored because it holds a machine-specific path. Merge rules:

- Preserve every other key and hook. Before appending, remove existing hooks whose
  `command` contains `agentlog` and ` hook-`, then drop groups left empty.
- If the file isn't valid JSON, don't touch it. Print the snippet to add manually and continue.
- `agentlog uninstall` removes these entries.
- If the plugin is **also** enabled, both hooks would run. The binary dedupes by writing
  a lock file `.git/agentlog/stop-<session_id>` with a 5 s TTL. The second invocation
  within the TTL exits 0 silently.

## 9. Agent detection (`DetectAgent(author, body) string`)

Returns a display label, or `""` for a human commit. First match wins:

1. Trailer `^Agent:\s*(.+)$` (case-insensitive) → the value, trimmed.
2. Author name exactly `agentlog` → `auto`.
3. Each `^Co-Authored-By:\s*(.+)$` value → `claude`→`Claude`, `codex|openai`→`Codex`,
   `cursor`→`Cursor`, `copilot`→`Copilot`, `gemini`→`Gemini`, `aider`→`Aider`, `devin`→`Devin`.
4. Otherwise `""`. **Never infer from the author name** (prototype pitfall #1).

## 10. Other agents

- **Codex and most CLI agents** read `AGENTS.md`, so the rules apply. Recommend `agentlog watch` as a safety net.
- **Cursor:** the README documents copying the rules into `.cursor/rules/agentlog.mdc` (not automated in v1).
