package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sessionRepo builds a repo like a real session: init → agent commits →
// human commit → auto commit → two dirty files.
func sessionRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	g := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	w := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("main.go", "package main\n")
	if code, _, errOut := runWithStdin("", "init", dir); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	g("config", "commit.gpgsign", "false")
	for i, name := range []string{"a.go", "b.go", "c.go"} {
		w(name, "x\n")
		g("add", "-A")
		g("commit", "-q", "-m", "Add "+name, "-m", "Agent: Claude")
		_ = i
	}
	w("human.txt", "h\n")
	g("add", "-A")
	g("commit", "-q", "-m", "Human change")
	w("auto.txt", "a\n")
	g("add", "-A")
	g("-c", "user.name=agentlog", "-c", "user.email=agentlog@localhost", "commit", "-q", "-m", "[auto] edits: auto.txt", "-m", "Agent: auto")
	w("main.go", "package main // edited\n")
	w("new.txt", "n\n")
	return dir
}

func TestStatusHumanOutput(t *testing.T) {
	dir := sessionRepo(t)
	code, out, errOut := runWithStdin("", "status", dir)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	for _, want := range []string{
		"enabled:      yes (this repo)",
		"commits since baseline: 5 (3 agent, 1 human, 1 auto)",
		"uncommitted:  2 files",
		"not running",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatusJSON(t *testing.T) {
	dir := sessionRepo(t)
	code, out, _ := runWithStdin("", "status", dir, "--json")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var st struct {
		Enabled     bool   `json:"enabled"`
		EnabledVia  string `json:"enabled_via"`
		Branch      string `json:"branch"`
		HasBaseline bool   `json:"has_baseline"`
		Commits     struct {
			Agent, Human, Auto int
		} `json:"commits_since_baseline"`
		Uncommitted int    `json:"uncommitted_files"`
		UIURL       string `json:"ui_url"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !st.Enabled || st.EnabledVia != "repo" || !st.HasBaseline || st.Branch == "" ||
		st.Commits.Agent != 3 || st.Commits.Human != 1 || st.Commits.Auto != 1 ||
		st.Uncommitted != 2 || st.UIURL != "" {
		t.Fatalf("unexpected status: %+v\n%s", st, out)
	}
}

func TestStatusNotEnabledAndEnforceEverywhere(t *testing.T) {
	dir := t.TempDir()
	_ = exec.Command("git", "-C", dir, "init", "-q").Run()
	_, out, _ := runWithStdin("", "status", dir)
	if !strings.Contains(out, "enabled:      no") || !strings.Contains(out, "commits: 0") {
		t.Fatalf("out:\n%s", out)
	}
	t.Setenv("CLAUDE_PLUGIN_OPTION_ENFORCE_EVERYWHERE", "true")
	_, out, _ = runWithStdin("", "status", dir)
	if !strings.Contains(out, "yes (enforce_everywhere is on)") {
		t.Fatalf("out:\n%s", out)
	}
	_, js, _ := runWithStdin("", "status", dir, "--json")
	if !strings.Contains(js, `"enabled_via": "enforce_everywhere"`) {
		t.Fatalf("json:\n%s", js)
	}
}

func TestStatusNotARepo(t *testing.T) {
	code, out, errOut := runWithStdin("", "status", t.TempDir())
	if code != 1 || out != "" || !strings.Contains(errOut, "not inside a git repo") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestDisableKeepsCommitsAndRules(t *testing.T) {
	dir := sessionRepo(t)
	headBefore, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()

	code, out, errOut := runWithStdin("", "disable", dir)
	if code != 0 || errOut != "" || !strings.Contains(out, "✓ agentlog disabled in") || !strings.Contains(out, "(commits untouched)") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if v, _ := exec.Command("git", "-C", dir, "config", "--local", "--get", "agentlog.enabled").Output(); len(v) != 0 {
		t.Fatalf("agentlog.enabled still set: %q", v)
	}
	headAfter, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if string(headBefore) != string(headAfter) {
		t.Fatal("disable changed history")
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatal("disable must leave AGENTS.md alone")
	}

	// Idempotent, and status reflects it.
	if code, _, _ := runWithStdin("", "disable", dir); code != 0 {
		t.Fatal("second disable should succeed")
	}
	if _, out, _ := runWithStdin("", "status", dir); !strings.Contains(out, "enabled:      no") {
		t.Fatalf("status after disable:\n%s", out)
	}
	// And the Stop hook is silent again.
	stdin := `{"cwd":"` + dir + `","stop_hook_active":false}`
	if _, out, _ := runWithStdin(stdin, "hook-stop"); out != "" {
		t.Fatalf("hook-stop still active after disable: %q", out)
	}
}

func TestDisableNotARepo(t *testing.T) {
	if code, _, errOut := runWithStdin("", "disable", t.TempDir()); code != 1 || !strings.Contains(errOut, "not inside a git repo") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}
