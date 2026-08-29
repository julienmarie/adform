package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// RunReader exposes the read-only subset of the CLI used by Karajan.
func RunReader(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "stats" {
		fmt.Fprintln(stderr, "error: only stats is supported by adform-reader")
		return 1
	}
	for _, arg := range args[1:] {
		name := "--" + strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		switch name {
		case "--export", "--save-snapshot", "--state":
			fmt.Fprintf(stderr, "error: %s is not available in adform-reader\n", name)
			return 1
		}
	}
	return runStats(ctx, args[1:], stdout, stderr)
}
