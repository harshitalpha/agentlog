package agent

import (
	"os"
	"strings"

	"github.com/OWNER/agentlog/internal/gitx"
)

// Names shared with the CLI and plugin manifest.
const (
	// ConfigKey is the repo-local git config key that opts a repo in.
	ConfigKey = "agentlog.enabled"
	// EnvEnforceEverywhere is how Claude Code passes the plugin's
	// enforce_everywhere option to hook processes.
	EnvEnforceEverywhere = "CLAUDE_PLUGIN_OPTION_ENFORCE_EVERYWHERE"
)

// Env reads environment variables; tests inject a fake.
type Env func(key string) string

// OSEnv is the real process environment.
func OSEnv(key string) string { return os.Getenv(key) }

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// EnforceEverywhere reports whether the plugin option treating every git
// repo as enabled is on.
func EnforceEverywhere(env Env) bool {
	return env != nil && truthy(env(EnvEnforceEverywhere))
}

// IsEnabled reports whether agentlog may act in repo. It is true when the
// repo opted in with `git config --local agentlog.enabled true`, or when the
// enforce_everywhere option is on. A nil repo (not inside git) is never
// enabled. Every hook calls this first.
func IsEnabled(repo *gitx.Repo, env Env) bool {
	if repo == nil {
		return false
	}
	if EnforceEverywhere(env) {
		return true
	}
	v, ok, err := repo.GetLocal(ConfigKey)
	return err == nil && ok && truthy(v)
}
