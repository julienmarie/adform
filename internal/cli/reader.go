package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RunReader exposes the read-only subset of the CLI used by Karajan.
func RunReader(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := validateReaderArgs(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return runStats(ctx, args[1:], stdout, stderr)
}

func validateReaderArgs(args []string) error {
	if len(args) == 0 || args[0] != "stats" {
		return fmt.Errorf("only stats is supported by adform-reader")
	}

	values := make(map[string]string)
	seenJSON := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			return fmt.Errorf("%q is not available in adform-reader", arg)
		}
		parts := strings.SplitN(arg, "=", 2)
		name := parts[0]
		if name == "--json" {
			if len(parts) != 1 || seenJSON {
				return fmt.Errorf("%s is not available in adform-reader", arg)
			}
			seenJSON = true
			continue
		}
		switch name {
		case "--root", "--account", "--level", "--last", "--event":
		default:
			return fmt.Errorf("%s is not available in adform-reader", name)
		}
		if _, duplicate := values[name]; duplicate {
			return fmt.Errorf("duplicate %s is not available in adform-reader", name)
		}
		var value string
		if len(parts) == 2 {
			value = parts[1]
		} else {
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("%s requires a value", name)
			}
			value = args[i]
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s requires a value", name)
		}
		values[name] = value
	}

	if level := values["--level"]; level != "" && level != "campaign" && level != "adset" {
		return fmt.Errorf("--level value %q is not available in adform-reader", level)
	}
	if last := values["--last"]; last != "" && last != "today" && last != "last_7d" {
		return fmt.Errorf("--last value %q is not available in adform-reader", last)
	}
	if event := values["--event"]; event != "" && event != "purchase" {
		return fmt.Errorf("--event value %q is not available in adform-reader", event)
	}
	for _, required := range []string{"--root", "--account", "--level", "--last", "--event"} {
		if values[required] == "" {
			return fmt.Errorf("%s is required by adform-reader", required)
		}
	}
	if !seenJSON {
		return errors.New("--json is required by adform-reader")
	}
	return nil
}
