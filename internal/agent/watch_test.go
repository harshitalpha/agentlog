package agent

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OWNER/agentlog/internal/gitx"
)

// safeBuf is a bytes.Buffer that is safe to read while Watch writes to it.
type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *safeBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// startWatch runs Watch in the background and returns a stop function that
// cancels it and waits for it to return.
func startWatch(t *testing.T, r *gitx.Repo, opts WatchOptions) (out *safeBuf, stop func()) {
	t.Helper()
	out = &safeBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Watch(ctx, r, opts, out, &bytes.Buffer{}) }()
	return out, func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Watch returned %v on cancel", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Watch did not stop after cancel")
		}
	}
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func watchRepo(t *testing.T) *gitx.Repo {
	t.Helper()
	r := newRepo(t) // NOT enabled: watch ignores the flag
	writeFile(t, r, "base.txt", "base\n")
	_ = r.AddAll()
	if err := r.Commit("Base", "", gitx.CommitOpts{}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWatchCommitsAfterQuietPeriod(t *testing.T) {
	r := watchRepo(t)
	out, stop := startWatch(t, r, WatchOptions{Quiet: 300 * time.Millisecond, Interval: 40 * time.Millisecond})
	defer stop()

	writeFile(t, r, "a.go", "package a\n")
	writeFile(t, r, "b.go", "package b\n")
	waitFor(t, "auto-commit", 5*time.Second, func() bool { return commitCount(t, r) == 2 })

	cs, _ := r.Log(1)
	if cs[0].Subject != "[auto] edits: a.go, b.go" || cs[0].Author != "agentlog" {
		t.Fatalf("commit = %+v", cs[0])
	}
	waitFor(t, "printed line", time.Second, func() bool { return out.String() != "" })
	line := strings.TrimSpace(out.String())
	// "HH:MM:SS [auto] edits: a.go, b.go"
	if len(line) < 9 || line[2] != ':' || line[5] != ':' || !strings.HasSuffix(line, " [auto] edits: a.go, b.go") {
		t.Fatalf("output line = %q", line)
	}
}

func TestWatchWaitsWhileEditsContinue(t *testing.T) {
	r := watchRepo(t)
	_, stop := startWatch(t, r, WatchOptions{Quiet: 400 * time.Millisecond, Interval: 40 * time.Millisecond})
	defer stop()

	// Keep editing every 100 ms for 1.2 s: the quiet period never elapses.
	end := time.Now().Add(1200 * time.Millisecond)
	for i := 0; time.Now().Before(end); i++ {
		writeFile(t, r, "busy.txt", strings.Repeat("x", i%7+1)+"\n")
		time.Sleep(100 * time.Millisecond)
		if n := commitCount(t, r); n != 1 {
			t.Fatalf("committed while edits were still arriving (commits=%d)", n)
		}
	}
	waitFor(t, "commit after edits stop", 5*time.Second, func() bool { return commitCount(t, r) == 2 })
}

func TestWatchLeavesCleanTreeAlone(t *testing.T) {
	r := watchRepo(t)
	out, stop := startWatch(t, r, WatchOptions{Quiet: 100 * time.Millisecond, Interval: 30 * time.Millisecond})
	time.Sleep(500 * time.Millisecond)
	stop()
	if commitCount(t, r) != 1 || out.String() != "" {
		t.Fatalf("clean tree produced commits=%d out=%q", commitCount(t, r), out.String())
	}
}

func TestWatchStopsPromptlyOnCancel(t *testing.T) {
	r := watchRepo(t)
	_, stop := startWatch(t, r, WatchOptions{Quiet: time.Hour, Interval: time.Hour})
	stop() // fails the test if Watch doesn't return within 3 s
}

func TestWatchCommitsSeparateBatchesSeparately(t *testing.T) {
	r := watchRepo(t)
	_, stop := startWatch(t, r, WatchOptions{Quiet: 250 * time.Millisecond, Interval: 40 * time.Millisecond})
	defer stop()
	writeFile(t, r, "one.txt", "1\n")
	waitFor(t, "first batch", 5*time.Second, func() bool { return commitCount(t, r) == 2 })
	writeFile(t, r, "two.txt", "2\n")
	waitFor(t, "second batch", 5*time.Second, func() bool { return commitCount(t, r) == 3 })
}
