package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain isolates the tests from the developer's global git config (signing
// hooks, identities, templates).
func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Exit(m.Run())
}

// newRepo creates an initialised temp repo with a repo-local identity.
func newRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	for k, v := range map[string]string{
		"user.name": "Test User", "user.email": "test@example.com", "commit.gpgsign": "false",
	} {
		if err := r.SetLocal(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// write creates (or overwrites) a file relative to the repo root.
func write(t *testing.T, r *Repo, rel, content string) {
	t.Helper()
	p := filepath.Join(r.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// git runs a raw git command in the repo and fails the test on error.
func git(t *testing.T, r *Repo, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.Root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// commitFile writes a file and commits it with the given subject/body.
func commitFile(t *testing.T, r *Repo, rel, content, subject, body string) {
	t.Helper()
	write(t, r, rel, content)
	git(t, r, "add", "-A")
	if body != "" {
		git(t, r, "commit", "-q", "-m", subject, "-m", body)
	} else {
		git(t, r, "commit", "-q", "-m", subject)
	}
}
