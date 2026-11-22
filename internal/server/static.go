package server

import (
	"io/fs"
	"mime"
)

// fsReadFile reads a regular file from the embedded UI. Directories and
// missing files are errors.
func fsReadFile(s *Server, name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrNotExist
	}
	info, err := fs.Stat(s.static, name)
	if err != nil || info.IsDir() {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(s.static, name)
}

// mimeOf is the Content-Type the server uses for a file extension.
func mimeOf(ext string) string { return mime.TypeByExtension(ext) }
