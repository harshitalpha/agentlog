package server

import (
	"net/http"
	"strings"
	"testing"
)

// Embedded UI: index, MIME types, unknown paths.

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
		csp := rec.Header().Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'none'", "script-src 'self'", "frame-ancestors 'none'", "connect-src 'self'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q is missing %q", p, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: CSP must not allow inline or eval: %q", p, csp)
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
