package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OWNER/agentlog/internal/gitx"
)

// Environment variables Claude Code sets for hook processes.
const (
	EnvProjectDir = "CLAUDE_PROJECT_DIR"
	EnvPluginData = "CLAUDE_PLUGIN_DATA"
)

// HookInput is the subset of the hook stdin JSON that agentlog uses. Unknown
// fields are ignored.
type HookInput struct {
	SessionID      string `json:"session_id"`
	Cwd            string `json:"cwd"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// ParseHookInput decodes hook stdin. Empty input is valid (all defaults);
// malformed JSON returns ok=false.
func ParseHookInput(raw []byte) (in HookInput, ok bool) {
	if strings.TrimSpace(string(raw)) == "" {
		return in, true
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return HookInput{}, false
	}
	return in, true
}

// openRepo opens the repo for a hook: input.cwd, else $CLAUDE_PROJECT_DIR,
// else the process working directory. It returns nil outside a git repo.
func openRepo(in HookInput, env Env) *gitx.Repo {
	dir := in.Cwd
	if dir == "" && env != nil {
		dir = env(EnvProjectDir)
	}
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}
	if dir == "" {
		return nil
	}
	repo, err := gitx.Open(dir)
	if err != nil {
		return nil
	}
	return repo
}

// LogError records an internal hook error to stderr and, when
// $CLAUDE_PLUGIN_DATA is set, to <data>/agentlog.log. It never fails.
func LogError(env Env, format string, args ...any) {
	msg := fmt.Sprintf("agentlog: "+format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if env == nil {
		return
	}
	dir := env(EnvPluginData)
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "agentlog.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, msg)
}

// marshalLine encodes v as one line of JSON without HTML escaping.
func marshalLine(v any) string {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimRight(sb.String(), "\n")
}
