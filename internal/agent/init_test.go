package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OWNER/agentlog/internal/gitx"
)

// project makes a plain (non-git) folder with a few files.
func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"main.go":     "package main\n",
		"lib/util.go": "package lib\n",
		".env":        "SECRET=hunter2\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runInit(t *testing.T, path string, opts InitOptions) (*gitx.Repo, string) {
	t.Helper()
	var out bytes.Buffer
	repo, err := Init(path, opts, &out)
	if err != nil {
		t.Fatalf("Init: %v\n%s", err, out.String())
	}
	return repo, out.String()
}

func subjects(t *testing.T, r *gitx.Repo) []string {
	t.Helper()
	cs, err := r.Log(100)
	if err != nil {
		t.Fatal(err)
	}
	var s []string
	for _, c := range cs {
		s = append(s, c.Subject)
	}
	return s
}

func TestInitFreshFolder(t *testing.T) {
	dir := project(t)
	repo, out := runInit(t, dir, InitOptions{})

	for _, want := range []string{"✓ initialised local git repo", "✓ committed baseline snapshot",
		"✓ agentlog enabled in", "/agentlog:ui"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	got := subjects(t, repo)
	if len(got) != 2 || got[0] != RulesSubject || got[1] != BaselineSubject {
		t.Fatalf("history = %v", got)
	}
	if v, ok, _ := repo.GetLocal(ConfigKey); !ok || v != "true" {
		t.Fatal("agentlog.enabled not set")
	}
	if n, _, _ := repo.GetLocal("user.name"); n != gitx.DefaultName {
		t.Fatalf("user.name = %q", n)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	for _, l := range gitignoreLines {
		if !strings.Contains(string(gi), l+"\n") {
			t.Errorf(".gitignore missing %q:\n%s", l, gi)
		}
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(agents) != Block()+"\n" {
		t.Fatalf("AGENTS.md = %q", agents)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("CLAUDE.md must not be written without --claude-md")
	}
	if files, _ := repo.DirtyFiles(); len(files) != 0 {
		t.Fatalf("tree not clean after init: %v", files)
	}
	cs, _ := repo.Log(10)
	base := cs[1]
	if base.Body != baselineBody {
		t.Fatalf("baseline body = %q", base.Body)
	}
	if DetectAgent(base.Author, base.Body) != "" {
		t.Fatal("init commits are human commits, not agent commits")
	}
}

// Secrets must never reach the baseline: .gitignore is written first.
func TestInitNeverCommitsDotEnv(t *testing.T) {
	dir := project(t)
	repo, _ := runInit(t, dir, InitOptions{})
	tracked, _ := exec_git(t, repo, "ls-files")
	if strings.Contains(tracked, ".env") {
		t.Fatalf(".env was committed:\n%s", tracked)
	}
	if !strings.Contains(tracked, "main.go") || !strings.Contains(tracked, "lib/util.go") {
		t.Fatalf("project files missing from baseline:\n%s", tracked)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	dir := project(t)
	repo, _ := runInit(t, dir, InitOptions{ClaudeMD: true})
	head1, _ := repo.Head()
	agents1, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	gi1, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))

	repo2, out := runInit(t, dir, InitOptions{ClaudeMD: true})
	head2, _ := repo2.Head()
	if head1 != head2 {
		t.Fatalf("second init created a commit: %v", subjects(t, repo2))
	}
	agents2, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	gi2, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(agents1) != string(agents2) || string(gi1) != string(gi2) {
		t.Fatal("second init rewrote files")
	}
	if strings.Contains(out, "baseline") || strings.Contains(out, "initialised") {
		t.Fatalf("second init should be quiet about work already done:\n%s", out)
	}
	if !strings.Contains(out, "✓ agentlog enabled in") {
		t.Fatalf("missing final line:\n%s", out)
	}
}

func TestInitFlags(t *testing.T) {
	dir := project(t)
	runInit(t, dir, InitOptions{NoAgentsMD: true, ClaudeMD: true})
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("--no-agents-md still wrote AGENTS.md")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(b) != Block()+"\n" {
		t.Fatalf("CLAUDE.md = %q", b)
	}

	dir2 := project(t)
	repo, _ := runInit(t, dir2, InitOptions{NoBaseline: true})
	got := subjects(t, repo)
	if len(got) != 1 || got[0] != RulesSubject {
		t.Fatalf("--no-baseline history = %v", got)
	}
	// The user's own files stay uncommitted.
	files, _ := repo.DirtyFiles()
	if len(files) == 0 {
		t.Fatal("--no-baseline must leave the project files uncommitted")
	}
}

func TestInitExistingCleanRepoOnlyAddsRulesCommit(t *testing.T) {
	r := newRepo(t)
	writeFile(t, r, "a.txt", "1\n")
	_ = r.AddAll()
	_ = r.Commit("My first commit", "", gitx.CommitOpts{})

	repo, _ := runInit(t, r.Root, InitOptions{})
	got := subjects(t, repo)
	if len(got) != 2 || got[0] != RulesSubject || got[1] != "My first commit" {
		t.Fatalf("history = %v (no baseline expected for a clean repo)", got)
	}
	// Existing identity must be left alone.
	if n, _, _ := repo.GetLocal("user.name"); n != "Test User" {
		t.Fatalf("user.name = %q", n)
	}
}

func TestInitExistingDirtyRepoGetsBaseline(t *testing.T) {
	r := newRepo(t)
	writeFile(t, r, "a.txt", "1\n")
	_ = r.AddAll()
	_ = r.Commit("My first commit", "", gitx.CommitOpts{})
	writeFile(t, r, "a.txt", "2\n")
	writeFile(t, r, "wip.txt", "w\n")

	repo, _ := runInit(t, r.Root, InitOptions{})
	got := subjects(t, repo)
	if len(got) != 3 || got[0] != RulesSubject || got[1] != BaselineSubject {
		t.Fatalf("history = %v", got)
	}
	if files, _ := repo.DirtyFiles(); len(files) != 0 {
		t.Fatalf("tree should be clean: %v", files)
	}
}

func TestInitNoBaselineKeepsUserEditsOutOfRulesCommit(t *testing.T) {
	r := newRepo(t)
	writeFile(t, r, "a.txt", "1\n")
	_ = r.AddAll()
	_ = r.Commit("First", "", gitx.CommitOpts{})
	writeFile(t, r, "a.txt", "user edit\n")

	repo, _ := runInit(t, r.Root, InitOptions{NoBaseline: true})
	files, _ := repo.DirtyFiles()
	if len(files) != 1 || files[0].Path != "a.txt" {
		t.Fatalf("user's edit must remain uncommitted, dirty = %v", files)
	}
}

func TestInitPreservesExistingGitignore(t *testing.T) {
	dir := project(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.env"), 0o644); err != nil {
		t.Fatal(err)
	}
	runInit(t, dir, InitOptions{})
	b, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	want := "node_modules\n.env\n.claude/settings.local.json\n.env.*\n"
	if string(b) != want {
		t.Fatalf(".gitignore = %q, want %q", b, want)
	}
}

func TestInitFromSubdirectoryUsesRoot(t *testing.T) {
	r := newRepo(t)
	writeFile(t, r, "sub/a.txt", "1\n")
	repo, _ := runInit(t, filepath.Join(r.Root, "sub"), InitOptions{})
	if _, err := os.Stat(filepath.Join(r.Root, "AGENTS.md")); err != nil {
		t.Fatalf("AGENTS.md should be at the repo root: %v", err)
	}
	if v, ok, _ := repo.GetLocal(ConfigKey); !ok || v != "true" {
		t.Fatal("not enabled")
	}
}

func TestInitErrors(t *testing.T) {
	var out bytes.Buffer
	if _, err := Init(filepath.Join(t.TempDir(), "missing"), InitOptions{}, &out); err == nil {
		t.Fatal("expected error for a missing directory")
	}
	f := filepath.Join(t.TempDir(), "file.txt")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	if _, err := Init(f, InitOptions{}, &out); err == nil {
		t.Fatal("expected error for a file path")
	}
}

func TestInitRefusesHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var out bytes.Buffer
	if _, err := Init(home, InitOptions{}, &out); err == nil || !strings.Contains(err.Error(), "home directory") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".git")); !os.IsNotExist(err) {
		t.Fatal("a repo was created in the home directory")
	}
	// A project folder inside home is fine.
	sub := filepath.Join(home, "proj")
	_ = os.Mkdir(sub, 0o755)
	if _, err := Init(sub, InitOptions{}, &out); err != nil {
		t.Fatalf("subfolder of home should work: %v", err)
	}
}

func TestInitStandaloneHookNotYetSupported(t *testing.T) {
	var out bytes.Buffer
	if _, err := Init(project(t), InitOptions{StandaloneHook: true}, &out); err == nil {
		t.Fatal("expected a clear error until T2.8")
	}
}

// exec_git runs a raw git command in a repo for assertions.
func exec_git(t *testing.T, r *gitx.Repo, args ...string) (string, error) {
	t.Helper()
	out, err := execCommand("git", append([]string{"-C", r.Root}, args...)...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out, err
}
