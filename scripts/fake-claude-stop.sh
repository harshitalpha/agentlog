#!/bin/sh
# Pipes a sample Claude Code Stop hook payload into `agentlog hook-stop`.
#
#   scripts/fake-claude-stop.sh <repo-dir> [true|false] [session-id]
#
# The second argument is stop_hook_active (default false). Prints exactly what
# the hook wrote to stdout. Set AGENTLOG_BIN to use a different binary.
set -eu
dir="${1:?usage: fake-claude-stop.sh <repo-dir> [true|false] [session-id]}"
active="${2:-false}"
session="${3:-fake-session}"
here="$(cd "$(dirname "$0")/.." && pwd)"
bin="${AGENTLOG_BIN:-$here/bin/agentlog}"
dir="$(cd "$dir" && pwd)"
case "$active" in true|false) ;; *) echo "stop_hook_active must be true or false" >&2; exit 2 ;; esac

printf '{"session_id":"%s","transcript_path":"/tmp/none.jsonl","cwd":"%s","permission_mode":"default","hook_event_name":"Stop","stop_hook_active":%s,"last_assistant_message":"Done."}' \
  "$session" "$dir" "$active" | "$bin" hook-stop
