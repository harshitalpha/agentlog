#!/usr/bin/env python3
"""
agentlog: make AI coding agents commit every change locally with a real
message, then browse those commits as a timeline in your browser.

Nothing is ever pushed. Everything stays in your local .git.

Commands
  agentlog init [path]      set up a repo (git init, baseline commit, Claude Code hook, agent rules)
  agentlog ui   [path]      open the timeline UI   (default http://127.0.0.1:7777)
  agentlog watch [path]     fallback for agents without hooks: auto-commit after edits settle
  agentlog hook-stop        (used by the Claude Code Stop hook, reads JSON on stdin)
  agentlog uninstall [path] remove the hook and the rules block

Python 3.8+ stdlib only. No dependencies.
"""
import argparse
import json
import os
import re
import subprocess
import sys
import threading
import time
import webbrowser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

HERE = Path(__file__).resolve().parent
UI_FILE = HERE / "ui.html"
SCRIPT = Path(__file__).resolve()

RULES_START = "<!-- agentlog:start -->"
RULES_END = "<!-- agentlog:end -->"
RULES = f"""{RULES_START}
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
- When fixing something you wrote earlier, make a new commit ("Fix nil check in
  GetProductsByDate") instead of amending, so the history shows the order of events.
- Stage with `git add -A` (respects .gitignore). Never commit secrets or .env files.
- NEVER run `git push`, `git commit --amend`, `git rebase`, `git reset --hard`,
  or anything else that rewrites or publishes history.
{RULES_END}"""

AGENT_PATTERNS = [
    ("Claude", re.compile(r"claude", re.I)),
    ("Codex", re.compile(r"codex|openai", re.I)),
    ("Cursor", re.compile(r"cursor", re.I)),
    ("Copilot", re.compile(r"copilot", re.I)),
    ("Gemini", re.compile(r"gemini", re.I)),
    ("Aider", re.compile(r"aider", re.I)),
]


# --------------------------------------------------------------------- git

def git(repo, *args, check=True, input_text=None):
    r = subprocess.run(
        ["git", "-C", str(repo), *args],
        capture_output=True, text=True, input=input_text,
        encoding="utf-8", errors="replace",
    )
    if check and r.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {r.stderr.strip()}")
    return r.stdout


def repo_root(path):
    r = subprocess.run(["git", "-C", str(path), "rev-parse", "--show-toplevel"],
                       capture_output=True, text=True)
    return Path(r.stdout.strip()) if r.returncode == 0 else None


def has_commits(repo):
    return subprocess.run(["git", "-C", str(repo), "rev-parse", "--verify", "-q", "HEAD"],
                          capture_output=True).returncode == 0


def dirty_files(repo):
    out = git(repo, "status", "--porcelain", "--untracked-files=all", check=False)
    return [line[3:] for line in out.splitlines() if line.strip()]


def detect_agent(author, body):
    text = body or ""
    m = re.search(r"^Agent:\s*(.+)$", text, re.M | re.I)
    if m:
        return m.group(1).strip()
    for line in re.findall(r"^Co-Authored-By:\s*(.+)$", text, re.M | re.I):
        for name, pat in AGENT_PATTERNS:
            if pat.search(line):
                return name
    if author == "agentlog":
        return "auto"
    for name, pat in AGENT_PATTERNS:
        if pat.search(author or ""):
            return name
    return None


def auto_commit(repo, label):
    files = dirty_files(repo)
    if not files:
        return None
    names = [Path(f.split(" -> ")[-1]).name for f in files]
    shown = ", ".join(names[:3]) + (f" (+{len(names) - 3} more)" if len(names) > 3 else "")
    subject = f"[auto] {label}: {shown}"[:100]
    body = "Committed automatically by agentlog because the agent left changes uncommitted.\n\nAgent: auto"
    git(repo, "add", "-A")
    git(repo, "-c", "user.name=agentlog", "-c", "user.email=agentlog@localhost",
        "commit", "-q", "--no-verify", "-m", subject, "-m", body)
    return subject


# ------------------------------------------------------------------ init

