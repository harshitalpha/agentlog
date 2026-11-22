package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCommandEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runWithStdin("", "init", dir)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q out=%q", code, errOut, out)
	}
	if !strings.Contains(out, "✓ agentlog enabled in") {
		t.Fatalf("out = %q", out)
	}
	log, _ := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if strings.TrimSpace(string(log)) != "Add agentlog commit rules for AI agents\nBaseline snapshot before agent work" {
		t.Fatalf("log = %q", log)
	}
	// Second run: still success, no new commit.
	code, _, _ = runWithStdin("", "init", dir)
	log2, _ := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if code != 0 || string(log2) != string(log) {
		t.Fatalf("second run changed history: %q", log2)
	}
}

func TestInitFlagsMayFollowThePath(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644)
	code, _, errOut := runWithStdin("", "init", dir, "--claude-md", "--no-agents-md")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal("--claude-md after the path was ignored")
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("--no-agents-md after the path was ignored")
	}
}

func TestInitUsageAndRuntimeErrors(t *testing.T) {
	code, _, errOut := runWithStdin("", "init", "--bogus")
	if code != 2 || !strings.Contains(errOut, "agentlog: init:") {
		t.Fatalf("bad flag: code=%d err=%q", code, errOut)
	}
	code, _, errOut = runWithStdin("", "init", "a", "b")
	if code != 2 || !strings.Contains(errOut, "at most one path") {
		t.Fatalf("two paths: code=%d err=%q", code, errOut)
	}
	code, out, errOut := runWithStdin("", "init", "/definitely/not/a/dir")
	if code != 1 || out != "" || !strings.HasPrefix(errOut, "agentlog: ") {
		t.Fatalf("missing dir: code=%d out=%q err=%q", code, out, errOut)
	}
}
