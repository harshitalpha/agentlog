package agent

import "testing"

func TestDetectAgent(t *testing.T) {
	cases := []struct {
		name, author, body, want string
	}{
		// Rule 1: Agent trailer wins.
		{"agent trailer", "Jane", "Subject body\n\nAgent: Claude", "Claude"},
		{"agent trailer other agent", "Jane", "x\n\nAgent: Codex", "Codex"},
		{"agent trailer case-insensitive", "Jane", "x\n\nagent: Aider", "Aider"},
		{"agent trailer trimmed", "Jane", "x\n\nAgent:    Custom Bot   ", "Custom Bot"},
		{"agent trailer beats auto author", "agentlog", "x\n\nAgent: auto", "auto"},
		{"agent trailer beats co-author", "Jane", "Agent: Mine\nCo-Authored-By: Claude <c@x>", "Mine"},
		{"empty agent trailer ignored", "Jane", "x\n\nAgent:   ", ""},
		{"any line starting with Agent: counts", "Jane", "Agent: line one", "line one"},
		{"mid-line mention is not a trailer", "Jane", "the Agent: Claude did this", ""},

		// Rule 2: reserved author.
		{"agentlog author", "agentlog", "no trailers", "auto"},
		{"agentlog author beats co-author", "agentlog", "Co-Authored-By: Claude <c@x>", "auto"},
		{"similar author is not reserved", "agentlog-fan", "no trailers", ""},
		{"author case matters", "AgentLog", "no trailers", ""},

		// Rule 3: Co-Authored-By values.
		{"claude", "Jane", "x\n\nCo-Authored-By: Claude Opus <noreply@anthropic.com>", "Claude"},
		{"codex", "Jane", "x\n\nCo-Authored-By: Codex <c@openai.com>", "Codex"},
		{"openai", "Jane", "x\n\nCo-authored-by: OpenAI Bot <b@x>", "Codex"},
		{"cursor", "Jane", "x\n\nCo-Authored-By: Cursor Agent <c@x>", "Cursor"},
		{"copilot", "Jane", "x\n\nCo-Authored-By: GitHub Copilot <c@x>", "Copilot"},
		{"gemini", "Jane", "x\n\nCo-Authored-By: Gemini <g@x>", "Gemini"},
		{"aider", "Jane", "x\n\nCo-Authored-By: aider (gpt) <a@x>", "Aider"},
		{"devin", "Jane", "x\n\nCo-Authored-By: Devin <d@x>", "Devin"},
		{"unknown co-author is human", "Jane", "x\n\nCo-Authored-By: Bob Human <bob@x>", ""},
		{"second co-author matches", "Jane", "x\n\nCo-Authored-By: Bob <bob@x>\nCo-Authored-By: Claude <c@x>", "Claude"},

		// Rule 4 / pitfall #1: never infer from the author name.
		{"author Claude Dev, no trailers", "Claude Dev", "plain human commit", ""},
		{"author Claude, no body", "Claude", "", ""},
		{"author Cursor Fan", "Cursor Fan", "x", ""},
		{"empty everything", "", "", ""},
	}
	for _, c := range cases {
		if got := DetectAgent(c.author, c.body); got != c.want {
			t.Errorf("%s: DetectAgent(%q, %q) = %q, want %q", c.name, c.author, c.body, got, c.want)
		}
	}
}