def ensure_identity(repo):
    name = git(repo, "config", "user.name", check=False).strip()
    email = git(repo, "config", "user.email", check=False).strip()
    if not name or not email:
        git(repo, "config", "user.name", name or "local-dev")
        git(repo, "config", "user.email", email or "local-dev@localhost")
        print("  set a repo-local git identity (no global one found)")


def write_rules(path):
    text = path.read_text() if path.exists() else ""
    if RULES_START in text:
        text = re.sub(re.escape(RULES_START) + r".*?" + re.escape(RULES_END), RULES, text, flags=re.S)
    else:
        text = (text.rstrip() + "\n\n" if text.strip() else "") + RULES + "\n"
    path.write_text(text)


def install_hook(repo):
    settings_path = repo / ".claude" / "settings.local.json"
    settings_path.parent.mkdir(exist_ok=True)
    settings = {}
    if settings_path.exists():
        try:
            settings = json.loads(settings_path.read_text() or "{}")
        except json.JSONDecodeError:
            print(f"  ! {settings_path} is not valid JSON, leaving it alone")
            return
    cmd = f'"{sys.executable}" "{SCRIPT}" hook-stop'
    stop = settings.setdefault("hooks", {}).setdefault("Stop", [])
    for group in stop:
        group["hooks"] = [h for h in group.get("hooks", []) if "agentlog" not in h.get("command", "")]
    stop[:] = [g for g in stop if g.get("hooks")]
    stop.append({"hooks": [{"type": "command", "command": cmd, "timeout": 30}]})
    settings_path.write_text(json.dumps(settings, indent=2) + "\n")


def cmd_init(args):
    repo = Path(args.path).resolve()
    if not repo.is_dir():
        sys.exit(f"not a directory: {repo}")
    root = repo_root(repo)
    if root is None:
        git(repo, "init", "-q")
        print(f"  initialised a local git repo in {repo}")
        root = repo
    repo = root
    ensure_identity(repo)

    gi = repo / ".gitignore"
    gi_text = gi.read_text() if gi.exists() else ""
    wanted = [".claude/settings.local.json", ".env", ".env.*"]
    missing = [w for w in wanted if w not in gi_text.split("\n")]
    if missing:
        gi.write_text(gi_text.rstrip("\n") + ("\n" if gi_text.strip() else "") + "\n".join(missing) + "\n")

    if not has_commits(repo) or dirty_files(repo):
        git(repo, "add", "-A")
        if git(repo, "diff", "--cached", "--name-only").strip():
            git(repo, "commit", "-q", "-m", "Baseline snapshot before agent work",
                "-m", "Created by agentlog init so later agent commits diff against this state.")
            print("  committed a baseline snapshot of the current code")

    install_hook(repo)
    print("  installed Claude Code Stop hook in .claude/settings.local.json")
    for name in ("CLAUDE.md", "AGENTS.md"):
        if name == "AGENTS.md" and args.no_agents_md:
            continue
        write_rules(repo / name)
        print(f"  wrote commit rules to {name}")
    git(repo, "add", "-A")
    if git(repo, "diff", "--cached", "--name-only").strip():
        git(repo, "commit", "-q", "-m", "Add agentlog commit rules for AI agents")
    print(f"\nDone. Run:  python3 {SCRIPT} ui {repo}")


def cmd_uninstall(args):
    repo = repo_root(Path(args.path).resolve()) or Path(args.path).resolve()
    sp = repo / ".claude" / "settings.local.json"
    if sp.exists():
        s = json.loads(sp.read_text() or "{}")
        stop = s.get("hooks", {}).get("Stop", [])
        for g in stop:
            g["hooks"] = [h for h in g.get("hooks", []) if "agentlog" not in h.get("command", "")]
        s.get("hooks", {})["Stop"] = [g for g in stop if g.get("hooks")]
        sp.write_text(json.dumps(s, indent=2) + "\n")
    for name in ("CLAUDE.md", "AGENTS.md"):
        p = repo / name
        if p.exists() and RULES_START in p.read_text():
            t = re.sub(r"\n*" + re.escape(RULES_START) + r".*?" + re.escape(RULES_END) + r"\n?", "\n", p.read_text(), flags=re.S)
            p.write_text(t.strip() + "\n" if t.strip() else "")
    print("agentlog hook and rules removed (your commits are untouched).")


