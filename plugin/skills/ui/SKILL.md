---
name: ui
description: Open the agentlog timeline UI for the current project, a local page listing every commit with diffs. Use when the user wants to see or review the commit history. Pass "stop" to shut the UI down.
---
If the argument is "stop" (arguments: $ARGUMENTS), run this with the Bash tool and confirm it stopped:

"${CLAUDE_PLUGIN_ROOT}/scripts/agentlog" ui --stop "${CLAUDE_PROJECT_DIR}"

Otherwise run this with the Bash tool. It starts the server in the background, or reuses one that
is already running, and prints the URL:

"${CLAUDE_PLUGIN_ROOT}/scripts/agentlog" ui --detach "${CLAUDE_PROJECT_DIR}"

Reply with the URL only. If the command fails, show the error.
