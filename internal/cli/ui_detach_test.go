package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OWNER/agentlog/internal/uiproc"
)

// useRealBinary makes `ui --detach` re-execute a freshly built agentlog
// instead of the test binary.
func useRealBinary(t *testing.T) {
	t.Helper()
	old := uiExecutable
	uiExecutable = buildBinary(t)
	t.Cleanup(func() { uiExecutable = old })
}

// stopUIAfter guarantees a spawned server never outlives its test.
func stopUIAfter(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() { runWithStdin("", "ui", dir, "--stop") })
}

func isUp(url string) bool {
	resp, err := http.Get(url + "/api/repo")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func TestUIDetachReuseStatusAndStop(t *testing.T) {
	useRealBinary(t)
	dir := initedRepo(t)
	stopUIAfter(t, dir)

	code, out1, errOut := runWithStdin("", "ui", dir, "--detach", "--no-open", "--port", "0")
	url1 := strings.TrimSpace(out1)
	if code != 0 || !strings.HasPrefix(url1, "http://127.0.0.1:") || strings.Contains(url1, "\n") {
		t.Fatalf("first detach: code=%d out=%q err=%q", code, out1, errOut)
	}
	if !isUp(url1) {
		t.Fatal("detached server is not answering")
	}

	// Second call: same URL, same process (the server is reused).
	code, out2, _ := runWithStdin("", "ui", dir, "--detach", "--no-open", "--port", "0")
	if code != 0 || strings.TrimSpace(out2) != url1 {
		t.Fatalf("second detach printed %q, want %q", out2, url1)
	}

	// status sees it, in both formats.
	if _, out, _ := runWithStdin("", "status", dir); !strings.Contains(out, "ui:           "+url1) {
		t.Fatalf("status:\n%s", out)
	}
	_, js, _ := runWithStdin("", "status", dir, "--json")
	var st struct {
		UIURL string `json:"ui_url"`
	}
	if err := json.Unmarshal([]byte(js), &st); err != nil || st.UIURL != url1 {
		t.Fatalf("status --json ui_url = %q (%v)", st.UIURL, err)
	}

	code, out, errOut := runWithStdin("", "ui", dir, "--stop")
	if code != 0 || strings.TrimSpace(out) != "✓ stopped" || errOut != "" {
		t.Fatalf("stop: code=%d out=%q err=%q", code, out, errOut)
	}
	deadline := time.Now().Add(3 * time.Second)
	for isUp(url1) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if isUp(url1) {
		t.Fatal("server still up after --stop")
	}
	if _, out, _ := runWithStdin("", "status", dir); !strings.Contains(out, "not running") {
		t.Fatalf("status after stop:\n%s", out)
	}
	// Stopping twice is fine.
	if code, out, _ := runWithStdin("", "ui", dir, "--stop"); code != 0 || !strings.Contains(out, "not running") {
		t.Fatalf("second stop: code=%d out=%q", code, out)
	}
}

func TestUIDetachOpensBrowserOnlyForNewServer(t *testing.T) {
	useRealBinary(t)
	var mu sync.Mutex
	var opened []string
	old := uiproc.OpenURL
	uiproc.OpenURL = func(u string) error { mu.Lock(); opened = append(opened, u); mu.Unlock(); return nil }
	defer func() { uiproc.OpenURL = old }()

	dir := initedRepo(t)
	stopUIAfter(t, dir)
	_, out, _ := runWithStdin("", "ui", dir, "--detach", "--port", "0")
	url := strings.TrimSpace(out)
	runWithStdin("", "ui", dir, "--detach", "--port", "0") // reuse: no second open

	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 1 || opened[0] != url {
		t.Fatalf("opened = %v, want exactly [%s]", opened, url)
	}
}

func TestUIDetachNoOpenAndErrors(t *testing.T) {
	useRealBinary(t)
	called := false
	old := uiproc.OpenURL
	uiproc.OpenURL = func(string) error { called = true; return nil }
	defer func() { uiproc.OpenURL = old }()

	dir := initedRepo(t)
	stopUIAfter(t, dir)
	runWithStdin("", "ui", dir, "--detach", "--no-open", "--port", "0")
	if called {
		t.Fatal("--no-open still opened the browser")
	}
	if code, _, errOut := runWithStdin("", "ui", t.TempDir(), "--detach"); code != 1 || !strings.Contains(errOut, "not inside a git repo") {
		t.Fatalf("non-repo detach: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runWithStdin("", "ui", t.TempDir(), "--stop"); code != 1 || !strings.Contains(errOut, "not inside a git repo") {
		t.Fatalf("non-repo stop: code=%d err=%q", code, errOut)
	}
}

func TestDisableAndUninstallStopTheServer(t *testing.T) {
	useRealBinary(t)
	for _, cmd := range []string{"disable", "uninstall"} {
		dir := initedRepo(t)
		stopUIAfter(t, dir)
		_, out, _ := runWithStdin("", "ui", dir, "--detach", "--no-open", "--port", "0")
		url := strings.TrimSpace(out)
		if !isUp(url) {
			t.Fatalf("%s: server did not start", cmd)
		}
		if code, _, errOut := runWithStdin("", cmd, dir); code != 0 {
			t.Fatalf("%s: code=%d err=%q", cmd, code, errOut)
		}
		deadline := time.Now().Add(3 * time.Second)
		for isUp(url) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if isUp(url) {
			t.Errorf("%s left the UI server running", cmd)
		}
	}
}