# ------------------------------------------------------------- stop hook

def cmd_hook_stop(args):
    """Claude Code Stop hook. If the working tree is dirty, block the stop and
    tell Claude to commit. On the second attempt, auto-commit as a safety net."""
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except json.JSONDecodeError:
        payload = {}
    repo = repo_root(payload.get("cwd") or os.getcwd())
    if repo is None:
        return
    files = dirty_files(repo)
    if not files:
        return
    if payload.get("stop_hook_active"):
        # Claude already got one nudge and still left changes. Don't loop; save them.
        auto_commit(repo, "uncommitted changes")
        return
    listing = "\n".join(f"  {f}" for f in files[:40])
    if len(files) > 40:
        listing += f"\n  ... and {len(files) - 40} more"
    reason = (
        "agentlog: you have uncommitted changes. Before finishing, commit them locally.\n"
        "Split them into one commit per logical step if they cover more than one concern "
        "(use `git add <paths>` per commit). Each subject should say what was done in plain "
        "words, e.g. 'Add repo interface and impl for product DB'. Do NOT push, amend or rebase.\n\n"
        f"Uncommitted files:\n{listing}"
    )
    print(json.dumps({"decision": "block", "reason": reason}))


# ----------------------------------------------------------------- watch

def cmd_watch(args):
    repo = repo_root(Path(args.path).resolve())
    if repo is None:
        sys.exit("not a git repo, run `agentlog init` first")
    print(f"watching {repo}  (auto-commit after {args.quiet}s with no new edits, Ctrl+C to stop)")
    last_sig, since = None, None
    while True:
        status = git(repo, "status", "--porcelain", "--untracked-files=all", check=False)
        diff = git(repo, "diff", "--stat", check=False) if status else ""
        sig = (status, diff) if status else None
        now = time.time()
        if sig != last_sig:
            last_sig, since = sig, now
        elif sig and now - since >= args.quiet:
            msg = auto_commit(repo, "edits")
            if msg:
                print(time.strftime("%H:%M:%S"), msg)
            last_sig, since = None, None
        time.sleep(2)


# -------------------------------------------------------------------- ui

HASH_RE = re.compile(r"^[0-9a-f]{4,64}$")


def list_commits(repo, limit):
    if not has_commits(repo):
        return []
    fmt = "%x1e%H%x1f%h%x1f%P%x1f%an%x1f%ae%x1f%aI%x1f%s%x1f%b%x1f"
    out = git(repo, "log", f"-n{limit}", "--first-parent", "-M", f"--format={fmt}", "--numstat")
    commits = []
    for rec in out.split("\x1e")[1:]:
        parts = rec.split("\x1f")
        if len(parts) < 9:
            continue
        h, short, parents, an, ae, date, subj, body, rest = parts[:9]
        ins = dels = nfiles = 0
        for line in rest.strip().splitlines():
            cols = line.split("\t")
            if len(cols) >= 3:
                nfiles += 1
                ins += int(cols[0]) if cols[0].isdigit() else 0
                dels += int(cols[1]) if cols[1].isdigit() else 0
        commits.append({
            "hash": h, "short": short, "parents": parents.split(),
            "author": an, "email": ae, "date": date, "subject": subj,
            "body": body.strip(), "files": nfiles, "ins": ins, "dels": dels,
            "agent": detect_agent(an, body),
        })
    return commits


def commit_patch(repo, h):
    return git(repo, "show", "--format=", "--patch", "-M", "--no-color", "--first-parent", h)


def working_patch(repo):
    base = "HEAD" if has_commits(repo) else git(repo, "hash-object", "-t", "tree", "/dev/null").strip()
    patch = git(repo, "diff", base, "--patch", "-M", "--no-color", check=False)
    untracked = git(repo, "ls-files", "--others", "--exclude-standard", check=False).splitlines()
    for f in untracked[:200]:
        r = subprocess.run(["git", "-C", str(repo), "diff", "--no-index", "--no-color", "/dev/null", f],
                           capture_output=True, text=True, encoding="utf-8", errors="replace")
        patch += r.stdout
    return patch


