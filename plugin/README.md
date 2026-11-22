# agentlog (Claude Code plugin)

Makes Claude commit every change locally with a clear message, and shows the history as a
clickable timeline with diffs. Nothing is pushed, and no data leaves your machine.

## Install

```
/plugin marketplace add harshitalpha/agentlog
/plugin install agentlog@agentlog
```

## Use

| Command | What it does |
| --- | --- |
| `/agentlog:init` | Enable agentlog in this project (creates a local git repo if needed, a baseline commit, commit rules in `AGENTS.md`) |
| `/agentlog:ui` | Start the timeline UI and print its URL (`stop` as the argument shuts it down) |
| `/agentlog:status` | Show whether agentlog is on, what it committed, and any uncommitted files |
| `/agentlog:disable` | Turn enforcement off for this repo. Commits are kept |

agentlog only acts in repos where you ran `/agentlog:init`, unless you turn on the plugin option
**Enforce in every git repo**.

## Platforms

Prebuilt binaries: macOS (arm64, amd64), Linux (arm64, amd64), Windows (amd64, needs Git Bash
for the hooks). PowerShell-only Windows is not supported in v1.

Full documentation: https://github.com/harshitalpha/agentlog#readme
