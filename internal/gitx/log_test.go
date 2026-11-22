package gitx

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Edge case 1 (log part): no commits → empty, non-nil slice.
func TestLogEmptyRepo(t *testing.T) {
	r := newRepo(t)
	commits, err := r.Log(10)
	if err != nil || commits == nil || len(commits) != 0 {
		t.Fatalf("commits=%v err=%v", commits, err)
	}
}

func TestLogOrderAndTotals(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n2\n3\n", "Add a", "")
	write(t, r, "a.txt", "1\nTWO\n3\n4\n") // +2 -1
	write(t, r, "b.txt", "x\n")            // +1
	git(t, r, "add", "-A")
	git(t, r, "commit", "-q", "-m", "Change a and add b")

	commits, err := r.Log(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("got %d commits", len(commits))
	}
	top, root := commits[0], commits[1]
	if top.Subject != "Change a and add b" || root.Subject != "Add a" {
		t.Fatalf("order wrong: %q, %q", top.Subject, root.Subject)
	}
	if top.Files != 2 || top.Ins != 3 || top.Dels != 1 {
		t.Fatalf("totals = %d files +%d -%d", top.Files, top.Ins, top.Dels)
	}
	if root.Files != 1 || root.Ins != 3 || root.Dels != 0 {
		t.Fatalf("root totals = %d files +%d -%d", root.Files, root.Ins, root.Dels)
	}
	if len(root.Parents) != 0 || len(top.Parents) != 1 || top.Parents[0] != root.Hash {
		t.Fatalf("parents wrong: %v / %v", top.Parents, root.Parents)
	}
	if len(top.Hash) != 40 || top.Short == "" || !strings.HasPrefix(top.Hash, top.Short) {
		t.Fatalf("hash/short: %q %q", top.Hash, top.Short)
	}
	if top.Author != "Test User" || top.Email != "test@example.com" || top.Date.IsZero() {
		t.Fatalf("author fields: %+v", top)
	}
	if top.Agent != "" {
		t.Fatalf("gitx must not fill Agent, got %q", top.Agent)
	}
}

func TestLogLimit(t *testing.T) {
	r := newRepo(t)
	for _, n := range []string{"1", "2", "3"} {
		commitFile(t, r, n+".txt", n+"\n", "Commit "+n, "")
	}
	commits, err := r.Log(2)
	if err != nil || len(commits) != 2 || commits[0].Subject != "Commit 3" {
		t.Fatalf("commits=%v err=%v", commits, err)
	}
}

// Edge case 7: bodies containing |, tabs and blank lines.
func TestLogBodyWithSpecialCharacters(t *testing.T) {
	r := newRepo(t)
	body := "Why: a | b\twith tab\n\nSecond paragraph\n\nAgent: Claude"
	commitFile(t, r, "a.txt", "1\n", "Subject | with pipe", body)
	commits, err := r.Log(10)
	if err != nil || len(commits) != 1 {
		t.Fatalf("commits=%v err=%v", commits, err)
	}
	if commits[0].Subject != "Subject | with pipe" {
		t.Fatalf("subject = %q", commits[0].Subject)
	}
	if commits[0].Body != body {
		t.Fatalf("body = %q, want %q", commits[0].Body, body)
	}
	if commits[0].Files != 1 || commits[0].Ins != 1 {
		t.Fatalf("totals = %d files +%d", commits[0].Files, commits[0].Ins)
	}
}

// Edge case 5 (log part): binary files show "-" in numstat, count as one file, 0 lines.
func TestLogBinaryFile(t *testing.T) {
	r := newRepo(t)
	p := filepath.Join(r.Root, "img.bin")
	if err := os.WriteFile(p, []byte{0, 1, 2, 0, 255, 254, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r, "add", "-A")
	git(t, r, "commit", "-q", "-m", "Add binary")
	commits, err := r.Log(10)
	if err != nil {
		t.Fatal(err)
	}
	if commits[0].Files != 1 || commits[0].Ins != 0 || commits[0].Dels != 0 {
		t.Fatalf("totals = %d files +%d -%d", commits[0].Files, commits[0].Ins, commits[0].Dels)
	}
}

// Edge case 4 (log part): unicode and spaces in paths and subjects.
func TestLogUnicode(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "日本語 dir/ファイル.txt", "x\n", "Añadir módulo ✓", "")
	commits, err := r.Log(10)
	if err != nil || commits[0].Subject != "Añadir módulo ✓" || commits[0].Files != 1 {
		t.Fatalf("commits=%+v err=%v", commits, err)
	}
}

// Edge case 8 (log part): a merge on the first-parent line lists only first-parent
// history, and the merge commit's totals are against its first parent.
func TestLogFirstParentMerge(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "base.txt", "base\n", "Base", "")
	main := r.Branch()
	git(t, r, "checkout", "-q", "-b", "feature")
	commitFile(t, r, "feat.txt", "f\n", "Feature work", "")
	git(t, r, "checkout", "-q", main)
	commitFile(t, r, "main.txt", "m\n", "Main work", "")
	git(t, r, "merge", "-q", "--no-ff", "-m", "Merge feature", "feature")

	commits, err := r.Log(10)
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range commits {
		subjects = append(subjects, c.Subject)
	}
	want := []string{"Merge feature", "Main work", "Base"}
	if !reflect.DeepEqual(subjects, want) {
		t.Fatalf("subjects = %v, want %v", subjects, want)
	}
	if merge := commits[0]; len(merge.Parents) != 2 || merge.Files != 1 || merge.Ins != 1 {
		t.Fatalf("merge = parents %d, %d files +%d", len(merge.Parents), merge.Files, merge.Ins)
	}
}

func TestParseNumstat(t *testing.T) {
	f, i, d := parseNumstat("\n\n3\t1\ta.go\n-\t-\timg.png\n2\t0\told => new\n")
	if f != 3 || i != 5 || d != 1 {
		t.Fatalf("files=%d ins=%d dels=%d", f, i, d)
	}
}

func TestParseLogRejectsMalformed(t *testing.T) {
	if _, err := parseLog("\x1eonly\x1fthree\x1ffields"); err == nil {
		t.Fatal("expected error for malformed record")
	}
}
