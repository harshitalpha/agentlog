---
name: status
description: Show whether agentlog is enabled in the current project and what it has committed. Use when the user asks about agentlog's state or recent agent activity.
---
Run this with the Bash tool:

"${CLAUDE_PLUGIN_ROOT}/scripts/agentlog" status "${CLAUDE_PROJECT_DIR}"

Summarise in a few lines: whether agentlog is enabled, how many commits since the baseline were made
by an agent, by a human and automatically, how many files are uncommitted, and the UI URL if the UI is
running. If the command fails, show the error.
