package cli

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runCapture(args ...string) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = run(args, streams{in: strings.NewReader(""), out: &o, err: &e})
	return code, o.String(), e.String()
}

func TestNoArgsIsUsageError(t *testing.T) {
	code, out, errOut := runCapture()
	if code != 2 || out != "" || !strings.Contains(errOut, "Usage:") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, out, errOut := runCapture("bogus")
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if out != "" || !strings.Contains(errOut, `agentlog: unknown command "bogus"`) {
		t.Fatalf("out=%q err=%q", out, errOut)
	}
}

func TestVersion(t *testing.T) {
	old := Version
	Version = "9.9.9"
	defer func() { Version = old }()
	code, out, _ := runCapture("--version")
	if code != 0 || strings.TrimSpace(out) != "9.9.9" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	code, out, _ := runCapture("--help")
	if code != 0 {
		t.Fatalf("code = %d", code)
	}
	for _, c := range commands() {
		if !strings.Contains(out, c.name) {
			t.Errorf("help is missing %q", c.name)
		}
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	for _, c := range commands() {
		code, out, _ := runCapture(c.name, "--help")
		if code != 0 || !strings.Contains(out, c.usage) {
			t.Errorf("%s --help: code=%d out=%q", c.name, code, out)
		}
	}
}

// stub() is kept for future commands; make sure it fails loudly, not silently.
func TestStubFailsLoudly(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := stub("x")(nil, streams{out: &out, err: &errOut}); code != 1 || !strings.Contains(errOut.String(), "x is not implemented yet") {
		t.Fatalf("stub: code=%d err=%q", code, errOut.String())
	}
}

// Guards the test setup itself: tests must never run inside the real repo.
func TestWorkingDirectoryIsNotAGitRepo(t *testing.T) {
	wd, _ := os.Getwd()
	if out, err := exec.Command("git", "-C", wd, "rev-parse", "--show-toplevel").Output(); err == nil {
		t.Fatalf("tests run inside a git repo (%s); a stray command could modify it", strings.TrimSpace(string(out)))
	}
}
