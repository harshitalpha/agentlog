## Commit discipline (enforced by agentlog)

This repo is tracked locally with git. Every change you make must end up in a
local commit with a clear message, so the human can review your work step by step.

- Commit after each logical unit of work, not once at the end. One concern per commit.
  Good: "Add controller layer for the orders API",
        "Add ProductRepository interface and Postgres impl",
        "Add GetProductsByDate query to product DB".
  Bad:  "updates", "fix", "wip", "changes from claude".
- Subject: imperative mood, under 72 characters, no trailing period.
- Body (optional, after a blank line): why the change was made, anything non-obvious,
  and follow-ups left undone.
- End the body with a trailer naming yourself, e.g. `Agent: Claude` or `Agent: Codex`.
- When fixing something you wrote earlier, make a NEW commit ("Fix nil check in
  GetProductsByDate") instead of amending, so the history shows the order of events.
- Stage with `git add -A` or specific paths (respects .gitignore). Never commit secrets or .env files.
- NEVER run `git push`, `git commit --amend`, `git rebase`, `git reset --hard`,
  or anything else that rewrites or publishes history.
