package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OWNER/agentlog/internal/gitx"
)

const testPort = 7777

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	scratch, err := os.MkdirTemp("", "agentlog-server-test-")
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

func newRepo(t *testing.T) *gitx.Repo {
	t.Helper()
	r, err := gitx.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"user.name": "Jane Dev", "user.email": "jane@example.com", "commit.gpgsign": "false"} {
		if err := r.SetLocal(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func gitRun(t *testing.T, r *gitx.Repo, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", r.Root}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, r *gitx.Repo, rel, content string) {
	t.Helper()
	p := filepath.Join(r.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, r *gitx.Repo, rel, content, subject, body string) {
	t.Helper()
	writeFile(t, r, rel, content)
	gitRun(t, r, "add", "-A")
	args := []string{"commit", "-q", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	gitRun(t, r, args...)
}

func newServer(t *testing.T, r *gitx.Repo) *Server {
	return New(r, Options{Version: "9.9.9", Port: testPort})
}

// do sends a request with the given method, URL and Host straight to the handler.
func do(t *testing.T, h http.Handler, method, target, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	return do(t, h, http.MethodGet, target, fmt.Sprintf("127.0.0.1:%d", testPort))
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("invalid JSON (%d): %v\n%s", rec.Code, err, rec.Body.String())
	}
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	decode(t, rec, &e)
	return e.Error
}

func checkJSONHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
}

// ---------------------------------------------------------------- happy paths

func TestRepoEndpoint(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	writeFile(t, r, "a.txt", "2\n")
	writeFile(t, r, "new.txt", "n\n")
	h := newServer(t, r).Handler()

	rec := get(t, h, "/api/repo")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	checkJSONHeaders(t, rec)
	var info repoInfo
	decode(t, rec, &info)
	head, _ := r.Head()
	if info.Name != filepath.Base(r.Root) || info.Path != r.Root || info.Head != head ||
		info.Dirty != 2 || info.Version != "9.9.9" || info.Branch == "" {
		t.Fatalf("info = %+v", info)
	}
}

func TestRepoEndpointEmptyRepo(t *testing.T) {
	r := newRepo(t)
	h := newServer(t, r).Handler()
	var info repoInfo
	decode(t, get(t, h, "/api/repo"), &info)
	if info.Head != "" || info.Dirty != 0 || info.Branch == "" {
		t.Fatalf("info = %+v", info)
	}
	rec := get(t, h, "/api/commits")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty repo commits: %d %q", rec.Code, rec.Body)
	}
}

func TestCommitsEndpoint(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "Human change", "")
	commit(t, r, "b.txt", "1\n", "Agent change", "Why.\n\nAgent: Claude")
	gitRun(t, r, "-c", "user.name=agentlog", "-c", "user.email=agentlog@localhost",
		"commit", "-q", "--allow-empty", "-m", "[auto] edits: x.go", "-m", "Agent: auto")
	// Same display name as an agent but no trailer: must stay a human commit.
	gitRun(t, r, "-c", "user.name=Claude Dev", "commit", "-q", "--allow-empty", "-m", "Claude Dev wrote this")
	h := newServer(t, r).Handler()

	rec := get(t, h, "/api/commits")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	checkJSONHeaders(t, rec)
	var cs []gitx.Commit
	decode(t, rec, &cs)
	if len(cs) != 4 {
		t.Fatalf("got %d commits", len(cs))
	}
	wantAgents := []string{"", "auto", "Claude", ""}
	wantSubjects := []string{"Claude Dev wrote this", "[auto] edits: x.go", "Agent change", "Human change"}
	for i, c := range cs {
		if c.Subject != wantSubjects[i] || c.Agent != wantAgents[i] {
			t.Errorf("commit %d = %q agent %q, want %q agent %q", i, c.Subject, c.Agent, wantSubjects[i], wantAgents[i])
		}
	}
	if cs[2].Files != 1 || cs[2].Ins != 1 || len(cs[2].Hash) != 40 || cs[2].Date.IsZero() {
		t.Errorf("commit fields = %+v", cs[2])
	}

	var limited []gitx.Commit
	decode(t, get(t, h, "/api/commits?limit=2"), &limited)
	if len(limited) != 2 {
		t.Errorf("limit=2 gave %d", len(limited))
	}
	for _, bad := range []string{"limit=abc", "limit=0", "limit=-5", "limit=1.5"} {
		if rec := get(t, h, "/api/commits?"+bad); rec.Code != 400 {
			t.Errorf("%s → %d, want 400", bad, rec.Code)
		}
	}
	if rec := get(t, h, "/api/commits?limit=99999999"); rec.Code != 200 {
		t.Errorf("huge limit should be clamped, got %d", rec.Code)
	}
}

