package cli

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/OWNER/agentlog/internal/server"
	"github.com/OWNER/agentlog/internal/uiproc"
)

// envUIPort is how Claude Code passes the plugin's ui_port option to the binary.
const envUIPort = "CLAUDE_PLUGIN_OPTION_UI_PORT"

// uiIdleTimeout is how long the server may go without a request. Tests lower it.
var uiIdleTimeout = server.DefaultIdleTimeout

// resolvePort picks the UI port: the --port flag (-1 when unset), else the
// plugin option env var, else 7777. A bad env value is ignored, a bad flag is
// an error. 0 means "any free port" (useful for tests).
func resolvePort(flagPort int, env string) (int, error) {
	if flagPort != -1 {
		if flagPort < 0 || flagPort > 65535 {
			return 0, fmt.Errorf("--port must be between 0 and 65535, got %d", flagPort)
		}
		return flagPort, nil
	}
	if n, err := strconv.Atoi(env); err == nil && n >= 1 && n <= 65535 {
		return n, nil
	}
	return server.DefaultPort, nil
}

func uiCmd(args []string, s streams) int {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	port := fs.Int("port", -1, "port to listen on (default 7777, or the ui_port plugin option)")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	detach := fs.Bool("detach", false, "run in the background (or reuse a running server) and print its URL")
	stop := fs.Bool("stop", false, "stop the background server")
	path, ok := parseArgs("ui", fs, args, s.err)
	if !ok {
		return exitUsage
	}
	if *detach && *stop {
		fmt.Fprintln(s.err, "agentlog: ui: --detach and --stop cannot be used together")
		return exitUsage
	}
	startPort, err := resolvePort(*port, os.Getenv(envUIPort))
	if err != nil {
		fmt.Fprintf(s.err, "agentlog: ui: %v\n", err)
		return exitUsage
	}
	if *detach || *stop {
		fmt.Fprintln(s.err, "agentlog: ui: --detach and --stop are not implemented yet")
		return exitError
	}
	repo, ok := openRepoArg("ui", path, s)
	if !ok {
		return exitError
	}

	ln, err := server.Listen(startPort)
	if err != nil {
		fmt.Fprintf(s.err, "agentlog: ui: %v\n", err)
		return exitError
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)
	srv := server.New(repo, server.Options{Version: Version})
	fmt.Fprintf(s.out, "agentlog UI for %s → %s  (Ctrl+C to stop)\n", repo.Root, url)

	if !*noOpen {
		// Opening the browser must never take the server down.
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := uiproc.OpenURL(url); err != nil {
				fmt.Fprintf(s.err, "agentlog: could not open a browser (%v); open %s yourself\n", err, url)
			}
		}()
	}

	switch err := srv.Serve(s.context(), ln, uiIdleTimeout); {
	case errors.Is(err, server.ErrIdleTimeout):
		fmt.Fprintln(s.out, "agentlog: UI stopped after being idle")
	case err != nil:
		fmt.Fprintf(s.err, "agentlog: ui: %v\n", err)
		return exitError
	default:
		fmt.Fprintln(s.out, "agentlog: UI stopped")
	}
	return exitOK
}
