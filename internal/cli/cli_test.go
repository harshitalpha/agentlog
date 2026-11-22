package cli

import (
	"bytes"
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

func TestStubsDispatch(t *testing.T) {
	for _, c := range commands() {
		code, _, errOut := runCapture(c.name)
		switch c.name {
		case "init", "disable", "status", "uninstall":
			continue // implemented; see their own tests
		}
		if strings.HasPrefix(c.name, "hook-") {
			if code != 0 {
				t.Errorf("%s must always exit 0, got %d", c.name, code)
			}
			continue // implemented; behaviour is covered in hooks_test.go
		}
		if code != 1 || !strings.Contains(errOut, c.name+" is not implemented") {
			t.Errorf("%s: code=%d err=%q", c.name, code, errOut)
		}
	}
}