func TestPatchEndpoint(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "one\n", "First", "")
	commit(t, r, "a.txt", "one\ntwo\n", "Second", "")
	writeFile(t, r, "wip.txt", "wip\n")
	h := newServer(t, r).Handler()
	head, _ := r.Head()

	var p patchResponse
	rec := get(t, h, "/api/patch?hash="+head)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	checkJSONHeaders(t, rec)
	decode(t, rec, &p)
	if !strings.Contains(p.Patch, "diff --git a/a.txt b/a.txt") || !strings.Contains(p.Patch, "+two") || p.Truncated {
		t.Fatalf("patch = %+v", p)
	}
	if strings.Contains(rec.Body.String(), "truncated") {
		t.Error("truncated must be omitted when false")
	}

	decode(t, get(t, h, "/api/patch?hash="+head[:8]), &p) // short hashes work
	if !strings.Contains(p.Patch, "+two") {
		t.Error("short hash patch wrong")
	}

	decode(t, get(t, h, "/api/patch?hash=WORKING"), &p)
	if !strings.Contains(p.Patch, "wip.txt") || !strings.Contains(p.Patch, "+wip") {
		t.Fatalf("working patch = %q", p.Patch)
	}
}

func TestPatchUnknownCommitIs404(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	h := newServer(t, r).Handler()
	if rec := get(t, h, "/api/patch?hash=deadbeef"); rec.Code != 404 || errorOf(t, rec) != "commit not found" {
		t.Fatalf("unknown commit: %d %s", rec.Code, rec.Body)
	}
}

func TestPatchTruncation(t *testing.T) {
	big := strings.Repeat("0123456789abcdef\n", 400000) // ~6.8 MB
	got := truncateAtLine(big, maxPatchBytes)
	if len(got) > maxPatchBytes || !strings.HasSuffix(got, "\n") || len(got) < maxPatchBytes-100 {
		t.Fatalf("len=%d", len(got))
	}
	if truncateAtLine("short\n", 100) != "short\n" {
		t.Fatal("short input must be unchanged")
	}
	if truncateAtLine(strings.Repeat("x", 50), 10) != "" {
		t.Fatal("no newline within the limit → empty")
	}

	// End to end: a >5 MB working-tree patch comes back truncated.
	r := newRepo(t)
	commit(t, r, "base.txt", "b\n", "Base", "")
	writeFile(t, r, "big.txt", strings.Repeat("x", 100)+"\n")
	content := strings.Repeat(strings.Repeat("y", 99)+"\n", 52000) // ~5.2 MB added lines
	if len(content) > 1<<20 {
		// WorkingPatch skips untracked files over 1 MB, so track it first.
		writeFile(t, r, "big.txt", "seed\n")
		gitRun(t, r, "add", "big.txt")
		gitRun(t, r, "commit", "-q", "-m", "Seed")
		writeFile(t, r, "big.txt", content)
	}
	h := newServer(t, r).Handler()
	var p patchResponse
	rec := get(t, h, "/api/patch?hash=WORKING")
	decode(t, rec, &p)
	if !p.Truncated || len(p.Patch) > maxPatchBytes || !strings.HasSuffix(p.Patch, "\n") {
		t.Fatalf("truncated=%v len=%d", p.Truncated, len(p.Patch))
	}
}

func TestFileEndpoint(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "dir/ü file.txt", "v1\n", "One", "")
	h1, _ := r.Head()
	commit(t, r, "dir/ü file.txt", "v2\n", "Two", "")
	writeFile(t, r, "dir/ü file.txt", "working\n")
	h := newServer(t, r).Handler()

	var f fileResponse
	rec := get(t, h, "/api/file?hash="+h1+"&path="+"dir/%C3%BC%20file.txt")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	checkJSONHeaders(t, rec)
	decode(t, rec, &f)
	if f.Content != "v1\n" || f.Binary {
		t.Fatalf("file = %+v", f)
	}
	decode(t, get(t, h, "/api/file?hash=WORKING&path=dir/%C3%BC%20file.txt"), &f)
	if f.Content != "working\n" {
		t.Fatalf("working file = %+v", f)
	}
	// "./" and "a/../" prefixes are normalised.
	decode(t, get(t, h, "/api/file?hash=WORKING&path=./dir/../dir/%C3%BC%20file.txt"), &f)
	if f.Content != "working\n" {
		t.Fatalf("normalised path file = %+v", f)
	}
}

