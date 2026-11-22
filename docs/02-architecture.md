# 02: Architecture

## 1. Components

```
 Claude Code ──(plugin enabled)──► plugin/hooks/hooks.json
     │                                 │ SessionStart              │ Stop
     │                                 ▼                           ▼
     │                 scripts/agentlog hook-session-start   scripts/agentlog hook-stop
     │                    (launcher → dist/<os-arch> binary)
     │                                 │                           │
     │       repo enabled? (git config agentlog.enabled / enforce_everywhere) ── no ──► exit 0, silent
     │                                 │ yes                       │ yes
     │◄── rules as additionalContext ──┘          dirty? ── no ──► allow stop
     │                                                │ yes, first time → {"decision":"block", reason}
     │── makes commits with real messages ◄───────────┘ yes, second time → auto-commit "[auto] …"
     │
     │  /agentlog:init  /agentlog:ui  /agentlog:status  /agentlog:disable   (plugin skills → binary)
     ▼
 user's repo (.git, local only, never pushed)
     ▲  git CLI (read-only)
     │
 agentlog ui --detach ── HTTP 127.0.0.1:7777 ──► browser (embedded static UI, polls every 3 s)
```

| Component | Location | Responsibility |
|---|---|---|
| Plugin shell | `plugin/` | Manifest, hooks config, skills, launcher, prebuilt binaries. No logic beyond picking a binary |
| CLI | `internal/cli` | Parse subcommands, wire packages, print output |
| Git layer | `internal/gitx` | The only code that runs `git`. Typed results |
| Agent integration | `internal/agent` | Rules text, enabled check, SessionStart and Stop logic, standalone settings merge, agent detection |
| UI process mgmt | `internal/uiproc` | Background server lifecycle: reuse, spawn, stop, `ui.json` |
| Server | `internal/server` | HTTP handlers, validation, idle timeout |
| Web UI | `web/` | Static HTML/CSS/JS embedded in the binary |
| Marketplace | `.claude-plugin/marketplace.json` | Lets users install from this repo directly |

Dependency direction: `cli → agent, uiproc, server → gitx`. `gitx` imports nothing internal.

## 2. Data flow

**Install and enable:**
1. User: `/plugin marketplace add <you>/agentlog`, then `/plugin install agentlog@agentlog`.
   Claude Code copies `plugin/` (from the `release` branch, binaries included) into its plugin cache.
2. In a project, the user runs `/agentlog:init`. The skill runs `scripts/agentlog init`, which does
   git init if needed, the baseline commit, `AGENTS.md` rules, and `git config agentlog.enabled true`.

**Each session in an enabled repo:**
1. SessionStart hook → the binary prints the rules as `additionalContext`, so Claude knows the commit discipline from the first turn.
2. Claude works and (ideally) commits per logical step.
3. Claude tries to end its turn → Stop hook → dirty tree? First time: block with a reason listing the files.
   Claude commits, then stops. If it's still dirty on the second try: auto-commit and allow the stop.

**Timeline UI:**
1. `/agentlog:ui` → `agentlog ui --detach .` starts (or reuses) a background server and prints its URL.
2. The browser polls `/api/repo` every 3 s. When `head` or `dirty` changes, it refetches `/api/commits`.
3. Clicking a commit fetches `/api/patch?hash=…`. The patch is parsed in the browser and rendered.

## 3. Stack and key decisions

| Decision | Choice | Why |
|---|---|---|
| Distribution | **Claude Code plugin** (primary) + standalone binary | Hooks and skills install in one step. Discoverable in `/plugin` → Discover once listed |
| Language | **Go 1.22+**, stdlib only | One static binary per platform, fast startup (the hook runs every turn), no runtime to install |
| Shipping the binary | Prebuilt for 5 platforms in `plugin/scripts/dist/`, chosen by a POSIX launcher | Works offline with no download step at install time. A plugin can't reference files outside its folder |
| Binaries in git | Only on a CI-built `release` branch; gitignored on `main` | Keeps the source history small. Users install from `release` |
| Enforcement scope | **Opt-in per repo** via `git config agentlog.enabled` (+ `enforce_everywhere` user option) | Plugin hooks fire in every project. Blocking Claude in unrelated repos would be unacceptable |
| Delivering rules to Claude | SessionStart `additionalContext` | No need to edit the user's CLAUDE.md. Always current with the plugin version |
| Delivering rules to other agents | Marker block in `AGENTS.md` | Read by Codex and many others; idempotent edits |
| Slash commands | Plugin **skills** (`skills/<name>/SKILL.md`) | Recommended over `commands/` for new plugins |
| CLI parsing | stdlib `flag` + manual dispatch | Few commands, no dependency |
| Git access | `git` CLI via `os/exec` | Exact parity with the user's git; no cgo |
| Frontend | Vanilla HTML/CSS/ES modules, no build step, `go:embed` | No npm in the product; easy to edit |
| Diff parsing | Client-side JS | Thin server; pure, testable parser |
| History view | `--first-parent`, linear | Agent sessions are linear |
| Bind address | `127.0.0.1` only + Host header check | Code must never be exposed to the network |
| Windows | Supported through Git Bash (hooks run there); PowerShell-only unsupported in v1 | The launcher is POSIX sh. Revisit later |

## 4. Prototype pitfalls (do not repeat)

Found while testing `reference/prototype/`:

1. **Agent detection by author name is wrong.** If the user's git identity contains "Claude",
   every human commit was badged as an agent commit. Detect agents **only from trailers** and the
   reserved `agentlog` author. See `05-agent-integration.md` §9.
2. **Diff line-number columns rendered at ~40% width each.** `table-layout: fixed` took widths from
   the first row (a `colspan=3` hunk header). Use a `<colgroup>`. See `08-ui.md` §5.
3. Auto-commits must use the reserved identity via `git -c user.name=… -c user.email=…`, never the user's config.
4. The prototype installed the hook globally per repo with no opt-in check. In plugin form that would
   block Claude everywhere. Always check `IsEnabled` first (`05-agent-integration.md` §2).
