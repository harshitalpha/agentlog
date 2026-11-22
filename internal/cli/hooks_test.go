package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Run from a throwaway directory so no command that defaults to "." can
	// ever touch the real repository the tests live in.
	scratch, err := os.MkdirTemp("", "agentlog-cli-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(scratch); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(scratch)
	os.Exit(code)
}

func runWithStdin(stdin string, args ...string) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = run(args, streams{in: strings.NewReader(stdin), out: &o, err: &e})
	return code, o.String(), e.String()
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestHookSessionStartEnabledRepo(t *testing.T) {
	dir := gitRepo(t)
	if out, err := exec.Command("git", "-C", dir, "config", "--local", "agentlog.enabled", "true").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	code, out, errOut := runWithStdin(`{"cwd":"`+dir+`","session_id":"s"}`, "hook-session-start")
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("want exactly one line, got %q", out)
	}
	var v struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if v.HookSpecificOutput.HookEventName != "SessionStart" ||
		!strings.Contains(v.HookSpecificOutput.AdditionalContext, "Commit discipline") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestHookSessionStartAlwaysExitsZeroAndStaysSilent(t *testing.T) {
	for name, stdin := range map[string]string{
		"malformed":   "{oops",
		"empty":       "",
		"not a repo":  `{"cwd":"` + t.TempDir() + `"}`,
		"missing dir": `{"cwd":"/definitely/not/here"}`,
	} {
		code, out, _ := runWithStdin(stdin, "hook-session-start")
		if code != 0 {
			t.Errorf("%s: exit code %d", name, code)
		}
		if name != "empty" && out != "" { // empty stdin falls back to the working dir, which may be a repo
			t.Errorf("%s: expected no stdout, got %q", name, out)
		}
	}
}

func TestHookSessionStartHintInNonEnabledRepo(t *testing.T) {
	dir := gitRepo(t)
	code, out, _ := runWithStdin(`{"cwd":"`+dir+`"}`, "hook-session-start")
	if code != 0 || !strings.Contains(out, "/agentlog:init") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestHookStopThroughCLI(t *testing.T) {
	dir := gitRepo(t)
	for _, kv := range [][2]string{{"agentlog.enabled", "true"}, {"user.name", "T"}, {"user.email", "t@x"}, {"commit.gpgsign", "false"}} {
		if out, err := exec.Command("git", "-C", dir, "config", "--local", kv[0], kv[1]).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	if err := os.WriteFile(dir+"/a.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runWithStdin(`{"cwd":"`+dir+`","stop_hook_active":false}`, "hook-stop")
	if code != 0 || errOut != "" || !strings.Contains(out, `"decision":"block"`) || strings.Count(out, "\n") != 1 {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	code, out, _ = runWithStdin(`{"cwd":"`+dir+`","stop_hook_active":true}`, "hook-stop")
	if code != 0 || out != "" {
		t.Fatalf("second attempt: code=%d out=%q", code, out)
	}
	log, _ := exec.Command("git", "-C", dir, "log", "--format=%an|%s").Output()
	if strings.TrimSpace(string(log)) != "agentlog|[auto] uncommitted changes: a.txt" {
		t.Fatalf("log = %q", log)
	}
}

func TestHookStopNeverNoisyOnBadInput(t *testing.T) {
	for _, in := range []string{"", "{bad", `{"cwd":"/nope"}`} {
		code, out, errOut := runWithStdin(in, "hook-stop")
		if code != 0 || errOut != "" {
			t.Errorf("input %q: code=%d err=%q", in, code, errOut)
		}
		_ = out
	}
}

// lockedWriter makes a bytes.Buffer safe for a goroutine to write while the
// test reads it.
type lockedWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
