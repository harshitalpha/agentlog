package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/harshitalpha/agentlog/internal/gitx"
)

// Listening, serving over real HTTP and the idle timeout.

func TestListenBindsLoopbackOnlyAndFallsBack(t *testing.T) {
	first, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	addr := first.Addr().(*net.TCPAddr)
	if !addr.IP.IsLoopback() || addr.IP.String() != "127.0.0.1" {
		t.Fatalf("bound to %v, must be 127.0.0.1", addr)
	}

	// Occupy a port, then ask for it: the next one is used.
	busy := addr.Port
	second, err := Listen(busy)
	if err != nil {
		t.Skipf("neighbouring ports busy: %v", err)
	}
	defer second.Close()
	if p := second.Addr().(*net.TCPAddr).Port; p <= busy || p > busy+PortFallbackRange {
		t.Fatalf("fallback port = %d, busy = %d", p, busy)
	}
}

func TestListenFailsWhenAllPortsBusy(t *testing.T) {
	var held []net.Listener
	defer func() {
		for _, l := range held {
			l.Close()
		}
	}()
	base, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	held = append(held, base)
	start := base.Addr().(*net.TCPAddr).Port
	for p := start + 1; p <= start+PortFallbackRange; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			t.Skipf("cannot hold port %d: %v", p, err)
		}
		held = append(held, l)
	}
	if _, err := Listen(start); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v, want a 'busy' error", err)
	}
}

func startServing(t *testing.T, r *gitx.Repo, idle time.Duration) (base string, stop func() error) {
	t.Helper()
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(r, Options{Version: "t"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, ln, idle) }()
	base = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	return base, func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("Serve did not return")
			return nil
		}
	}
}

func TestServeOverRealHTTP(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	base, stop := startServing(t, r, time.Hour)

	resp, err := http.Get(base + "/api/repo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info repoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Path != r.Root || info.Head == "" {
		t.Fatalf("info=%+v err=%v", info, err)
	}

	// A browser on another site rebinding its name to 127.0.0.1 is refused.
	req, _ := http.NewRequest("GET", base+"/api/repo", nil)
	req.Host = "evil.com"
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("evil Host → %d", resp2.StatusCode)
	}
	// localhost:<port> works too.
	resp3, err := http.Get(strings.Replace(base, "127.0.0.1", "localhost", 1) + "/api/repo")
	if err == nil {
		resp3.Body.Close()
		if resp3.StatusCode != 200 {
			t.Fatalf("localhost → %d", resp3.StatusCode)
		}
	}
	if err := stop(); err != nil {
		t.Fatalf("clean stop returned %v", err)
	}
	if _, err := http.Get(base + "/api/repo"); err == nil {
		t.Fatal("server still answering after stop")
	}
}

func TestIdleTimeoutStopsServer(t *testing.T) {
	r := newRepo(t)
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(r, Options{})
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), ln, 300*time.Millisecond) }()
	select {
	case err := <-done:
		if err != ErrIdleTimeout {
			t.Fatalf("err = %v, want ErrIdleTimeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never stopped on idle")
	}
}

func TestRequestsKeepServerAlive(t *testing.T) {
	r := newRepo(t)
	base, stop := startServing(t, r, 400*time.Millisecond)
	end := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(end) { // poll like the UI does, well inside the idle window
		resp, err := http.Get(base + "/api/repo")
		if err != nil {
			t.Fatalf("server died while being polled: %v", err)
		}
		resp.Body.Close()
		time.Sleep(100 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatalf("stop = %v (should be a clean cancel, not idle)", err)
	}
}

func TestRejectedRequestsDoNotKeepServerAlive(t *testing.T) {
	r := newRepo(t)
	ln, _ := Listen(0)
	srv := New(r, Options{})
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background(), ln, 500*time.Millisecond) }()
	base := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	stopSpam := time.After(1500 * time.Millisecond)
	for spamming := true; spamming; {
		select {
		case err := <-done:
			if err != ErrIdleTimeout {
				t.Fatalf("err = %v", err)
			}
			return
		case <-stopSpam:
			spamming = false
		default:
			req, _ := http.NewRequest("GET", base+"/api/repo", nil)
			req.Host = "evil.com"
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	select {
	case err := <-done:
		if err != ErrIdleTimeout {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("requests with a bad Host header kept the server alive")
	}
}
