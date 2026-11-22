package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/harshitalpha/agentlog/internal/server"
	"github.com/harshitalpha/agentlog/internal/uiproc"
)

func TestResolvePort(t *testing.T) {
	cases := []struct {
		flag    int
		env     string
		want    int
		wantErr bool
	}{
		{-1, "", server.DefaultPort, false},
		{-1, "8123", 8123, false},
		{9000, "8123", 9000, false}, // flag beats env
		{0, "8123", 0, false},       // 0 = any free port
		{-1, "abc", server.DefaultPort, false},
		{-1, "0", server.DefaultPort, false},
		{-1, "70000", server.DefaultPort, false},
		{-1, "-5", server.DefaultPort, false},
		{70000, "", 0, true},
		{-7, "", 0, true},
	}
	for _, c := range cases {
		got, err := resolvePort(c.flag, c.env)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("resolvePort(%d,%q) = %d, %v; want %d err=%v", c.flag, c.env, got, err, c.want, c.wantErr)
		}
	}
}

// syncBuf is a goroutine-safe buffer.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

var urlRe = regexp.MustCompile(`→ (http://127\.0\.0\.1:\d+)`)

// startUI runs `agentlog ui args...` in the background and returns its base
// URL, buffers and a stop function that cancels it and returns the exit code.
func startUI(t *testing.T, args ...string) (base string, out, errOut *syncBuf, stop func() int) {
	t.Helper()
	out, errOut = &syncBuf{}, &syncBuf{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() {
		done <- run(append([]string{"ui"}, args...), streams{ctx: ctx, in: strings.NewReader(""), out: out, err: errOut})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m := urlRe.FindStringSubmatch(out.String()); m != nil {
			base = m[1]
			break
		}
		select {
		case code := <-done:
			t.Fatalf("ui exited early with %d\nstdout: %s\nstderr: %s", code, out.String(), errOut.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if base == "" {
		cancel()
		t.Fatalf("no URL printed; stdout=%q stderr=%q", out.String(), errOut.String())
	}
	return base, out, errOut, func() int {
		cancel()
		select {
		case code := <-done:
			return code
		case <-time.After(5 * time.Second):
			t.Fatal("ui did not stop after cancel")
			return -1
		}
	}
}

func initedRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)
	for _, kv := range [][2]string{{"user.name", "T"}, {"user.email", "t@x"}, {"commit.gpgsign", "false"}} {
		if out, err := exec.Command("git", "-C", dir, "config", "--local", kv[0], kv[1]).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	_ = os.WriteFile(dir+"/a.txt", []byte("x\n"), 0o644)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "First commit"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	}
	return dir
}

func TestUIForegroundServesAndStopsCleanly(t *testing.T) {
	var opened []string
	var mu sync.Mutex
	old := uiproc.OpenURL
	uiproc.OpenURL = func(u string) error { mu.Lock(); opened = append(opened, u); mu.Unlock(); return nil }
	defer func() { uiproc.OpenURL = old }()

	dir := initedRepo(t)
	base, out, _, stop := startUI(t, dir, "--port", "0")

	resp, err := http.Get(base + "/api/repo")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Path, Head, Version string
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	wantPath, _ := filepath.EvalSymlinks(dir)
	gotPath, _ := filepath.EvalSymlinks(info.Path)
	if info.Head == "" || gotPath != wantPath {
		t.Fatalf("info = %+v, want path %s", info, wantPath)
	}
	if !strings.Contains(out.String(), "agentlog UI for ") || !strings.Contains(out.String(), "(Ctrl+C to stop)") {
		t.Fatalf("banner = %q", out.String())
	}

	// The browser is opened once, with the served URL.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(opened)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	if len(opened) != 1 || opened[0] != base {
		t.Fatalf("opened = %v, want [%s]", opened, base)
	}
	mu.Unlock()

	if code := stop(); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "UI stopped") {
		t.Fatalf("stdout after stop = %q", out.String())
	}
	if _, err := http.Get(base + "/api/repo"); err == nil {
		t.Fatal("server still up after stop")
	}
}

func TestUINoOpenDoesNotOpenBrowser(t *testing.T) {
	called := make(chan string, 1)
	old := uiproc.OpenURL
	uiproc.OpenURL = func(u string) error { called <- u; return nil }
	defer func() { uiproc.OpenURL = old }()

	_, _, _, stop := startUI(t, initedRepo(t), "--port", "0", "--no-open")
	select {
	case u := <-called:
		t.Fatalf("--no-open still opened %s", u)
	case <-time.After(700 * time.Millisecond):
	}
	stop()
}

func TestUIBrowserFailureDoesNotKillServer(t *testing.T) {
	old := uiproc.OpenURL
	uiproc.OpenURL = func(string) error { return os.ErrNotExist }
	defer func() { uiproc.OpenURL = old }()

	base, _, errOut, stop := startUI(t, initedRepo(t), "--port", "0")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(errOut.String(), "could not open a browser") {
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(errOut.String(), "could not open a browser") || !strings.Contains(errOut.String(), base) {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if resp, err := http.Get(base + "/api/repo"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("server not serving after browser failure: %v", err)
	}
	if code := stop(); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestUIPortFromEnvAndFlagPrecedence(t *testing.T) {
	pick := func() string {
		ln, err := server.Listen(0)
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		ln.Close()
		return addr[strings.LastIndex(addr, ":")+1:]
	}
	envPort, flagPort := pick(), pick()
	t.Setenv(envUIPort, envPort)

	base, _, _, stop := startUI(t, initedRepo(t), "--no-open")
	if !strings.HasSuffix(base, ":"+envPort) {
		t.Errorf("env port: base = %s, want port %s", base, envPort)
	}
	stop()

	base, _, _, stop = startUI(t, initedRepo(t), "--no-open", "--port", flagPort)
	if !strings.HasSuffix(base, ":"+flagPort) {
		t.Errorf("flag port: base = %s, want port %s", base, flagPort)
	}
	stop()
}

func TestUIIdleTimeoutExitsZero(t *testing.T) {
	old := uiIdleTimeout
	uiIdleTimeout = 300 * time.Millisecond
	defer func() { uiIdleTimeout = old }()

	out, errOut := &syncBuf{}, &syncBuf{}
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"ui", initedRepo(t), "--no-open", "--port", "0"},
			streams{ctx: context.Background(), in: strings.NewReader(""), out: out, err: errOut})
	}()
	select {
	case code := <-done:
		if code != 0 || !strings.Contains(out.String(), "stopped after being idle") {
			t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("idle timeout never fired")
	}
}

func TestUIErrors(t *testing.T) {
	code, _, errOut := runWithStdin("", "ui", t.TempDir(), "--no-open")
	if code != 1 || !strings.Contains(errOut, "not inside a git repo") || !strings.Contains(errOut, "agentlog init") {
		t.Fatalf("non-repo: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runWithStdin("", "ui", "--port", "99999"); code != 2 || !strings.Contains(errOut, "--port") {
		t.Fatalf("bad port: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runWithStdin("", "ui", "--detach", "--stop"); code != 2 || !strings.Contains(errOut, "cannot be used together") {
		t.Fatalf("detach+stop: code=%d err=%q", code, errOut)
	}
	if code, _, _ := runWithStdin("", "ui", "--bogus"); code != 2 {
		t.Fatalf("bad flag: code=%d", code)
	}
}
