package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/OWNER/agentlog/internal/gitx"

	"github.com/OWNER/agentlog/internal/agent"
)

// moduleRoot is the repository root, found from this file's location (the
// test binary runs in a scratch directory, see TestMain).
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// buildBinary compiles the real agentlog binary into a temp dir.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "agentlog")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/agentlog")
	cmd.Dir = moduleRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// M2 acceptance: the demo repo has exactly the expected history, and the
// hook scripts behave like the real hooks.
func TestMakeDemoRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts need a POSIX sh")
	}
	bin := buildBinary(t)
	root := moduleRoot(t)
	dir := filepath.Join(t.TempDir(), "demo")

	cmd := exec.Command("sh", filepath.Join(root, "scripts", "make-demo-repo.sh"), dir)
	cmd.Env = append(os.Environ(), "AGENTLOG_BIN="+bin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make-demo-repo.sh: %v\n%s", err, out)
	}

	repo, err := gitx.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := repo.Log(100)
	if err != nil {
		t.Fatal(err)
	}
	// Newest first.
	want := []struct{ subject, author, agent string }{
		{"[auto] uncommitted changes: x.go", "agentlog", "auto"},
		{"Update README wording", "Claude Dev", ""}, // pitfall #1: not an agent
		{"Fix empty-id handling in ProductRepository.Get", "local-dev", "Claude"},
		{"Add GetProductsByDate query to product DB", "local-dev", "Claude"},
		{"Add ProductRepository interface and Postgres impl", "local-dev", "Claude"},
		{"Add controller layer for the orders API", "local-dev", "Claude"},
		{"Add agentlog commit rules for AI agents", "local-dev", ""},
		{"Baseline snapshot before agent work", "local-dev", ""},
	}
	if len(commits) != len(want) {
		t.Fatalf("got %d commits, want %d", len(commits), len(want))
	}
	for i, w := range want {
		c := commits[i]
		if c.Subject != w.subject || c.Author != w.author {
			t.Errorf("commit %d = %q by %q, want %q by %q", i, c.Subject, c.Author, w.subject, w.author)
		}
		if got := agent.DetectAgent(c.Author, c.Body); got != w.agent {
			t.Errorf("commit %d (%q): agent = %q, want %q", i, c.Subject, got, w.agent)
		}
	}

	// The fix commit edits a file that an earlier commit created.
	out, _ := exec.Command("git", "-C", dir, "show", "--name-status", "--format=", commits[2].Hash).Output()
	if strings.TrimSpace(string(out)) != "M\tinternal/repo/product_pg.go" {
		t.Errorf("fix commit changes %q", out)
	}

	// Exactly two uncommitted changes: one modified, one untracked.
	files, err := repo.DirtyFiles()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = f.Status
	}
	if len(got) != 2 || got["internal/api/orders.go"] != " M" || got["notes.txt"] != "??" {
		t.Errorf("dirty files = %v", got)
	}

	// Nothing pushed anywhere.
	if remotes, _ := exec.Command("git", "-C", dir, "remote", "-v").Output(); len(remotes) != 0 {
		t.Errorf("demo repo has remotes: %s", remotes)
	}

	// status agrees: 4 agent, 1 human, 1 auto since the baseline.
	var st struct {
		Enabled bool                             `json:"enabled"`
		Commits struct{ Agent, Human, Auto int } `json:"commits_since_baseline"`
		Dirty   int                              `json:"uncommitted_files"`
	}
	statusOut, err := exec.Command(bin, "status", dir, "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(statusOut, &st); err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Commits.Agent != 4 || st.Commits.Human != 1 || st.Commits.Auto != 1 || st.Dirty != 2 {
		t.Errorf("status = %+v", st)
	}

	// The fake session-start script prints the rules, as the real hook does.
	cmd = exec.Command("sh", filepath.Join(root, "scripts", "fake-session-start.sh"), dir)
	cmd.Env = append(os.Environ(), "AGENTLOG_BIN="+bin)
	ssOut, err := cmd.Output()
	if err != nil || !strings.Contains(string(ssOut), `"hookEventName":"SessionStart"`) || !strings.Contains(string(ssOut), "Commit discipline") {
		t.Errorf("fake-session-start: err=%v out=%s", err, ssOut)
	}

	// And fake-claude-stop reports a block for a fresh dirty file, then stays silent.
	cmd = exec.Command("sh", filepath.Join(root, "scripts", "fake-claude-stop.sh"), dir, "false", "another-session")
	cmd.Env = append(os.Environ(), "AGENTLOG_BIN="+bin)
	stopOut, err := cmd.Output()
	if err != nil || !strings.Contains(string(stopOut), `"decision":"block"`) || !strings.Contains(string(stopOut), "orders.go") {
		t.Errorf("fake-claude-stop: err=%v out=%s", err, stopOut)
	}
}

func TestFakeStopRejectsBadActiveFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts need a POSIX sh")
	}
	cmd := exec.Command("sh", filepath.Join(moduleRoot(t), "scripts", "fake-claude-stop.sh"), t.TempDir(), "maybe")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected a usage error for stop_hook_active=maybe")
	}
}
