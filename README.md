# agentlog: build pack

This folder is a complete spec for a coding agent (Claude Code, Codex, Cursor…) to
build **agentlog** end to end.

**agentlog** is a **Claude Code plugin** that makes AI coding agents commit every change
*locally* with a meaningful message, then shows those commits as a clickable timeline in a
local web UI. Nothing is pushed anywhere. It ships as a plugin (hooks + `/agentlog:*`
skills + prebuilt Go binaries) through its own marketplace, and is designed to be submitted to
Anthropic's plugin directory.

## How to use this pack

1. Create an empty folder for the project, e.g. `~/code/agentlog`, and `git init` it.
2. Copy everything in this pack into it (keep the folder layout).
3. Open your coding agent in that folder and say:

   > Read CLAUDE.md, then the docs in `docs/` in numbered order. Build agentlog
   > milestone by milestone following `docs/10-tasks.md`. Commit after every task.

4. When it finishes, run the manual checks in `docs/09-testing.md` §6 yourself.
5. Publish (needs your accounts): follow `RELEASING.md`, which the agent writes in the last milestone.

## What's inside

| Path | What it is |
|---|---|
| `CLAUDE.md` | Standing instructions for the agent (also read by Claude Code automatically) |
| `AGENTS.md` | Same instructions for Codex / other agents that read AGENTS.md |
| `docs/01-product.md` | Problem, goals, non-goals, user stories |
| `docs/02-architecture.md` | Components, data flow, key decisions and why |
| `docs/03-project-structure.md` | Final folder tree, what every file is responsible for |
| `docs/04-cli.md` | Every command, flag, output and exit code |
| `docs/05-agent-integration.md` | Opt-in model, SessionStart + Stop hooks, rules, standalone mode, other agents |
| `docs/06-git-layer.md` | Exact git commands, parsing formats, edge cases |
| `docs/07-http-api.md` | Local HTTP API: endpoints, JSON schemas, security rules |
| `docs/08-ui.md` | UI layout, components, interactions, design tokens, states |
| `docs/09-testing.md` | Unit, integration and browser tests plus a manual acceptance checklist |
| `docs/10-tasks.md` | Ordered milestones and tasks with acceptance criteria |
| `docs/11-plugin-packaging.md` | Plugin layout, manifest, hooks, skills, launcher, binaries, marketplace, publishing |
| `reference/prototype/` | A working single-file Python prototype (behaviour reference, **not** the target stack) |
| `reference/screenshots/` | Screenshots of the prototype UI (visual reference) |

## Stack chosen

Claude Code plugin wrapping a Go binary (prebuilt for 5 platforms, UI embedded with
`go:embed`), vanilla HTML/CSS/JS frontend with no build step, git called through the `git` CLI. Rationale is in
`docs/02-architecture.md`. To change the stack, edit section 3 of that doc before
handing the pack to the agent.
