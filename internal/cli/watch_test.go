package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWatchCommandAutoCommitsAndStopsOnCancel(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "T"}, {"config", "user.email", "t@x"}, {"config", "commit.gpgsign", "false"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	var out, errOut bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"watch", dir, "--quiet", "0.3", "--interval", "0.05"},
			streams{ctx: ctx, in: strings.NewReader(""), out: &lockedWriter{w: &out}, err: &errOut})
	}()

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		log, _ := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
		if strings.Contains(string(log), "[auto] edits: a.txt") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 on Ctrl+C", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not exit after cancel")
	}
	log, _ := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if !strings.Contains(string(log), "[auto] edits: a.txt") {
		t.Fatalf("no auto-commit happened; log=%q stderr=%q", log, errOut.String())
	}
	if !strings.Contains(out.String(), "watching ") || !strings.Contains(out.String(), "[auto] edits: a.txt") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestWatchArgumentErrors(t *testing.T) {
	if code, _, errOut := runWithStdin("", "watch", "--quiet", "0"); code != 2 || !strings.Contains(errOut, "greater than 0") {
		t.Fatalf("zero quiet: code=%d err=%q", code, errOut)
	}
	if code, _, _ := runWithStdin("", "watch", "--nope"); code != 2 {
		t.Fatalf("bad flag: code=%d", code)
	}
	if code, _, errOut := runWithStdin("", "watch", t.TempDir()); code != 1 || !strings.Contains(errOut, "not inside a git repo") {
		t.Fatalf("non-repo: code=%d err=%q", code, errOut)
	}
}
