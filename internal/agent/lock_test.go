package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSanitizeID(t *testing.T) {
	for in, want := range map[string]string{
		"abc-123_X":        "abc-123_X",
		"../../etc/passwd": "______etc_passwd",
		"a b/c\x00d":       "a_b_c_d",
		"":                 "",
	} {
		if got := sanitizeID(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitizeID(strings.Repeat("x", 500)); len(got) != 64 {
		t.Errorf("long id not capped: %d", len(got))
	}
}

func TestAcquireLock(t *testing.T) {
	r := newRepo(t)
	if !acquireLock(r, "stop-s1-0") {
		t.Fatal("first caller must win")
	}
	if acquireLock(r, "stop-s1-0") {
		t.Fatal("identical caller within the TTL must be refused")
	}
	if !acquireLock(r, "stop-s1-1") {
		t.Fatal("a different key must not be blocked")
	}
	if !acquireLock(r, "") {
		t.Fatal("an empty key means no dedupe")
	}

	// An expired lock is taken over.
	dir, _ := r.GitPath("agentlog")
	old := time.Now().Add(-2 * lockTTL)
	if err := os.Chtimes(filepath.Join(dir, "stop-s1-0"), old, old); err != nil {
		t.Fatal(err)
	}
	if !acquireLock(r, "stop-s1-0") {
		t.Fatal("expired lock must be taken over")
	}
}

func TestSweepStaleLocks(t *testing.T) {
	r := newRepo(t)
	acquireLock(r, "stop-old-0")
	dir, _ := r.GitPath("agentlog")
	ancient := time.Now().Add(-2 * staleLockAge)
	_ = os.Chtimes(filepath.Join(dir, "stop-old-0"), ancient, ancient)
	other := filepath.Join(dir, "ui.json")
	_ = os.WriteFile(other, []byte("{}"), 0o644)
	_ = os.Chtimes(other, ancient, ancient)

	acquireLock(r, "stop-new-0") // triggers the sweep
	if _, err := os.Stat(filepath.Join(dir, "stop-old-0")); !os.IsNotExist(err) {
		t.Fatal("stale lock should be swept")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("sweep must never touch files that aren't locks (ui.json)")
	}
}

func TestLockKeysDistinguishStopAttempts(t *testing.T) {
	a := stopLockKey(HookInput{SessionID: "s", StopHookActive: false})
	b := stopLockKey(HookInput{SessionID: "s", StopHookActive: true})
	if a == b || a == "" {
		t.Fatalf("keys must differ per attempt: %q vs %q", a, b)
	}
	if stopLockKey(HookInput{}) != "" || startLockKey(HookInput{}) != "" {
		t.Fatal("no session id → no key")
	}
}

// Plugin + standalone both fire the same Stop hook: only one may act.
func TestStopDedupesIdenticalInvocations(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, "a.txt", "x\n")
	in := stopInput(t, r.Root, false)

	first := HandleStop(in, env(nil))
	second := HandleStop(in, env(nil))
	if !strings.Contains(first, `"decision":"block"`) {
		t.Fatalf("first call should block, got %q", first)
	}
	if second != "" {
		t.Fatalf("duplicate call must be silent, got %q", second)
	}
}

// The safety net must survive: Claude's real second attempt arrives within
// the TTL but carries stop_hook_active=true, so it is not a duplicate.
func TestStopSecondAttemptWithinTTLStillAutoCommits(t *testing.T) {
	r := enabledRepo(t)
	writeFile(t, r, "a.txt", "x\n")
	if out := HandleStop(stopInput(t, r.Root, false), env(nil)); !strings.Contains(out, "block") {
		t.Fatalf("first attempt: %q", out)
	}
	before := commitCount(t, r)
	if out := HandleStop(stopInput(t, r.Root, true), env(nil)); out != "" {
		t.Fatalf("second attempt output: %q", out)
	}
	if commitCount(t, r) != before+1 {
		t.Fatal("second attempt was wrongly deduped: nothing was auto-committed")
	}
}

func TestSessionStartDedupesAndOnlyForEnabledRepos(t *testing.T) {
	r := enabledRepo(t)
	in := input(t, r.Root)
	if out := HandleSessionStart(in, env(nil)); out == "" {
		t.Fatal("first call should print the rules")
	}
	if out := HandleSessionStart(in, env(nil)); out != "" {
		t.Fatalf("duplicate call must be silent, got %q", out)
	}

	// A repo that is not enabled never gets a lock directory.
	r2 := newRepo(t)
	HandleStop(stopInput(t, r2.Root, false), env(nil))
	HandleSessionStart(input(t, r2.Root), env(nil))
	dir, _ := r2.GitPath("agentlog")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("hooks must not touch .git in repos that are not enabled")
	}
}
