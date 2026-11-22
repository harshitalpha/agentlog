#!/bin/sh
# Static checks on plugin/ that need no `claude` CLI (docs/09-testing.md §5):
# layout rules, hooks.json shape, version agreement, and the launcher's behaviour.
# Usage: scripts/check-plugin.sh   (run from the repo root)
set -u
cd "$(dirname "$0")/.." || exit 1

fail=0
bad() { echo "check-plugin: FAIL: $*" >&2; fail=1; }

P=plugin
[ -f "$P/.claude-plugin/plugin.json" ] || { echo "check-plugin: no plugin/ yet, skipping"; exit 0; }

# --- layout -----------------------------------------------------------------
[ ! -e "$P/bin" ]       || bad "plugin/bin must not exist (claude.ai refuses plugins with a top-level bin/)"
[ ! -e "$P/CLAUDE.md" ] || bad "plugin/CLAUDE.md must not exist (it is not loaded)"
[ -x "$P/scripts/agentlog" ] || bad "plugin/scripts/agentlog must be executable"
for s in init ui disable status; do
  [ -f "$P/skills/$s/SKILL.md" ] || bad "missing skills/$s/SKILL.md"
done
# Nothing in plugin/ may point outside plugin/ (it is copied into a cache on install).
if grep -rIn --exclude-dir=dist -e '\.\./' "$P"; then bad "plugin/ references a path with .."; fi
# plugin/ must not reach back into the repo by name either.
if grep -rIn --exclude-dir=dist -e 'internal/' -e 'cmd/agentlog' -e 'web/static' "$P" | grep -v 'go install'; then
  bad "plugin/ mentions files outside plugin/"
fi

# --- hooks.json ---------------------------------------------------------------
python3 - "$P/hooks/hooks.json" <<'PY' || fail=1
import json, re, sys
path = sys.argv[1]
try:
    data = json.load(open(path))
except Exception as e:
    print(f"check-plugin: FAIL: {path} does not parse: {e}", file=sys.stderr); sys.exit(1)
ok = True
if not isinstance(data.get("hooks"), dict):
    print("check-plugin: FAIL: hooks.json needs a top-level \"hooks\" key", file=sys.stderr); sys.exit(1)
for event in ("SessionStart", "Stop"):
    if event not in data["hooks"]:
        print(f"check-plugin: FAIL: hooks.json has no {event} hook", file=sys.stderr); ok = False
for event, groups in data["hooks"].items():
    for g in groups:
        for h in g.get("hooks", []):
            cmd = h.get("command", "")
            for m in re.finditer(r'\$\{CLAUDE_PLUGIN_ROOT\}', cmd):
                before, after = cmd[:m.start()], cmd[m.end():]
                if not (before.endswith('"') and after.startswith('"')):
                    print(f"check-plugin: FAIL: unquoted ${{CLAUDE_PLUGIN_ROOT}} in {event}: {cmd}", file=sys.stderr); ok = False
sys.exit(0 if ok else 1)
PY

# --- version agreement (plugin.json is the single source) ----------------------
pv=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$P/.claude-plugin/plugin.json" | head -1)
mv=$(sed -n 's/^var version = "\([^"]*\)".*/\1/p' cmd/agentlog/main.go)
[ -n "$pv" ] && [ "$pv" = "$mv" ] || bad "version mismatch: plugin.json=$pv main.version=$mv"

# --- launcher -------------------------------------------------------------------
tmp=$(mktemp -d) || exit 1
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts/dist"
cp "$P/scripts/agentlog" "$tmp/scripts/agentlog"

os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; esac
case "$os" in mingw*|msys*|cygwin*) os=windows ;; esac

# 1. No matching binary: hooks exit 0 with empty stdout, other commands exit 1 with the install hint.
out=$(echo '{}' | "$tmp/scripts/agentlog" hook-stop 2>"$tmp/err"); code=$?
[ "$code" = 0 ] && [ -z "$out" ] || bad "launcher: hook-stop without a binary must exit 0 silently (code=$code out=$out)"
out=$("$tmp/scripts/agentlog" ui 2>"$tmp/err" </dev/null); code=$?
[ "$code" = 1 ] || bad "launcher: ui without a binary must exit 1 (code=$code)"
grep -q 'go install' "$tmp/err" || bad "launcher: ui without a binary must print the install hint"

# 2. A binary for this platform is exec'd with the arguments passed through.
name="agentlog-$os-$arch"; [ "$os" = windows ] && name="$name.exe"
printf '#!/bin/sh\necho "fake:$*"\n' > "$tmp/scripts/dist/$name"
chmod +x "$tmp/scripts/dist/$name"
out=$("$tmp/scripts/agentlog" status --json 2>&1); [ "$out" = "fake:status --json" ] || bad "launcher: did not exec the matching binary (got: $out)"

# 3. A binary for another platform is ignored.
rm "$tmp/scripts/dist/$name"
other="agentlog-plan9-mips"; printf '#!/bin/sh\necho wrong\n' > "$tmp/scripts/dist/$other"; chmod +x "$tmp/scripts/dist/$other"
out=$(echo '{}' | "$tmp/scripts/agentlog" hook-stop 2>/dev/null); [ -z "$out" ] || bad "launcher: ran a binary for another platform"

[ "$fail" = 0 ] && echo "check-plugin: ok"
exit "$fail"
