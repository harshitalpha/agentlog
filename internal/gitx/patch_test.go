package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustContain(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("missing %q in:\n%s", n, haystack)
		}
	}
}

// Edge case 1: fresh repo, WorkingPatch shows untracked files.
func TestWorkingPatchFreshRepo(t *testing.T) {
	r := newRepo(t)
	write(t, r, "hello.txt", "hi\n")
	p, err := r.WorkingPatch()
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "diff --git a/hello.txt b/hello.txt", "new file mode", "--- /dev/null", "+hi")
}

func TestWorkingPatchTrackedAndUntracked(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	write(t, r, "a.txt", "1\n2\n")
	write(t, r, "dir/new.txt", "n\n")
	p, err := r.WorkingPatch()
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "diff --git a/a.txt b/a.txt", "+2", "diff --git a/dir/new.txt b/dir/new.txt", "+n")
}

func TestWorkingPatchCleanIsEmpty(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	if p, err := r.WorkingPatch(); err != nil || p != "" {
		t.Fatalf("patch=%q err=%v", p, err)
	}
}

// Edge case 9: ignored files never appear.
func TestWorkingPatchIgnored(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, ".gitignore", "secret.env\n", "Ignore", "")
	write(t, r, "secret.env", "k=v\n")
	write(t, r, "keep.txt", "x\n")
	p, err := r.WorkingPatch()
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "keep.txt")
	if strings.Contains(p, "secret.env") {
		t.Fatalf("ignored file leaked:\n%s", p)
	}
}

func TestWorkingPatchSkipsBigAndCapsCount(t *testing.T) {
	r := newRepo(t)
	big := filepath.Join(r.Root, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", maxUntrackedBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxUntrackedFiles+5; i++ {
		write(t, r, filepath.Join("many", strings.Repeat("a", 1)+string(rune('A'+i%26))+"-"+itoa(i)+".txt"), "x\n")
	}
	p, err := r.WorkingPatch()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p, "big.txt") {
		t.Fatal("file over 1 MB should be skipped")
	}
	if n := strings.Count(p, "diff --git "); n != maxUntrackedFiles {
		t.Fatalf("got %d files, want cap of %d", n, maxUntrackedFiles)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for ; i > 0; i /= 10 {
		d = append([]byte{byte('0' + i%10)}, d...)
	}
	return string(d)
}

// Edge case 2: a root commit renders, every file added.
func TestCommitPatchRootCommit(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a.txt", "a\n")
	commitFile(t, r, "b.txt", "b\n", "Root", "")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "diff --git a/a.txt b/a.txt", "diff --git a/b.txt b/b.txt", "new file mode", "+a", "+b")
	if strings.Count(p, "new file mode") != 2 {
		t.Fatalf("both files should be added:\n%s", p)
	}
}

// Edge case 3: rename detection reports the new path.
func TestCommitPatchRename(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "old.txt", "one\ntwo\nthree\nfour\n", "First", "")
	git(t, r, "mv", "old.txt", "new.txt")
	git(t, r, "commit", "-q", "-m", "Rename")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "rename from old.txt", "rename to new.txt", "similarity index 100%")
}

// Edge case 4: spaces and unicode in paths.
func TestCommitPatchUnicodePath(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "dir with space/ünï.txt", "x\n", "Add", "")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "ünï.txt", "+x")
}

// Edge case 5: binary file → "Binary files … differ".
func TestCommitPatchBinary(t *testing.T) {
	r := newRepo(t)
	if err := os.WriteFile(filepath.Join(r.Root, "img.bin"), []byte{0, 1, 2, 0, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r, "add", "-A")
	git(t, r, "commit", "-q", "-m", "Binary")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "Binary files /dev/null and b/img.bin differ")
}

// Edge case 6: no trailing newline marker.
func TestCommitPatchNoNewlineAtEOF(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "no newline", "Add", "")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, `\ No newline at end of file`)
}

// Edge case 8: merge patch is against the first parent only.
func TestCommitPatchMergeFirstParent(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "base.txt", "base\n", "Base", "")
	main := r.Branch()
	git(t, r, "checkout", "-q", "-b", "feature")
	commitFile(t, r, "feat.txt", "f\n", "Feature", "")
	git(t, r, "checkout", "-q", main)
	commitFile(t, r, "main.txt", "m\n", "Main", "")
	git(t, r, "merge", "-q", "--no-ff", "-m", "Merge feature", "feature")
	h, _ := r.Head()
	p, err := r.CommitPatch(h)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, p, "diff --git a/feat.txt b/feat.txt")
	if strings.Contains(p, "main.txt") {
		t.Fatalf("patch must be against the first parent only:\n%s", p)
	}
}

