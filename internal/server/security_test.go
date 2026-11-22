package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Input validation, the Host check, method limits and panic recovery (docs/07-http-api.md, Security).

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