func TestFileBinaryMissingAndTooLarge(t *testing.T) {
	r := newRepo(t)
	if err := os.WriteFile(filepath.Join(r.Root, "img.bin"), []byte{1, 0, 2, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Root, "huge.txt"), []byte(strings.Repeat("y", maxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, r, "add", "-A")
	gitRun(t, r, "commit", "-q", "-m", "Files")
	h := newServer(t, r).Handler()
	head, _ := r.Head()

	for _, hash := range []string{head, "WORKING"} {
		rec := get(t, h, "/api/file?hash="+hash+"&path=img.bin")
		var f fileResponse
		decode(t, rec, &f)
		if rec.Code != 200 || !f.Binary || f.Content != "" {
			t.Errorf("%s binary: %d %s", hash, rec.Code, rec.Body)
		}
		if rec := get(t, h, "/api/file?hash="+hash+"&path=nope.txt"); rec.Code != 404 {
			t.Errorf("%s missing: %d", hash, rec.Code)
		}
		if rec := get(t, h, "/api/file?hash="+hash+"&path=huge.txt"); rec.Code != 413 {
			t.Errorf("%s too large: %d", hash, rec.Code)
		}
	}
	// Directories are not files.
	writeFile(t, r, "d/x.txt", "x\n")
	if rec := get(t, h, "/api/file?hash=WORKING&path=d"); rec.Code != 404 {
		t.Errorf("directory: %d", rec.Code)
	}
}

// ------------------------------------------------------------------- security

func TestBadHashIs400(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	h := newServer(t, r).Handler()
	for _, bad := range []string{
		"../../etc", "--output=/tmp/x", "-p", "ABCDEF12", "abc", "working", "WORKING ", "HEAD", "HEAD~1",
		"deadbeef;ls", "zzzz", strings.Repeat("a", 65), "", "\x00abcd", "abcd\nef",
	} {
		for _, ep := range []string{"/api/patch?hash=", "/api/file?path=a.txt&hash="} {
			if rec := get(t, h, ep+url.QueryEscape(bad)); rec.Code != 400 {
				t.Errorf("%s%q → %d, want 400", ep, bad, rec.Code)
			}
		}
	}
	// Missing parameter entirely.
	if rec := get(t, h, "/api/patch"); rec.Code != 400 {
		t.Errorf("no hash → %d", rec.Code)
	}
}

func TestBadPathIs400(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	h := newServer(t, r).Handler()
	head, _ := r.Head()
	for _, bad := range []string{
		"../secret", "../../etc/passwd", "a/../../secret", "-p", "--help", "./-p", "/etc/passwd", `\windows\x`,
		"..", ".", "", "a\x00b", `..\..\etc`, "a b/../../c",
	} {
		for _, hash := range []string{head, "WORKING"} {
			if rec := get(t, h, "/api/file?hash="+hash+"&path="+url.QueryEscape(bad)); rec.Code != 400 {
				t.Errorf("path %q hash %s → %d, want 400", bad, hash[:min(len(hash), 7)], rec.Code)
			}
		}
	}
	if rec := get(t, h, "/api/file?hash="+head); rec.Code != 400 {
		t.Errorf("no path → %d", rec.Code)
	}
}

func TestWorkingFileSymlinkOutsideRepoIs404(t *testing.T) {
	r := newRepo(t)
	commit(t, r, "a.txt", "1\n", "First", "")
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(r.Root, "link.txt")); err != nil {
		t.Skip("symlinks not supported: ", err)
	}
	outsideDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(outsideDir, "f.txt"), []byte("TOP SECRET\n"), 0o644)
	if err := os.Symlink(outsideDir, filepath.Join(r.Root, "linkdir")); err != nil {
		t.Skip("symlinks not supported: ", err)
	}
	h := newServer(t, r).Handler()
	for _, p := range []string{"link.txt", "linkdir/f.txt"} {
		rec := get(t, h, "/api/file?hash=WORKING&path="+p)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "SECRET") {
			t.Errorf("%s → %d %s", p, rec.Code, rec.Body)
		}
	}
	// A symlink that stays inside the repo is fine.
	if err := os.Symlink("a.txt", filepath.Join(r.Root, "inside.txt")); err == nil {
		var f fileResponse
		rec := get(t, h, "/api/file?hash=WORKING&path=inside.txt")
		decode(t, rec, &f)
		if rec.Code != 200 || f.Content != "1\n" {
			t.Errorf("inside symlink: %d %s", rec.Code, rec.Body)
		}
	}
}

