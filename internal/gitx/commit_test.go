package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lastCommit(t *testing.T, r *Repo) Commit {
	t.Helper()
	commits, err := r.Log(1)
	if err != nil || len(commits) != 1 {
		t.Fatalf("Log: %v %v", commits, err)
	}
	return commits[0]
}

func TestAddAllAndStagedEmpty(t *testing.T) {
	r := newRepo(t)
	if empty, err := r.StagedEmpty(); err != nil || !empty {
		t.Fatalf("fresh repo: empty=%v err=%v", empty, err)
	}
	write(t, r, "a.txt", "x\n")
	if empty, _ := r.StagedEmpty(); !empty {
		t.Fatal("untracked file must not count as staged")
	}
	if err := r.AddAll(); err != nil {
		t.Fatal(err)
	}
	if empty, err := r.StagedEmpty(); err != nil || empty {
		t.Fatalf("after add: empty=%v err=%v", empty, err)
	}
}

func TestAddAllStagesDeletionsAndRespectsIgnore(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, ".gitignore", "ignored.txt\n", "Ignore", "")
	commitFile(t, r, "gone.txt", "x\n", "Add gone", "")
	if err := os.Remove(filepath.Join(r.Root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, r, "ignored.txt", "x\n")
	if err := r.AddAll(); err != nil {
		t.Fatal(err)
	}
	staged := git(t, r, "diff", "--cached", "--name-status")
	if !strings.Contains(staged, "D\tgone.txt") || strings.Contains(staged, "ignored.txt") {
		t.Fatalf("staged:\n%s", staged)
	}
}

func TestCommitWithAndWithoutBody(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a.txt", "x\n")
	_ = r.AddAll()
	if err := r.Commit("Add a", "Because.\n\nAgent: Claude", CommitOpts{}); err != nil {
		t.Fatal(err)
	}
	c := lastCommit(t, r)
	if c.Subject != "Add a" || c.Body != "Because.\n\nAgent: Claude" || c.Author != "Test User" {
		t.Fatalf("commit = %+v", c)
	}
	write(t, r, "b.txt", "y\n")
	_ = r.AddAll()
	if err := r.Commit("Add b", "", CommitOpts{}); err != nil {
		t.Fatal(err)
	}
	if c := lastCommit(t, r); c.Subject != "Add b" || c.Body != "" {
		t.Fatalf("commit = %+v", c)
	}
}

func TestCommitIdentityOverrideLeavesConfigAlone(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a.txt", "x\n")
	_ = r.AddAll()
	id := Identity{Name: "someone", Email: "s@example.com"}
	if err := r.Commit("Add a", "", CommitOpts{Identity: &id}); err != nil {
		t.Fatal(err)
	}
	if c := lastCommit(t, r); c.Author != "someone" || c.Email != "s@example.com" {
		t.Fatalf("author = %q <%s>", c.Author, c.Email)
	}
	if v, _, _ := r.GetLocal("user.name"); v != "Test User" {
		t.Fatalf("config user.name changed to %q", v)
	}
}

func TestCommitFailsWhenNothingStaged(t *testing.T) {
	r := newRepo(t)
	if err := r.Commit("Empty", "", CommitOpts{}); err == nil {
		t.Fatal("expected error committing nothing")
	}
}

func TestAutoCommitNothingToDo(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	subj, ok, err := r.AutoCommit("edits")
	if err != nil || ok || subj != "" {
		t.Fatalf("subj=%q ok=%v err=%v", subj, ok, err)
	}
	commits, _ := r.Log(10)
	if len(commits) != 1 {
		t.Fatalf("a commit was created: %d", len(commits))
	}
}

func TestAutoCommitSubjectAndBody(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "base.txt", "1\n", "Base", "")
	write(t, r, "x.go", "x\n")
	subj, ok, err := r.AutoCommit("uncommitted changes")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if subj != "[auto] uncommitted changes: x.go" {
		t.Fatalf("subject = %q", subj)
	}
	c := lastCommit(t, r)
	if c.Subject != subj || c.Author != "agentlog" || c.Email != "agentlog@localhost" {
		t.Fatalf("commit = %+v", c)
	}
	if !strings.Contains(c.Body, "Committed automatically by agentlog") || !strings.HasSuffix(c.Body, "\n\nAgent: auto") {
		t.Fatalf("body = %q", c.Body)
	}
	if dirty, _ := r.DirtyFiles(); len(dirty) != 0 {
		t.Fatalf("tree still dirty: %v", dirty)
	}
}

func TestAutoCommitMoreThanThreeFiles(t *testing.T) {
	r := newRepo(t)
	for _, f := range []string{"a.go", "dir/b.go", "c.go", "d.go", "e.go"} {
		write(t, r, f, "x\n")
	}
	subj, _, err := r.AutoCommit("edits")
	if err != nil {
		t.Fatal(err)
	}
	if subj != "[auto] edits: a.go, c.go, d.go (+2 more)" {
		t.Fatalf("subject = %q", subj)
	}
}

func TestAutoCommitTruncatesLongSubject(t *testing.T) {
	r := newRepo(t)
	long := strings.Repeat("é", 60)
	for _, n := range []string{"a", "b", "c"} {
		write(t, r, long+n+".txt", "x\n")
	}
	subj, _, err := r.AutoCommit("edits")
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(subj)); n != 100 {
		t.Fatalf("subject length = %d runes", n)
	}
}

func TestAutoCommitRenameUsesNewName(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "old.go", "one\ntwo\nthree\nfour\n", "Base", "")
	git(t, r, "mv", "old.go", "new.go")
	subj, _, err := r.AutoCommit("edits")
	if err != nil || subj != "[auto] edits: new.go" {
		t.Fatalf("subj=%q err=%v", subj, err)
	}
}

// The safety net must survive the user's signing and hook setup.
func TestAutoCommitIgnoresSigningAndHooks(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "Base", "")
	_ = r.SetLocal("commit.gpgsign", "true")
	_ = r.SetLocal("gpg.program", "/nonexistent/gpg")
	hook := filepath.Join(r.Root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, r, "b.txt", "x\n")
	if _, ok, err := r.AutoCommit("edits"); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAutoCommitWithoutAnyUserIdentity(t *testing.T) {
	r, err := Init(t.TempDir()) // no user.name/email anywhere
	if err != nil {
		t.Fatal(err)
	}
	write(t, r, "a.txt", "x\n")
	if _, ok, err := r.AutoCommit("edits"); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
