package cli

import "github.com/harshitalpha/agentlog/internal/agent"

func hookStop(_ []string, s streams) int {
	return runHook("hook-stop", s, agent.HandleStop)
}
