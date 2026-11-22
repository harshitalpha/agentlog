// Package server implements the local read-only HTTP API and serves the
// embedded web UI. It only ever listens on 127.0.0.1.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/harshitalpha/agentlog/internal/gitx"
	"github.com/harshitalpha/agentlog/web"
)

// Defaults from docs/04-cli.md and docs/07-http-api.md.
const (
	DefaultPort        = 7777
	PortFallbackRange  = 10
	DefaultIdleTimeout = 8 * time.Hour
)

// ErrIdleTimeout is returned by Serve when it stopped because no request
// arrived for the idle timeout. It is a normal way to finish.
var ErrIdleTimeout = errors.New("stopped after idle timeout")

func init() {
	// Don't rely on the OS mime database: .js must be text/javascript.
	_ = mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	_ = mime.AddExtensionType(".css", "text/css; charset=utf-8")
	_ = mime.AddExtensionType(".html", "text/html; charset=utf-8")
}

// Options configure a Server.
type Options struct {
	Version string // reported by /api/repo
	Port    int    // port used by the Host check; Serve sets it from the listener
}

// Server is the HTTP handler for one repo.
type Server struct {
	repo    *gitx.Repo
	version string
	port    atomic.Int32
	lastReq atomic.Int64 // unix nanos of the last accepted request
	static  fs.FS
}

// New creates a Server for repo.
func New(repo *gitx.Repo, opts Options) *Server {
	s := &Server{repo: repo, version: opts.Version}
	s.port.Store(int32(opts.Port))
	s.touch()
	sub, err := fs.Sub(web.Static, "static")
	if err != nil {
		panic(err) // the embed directive guarantees "static" exists
	}
	s.static = sub
	return s
}

// SetPort sets the port the Host check expects.
func (s *Server) SetPort(p int) { s.port.Store(int32(p)) }

func (s *Server) touch() { s.lastReq.Store(time.Now().UnixNano()) }

// idleFor is how long it has been since the last accepted request.
func (s *Server) idleFor() time.Duration {
	return time.Since(time.Unix(0, s.lastReq.Load()))
}

// Handler returns the full handler chain:
// recover → Host check → GET only → idle bookkeeping → routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/repo", s.handleRepo)
	mux.HandleFunc("/api/commits", s.handleCommits)
	mux.HandleFunc("/api/patch", s.handlePatch)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.HandleFunc("/", s.handleStatic)

	var h http.Handler = mux
	h = s.touchMiddleware(h)
	h = methodMiddleware(h)
	h = s.hostMiddleware(h)
	h = recoverMiddleware(h)
	return h
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host, int(s.port.Load())) {
			writeError(w, http.StatusForbidden, "forbidden host")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func methodMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// touchMiddleware records activity. It sits after the Host check so requests
// from other origins cannot keep the server alive.
func (s *Server) touchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.touch()
		next.ServeHTTP(w, r)
	})
}

// writeJSON sends v as JSON with the headers required for every API response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		status, body = http.StatusInternalServerError, []byte(`{"error":"internal error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Listen binds 127.0.0.1:port, trying the next PortFallbackRange ports when
// it is busy. port 0 picks any free port. It never binds a public address.
func Listen(port int) (net.Listener, error) {
	if port == 0 {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	var lastErr error
	for p := port; p <= port+PortFallbackRange && p <= 65535; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("ports %d-%d are all busy: %w", port, port+PortFallbackRange, lastErr)
}

// Serve runs the server on ln until ctx is cancelled (returns nil) or no
// request has arrived for idle (returns ErrIdleTimeout). idle <= 0 means
// DefaultIdleTimeout.
func (s *Server) Serve(ctx context.Context, ln net.Listener, idle time.Duration) error {
	if idle <= 0 {
		idle = DefaultIdleTimeout
	}
	s.SetPort(ln.Addr().(*net.TCPAddr).Port)
	s.touch()
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	check := idle / 10
	if check < 10*time.Millisecond {
		check = 10 * time.Millisecond
	}
	if check > time.Minute {
		check = time.Minute
	}
	ticker := time.NewTicker(check)
	defer ticker.Stop()

	var result error
	for done := false; !done; {
		select {
		case err := <-errc:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-ctx.Done():
			done = true
		case <-ticker.C:
			if s.idleFor() >= idle {
				result, done = ErrIdleTimeout, true
			}
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		_ = srv.Close()
	}
	return result
}
