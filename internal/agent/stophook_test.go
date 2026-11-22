package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OWNER/agentlog/internal/gitx"
)

func stopInput(t *testing.T, cwd string, active bool) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"session_id": "s1", "cwd": cwd, "hook_event_name": "Stop",
		"stop_hook_active": active, "transcript_path": "/x", "last_assistant_message": "done",
		"background_tasks": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func enabledRepo(t *testing.T) *gitx.Repo {
	t.Helper()
	r := newRepo(t)
	if err := r.SetLocal(ConfigKey, "true"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, r, "base.txt", "base\n")
	if _, _, err := r.AutoCommit("base"); err != nil {
		t.Fatal(err)
	}
	return r
}

func writeFile(t *testing.T, r *gitx.Repo, rel, content string) {
	t.Helper()
	p := filepath.Join(r.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitCount(t *testing.T, r *gitx.Repo) int {
	t.Helper()
	cs, err := r.Log(100)
	if err != nil {
		t.Fatal(err)
	}
	return len(cs)
}

// CRITICAL: a repo that never opted in must never be blocked or committed to.
func TestStopNotEnabledAndDirtyIsSilentAndUntouched(t *testing.T) {
	r := newRepo(t)
	writeFile(t, r, "base.txt", "base\n")
	_ = r.AddAll()
	_ = r.Commit("Base", "", gitx.CommitOpts{})
	writeFile(t, r, "dirty.txt", "x\n")
	before := commitCount(t, r)

	for _, active := range []bool{false, true} {
		if out := HandleStop(stopInput(t, r.Root, active), env(nil)); out != "" {
			t.Fatalf("active=%v: expected no output, got %q", active, out)
		}
	}
	if commitCount(t, r) != before {
		t.Fatal("a commit was created in a repo that is not enabled")
	}
	if files, _ := r.DirtyFiles(); len(files) != 1 {
		t.Fatalf("working tree was changed: %v", files)
	}
}

func TestStopCleanTreeAllowsStop(t *testing.T) {
	r := enabledRepo(t)
	for _, active := range []bool{false, true} {
		if out := HandleStop(stopInput(t, r.Root, active), env(nil)); out != "" {
			t.Fatalf("active=%v: expected no output, got %q", active, out)
		}
	}
}

func TestStopDirtyFirstAttemptBlocks(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, "base.txt", "changed\n")
	writeFile(t, r, "internal/new.go", "package x\n")
	before := commitCount(t, r)

	out := HandleStop(stopInput(t, r.Root, false), env(nil))
	if strings.Contains(out, "\n") {
		t.Fatalf("output must be one line: %q", out)
	}
	var v stopBlock
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if v.Decision != "block" {
		t.Fatalf("decision = %q", v.Decision)
	}
	for _, want := range []string{
		"agentlog: you have uncommitted changes",
		"one commit per logical step",
		"Do NOT push, amend or rebase",
		`trailer "Agent: Claude"`,
		"Uncommitted files:",
		"  base.txt",
		"  internal/new.go",
	} {
		if !strings.Contains(v.Reason, want) {
			t.Errorf("reason missing %q:\n%s", want, v.Reason)
		}
	}
	if commitCount(t, r) != before {
		t.Fatal("first attempt must not commit anything")
	}
}

func TestStopDirtySecondAttemptAutoCommits(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, "x.go", "x\n")
	before := commitCount(t, r)

	if out := HandleStop(stopInput(t, r.Root, true), env(nil)); out != "" {
		t.Fatalf("expected no output, got %q", out)
	}
	cs, _ := r.Log(1)
	if commitCount(t, r) != before+1 {
		t.Fatal("no auto-commit was created")
	}
	if cs[0].Subject != "[auto] uncommitted changes: x.go" || cs[0].Author != "agentlog" {
		t.Fatalf("commit = %+v", cs[0])
	}
	if DetectAgent(cs[0].Author, cs[0].Body) != "auto" {
		t.Fatal("auto-commit must be detected as auto")
	}
	if files, _ := r.DirtyFiles(); len(files) != 0 {
		t.Fatalf("tree still dirty: %v", files)
	}
}

func TestStopNonRepoCwdIsSilent(t *testing.T) {
	if out := HandleStop(stopInput(t, t.TempDir(), false), env(nil)); out != "" {
		t.Fatalf("got %q", out)
	}
}

func TestStopMalformedInputIsSilent(t *testing.T) {
	for _, bad := range []string{"{nope", "[", "\xff\xfe"} {
		if out := HandleStop([]byte(bad), env(nil)); out != "" {
			t.Errorf("input %q gave %q", bad, out)
		}
	}
}

func TestStopListIsCappedAt40(t *testing.T) {
	r := enabledRepo(t)
	for i := 0; i < 55; i++ {
		writeFile(t, r, fmt.Sprintf("many/f%02d.txt", i), "x\n")
	}
	out := HandleStop(stopInput(t, r.Root, false), env(nil))
	var v stopBlock
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(v.Reason, "  many/f"); n != 40 {
		t.Fatalf("listed %d files, want 40", n)
	}
	if !strings.Contains(v.Reason, "… and 15 more") {
		t.Fatalf("missing overflow line:\n%s", v.Reason)
	}
}

func TestStopEnforceEverywhereBlocksUnenabledRepo(t *testing.T) {
	r := newRepo(t) // never opted in
	writeFile(t, r, "a.txt", "x\n")
	out := HandleStop(stopInput(t, r.Root, false), env(map[string]string{EnvEnforceEverywhere: "true"}))
	if !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("expected block, got %q", out)
	}
}

func TestStopIgnoredFilesDoNotBlock(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, ".gitignore", "*.log\n")
	_, _, _ = r.AutoCommit("ignore")
	writeFile(t, r, "debug.log", "noise\n")
	if out := HandleStop(stopInput(t, r.Root, false), env(nil)); out != "" {
		t.Fatalf("ignored file triggered the hook: %q", out)
	}
}

func TestStopAutoCommitFailureIsSwallowed(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, "a.txt", "x\n")
	// Make git unable to commit: corrupt HEAD's index lock.
	lock := filepath.Join(r.Root, ".git", "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(lock)
	if out := HandleStop(stopInput(t, r.Root, true), env(nil)); out != "" {
		t.Fatalf("failure must not produce output, got %q", out)
	}
}
