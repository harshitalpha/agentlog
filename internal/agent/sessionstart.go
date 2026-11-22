package agent

// NotEnabledHint is the one-line context added in git repos where the plugin
// is installed but the repo has not opted in.
const NotEnabledHint = "agentlog is installed but not enabled in this repo. The user can run /agentlog:init to enable it."

type sessionStartOutput struct {
	HookSpecificOutput sessionStartSpecific `json:"hookSpecificOutput"`
}

type sessionStartSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// HandleSessionStart implements the SessionStart hook. It returns the single
// JSON line to print, or "" when nothing should be printed (not in a git
// repo, or unreadable input). Enabled repos get the commit rules as
// additionalContext; other git repos get the one-line hint.
func HandleSessionStart(stdin []byte, env Env) string {
	in, ok := ParseHookInput(stdin)
	if !ok {
		return ""
	}
	repo := openRepo(in, env)
	if repo == nil {
		return ""
	}
	context := NotEnabledHint
	if IsEnabled(repo, env) {
		context = Rules()
	}
	return marshalLine(sessionStartOutput{sessionStartSpecific{
		HookEventName:     "SessionStart",
		AdditionalContext: context,
	}})
}
