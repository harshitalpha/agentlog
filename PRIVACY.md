# Privacy

agentlog sends no data anywhere. Everything stays on your machine.

- It runs git commands in your local repositories and writes commits there. It never pushes.
- The timeline UI is a local web server bound to `127.0.0.1`. It is read-only and needs no network.
- There is no telemetry, no analytics, no account and no update check.
- State (the UI process info and short-lived hook locks) lives inside `.git/agentlog/` of the repo.
