package uiproc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OWNER/agentlog/internal/gitx"
)

var (
	binOnce sync.Once
	binPath string
	binErr  error
)

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	scratch, err := os.MkdirTemp("", "agentlog-uiproc-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(scratch); err != nil {
		panic(err)
	}
	code := m.Run()
	if binPath != "" {
		os.RemoveAll(filepath.Dir(binPath))
	}
	os.RemoveAll(scratch)
	os.Exit(code)
}

// agentlogBinary builds the real binary once for the whole package.
func agentlogBinary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		dir, err := os.MkdirTemp("", "agentlog-bin-")
		if err != nil {
			binErr = err
			return
		}
		binPath = filepath.Join(dir, "agentlog")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/agentlog")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = &buildError{err, string(out)}
		}
	})
	if binErr != nil {
		t.Fatalf("building agentlog: %v", binErr)
	}
	return binPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func newRepo(t *testing.T) *gitx.Repo {
	t.Helper()
	r, err := gitx.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"user.name": "T", "user.email": "t@x", "commit.gpgsign": "false"} {
		if err := r.SetLocal(k, v); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(r.Root, "a.txt"), []byte("x\n"), 0o644)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "First"}} {
		if out, err := exec.Command("git", append([]string{"-C", r.Root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	return r
}

// detach starts (or reuses) a server and registers cleanup that always kills it.
func detach(t *testing.T, r *gitx.Repo) (State, bool) {
	t.Helper()
	st, reused, err := Detach(r, DetachOptions{Exe: agentlogBinary(t), Port: 0, Wait: 10 * time.Second})
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}
	t.Cleanup(func() { _, _ = Stop(r) })
	return st, reused
}

func httpOK(url string) bool {
	resp, err := http.Get(url + "/api/repo")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid %d is still alive", pid)
}

func TestDetachReuseAndStop(t *testing.T) {
	r := newRepo(t)
	st, reused := detach(t, r)
	if reused {
		t.Fatal("first Detach cannot be a reuse")
	}
	if st.PID <= 1 || st.Port == 0 || !strings.HasPrefix(st.URL, "http://127.0.0.1:") || st.Started.IsZero() {
		t.Fatalf("state = %+v", st)
	}
	if !httpOK(st.URL) || !pidAlive(st.PID) {
		t.Fatal("server is not running")
	}

	// ui.json holds exactly the documented fields.
	dir, _ := r.GitPath("agentlog")
	raw, err := os.ReadFile(filepath.Join(dir, "ui.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"pid", "port", "url", "started"} {
		if _, ok := fields[k]; !ok {
			t.Errorf("ui.json is missing %q: %s", k, raw)
		}
	}
	if log, _ := os.ReadFile(filepath.Join(dir, "ui.log")); !strings.Contains(string(log), "agentlog UI for") {
		t.Errorf("ui.log = %q", log)
	}

	// Detach again: same server, nothing new spawned.
	st2, reused2 := detach(t, r)
	if !reused2 || st2.PID != st.PID || st2.URL != st.URL {
		t.Fatalf("second Detach = %+v reused=%v, want the same server %+v", st2, reused2, st)
	}
	if running, ok := Running(r); !ok || running.PID != st.PID {
		t.Fatalf("Running = %+v, %v", running, ok)
	}

	res, err := Stop(r)
	if err != nil || res != Stopped {
		t.Fatalf("Stop = %v, %v", res, err)
	}
	waitDead(t, st.PID)
	if httpOK(st.URL) {
		t.Fatal("server still answers after Stop")
	}
	if _, err := os.Stat(filepath.Join(dir, "ui.json")); !os.IsNotExist(err) {
		t.Fatal("ui.json should be deleted")
	}
	if res, err := Stop(r); err != nil || res != NotRunning {
		t.Fatalf("second Stop = %v, %v", res, err)
	}
	if _, ok := Running(r); ok {
		t.Fatal("Running after Stop")
	}
}

func TestDetachedServerLeavesParentSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sessions are a unix concept")
	}
	st, _ := detach(t, newRepo(t))
	childSid, err := sessionID(st.PID)
	if err != nil {
		t.Skip("cannot read session id: ", err)
	}
	if mySid, _ := sessionID(os.Getpid()); childSid == mySid {
		t.Fatalf("child shares our session (%d): it would die with the terminal", childSid)
	}
}

func TestDetachReplacesServerKilledBehindOurBack(t *testing.T) {
	r := newRepo(t)
	st, _ := detach(t, r)
	// Kill it behind agentlog's back: ui.json is now stale.
	if p, err := os.FindProcess(st.PID); err == nil {
		_ = p.Kill()
	}
	waitDead(t, st.PID)
	if _, ok := Running(r); ok {
		t.Fatal("a dead server must not count as running")
	}
	st2, reused := detach(t, r)
	if reused || st2.PID == st.PID {
		t.Fatalf("stale state was reused: %+v", st2)
	}
	if !httpOK(st2.URL) {
		t.Fatal("replacement server is not answering")
	}
}

func TestStopNeverKillsAnUnrelatedProcess(t *testing.T) {
	r := newRepo(t)
	dir, _ := r.GitPath("agentlog")
	_ = os.MkdirAll(dir, 0o755)

	// A live process that is not an agentlog server: the test itself, plus a
	// URL that answers 404 for everything.
	imposter := httptest.NewServer(http.NotFoundHandler())
	defer imposter.Close()
	cases := map[string]State{
		"live pid, wrong server": {PID: os.Getpid(), Port: 1, URL: imposter.URL, Started: time.Now()},
		"live pid, no url":       {PID: os.Getpid()},
		"live pid, dead url":     {PID: os.Getpid(), URL: "http://127.0.0.1:1"},
	}
	for name, st := range cases {
		if err := writeState(r, st); err != nil {
			t.Fatal(err)
		}
		res, err := Stop(r)
		if err != nil || res != NotOurs {
			t.Errorf("%s: Stop = %v, %v; want NotOurs", name, res, err)
		}
		if !pidAlive(os.Getpid()) {
			t.Fatalf("%s: the test process was signalled!", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "ui.json")); !os.IsNotExist(err) {
			t.Errorf("%s: stale ui.json must be removed", name)
		}
	}
}

func TestDangerousPidsAreNeverSignalled(t *testing.T) {
	r := newRepo(t)
	dir, _ := r.GitPath("agentlog")
	_ = os.MkdirAll(dir, 0o755)
	for _, pid := range []int{0, 1, -1, -2, -999999} {
		if pidAlive(pid) {
			t.Errorf("pidAlive(%d) = true", pid)
		}
		if err := terminate(pid, time.Millisecond); err == nil {
			t.Errorf("terminate(%d) should refuse", pid)
		}
		_ = writeState(r, State{PID: pid, URL: "http://127.0.0.1:1"})
		if res, err := Stop(r); err != nil || res != NotRunning {
			t.Errorf("Stop with pid %d = %v, %v; want NotRunning", pid, res, err)
		}
		if _, ok := Running(r); ok {
			t.Errorf("Running with pid %d", pid)
		}
	}
}

func TestCorruptStateFiles(t *testing.T) {
	r := newRepo(t)
	dir, _ := r.GitPath("agentlog")
	_ = os.MkdirAll(dir, 0o755)
	for name, content := range map[string]string{
		"garbage": "not json", "empty": "", "array": "[1,2]", "wrong types": `{"pid":"x","url":5}`, "null": "null",
	} {
		_ = os.WriteFile(filepath.Join(dir, "ui.json"), []byte(content), 0o644)
		if _, ok := Running(r); ok {
			t.Errorf("%s: Running = true", name)
		}
		if res, err := Stop(r); err != nil || res != NotRunning {
			t.Errorf("%s: Stop = %v, %v", name, res, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "ui.json")); !os.IsNotExist(err) {
			t.Errorf("%s: corrupt ui.json should be removed by Stop", name)
		}
	}
}

func TestRunningRequiresThisRepo(t *testing.T) {
	// A live server for repo A must not count as running for repo B, even if
	// B's ui.json points at it (pid and port reuse, copied .git, worktrees).
	a, b := newRepo(t), newRepo(t)
	stA, _ := detach(t, a)
	if err := writeState(b, stA); err != nil {
		t.Fatal(err)
	}
	if _, ok := Running(b); ok {
		t.Fatal("repo B adopted repo A's server")
	}
	if res, err := Stop(b); err != nil || res != NotOurs {
		t.Fatalf("Stop(b) = %v, %v; want NotOurs", res, err)
	}
	if !pidAlive(stA.PID) || !httpOK(stA.URL) {
		t.Fatal("stopping repo B killed repo A's server")
	}
}

func TestDetachReportsStartupFailures(t *testing.T) {
	r := newRepo(t)
	if _, _, err := Detach(r, DetachOptions{Exe: filepath.Join(t.TempDir(), "missing"), Port: 0, Wait: time.Second}); err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	// A binary that exits at once (here: `false`) is reported with its log.
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("no `false` binary")
	}
	_, _, err = Detach(r, DetachOptions{Exe: falseBin, Port: 0, Wait: 5 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "exited immediately") {
		t.Fatalf("err = %v", err)
	}
	if _, ok := Running(r); ok {
		t.Fatal("failed start left a running record")
	}
}

func TestDetachTimesOutWhenServerNeverAnswers(t *testing.T) {
	r := newRepo(t)
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	// `sleep ui --no-open ...` fails immediately on most systems; use a script
	// that ignores its arguments and idles, so only the wait loop can end it.
	script := filepath.Join(t.TempDir(), "idle.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec "+sleep+" 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _, err = Detach(r, DetachOptions{Exe: script, Port: 0, Wait: 600 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "did not start within") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("took far longer than the wait")
	}
}
