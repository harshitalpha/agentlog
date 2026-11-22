package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRulesContent(t *testing.T) {
	r := Rules()
	for _, want := range []string{
		"## Commit discipline (enforced by agentlog)",
		"NEVER run `git push`",
		"Agent: Claude",
		"NEW commit",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("rules missing %q", want)
		}
	}
	if strings.Contains(r, StartMarker) || strings.HasSuffix(r, "\n") {
		t.Error("Rules() must have no markers and no trailing newline")
	}
	b := Block()
	if !strings.HasPrefix(b, StartMarker+"\n") || !strings.HasSuffix(b, "\n"+EndMarker) {
		t.Errorf("Block() not wrapped in markers:\n%s", b)
	}
}

func TestUpsertBlockCases(t *testing.T) {
	user := "# My project\n\nSome user notes.\n"
	oldBlock := StartMarker + "\nOLD RULES\n" + EndMarker
	cases := []struct{ name, in string }{
		{"empty", ""},
		{"whitespace only", "\n\n"},
		{"user content", user},
		{"user content without trailing newline", "# My project"},
	}
	for _, c := range cases {
		got := UpsertBlock(c.in)
		if strings.Count(got, StartMarker) != 1 || strings.Count(got, EndMarker) != 1 {
			t.Errorf("%s: markers not exactly once:\n%s", c.name, got)
		}
		if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
			t.Errorf("%s: should end with exactly one newline: %q", c.name, got)
		}
		if strings.TrimSpace(c.in) != "" && !strings.HasPrefix(got, strings.TrimRight(c.in, "\n")+"\n\n"+StartMarker) {
			t.Errorf("%s: block must follow one blank line:\n%s", c.name, got)
		}
		if again := UpsertBlock(got); again != got {
			t.Errorf("%s: not idempotent", c.name)
		}
		if back := RemoveBlock(got); back != normalise(c.in) {
			t.Errorf("%s: remove did not restore the original.\nwant %q\ngot  %q", c.name, normalise(c.in), back)
		}
	}

	// An old block is replaced in place and the user's content is intact.
	withOld := "before\n\n" + oldBlock + "\n\nafter\n"
	got := UpsertBlock(withOld)
	if strings.Contains(got, "OLD RULES") {
		t.Fatal("old block not replaced")
	}
	if !strings.HasPrefix(got, "before\n\n"+StartMarker) || !strings.HasSuffix(got, EndMarker+"\n\nafter\n") {
		t.Fatalf("surroundings changed:\n%s", got)
	}
	if strings.Count(got, StartMarker) != 1 {
		t.Fatal("duplicated block")
	}
}

// normalise is what RemoveBlock should give back for an original input:
// content kept, trailing newline normalised, blank-only input becomes "".
func normalise(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimRight(s, "\n") + "\n"
}

func TestRemoveBlockMiddleKeepsOneBlankLine(t *testing.T) {
	in := "top\n\n" + Block() + "\n\nbottom\n"
	if got := RemoveBlock(in); got != "top\n\nbottom\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveBlockAtStart(t *testing.T) {
	if got := RemoveBlock(Block() + "\n\nrest\n"); got != "rest\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveBlockNoBlockIsUnchanged(t *testing.T) {
	in := "just user text\n"
	if got := RemoveBlock(in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestIncompleteMarkersAreNotABlock(t *testing.T) {
	in := "x\n" + StartMarker + "\nno end\n"
	if _, _, ok := findBlock(in); ok {
		t.Fatal("start without end must not count as a block")
	}
	if got := RemoveBlock(in); got != in {
		t.Fatal("remove must leave it alone")
	}
}

func TestUpsertRulesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AGENTS.md")

	// Missing file: created.
	if changed, err := UpsertRules(path); err != nil || !changed {
		t.Fatalf("create: changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != Block()+"\n" {
		t.Fatalf("unexpected content:\n%s", data)
	}
	// Second run: nothing changes.
	if changed, err := UpsertRules(path); err != nil || changed {
		t.Fatalf("second run: changed=%v err=%v", changed, err)
	}
}

func TestRemoveRulesFile(t *testing.T) {
	dir := t.TempDir()

	// File that only held the block is deleted.
	only := filepath.Join(dir, "only.md")
	_, _ = UpsertRules(only)
	if removed, err := RemoveRules(only); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(only); !os.IsNotExist(err) {
		t.Fatal("empty file should be deleted")
	}

	// File with user content keeps it, byte for byte.
	mixed := filepath.Join(dir, "mixed.md")
	original := "# Notes\n\nkeep me\n"
	if err := os.WriteFile(mixed, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = UpsertRules(mixed)
	if removed, err := RemoveRules(mixed); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if data, _ := os.ReadFile(mixed); string(data) != original {
		t.Fatalf("original not restored: %q", data)
	}

	// Missing file and block-less file: no-ops.
	if removed, err := RemoveRules(filepath.Join(dir, "missing.md")); err != nil || removed {
		t.Fatalf("missing: removed=%v err=%v", removed, err)
	}
	plain := filepath.Join(dir, "plain.md")
	_ = os.WriteFile(plain, []byte("hi\n"), 0o644)
	if removed, err := RemoveRules(plain); err != nil || removed {
		t.Fatalf("plain: removed=%v err=%v", removed, err)
	}
}

func TestUpsertRulesPreservesFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertRules(path); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}