func TestCommitPatchRejectsOptionLikeHash(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	if _, err := r.CommitPatch("--output=/tmp/agentlog-should-not-exist"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat("/tmp/agentlog-should-not-exist"); err == nil {
		t.Fatal("option injection wrote a file")
	}
}

func TestFileAt(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "d/ü file.txt", "v1\n", "One", "")
	h1, _ := r.Head()
	commitFile(t, r, "d/ü file.txt", "v2\n", "Two", "")
	h2, _ := r.Head()

	if got, err := r.FileAt(h1, "d/ü file.txt"); err != nil || got != "v1\n" {
		t.Fatalf("h1: %q %v", got, err)
	}
	if got, err := r.FileAt(h2, "d/ü file.txt"); err != nil || got != "v2\n" {
		t.Fatalf("h2: %q %v", got, err)
	}
}

func TestFileAtErrors(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "dir/a.txt", "x\n", "One", "")
	if err := os.WriteFile(filepath.Join(r.Root, "bin.dat"), []byte("ab\x00cd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.Root, "huge.txt"), []byte(strings.Repeat("y", maxFileBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r, "add", "-A")
	git(t, r, "commit", "-q", "-m", "More")
	h, _ := r.Head()

	cases := []struct {
		name, rev, path string
		want            error
	}{
		{"binary", h, "bin.dat", ErrBinary},
		{"too large", h, "huge.txt", ErrTooLarge},
		{"missing path", h, "nope.txt", ErrNotFound},
		{"directory", h, "dir", ErrNotFound},
		{"unknown rev", "deadbeef", "dir/a.txt", ErrNotFound},
		{"option-like path", h, "--help", ErrNotFound},
	}
	for _, c := range cases {
		if _, err := r.FileAt(c.rev, c.path); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestCommitPatchUnknownCommit(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	for _, h := range []string{"deadbeef", "0000", strings.Repeat("a", 40)} {
		if _, err := r.CommitPatch(h); !errors.Is(err, ErrNotFound) {
			t.Errorf("CommitPatch(%q) err = %v, want ErrNotFound", h, err)
		}
	}
	// A blob hash is not a commit.
	blob := strings.TrimSpace(git(t, r, "rev-parse", "HEAD:a.txt"))
	if _, err := r.CommitPatch(blob); !errors.Is(err, ErrNotFound) {
		t.Errorf("blob hash err = %v, want ErrNotFound", err)
	}
	// Short hashes of real commits still work.
	h, _ := r.Head()
	if _, err := r.CommitPatch(h[:8]); err != nil {
		t.Errorf("short hash: %v", err)
	}
}

// The patch format must not depend on the user's diff config.
func TestPatchesIgnoreUserDiffConfig(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "dir/a.txt", "one\n", "First", "")
	write(t, r, "dir/a.txt", "one\ntwo\n")
	write(t, r, "dir/new.txt", "n\n")
	for k, v := range map[string]string{
		"diff.noprefix": "true", "diff.mnemonicPrefix": "true", "diff.external": "/nonexistent/diff-tool",
	} {
		if err := r.SetLocal(k, v); err != nil {
			t.Fatal(err)
		}
	}
	work, err := r.WorkingPatch()
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, work, "diff --git a/dir/a.txt b/dir/a.txt", "diff --git a/dir/new.txt b/dir/new.txt",
		"--- a/dir/a.txt", "+++ b/dir/a.txt", "+two")
	git(t, r, "add", "-A")
	git(t, r, "commit", "-q", "-m", "Second")
	head, _ := r.Head()
	commit, err := r.CommitPatch(head)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, commit, "diff --git a/dir/a.txt b/dir/a.txt", "diff --git a/dir/new.txt b/dir/new.txt")
	for _, bad := range []string{"diff --git i/", "diff --git c/", "diff --git w/", " w/dir"} {
		if strings.Contains(work+commit, bad) {
			t.Errorf("patch contains %q", bad)
		}
	}
}
