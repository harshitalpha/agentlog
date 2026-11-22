# 03: Project structure

The repo is a Go module, a Claude Code plugin (`plugin/`) and a plugin marketplace
(`.claude-plugin/marketplace.json`) at the same time. See `11-plugin-packaging.md` for why.
Build exactly this layout. Don't add top-level folders without recording why in `docs/DECISIONS.md`.

```
agentlog/
├── .claude-plugin/
│   └── marketplace.json            # marketplace catalog → points at plugin/ on the release branch
├── plugin/                         # ← the Claude Code plugin (only this folder is installed)
│   ├── .claude-plugin/plugin.json  # manifest: name, version, listing fields, userConfig
│   ├── hooks/hooks.json            # SessionStart + Stop → scripts/agentlog hook-*
│   ├── skills/
│   │   ├── init/SKILL.md           # /agentlog:init
│   │   ├── ui/SKILL.md             # /agentlog:ui
│   │   ├── disable/SKILL.md        # /agentlog:disable
│   │   └── status/SKILL.md         # /agentlog:status
│   ├── scripts/
│   │   ├── agentlog                # POSIX launcher (executable bit), picks dist/ binary
│   │   └── dist/                   # built binaries (gitignored on main, committed on release branch)
│   ├── assets/icon.png
│   ├── README.md
│   └── LICENSE
├── cmd/
│   └── agentlog/
│       └── main.go                 # entrypoint: os.Exit(cli.Run(os.Args[1:])); var version
├── internal/
│   ├── cli/
│   │   ├── cli.go                  # Run(args) int: dispatch, usage, version
│   │   ├── init.go                 # agentlog init
│   │   ├── disable.go              # agentlog disable
│   │   ├── status.go               # agentlog status
│   │   ├── ui.go                   # agentlog ui (foreground, --detach, --stop)
│   │   ├── watch.go                # agentlog watch
│   │   ├── hooks.go                # hook-session-start, hook-stop (stdin → agent → stdout)
│   │   ├── uninstall.go            # agentlog uninstall (standalone)
│   │   └── cli_test.go
│   ├── gitx/
│   │   ├── run.go                  # Repo, run(args...) with timeout, error with stderr
│   │   ├── repo.go                 # Open, Init, HasCommits, Head, Branch
│   │   ├── config.go               # GetLocal/SetLocal/UnsetLocal (agentlog.enabled)
│   │   ├── status.go               # DirtyFiles (porcelain -z)
│   │   ├── log.go                  # Log(limit) with numstat
│   │   ├── patch.go                # CommitPatch, WorkingPatch
│   │   ├── show.go                 # FileAt
│   │   ├── commit.go               # AddAll, StagedEmpty, Commit, AutoCommit
│   │   ├── identity.go             # EnsureIdentity
│   │   ├── testutil_test.go
│   │   └── *_test.go
│   ├── agent/
│   │   ├── rules.md                # rules text (go:embed)
│   │   ├── rules.go                # UpsertRules / RemoveRules (marker blocks)
│   │   ├── enabled.go              # IsEnabled(repo, env): git config OR enforce_everywhere
│   │   ├── sessionstart.go         # HandleSessionStart(input, env) → output
│   │   ├── stophook.go             # HandleStop(input, env) → output
│   │   ├── standalone.go           # Install/RemoveStandaloneHooks (settings.local.json merge)
│   │   ├── detect.go               # DetectAgent(author, body)
│   │   └── *_test.go
│   ├── uiproc/
│   │   ├── uiproc.go               # detach/reuse/stop: ui.json, pid checks, background spawn
│   │   ├── spawn_unix.go           # //go:build !windows  (Setsid)
│   │   └── spawn_windows.go        # //go:build windows
│   └── server/
│       ├── server.go               # New(repo) http.Handler, idle timeout
│       ├── handlers.go             # /api/repo, /api/commits, /api/patch, /api/file, static
│       ├── validate.go             # hash/path checks, Host check
│       └── server_test.go
├── web/
│   ├── embed.go                    # //go:embed static
│   └── static/
│       ├── index.html
│       ├── styles.css
│       ├── app.js                  # state, polling, events (ES module)
│       ├── diff.js                 # parsePatch (pure, no DOM)
│       ├── render.js               # DOM builders
│       └── util.js                 # esc, timeAgo, dayLabel
├── e2e/
│   ├── package.json                # @playwright/test (test-only)
│   ├── playwright.config.ts
│   └── tests/{timeline.spec.ts, diff.unit.spec.ts}
├── scripts/
│   ├── make-demo-repo.sh           # realistic demo repo in $1
│   ├── fake-claude-stop.sh         # pipes sample Stop JSON into the binary
│   └── fake-session-start.sh       # pipes sample SessionStart JSON into the binary
├── testdata/patches/               # sample patches: add, delete, rename, binary, multi-hunk, no-newline
├── .github/workflows/
│   ├── ci.yml                      # make check + e2e on push/PR
│   └── release.yml                 # on tag v*: make dist, push release branch
├── docs/                           # this spec (+ DECISIONS.md as you go)
├── reference/                      # prototype + screenshots (read-only)
├── .gitignore                      # bin/, plugin/scripts/dist/, e2e/node_modules/, e2e/test-results/
├── go.mod                          # module github.com/<you>/agentlog, go 1.22
├── Makefile
├── RELEASING.md                    # release checklist (11-plugin-packaging.md §9)
├── CLAUDE.md                       # instructions for the agent building this repo
├── AGENTS.md
└── README.md                       # user-facing: install via /plugin, quickstart, standalone, FAQ
```

## Makefile targets

| Target | Does |
|---|---|
| `make build` | `go build -o bin/agentlog ./cmd/agentlog` |
| `make check` | `gofmt -l .` empty, `go vet ./...`, `go test ./...`, `sh -n plugin/scripts/agentlog` |
| `make e2e` | build, `scripts/make-demo-repo.sh /tmp/agentlog-e2e`, run Playwright |
| `make demo` | build, demo repo, `bin/agentlog ui /tmp/agentlog-demo` |
| `make dist` | cross-compile all 5 platforms into `plugin/scripts/dist/` |
| `make dist-local` | current platform only, for `claude --plugin-dir ./plugin` |
| `make validate` | `claude plugin validate ./plugin && claude plugin validate .` (skips with a message if `claude` isn't installed) |
| `make install` | `go install ./cmd/agentlog` (standalone) |

## Rules of thumb

- Only `internal/gitx` calls `exec.Command("git", …)`.
- `internal/cli` holds no business logic.
- `web/static/diff.js` stays pure so Node can test it.
- Nothing in `plugin/` may reference paths outside `plugin/` (the folder is copied on install).
- No file over ~400 lines.
