# Releasing agentlog

Publishing needs your GitHub and claude.ai accounts, so it is a manual job. Everything up to
this point is built and tested locally; this file is the checklist for the rest.

## 0. What you need

- A **public** GitHub repo for the project (any name, these steps assume `<you>/agentlog`).
- For the Anthropic directory (step 6): the GitHub account connected on claude.ai, on a paid plan.
- Go 1.22+, `git`, `perl`, and the `claude` CLI (for `make validate`).

## 1. Replace the placeholders

The code uses the placeholder `OWNER` for the GitHub account (module path, imports, URLs) and for
the author's display name (manifest, marketplace, LICENSE). Replace both in one go from the repo root:

```sh
GH=your-github-username
NAME="Your Name"
git grep -lz 'OWNER' -- . ':!docs/DECISIONS.md' | xargs -0 perl -pi -e \
  "s#github\.com/OWNER#github.com/$GH#g; s#\bOWNER/agentlog#$GH/agentlog#g; s#\"OWNER\"#\"$NAME\"#g; s#Copyright \(c\) 2026 OWNER#Copyright (c) 2026 $NAME#g"
git grep -n OWNER          # should print only docs/DECISIONS.md
go build ./... && go vet ./... && make check
```

This was dry-run on a clean clone: it builds, `make check` passes and both validators pass.
Commit the result (`Replace OWNER placeholder with <you>`). Optional: set real `privacyPolicyUrl`
and other listing URLs if you host them elsewhere (they currently point at `PRIVACY.md` in this repo).

## 2. Check locally

- [ ] `make check` is green (fmt, vet, unit tests, plugin layout and launcher checks).
- [ ] `make e2e` is green (needs `e2e/node_modules`: run `npm install` in `e2e/` first; the
      Chromium download happens with `npx playwright install chromium`).
- [ ] `make dist` builds all 5 platforms into `plugin/scripts/dist/`.
- [ ] `make validate` is green (`claude plugin validate --strict ./plugin` and `claude plugin validate .`).
- [ ] The manual checklist in `docs/09-testing.md` §6 passes on a fresh folder. The results of
      the run done during development are in `docs/DECISIONS.md`; repeat the marketplace install
      item after publishing (step 5).

## 3. Version

The version lives in two places and `scripts/check-plugin.sh` (part of `make check`) fails if they differ:

- `plugin/.claude-plugin/plugin.json` → `"version"`
- `cmd/agentlog/main.go` → `var version = "…"`

Bump both for every release (`plugin.json` pins users to a version, so an unchanged version means
nobody receives an update). `make dist` takes its version from `plugin.json`.

## 4. Publish

```sh
git push origin main            # you push; the build agent never does
git tag v0.1.0                  # must equal "v" + the plugin.json version
git push origin v0.1.0
```

The `release` workflow (`.github/workflows/release.yml`) then checks the tag against `plugin.json`,
runs `make check dist`, and force-pushes the source plus `plugin/scripts/dist/` to the **`release`**
branch. That branch is what the marketplace entry installs from.

- [ ] Both workflows are green. **They have never run**: watch the first `ci` and `release` runs.
      Likely trouble spots: Playwright's browser install on the runner, and BSD vs GNU tools in
      `scripts/check-plugin.sh` on the macOS job.
- [ ] The `release` branch exists and contains `plugin/scripts/dist/agentlog-*` (5 files).

## 5. Test the real install

On a clean machine or a fresh Claude Code profile:

```
/plugin marketplace add <you>/agentlog
/plugin install agentlog@agentlog
```

- [ ] `/agentlog:init` in a scratch folder, then a session that changes files ends with commits.
- [ ] `/agentlog:ui` prints a URL and the timeline opens; running it twice gives the same URL.
- [ ] In a different git repo where init was never run, Claude finishes normally (no block, no commit).
- [ ] `/agentlog:disable` stops enforcement and keeps commits.

## 6. Submit to Anthropic's directory (optional)

1. The repo is public and the GitHub account is connected on claude.ai (paid plan).
2. Open `claude.ai/directory/manage` → **Submit new** → **Plugin bundle**, pick the repo and the
   `plugin` folder (on the `release` branch if the form asks for a ref).
3. Fill in the checklist. It reads the listing fields from `plugin.json`: `icon`, `documentationUrl`,
   `supportUrl` and `privacyPolicyUrl` must be real URLs. Privacy statement: *agentlog sends no data
   anywhere; everything stays on your machine* (also in `PRIVACY.md`).
4. Anthropic reviews it against the Software Directory Terms and Policy. After publication each new
   commit on the submitted ref is picked up and scanned automatically.

## 7. Later releases

Bump the version (step 3), commit, tag, push. Users update through `/plugin`.

## Known limits of v1

- PowerShell-only Windows is unsupported (the launcher is POSIX sh; Git Bash works).
- The Windows process-spawn code is cross-compiled but has never been run on Windows.
- Git worktrees of one repo share `.git/agentlog/`, so they share one UI process.
