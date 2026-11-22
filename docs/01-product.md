# 01: Product

## Problem

When a developer works locally with an AI coding agent, the agent edits the working tree
directly. After a few big changes and a few fixes on top, the code is a pile of
uncommitted edits:

- You can't tell **what** changed, in **what order**, or **why**.
- Reading the whole codebase to reconstruct the agent's work isn't realistic.
- You don't want to push to GitHub/GitLab just to get a readable history.

Git already solves this locally. The missing pieces are:

1. **Enforcement.** Agents don't commit unless they're forced to, and when they do the
   messages are vague.
2. **Readability.** `git log` in a terminal isn't a pleasant way to review a session of agent work.

## Solution

**agentlog** is a **Claude Code plugin** (backed by a single local Go binary, also usable standalone) that:

1. **Forces agents to commit** each logical step with a descriptive message:
   - For Claude Code, the plugin injects the commit rules at session start, and a **Stop hook** refuses to let the agent finish its turn while
     the working tree is dirty and tells it to commit (split by concern). On a second
     attempt it auto-commits as a safety net so nothing is lost.
   - For other agents, a **rules block** goes into `AGENTS.md` describing the commit discipline.
   - Enforcement only applies in repos the user has enabled with `/agentlog:init`.
   - For agents without hooks, a **watch mode** auto-commits after edits settle (fallback, lower-quality messages).
2. **Shows a timeline UI** on `127.0.0.1`: commits listed in order with message,
   agent badge, time and +/- stats; click one to see its message body and a per-file
   diff; open any file as it was at that commit; and see live uncommitted changes as
   they happen.

Everything stays in the local `.git`. agentlog never pushes, never rewrites history, never talks to the network.

## Target user

A developer who uses AI coding agents on local projects and wants a reviewable,
ordered history of what the agent did, without publishing the code.

## Goals (v1)

- G0: Install in two commands from `/plugin`, and get listed in Anthropic's plugin directory.
- G1: `/agentlog:init` in any folder makes it agent-commit-ready in under 5 seconds.
- G1b: In repos that aren't enabled, agentlog never blocks or changes anything.
- G2: When using Claude Code after init, ≥95% of agent edits land in agent-written
  commits (the rest in clearly labelled `[auto]` commits). No edit is ever lost.
- G3: `agentlog ui` opens a timeline where a user can understand a 20-commit session in
  under 2 minutes by reading subjects and clicking into diffs.
- G4: The UI updates live (≤3 s) while the agent works, with no manual refresh.
- G5: Zero runtime dependencies besides `git` on PATH.

## Non-goals (v1)

- Pushing, syncing or any remote integration.
- Editing code or committing from the UI (it's read-only).
- Rewriting history (squash, reorder, revert) from agentlog.
- Multi-repo dashboards or user accounts.
- Branch graph visualisation (first-parent linear history only).
- Syntax highlighting per language. Diff colouring only.

## User stories

0. *As a dev*, I find agentlog in `/plugin` → Discover (or add its marketplace) and install it.
1. *As a dev*, I run `/agentlog:init` in an existing project. It creates a git repo if
   needed and commits a baseline snapshot so later diffs only show agent work.
2. *As a dev*, I ask Claude Code for a big feature. When it finishes, there are 3–6
   commits like "Add controller layer for orders API" and "Add ProductRepository interface and Postgres impl".
3. *As a dev*, I ask for a fix on top. It appears as a **new** commit ("Fix empty-id
   handling in ProductRepository.Get"), not an amended one.
4. *As a dev*, I run `/agentlog:ui`, open the URL, switch to **Story order** (oldest first) and read
   the session top to bottom like a changelog.
5. *As a dev*, I click a commit to see its files, jump to one and expand its diff, or
   open the full file as of that commit.
6. *As a dev*, while the agent is mid-task I see an **Uncommitted changes** entry at the top, updating live.
7. *As a dev*, I filter by text ("product") to find every commit touching the product repo.
8. *As a dev*, I run `/agentlog:disable` in a repo. Enforcement stops there and my commits stay. Other repos where I never ran init are never affected.
9. *As a dev using Codex/Cursor*, I run `agentlog watch` in a terminal so changes still
   get committed (with `[auto]` messages) even though there's no hook.
