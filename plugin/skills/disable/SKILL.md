---
name: disable
description: Turn agentlog off for the current project. Existing commits are kept. Use when the user wants to stop automatic commits in this repo.
---
Run this with the Bash tool:

"${CLAUDE_PLUGIN_ROOT}/scripts/agentlog" disable "${CLAUDE_PROJECT_DIR}"

Confirm that enforcement is now off for this repo and that no commits were changed or removed.
The user can switch it back on with /agentlog:init. If the command fails, show the error.
