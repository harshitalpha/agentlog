# AGENTS.md: instructions for the agent building agentlog (same as CLAUDE.md)

You are building **agentlog**, a local developer tool. The full spec lives in `docs/`.
This file holds the rules for *how* you work. The docs define *what* you build.

## Read order (do this first, every new session)

1. `docs/01-product.md` (why it exists)
2. `docs/02-architecture.md` (how the pieces fit)
3. `docs/03-project-structure.md` (where code goes; do not invent other layouts)
4. `docs/04-cli.md` → `docs/08-ui.md` (detailed specs, read the ones relevant to your task)
5. `docs/09-testing.md` (how to prove it works)
6. `docs/10-tasks.md` (what to do next; find the first unchecked task)
7. `docs/11-plugin-packaging.md` (before touching anything in `plugin/`)

`reference/prototype/` is a working Python prototype. Use it to understand intended
behaviour and to compare output, but **do not port it line by line**. Where the docs and
the prototype disagree, the docs win. The docs record known prototype bugs under
"Prototype pitfalls".

## Working rules

- Work through `docs/10-tasks.md` in order. Tick the checkbox (`- [x]`) when a task's
  acceptance criteria pass. Don't skip ahead unless a task says it can run in parallel.
- **Commit discipline (we dogfood the product):**
  - One commit per task, or several when a task spans more than one concern. Never one giant commit.
  - Subject in imperative mood, ≤72 chars, says what changed:
    `Add git log parser with numstat totals`, not `wip` or `updates`.
  - Body: why it was done, anything non-obvious, follow-ups.
  - Never `git push`, `--amend`, `rebase`, or `reset --hard`.
- Stay inside the stack in `docs/02-architecture.md` §3. Allowed third-party Go
  dependencies: **none** for the binary (stdlib only). Test-only tooling is allowed as
  listed in `docs/09-testing.md`.
- Keep the binary self-contained: the UI is embedded, and no network access is needed at runtime.
- Plugin hooks run in **every** repo where the plugin is enabled: every hook path must check
  `IsEnabled` first and must never exit non-zero (`docs/05-agent-integration.md` §2).
- Nothing under `plugin/` may reference files outside `plugin/`.
- Run `make check` (fmt, vet, unit tests) before every commit once the Makefile exists.
- If a spec is ambiguous, choose the simplest behaviour consistent with
  `docs/01-product.md`, write the decision into `docs/DECISIONS.md` (create it if
  missing), and continue. Don't stop to ask unless the choice is irreversible.
- Don't add features outside the current milestone. Ideas go in `docs/DECISIONS.md` under "Later".

## Definition of done (whole project)

- Every task in `docs/10-tasks.md` is ticked.
- `make check`, `make e2e` and `make validate` pass.
- The manual acceptance checklist in `docs/09-testing.md` §6 passes on a fresh repo.
- `README.md` explains installing the plugin, `/agentlog:init`, `/agentlog:ui`, standalone use, and disabling.
- Publishing (tagging, directory submission) is left to the human. Prepare `RELEASING.md` and stop.

<!-- agentlog:start -->
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
<!-- agentlog:end -->
