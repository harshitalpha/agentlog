package cli

import (
	"fmt"
	"io"

	"github.com/harshitalpha/agentlog/internal/agent"
)

// maxHookInput bounds how much stdin a hook reads.
const maxHookInput = 1 << 20

// runHook wraps a hook handler with the contract from docs/05: always exit 0,
// print at most one JSON line to stdout, never panic, never block on a
// missing stdin.
func runHook(name string, s streams, handle func(stdin []byte, env agent.Env) string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			agent.LogError(agent.OSEnv, "%s: internal error: %v", name, r)
			code = exitOK
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(s.in, maxHookInput))
	if err != nil {
		agent.LogError(agent.OSEnv, "%s: reading stdin: %v", name, err)
		return exitOK
	}
	if out := handle(raw, agent.OSEnv); out != "" {
		fmt.Fprintln(s.out, out)
	}
	return exitOK
}

func hookSessionStart(_ []string, s streams) int {
	return runHook("hook-session-start", s, agent.HandleSessionStart)
}
