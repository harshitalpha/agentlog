package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exe = "/opt/my tools/agentlog"

func settingsFile(t *testing.T, content *string) (root, path string) {
	t.Helper()
	root = t.TempDir()
	path = SettingsPath(root)
	if content != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(*content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, path
}

func load(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("settings are not valid JSON: %v\n%s", err, b)
	}
	return v
}

// commandsFor lists every hook command registered for an event.
func commandsFor(settings map[string]any, event string) []string {
	var out []string
	hooks, _ := settings["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	for _, g := range groups {
		list, _ := g.(map[string]any)["hooks"].([]any)
		for _, h := range list {
			out = append(out, h.(map[string]any)["command"].(string))
		}
	}
	return out
}

func strp(s string) *string { return &s }

func TestStandaloneCommandQuotesPath(t *testing.T) {
	if got := StandaloneCommand(exe, "hook-stop"); got != `"/opt/my tools/agentlog" hook-stop` {
		t.Fatalf("got %s", got)
	}
}

func TestInstallIntoMissingFileAndEmptyObject(t *testing.T) {
	for name, content := range map[string]*string{"missing": nil, "empty object": strp("{}"), "empty file": strp("")} {
		root, path := settingsFile(t, content)
		if err := InstallStandaloneHooks(root, exe); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		s := load(t, path)
		if got := commandsFor(s, "Stop"); len(got) != 1 || got[0] != `"/opt/my tools/agentlog" hook-stop` {
			t.Errorf("%s: Stop = %v", name, got)
		}
		if got := commandsFor(s, "SessionStart"); len(got) != 1 || got[0] != `"/opt/my tools/agentlog" hook-session-start` {
			t.Errorf("%s: SessionStart = %v", name, got)
		}
	}
}

func TestInstallPreservesOtherSettingsAndHooks(t *testing.T) {
	existing := `{
  "permissions": {"allow": ["Bash(npm test)"], "ask": []},
  "env": {"FOO": "bar"},
  "maxThing": 12345678901234567890,
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "notify-send done", "timeout": 5}]},
      {"matcher": "x", "hooks": [{"type": "command", "command": "other --flag"}]}
    ],
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "check.sh"}]}]
  }
}`
	root, path := settingsFile(t, &existing)
	if err := InstallStandaloneHooks(root, exe); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "12345678901234567890") {
		t.Fatalf("large number was altered:\n%s", b)
	}
	s := load(t, path)
	if s["env"].(map[string]any)["FOO"] != "bar" || s["permissions"] == nil {
		t.Fatal("other keys were lost")
	}
	stop := commandsFor(s, "Stop")
	want := []string{"notify-send done", "other --flag", `"/opt/my tools/agentlog" hook-stop`}
	if strings.Join(stop, "|") != strings.Join(want, "|") {
		t.Fatalf("Stop hooks = %v, want %v", stop, want)
	}
	if got := commandsFor(s, "PreToolUse"); len(got) != 1 || got[0] != "check.sh" {
		t.Fatalf("PreToolUse = %v", got)
	}
}

func TestInstallReplacesOldAgentlogHooksWithoutDuplicating(t *testing.T) {
	old := `{"hooks":{"Stop":[
	  {"hooks":[{"type":"command","command":"\"/old/path/agentlog\" hook-stop","timeout":30}]},
	  {"hooks":[{"type":"command","command":"python3 /x/agentlog.py hook-stop"},{"type":"command","command":"keep-me"}]}
	]}}`
	root, path := settingsFile(t, &old)
	for i := 0; i < 2; i++ { // twice: must stay the same
		if err := InstallStandaloneHooks(root, exe); err != nil {
			t.Fatal(err)
		}
	}
	stop := commandsFor(load(t, path), "Stop")
	if strings.Join(stop, "|") != `keep-me|"/opt/my tools/agentlog" hook-stop` {
		t.Fatalf("Stop hooks = %v", stop)
	}
}

func TestInstallIsIdempotentByteForByte(t *testing.T) {
	root, path := settingsFile(t, nil)
	_ = InstallStandaloneHooks(root, exe)
	first, _ := os.ReadFile(path)
	_ = InstallStandaloneHooks(root, exe)
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Fatalf("second install changed the file:\n%s\n---\n%s", first, second)
	}
}

func TestInvalidJSONIsLeftUntouched(t *testing.T) {
	for _, bad := range []string{"{not json", `["array"]`, `{"a":1} trailing`, "null"} {
		root, path := settingsFile(t, &bad)
		err := InstallStandaloneHooks(root, exe)
		if err != ErrInvalidSettings {
			t.Errorf("%q: err = %v, want ErrInvalidSettings", bad, err)
		}
		if b, _ := os.ReadFile(path); string(b) != bad {
			t.Errorf("%q: file was modified to %q", bad, b)
		}
		if _, err := RemoveStandaloneHooks(root); err != ErrInvalidSettings {
			t.Errorf("remove %q: err = %v", bad, err)
		}
		if b, _ := os.ReadFile(path); string(b) != bad {
			t.Errorf("remove %q modified the file", bad)
		}
	}
	snippet := Snippet(exe)
	var v map[string]any
	if err := json.Unmarshal([]byte(snippet), &v); err != nil || !strings.Contains(snippet, "hook-stop") {
		t.Fatalf("snippet is not usable JSON: %v\n%s", err, snippet)
	}
}

func TestRemovePreservesEverythingElse(t *testing.T) {
	existing := `{"env":{"A":"1"},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"keep"}]}],"PreToolUse":[{"hooks":[{"type":"command","command":"pre"}]}]}}`
	root, path := settingsFile(t, &existing)
	_ = InstallStandaloneHooks(root, exe)
	removed, err := RemoveStandaloneHooks(root)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	s := load(t, path)
	if got := commandsFor(s, "Stop"); len(got) != 1 || got[0] != "keep" {
		t.Fatalf("Stop = %v", got)
	}
	if got := commandsFor(s, "PreToolUse"); len(got) != 1 || got[0] != "pre" {
		t.Fatalf("PreToolUse = %v", got)
	}
	if _, ok := s["hooks"].(map[string]any)["SessionStart"]; ok {
		t.Fatal("empty SessionStart event should be pruned")
	}
	if s["env"].(map[string]any)["A"] != "1" {
		t.Fatal("env lost")
	}
}

func TestRemoveDeletesFileThatBecomesEmpty(t *testing.T) {
	root, path := settingsFile(t, nil)
	_ = InstallStandaloneHooks(root, exe)
	if removed, err := RemoveStandaloneHooks(root); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("settings file should be deleted when nothing else is left")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("empty .claude directory should be removed too")
	}
}

func TestRemoveNoOps(t *testing.T) {
	root, _ := settingsFile(t, nil)
	if removed, err := RemoveStandaloneHooks(root); err != nil || removed {
		t.Fatalf("missing file: removed=%v err=%v", removed, err)
	}
	other := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify"}]}]}}`
	root2, path2 := settingsFile(t, &other)
	if removed, err := RemoveStandaloneHooks(root2); err != nil || removed {
		t.Fatalf("no agentlog hooks: removed=%v err=%v", removed, err)
	}
	if b, _ := os.ReadFile(path2); string(b) != other {
		t.Fatal("file without agentlog hooks must not be rewritten")
	}
}
