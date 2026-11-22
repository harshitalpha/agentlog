// Command agentlog makes AI coding agents commit every change locally and
// shows the history as a timeline in a local web UI.
package main

import (
	"os"

	"github.com/harshitalpha/agentlog/internal/cli"
)

// version is overridden at build time with -ldflags "-X main.version=…".
var version = "0.1.0"

func main() {
	cli.Version = version
	os.Exit(cli.Run(os.Args[1:]))
}
