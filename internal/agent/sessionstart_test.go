package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func input(t *testing.T, cwd string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"session_id": "s1", "cwd": cwd, "source": "startup", "hook_event_name": "SessionStart",
		"some_future_field": map[string]int{"x": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeContext(t *testing.T, out string) string {
	t.Helper()
	var v sessionStartOutput
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if v.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("hookEventName = %q", v.HookSpecificOutput.HookEventName)
	}
	return v.HookSpecificOutput.AdditionalContext
}

func TestSessionStartEnabledGetsRules(t *testing.T) {
	r := newRepo(t)
	_ = r.SetLocal(ConfigKey, "true")
	out := HandleSessionStart(input(t, r.Root), env(nil))
	if strings.Contains(out, "\n") {
		t.Fatalf("output must be a single line: %q", out)
	}
	if got := decodeContext(t, out); got != Rules() {
		t.Fatalf("context is not the rules text:\n%s", got)
	}
}

func TestSessionStartNotEnabledGetsHint(t *testing.T) {
	r := newRepo(t)
	if got := decodeContext(t, HandleSessionStart(input(t, r.Root), env(nil))); got != NotEnabledHint {
		t.Fatalf("context = %q", got)
	}
}

func TestSessionStartEnforceEverywhere(t *testing.T) {
	r := newRepo(t)
	out := HandleSessionStart(input(t, r.Root), env(map[string]string{EnvEnforceEverywhere: "true"}))
	if got := decodeContext(t, out); got != Rules() {
		t.Fatalf("context = %q", got)
	}
}

func TestSessionStartNonRepoIsSilent(t *testing.T) {
	if out := HandleSessionStart(input(t, t.TempDir()), env(nil)); out != "" {
		t.Fatalf("expected no output, got %q", out)
	}
}

func TestSessionStartMalformedInputIsSilent(t *testing.T) {
	for _, bad := range []string{"{not json", "[1,2", "\x00\x01"} {
		if out := HandleSessionStart([]byte(bad), env(nil)); out != "" {
			t.Errorf("input %q gave output %q", bad, out)
		}
	}
}

func TestSessionStartFallsBackToProjectDir(t *testing.T) {
	r := newRepo(t)
	_ = r.SetLocal(ConfigKey, "true")
	out := HandleSessionStart([]byte(`{"session_id":"s"}`), env(map[string]string{EnvProjectDir: r.Root}))
	if got := decodeContext(t, out); got != Rules() {
		t.Fatalf("context = %q", got)
	}
}

func TestSessionStartFromSubdirectory(t *testing.T) {
	r := newRepo(t)
	_ = r.SetLocal(ConfigKey, "true")
	sub := filepath.Join(r.Root, "deep", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := decodeContext(t, HandleSessionStart(input(t, sub), env(nil))); got != Rules() {
		t.Fatal("subdirectory cwd must resolve to the repo root")
	}
}

func TestSessionStartContextUnderCap(t *testing.T) {
	// Claude Code caps additionalContext at 10,000 characters.
	if n := len(Rules()); n > 10000 {
		t.Fatalf("rules are %d chars, over the 10,000 cap", n)
	}
}

func TestLogErrorWritesPluginDataLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	LogError(env(map[string]string{EnvPluginData: dir}), "boom %d", 7)
	b, err := os.ReadFile(filepath.Join(dir, "agentlog.log"))
	if err != nil || !strings.Contains(string(b), "agentlog: boom 7") {
		t.Fatalf("log = %q err=%v", b, err)
	}
}
