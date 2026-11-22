# 06: Git layer (`internal/gitx`)

All git access goes through this package. Always run `git -C <repoRoot> …` with an
explicit argument slice (never through a shell). Set a 15 s context timeout per call
and the env vars `GIT_TERMINAL_PROMPT=0` and `LC_ALL=C`. Return errors that include stderr.

## Types

```go
type Repo struct { Root string }

type Commit struct {
    Hash    string    `json:"hash"`
    Short   string    `json:"short"`
    Parents []string  `json:"parents"`
    Author  string    `json:"author"`
    Email   string    `json:"email"`
    Date    time.Time `json:"date"`     // author date, RFC3339 in JSON
    Subject string    `json:"subject"`
    Body    string    `json:"body"`     // trimmed
    Files   int       `json:"files"`
    Ins     int       `json:"ins"`
    Dels    int       `json:"dels"`
    Agent   string    `json:"agent"`    // from agent.DetectAgent; "" = human
}
```

(`Agent` is filled by the caller, the server, so `gitx` doesn't import `agent`.)

## Commands

| Function | Command | Notes |
|---|---|---|
| `Open(path)` | `git -C path rev-parse --show-toplevel` | Non-zero exit → `ErrNotRepo` |
| `Init(path)` | `git -C path init -q` | |
| `HasCommits()` | `rev-parse --verify -q HEAD` | exit code only |
| `Head()` | `rev-parse HEAD` | `""` when no commits |
| `Branch()` | `rev-parse --abbrev-ref HEAD` | when no commits, use `symbolic-ref --short HEAD` |
| `DirtyFiles()` | `status --porcelain=v1 -z --untracked-files=all` | parse NUL-separated; for renames (`R`), the entry is followed by the old path, so skip it |
| `Log(limit)` | see below | returns `[]` when no commits |
| `CommitPatch(h)` | `show --format= --patch -M --no-color --first-parent --no-ext-diff <h>` | works for root commits too |
| `WorkingPatch()` | `diff <base> --patch -M --no-color --no-ext-diff` + one `diff --no-index --no-color -- /dev/null <f>` per untracked file (cap at 200 files, skip files >1 MB) | `<base>` = `HEAD`, or the empty tree `4b825dc642cb6eb9a060e54bf8d69288fbee4904` when there are no commits. `diff --no-index` exits 1 when files differ; that's success here |
| `FileAt(h, p)` | `show <h>:<p>` | size cap 2 MB; binary (NUL in first 8 KB) → error `ErrBinary` |
| `AddAll()` | `add -A` | |
| `StagedEmpty()` | `diff --cached --quiet` | exit 0 = nothing staged |
| `Commit(subj, body, id *Identity)` | `[-c user.name=… -c user.email=…] commit -q [--no-verify] -m subj [-m body]` | |

## Log parsing

```
git log -n<limit> --first-parent -M --numstat \
  --format=%x1e%H%x1f%h%x1f%P%x1f%an%x1f%ae%x1f%aI%x1f%s%x1f%b%x1f
```

- Split output on `\x1e` and drop the first empty chunk.
- Split each record on `\x1f`. Expect ≥9 parts: 8 fields, then the numstat remainder.
- Numstat lines: `ins<TAB>dels<TAB>path`. Binary files show `-` and count 0. Each line counts as one file.
- Default limit 1000, max 5000.

## Edge cases that must have tests

1. Fresh repo with no commits: Log → `[]`, Branch → `main`/`master`, WorkingPatch shows untracked files.
2. Root commit patch renders (all files added).
3. Rename detection: status and patch report the new path.
4. File names with spaces and unicode.
5. Binary file in a commit: numstat `-`, patch says `Binary files … differ`.
6. File without a trailing newline (`\ No newline at end of file` line in the patch).
7. Commit body containing `|`, tabs and blank lines.
8. Merge commit on the first-parent line: patch against the first parent only.
9. `.gitignore`d files never appear in DirtyFiles or WorkingPatch.
10. Running from a subdirectory resolves to the root.
