package cli

import (
	"flag"
	"fmt"
	"io"
)

// parseArgs parses flags that may appear before or after the single optional
// [path] argument (Go's flag package stops at the first non-flag). It returns
// the path (default ".") and whether parsing succeeded; on failure a message
// has been written to errW.
func parseArgs(name string, fs *flag.FlagSet, args []string, errW io.Writer) (path string, ok bool) {
	fs.SetOutput(io.Discard)
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(errW, "agentlog: %s: %v\n", name, err)
			return "", false
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	switch len(positional) {
	case 0:
		return ".", true
	case 1:
		return positional[0], true
	}
	fmt.Fprintf(errW, "agentlog: %s: expected at most one path, got %d\n", name, len(positional))
	return "", false
}
