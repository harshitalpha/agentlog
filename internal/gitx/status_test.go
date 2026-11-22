package gitx

import (
	"reflect"
	"sort"
	"testing"
)

func paths(files []DirtyFile) []string {
	var p []string
	for _, f := range files {
		p = append(p, f.Path)
	}
	return p
}

func TestDirtyFilesCleanRepo(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	files, err := r.DirtyFiles()
	if err != nil || len(files) != 0 {
		t.Fatalf("files=%v err=%v", files, err)
	}
}

func TestDirtyFilesModifiedAndUntracked(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "a.txt", "1\n", "First", "")
	write(t, r, "a.txt", "2\n")
	write(t, r, "dir/new.txt", "x\n") // untracked inside a new dir: listed per file
	files, err := r.DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	got := paths(files)
	want := []string{"a.txt", "dir/new.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if files[0].Status != " M" || files[1].Status != "??" {
		t.Fatalf("statuses = %q %q", files[0].Status, files[1].Status)
	}
}

// Edge case 3: a staged rename reports the new path, and the old path
// field that follows is skipped.
func TestDirtyFilesRename(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, "old name.txt", "line one\nline two\nline three\n", "First", "")
	git(t, r, "mv", "old name.txt", "new name.txt")
	write(t, r, "extra.txt", "x\n")
	files, err := r.DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"extra.txt", "new name.txt"}
	got := paths(files)
	sort.Strings(got) // git lists the staged rename before untracked files
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

// Edge case 4: spaces and unicode survive (-z means no quoting).
func TestDirtyFilesUnicodeAndSpaces(t *testing.T) {
	r := newRepo(t)
	write(t, r, "héllo wörld/日本語 file.txt", "x\n")
	files, err := r.DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"héllo wörld/日本語 file.txt"}
	if got := paths(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

// Edge case 9: ignored files never show up.
func TestDirtyFilesIgnored(t *testing.T) {
	r := newRepo(t)
	commitFile(t, r, ".gitignore", "secret.env\nbuild/\n", "Ignore", "")
	write(t, r, "secret.env", "k=v\n")
	write(t, r, "build/out.bin", "x")
	write(t, r, "keep.txt", "x\n")
	files, err := r.DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(files); !reflect.DeepEqual(got, []string{"keep.txt"}) {
		t.Fatalf("paths = %v", got)
	}
}

func TestDirtyFilesFreshRepoUntracked(t *testing.T) {
	r := newRepo(t)
	write(t, r, "a.txt", "x\n")
	files, err := r.DirtyFiles()
	if err != nil || len(files) != 1 || files[0].Path != "a.txt" {
		t.Fatalf("files=%v err=%v", files, err)
	}
}

func TestParsePorcelainZHandlesCopyAndEmpty(t *testing.T) {
	if got := parsePorcelainZ(""); len(got) != 0 {
		t.Fatalf("empty input gave %v", got)
	}
	got := parsePorcelainZ("C  copy.txt\x00orig.txt\x00?? z.txt\x00")
	if p := paths(got); !reflect.DeepEqual(p, []string{"copy.txt", "z.txt"}) {
		t.Fatalf("paths = %v", p)
	}
}