func TestHostCheck(t *testing.T) {
	r := newRepo(t)
	h := newServer(t, r).Handler()
	cases := map[string]int{
		"127.0.0.1:7777":          200,
		"localhost:7777":          200,
		"LOCALHOST:7777":          200,
		"evil.com":                403,
		"evil.com:7777":           403,
		"127.0.0.1":               403, // no port
		"127.0.0.1:9999":          403, // wrong port
		"localhost":               403,
		"0.0.0.0:7777":            403,
		"[::1]:7777":              403,
		"127.0.0.1.evil.com:7777": 403,
		"evil.com@127.0.0.1:7777": 403,
		"":                        403,
	}
	for host, want := range cases {
		for _, target := range []string{"/api/repo", "/", "/api/commits", "/api/patch?hash=WORKING"} {
			rec := do(t, h, http.MethodGet, target, host)
			if rec.Code != want {
				t.Errorf("Host %q %s → %d, want %d", host, target, rec.Code, want)
			}
			if want == 403 {
				checkJSONHeaders(t, rec)
				if errorOf(t, rec) == "" {
					t.Errorf("Host %q: empty error body", host)
				}
			}
		}
	}
}

func TestMethodsOtherThanGetAre405(t *testing.T) {
	r := newRepo(t)
	h := newServer(t, r).Handler()
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"} {
		for _, target := range []string{"/api/repo", "/api/commits", "/", "/nope"} {
			rec := do(t, h, m, target, "127.0.0.1:7777")
			if rec.Code != 405 {
				t.Errorf("%s %s → %d, want 405", m, target, rec.Code)
			}
			if rec.Header().Get("Allow") != "GET" {
				t.Errorf("%s %s: Allow = %q", m, target, rec.Header().Get("Allow"))
			}
		}
	}
	// The host check runs before the method check.
	if rec := do(t, h, "POST", "/api/repo", "evil.com"); rec.Code != 403 {
		t.Errorf("POST with bad host → %d, want 403", rec.Code)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	s := newServer(t, newRepo(t))
	h := recoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || errorOf(t, rec) != "internal error" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	_ = s
}

func TestUnknownAPIAndStaticPaths(t *testing.T) {
	h := newServer(t, newRepo(t)).Handler()
	rec := get(t, h, "/api/nope")
	if rec.Code != 404 || errorOf(t, rec) != "not found" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	for _, p := range []string{"/nope.js", "/static/", "/../go.mod", "/%2e%2e/go.mod", "//etc/passwd", "/go.mod", "/internal/server/server.go"} {
		rec := get(t, h, p)
		// ServeMux may first redirect a non-canonical path to its cleaned form;
		// either way the final answer must be a 404 and never file content.
		if rec.Code == http.StatusTemporaryRedirect || rec.Code == http.StatusMovedPermanently {
			rec = get(t, h, rec.Header().Get("Location"))
		}
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "module ") || strings.Contains(rec.Body.String(), "package ") {
			t.Errorf("%s → %d %q, want 404", p, rec.Code, rec.Body.String())
		}
	}
}

// --------------------------------------------------------------------- static

func TestIndexIsServed(t *testing.T) {
	h := newServer(t, newRepo(t)).Handler()
	for _, p := range []string{"/", "/index.html"} {
		rec := get(t, h, p)
		if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
			!strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s → %d %q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", p)
		}
	}
}

func TestMimeTypes(t *testing.T) {
	for ext, want := range map[string]string{".js": "text/javascript", ".css": "text/css", ".html": "text/html"} {
		if got := mimeOf(ext); !strings.HasPrefix(got, want) {
			t.Errorf("%s → %q, want prefix %q", ext, got, want)
		}
	}
}

// ------------------------------------------------------ listen / serve / idle

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
