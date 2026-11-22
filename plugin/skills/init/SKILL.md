---
name: init
description: Enable agentlog in the current project so every change Claude makes is committed locally with a clear message. Use when the user asks to set up or enable agentlog.
---
Run this with the Bash tool:

"${CLAUDE_PLUGIN_ROOT}/scripts/agentlog" init "${CLAUDE_PROJECT_DIR}"

Then tell the user in 2-3 sentences what changed: a local git repo if there was none, a baseline
commit of the existing files, commit rules added to AGENTS.md, and enforcement switched on for
this repo. Mention they can open the history with /agentlog:ui.

If the command fails, show the error and stop. Never push anything.
