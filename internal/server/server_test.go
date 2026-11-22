package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
