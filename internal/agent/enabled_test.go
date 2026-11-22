package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/OWNER/agentlog/internal/gitx"
)

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Hooks fall back to the working directory when no cwd is given; run from a
	// throwaway directory so tests can never act on the real repository.
	scratch, err := os.MkdirTemp("", "agentlog-agent-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(scratch); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(scratch)
	os.Exit(code)
}

// newRepo makes a temp git repo with a local identity and no signing.
func newRepo(t *testing.T) *gitx.Repo {
	t.Helper()
	r, err := gitx.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"user.name": "Test User", "user.email": "t@example.com", "commit.gpgsign": "false"} {
		if err := r.SetLocal(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func env(m map[string]string) Env { return func(k string) string { return m[k] } }

func TestIsEnabled(t *testing.T) {
	none := env(nil)
	everywhere := env(map[string]string{EnvEnforceEverywhere: "true"})
	off := env(map[string]string{EnvEnforceEverywhere: "false"})

	r := newRepo(t)
	if IsEnabled(r, none) || IsEnabled(r, off) {
		t.Fatal("repo without config must not be enabled")
	}
	if !IsEnabled(r, everywhere) {
		t.Fatal("enforce_everywhere must enable any repo")
	}

	if err := r.SetLocal(ConfigKey, "true"); err != nil {
		t.Fatal(err)
	}
	if !IsEnabled(r, none) || !IsEnabled(r, off) {
		t.Fatal("config true must enable the repo")
	}

	if err := r.SetLocal(ConfigKey, "false"); err != nil {
		t.Fatal(err)
	}
	if IsEnabled(r, none) {
		t.Fatal("config false must not enable the repo")
	}
	_ = r.UnsetLocal(ConfigKey)
	if IsEnabled(r, none) {
		t.Fatal("unset must not enable the repo")
	}
}

func TestIsEnabledNonRepo(t *testing.T) {
	if IsEnabled(nil, env(map[string]string{EnvEnforceEverywhere: "true"})) {
		t.Fatal("a non-repo is never enabled, even with enforce_everywhere")
	}
}

func TestIsEnabledGlobalConfigDoesNotCount(t *testing.T) {
	// The marker is repo-local on purpose: a global setting would enable every repo.
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[agentlog]\n\tenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	r := newRepo(t)
	if out, _ := exec.Command("git", "-C", r.Root, "config", "--get", ConfigKey).Output(); len(out) == 0 {
		t.Fatal("test setup: global value should be visible to plain git config")
	}
	if IsEnabled(r, env(nil)) {
		t.Fatal("only the repo-local key may enable a repo")
	}
}

func TestEnforceEverywhereParsing(t *testing.T) {
	for v, want := range map[string]bool{"true": true, "TRUE": true, "1": true, " yes ": true, "false": false, "": false, "0": false, "nope": false} {
		if got := EnforceEverywhere(env(map[string]string{EnvEnforceEverywhere: v})); got != want {
			t.Errorf("%q → %v, want %v", v, got, want)
		}
	}
	if EnforceEverywhere(nil) {
		t.Error("nil env must be false")
	}
}
