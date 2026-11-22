#!/bin/sh
# Pipes a sample Claude Code SessionStart hook payload into
# `agentlog hook-session-start` and prints what the hook wrote to stdout.
#
#   scripts/fake-session-start.sh <repo-dir> [session-id]
set -eu
dir="${1:?usage: fake-session-start.sh <repo-dir> [session-id]}"
session="${2:-fake-session}"
here="$(cd "$(dirname "$0")/.." && pwd)"
bin="${AGENTLOG_BIN:-$here/bin/agentlog}"
dir="$(cd "$dir" && pwd)"

printf '{"session_id":"%s","transcript_path":"/tmp/none.jsonl","cwd":"%s","hook_event_name":"SessionStart","source":"startup"}' \
  "$session" "$dir" | "$bin" hook-session-start
