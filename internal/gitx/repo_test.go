package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenNotRepo(t *testing.T) {
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("err = %v, want ErrNotRepo", err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("missing dir: err = %v, want ErrNotRepo", err)
	}
}

// Edge case 10: running from a subdirectory resolves to the root.
func TestOpenFromSubdirectory(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a/b/c.txt", "x")
	got, err := Open(filepath.Join(r.Root, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(r.Root)
	have, _ := filepath.EvalSymlinks(got.Root)
	if have != want {
		t.Fatalf("root = %q, want %q", have, want)
	}
}

func TestFreshRepoHeadAndBranch(t *testing.T) {
	r := newRepo(t)
	if r.HasCommits() {
		t.Fatal("fresh repo should have no commits")
	}
	if h, err := r.Head(); err != nil || h != "" {
		t.Fatalf("Head = %q, %v; want empty", h, err)
	}
	if b := r.Branch(); b != "main" && b != "master" {
		t.Fatalf("Branch = %q, want main or master", b)
	}
}

func TestHeadAfterCommit(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	h, err := r.Head()
	if err != nil || len(h) != 40 {
		t.Fatalf("Head = %q, %v", h, err)
	}
	if !r.HasCommits() {
		t.Fatal("HasCommits should be true")
	}
}

func TestLocalConfigRoundTrip(t *testing.T) {
	r := newRepo(t)
	if _, ok, err := r.GetLocal("agentlog.enabled"); err != nil || ok {
		t.Fatalf("unset key: ok=%v err=%v", ok, err)
	}
	if err := r.SetLocal("agentlog.enabled", "true"); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := r.GetLocal("agentlog.enabled"); err != nil || !ok || v != "true" {
		t.Fatalf("got %q ok=%v err=%v", v, ok, err)
	}
	if err := r.UnsetLocal("agentlog.enabled"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.GetLocal("agentlog.enabled"); ok {
		t.Fatal("key should be unset")
	}
	// Unsetting an already-unset key is not an error.
	if err := r.UnsetLocal("agentlog.enabled"); err != nil {
		t.Fatalf("second unset: %v", err)
	}
}

func TestEnsureIdentity(t *testing.T) {
	dir := t.TempDir()
	r, err := Init(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := r.EnsureIdentity()
	if err != nil || !changed {
		t.Fatalf("first call: changed=%v err=%v", changed, err)
	}
	if v, _, _ := r.GetLocal("user.name"); v != DefaultName {
		t.Fatalf("user.name = %q", v)
	}
	if v, _, _ := r.GetLocal("user.email"); v != DefaultEmail {
		t.Fatalf("user.email = %q", v)
	}
	if changed, _ := r.EnsureIdentity(); changed {
		t.Fatal("second call should change nothing")
	}
}

func TestEnsureIdentityKeepsExisting(t *testing.T) {
	r := newRepo(t) // has Test User / test@example.com
	if changed, err := r.EnsureIdentity(); err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _, _ := r.GetLocal("user.name"); v != "Test User" {
		t.Fatalf("user.name = %q", v)
	}
}

func TestCommandsRunInRepoRoot(t *testing.T) {
	r := newRepo(t)
	if _, err := os.Stat(filepath.Join(r.Root, ".git")); err != nil {
		t.Fatal(err)
	}
}

func TestGitPath(t *testing.T) {
	r := newRepo(t)
	p, err := r.GitPath("agentlog")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(r.Root, ".git"))
	have, _ := filepath.EvalSymlinks(filepath.Dir(p))
	if have != want || filepath.Base(p) != "agentlog" || !filepath.IsAbs(p) {
		t.Fatalf("GitPath = %q (git dir %q)", p, want)
	}

	// A linked worktree keeps its own git dir elsewhere.
	commitFile(t, r, "a.txt", "1\n", "First", "")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, r, "worktree", "add", "-q", "-b", "other", wt)
	r2, err := Open(wt)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := r2.GitPath("agentlog")
	if err != nil || !filepath.IsAbs(p2) || strings.HasPrefix(p2, wt) {
		t.Fatalf("worktree GitPath = %q err=%v (must live in the main .git, not the worktree)", p2, err)
	}
}
