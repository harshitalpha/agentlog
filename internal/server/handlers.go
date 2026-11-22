package server

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/OWNER/agentlog/internal/agent"
	"github.com/OWNER/agentlog/internal/gitx"
)

// Limits (docs/07-http-api.md).
const (
	maxPatchBytes = 5 << 20 // 5 MB
	maxFileBytes  = 2 << 20 // 2 MB
	binarySniff   = 8 << 10
)

// contentSecurityPolicy is sent with the UI. It only allows our own scripts
// and styles, so even if untrusted text (commit messages, file names) ever
// reached the DOM unescaped, nothing could run or be exfiltrated, and no other
// site can frame the UI.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; " +
	"connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

type repoInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Head    string `json:"head"`
	Dirty   int    `json:"dirty"`
	Version string `json:"version"`
}

// handleRepo answers the UI's 3-second poll, so it runs its three git calls
// concurrently.
func (s *Server) handleRepo(w http.ResponseWriter, r *http.Request) {
	var (
		wg           sync.WaitGroup
		branch, head string
		files        []gitx.DirtyFile
		headErr      error
		filesErr     error
	)
	wg.Add(3)
	go func() { defer wg.Done(); branch = s.repo.Branch() }()
	go func() { defer wg.Done(); head, headErr = s.repo.Head() }()
	go func() { defer wg.Done(); files, filesErr = s.repo.DirtyFiles() }()
	wg.Wait()
	if err := errors.Join(headErr, filesErr); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repoInfo{
		Name:    filepath.Base(s.repo.Root),
		Path:    s.repo.Root,
		Branch:  branch,
		Head:    head,
		Dirty:   len(files),
		Version: s.version,
	})
}

func (s *Server) handleCommits(w http.ResponseWriter, r *http.Request) {
	limit := gitx.DefaultLogLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, gitx.MaxLogLimit)
	}
	commits, err := s.repo.Log(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range commits {
		commits[i].Agent = agent.DetectAgent(commits[i].Author, commits[i].Body)
	}
	writeJSON(w, http.StatusOK, commits) // Log never returns nil, so empty is []
}

type patchResponse struct {
	Patch     string `json:"patch"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (s *Server) handlePatch(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if !validHash(hash) {
		writeError(w, http.StatusBadRequest, "bad hash")
		return
	}
	var (
		patch string
		err   error
	)
	if hash == workingHash {
		patch, err = s.repo.WorkingPatch()
	} else {
		patch, err = s.repo.CommitPatch(hash)
	}
	switch {
	case errors.Is(err, gitx.ErrNotFound):
		writeError(w, http.StatusNotFound, "commit not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := patchResponse{Patch: patch}
	if len(patch) > maxPatchBytes {
		resp.Patch, resp.Truncated = truncateAtLine(patch, maxPatchBytes), true
	}
	writeJSON(w, http.StatusOK, resp)
}

// truncateAtLine cuts s to at most max bytes, at a line boundary.
func truncateAtLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := strings.LastIndexByte(s[:max], '\n')
	if cut < 0 {
		return ""
	}
	return s[:cut+1]
}

type fileResponse struct {
	Content string `json:"content"`
	Binary  bool   `json:"binary"`
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hash := q.Get("hash")
	if !validHash(hash) {
		writeError(w, http.StatusBadRequest, "bad hash")
		return
	}
	rel, ok := cleanRepoPath(q.Get("path"))
	if !ok {
		writeError(w, http.StatusBadRequest, "bad path")
		return
	}

	var (
		content string
		err     error
	)
	if hash == workingHash {
		content, err = s.readWorkingFile(rel)
	} else {
		content, err = s.repo.FileAt(hash, rel)
	}
	switch {
	case errors.Is(err, gitx.ErrBinary):
		writeJSON(w, http.StatusOK, fileResponse{Binary: true})
	case errors.Is(err, gitx.ErrNotFound), errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, gitx.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "file too large")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, fileResponse{Content: content})
	}
}

// readWorkingFile reads rel from disk, but only if it really lives inside the
// repo once symlinks are resolved. Anything else is reported as not found.
func (s *Server) readWorkingFile(rel string) (string, error) {
	root, err := filepath.EvalSymlinks(s.repo.Root)
	if err != nil {
		return "", err
	}
	full, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", os.ErrNotExist
	}
	inside, err := filepath.Rel(root, full)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", os.ErrNotExist
	}
	fi, err := os.Stat(full)
	if err != nil || !fi.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	if fi.Size() > maxFileBytes {
		return "", gitx.ErrTooLarge
	}
	f, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data[:min(len(data), binarySniff)], 0) >= 0 {
		return "", gitx.ErrBinary
	}
	return string(data), nil
}

// handleStatic serves the embedded UI. Only files that exist in the embedded
// FS are served; "/" maps to index.html.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	data, err := fsReadFile(s, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	_, _ = w.Write(data)
}