def make_handler(repo):
    class H(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def send(self, code, body, ctype="application/json"):
            data = body if isinstance(body, bytes) else body.encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", ctype + "; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            u = urlparse(self.path)
            q = {k: v[0] for k, v in parse_qs(u.query).items()}
            try:
                if u.path in ("/", "/index.html"):
                    return self.send(200, UI_FILE.read_bytes(), "text/html")
                if u.path == "/api/repo":
                    branch = git(repo, "rev-parse", "--abbrev-ref", "HEAD", check=False).strip() or "(no commits)"
                    dirty = dirty_files(repo)
                    head = git(repo, "rev-parse", "HEAD", check=False).strip()
                    return self.send(200, json.dumps({
                        "name": repo.name, "path": str(repo), "branch": branch,
                        "dirty": len(dirty), "head": head,
                    }))
                if u.path == "/api/commits":
                    limit = min(int(q.get("limit", 1000)), 5000)
                    return self.send(200, json.dumps(list_commits(repo, limit)))
                if u.path == "/api/patch":
                    h = q.get("hash", "")
                    if h == "WORKING":
                        return self.send(200, json.dumps({"patch": working_patch(repo)}))
                    if not HASH_RE.match(h):
                        return self.send(400, '{"error":"bad hash"}')
                    return self.send(200, json.dumps({"patch": commit_patch(repo, h)}))
                if u.path == "/api/file":
                    h, p = q.get("hash", ""), q.get("path", "")
                    if h == "WORKING":
                        fp = (repo / p).resolve()
                        if repo not in fp.parents or not fp.is_file():
                            return self.send(404, '{"error":"not found"}')
                        return self.send(200, json.dumps({"content": fp.read_text(errors="replace")}))
                    if not HASH_RE.match(h) or not p or p.startswith("-"):
                        return self.send(400, '{"error":"bad request"}')
                    content = git(repo, "show", f"{h}:{p}", check=False)
                    return self.send(200, json.dumps({"content": content}))
                self.send(404, '{"error":"not found"}')
            except Exception as e:  # keep the UI alive on any git hiccup
                self.send(500, json.dumps({"error": str(e)}))
    return H


def cmd_ui(args):
    repo = repo_root(Path(args.path).resolve())
    if repo is None:
        sys.exit("not a git repo, run `agentlog init` first")
    srv = ThreadingHTTPServer(("127.0.0.1", args.port), make_handler(repo))
    url = f"http://127.0.0.1:{args.port}"
    print(f"agentlog UI for {repo}\n  {url}   (Ctrl+C to stop)")
    if not args.no_open:
        threading.Timer(0.6, lambda: webbrowser.open(url)).start()
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass


def main():
    ap = argparse.ArgumentParser(prog="agentlog", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("init"); p.add_argument("path", nargs="?", default=".")
    p.add_argument("--no-agents-md", action="store_true", help="only write CLAUDE.md")
    p.set_defaults(fn=cmd_init)
    p = sub.add_parser("ui"); p.add_argument("path", nargs="?", default=".")
    p.add_argument("--port", type=int, default=7777); p.add_argument("--no-open", action="store_true")
    p.set_defaults(fn=cmd_ui)
    p = sub.add_parser("watch"); p.add_argument("path", nargs="?", default=".")
    p.add_argument("--quiet", type=int, default=20, help="seconds of no edits before auto-commit")
    p.set_defaults(fn=cmd_watch)
    p = sub.add_parser("hook-stop"); p.set_defaults(fn=cmd_hook_stop)
    p = sub.add_parser("uninstall"); p.add_argument("path", nargs="?", default=".")
    p.set_defaults(fn=cmd_uninstall)
    args = ap.parse_args()
    args.fn(args)


if __name__ == "__main__":
    main()
