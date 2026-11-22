# 07: Local HTTP API (`internal/server`)

Base: `http://127.0.0.1:<port>`. GET only (anything else → 405). All JSON responses
send `Content-Type: application/json; charset=utf-8` and `Cache-Control: no-store`.
Errors use the shape `{"error":"message"}` with the right status code. Handlers must
never panic. Wrap them with a recover middleware that returns 500.

## Security (must implement, must test)

- Listen on `127.0.0.1` only. Never `0.0.0.0`.
- **Host check:** reject (403) any request whose `Host` header isn't
  `127.0.0.1:<port>` or `localhost:<port>`. This blocks DNS-rebinding attacks from web pages.
- **Hash param:** must match `^[0-9a-f]{4,64}$` or equal the literal `WORKING`, otherwise 400.
- **Path param:** non-empty, no leading `-`, no NUL, not absolute, and after
  `filepath.Clean` it must not start with `..`. For `WORKING`, the resolved absolute path
  (after EvalSymlinks) must be inside the repo root, otherwise 404.
- Static files are served from the embedded FS only.

## Endpoints

### `GET /` and static assets
Serves `web/static/*` from `embed.FS`. `/` → `index.html`. Set correct MIME types (`.js` → `text/javascript`).

### `GET /api/repo`
```json
{ "name": "demo", "path": "/tmp/demo", "branch": "main",
  "head": "dcba54d7707bcdbdc7faa6932f682780f8a04be7", "dirty": 2, "version": "0.1.0" }
```
`head` is `""` when there are no commits. `dirty` = `len(DirtyFiles())`. Must respond in <100 ms on a typical repo, since it's polled every 3 s.

### `GET /api/commits?limit=1000`
Array of `Commit` (schema in `06-git-layer.md`), newest first, with `agent` filled by `agent.DetectAgent`.

### `GET /api/patch?hash=<hash|WORKING>`
```json
{ "patch": "diff --git a/x b/x\n…" }
```
Raw unified patch text. Cap at 5 MB. If longer, truncate at a line boundary and add `"truncated": true`.

### `GET /api/file?hash=<hash|WORKING>&path=<repo-relative path>`
```json
{ "content": "…", "binary": false }
```
Binary → `{"content":"","binary":true}`. Missing → 404.

## Tests (`server_test.go`, use `httptest` and a temp repo)

- Each endpoint's happy path.
- `hash=../../etc` → 400. `path=../secret` → 400. `path=-p` → 400.
- `WORKING` with a symlink pointing outside the repo → 404.
- Wrong `Host` header → 403.
- POST → 405.
- Empty repo: `/api/commits` → `[]`, `/api/repo` → `head: ""`.
