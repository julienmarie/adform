package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunReaderRejectsNonStatsCommands(t *testing.T) {
	for _, command := range []string{"apply", "plan", "validate", "import", "serve", "help"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := RunReader(context.Background(), []string{command}, &stdout, &stderr); code == 0 {
				t.Fatalf("RunReader(%q) succeeded; stderr=%q", command, stderr.String())
			}
			if !strings.Contains(stderr.String(), "only stats is supported") {
				t.Fatalf("stderr = %q, want least-privilege rejection", stderr.String())
			}
		})
	}
}

func TestRunReaderRejectsStatsMutationFlags(t *testing.T) {
	for _, flag := range []string{
		"--export=stats.json", "--save-snapshot", "--state=state.db",
		"-export=stats.json", "-save-snapshot", "-state=state.db",
		"--level=ad", "--verbose", "--breakdown=age", "--compare=yesterday",
		"--limit=10", "--unknown", "positional",
	} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"stats", "--account=test", flag}
			if code := RunReader(context.Background(), args, &stdout, &stderr); code == 0 {
				t.Fatalf("RunReader(%q) succeeded; stderr=%q", flag, stderr.String())
			}
			if !strings.Contains(stderr.String(), "not available in adform-reader") {
				t.Fatalf("stderr = %q, want mutation rejection", stderr.String())
			}
		})
	}
}

func TestValidateReaderArgsAcceptsExactKarajanContract(t *testing.T) {
	args := []string{
		"stats", "--root", "/workspace", "--account", "example",
		"--level", "campaign", "--last", "last_7d", "--event", "purchase", "--json",
	}
	if err := validateReaderArgs(args); err != nil {
		t.Fatalf("validateReaderArgs() error = %v", err)
	}
}

func TestValidateReaderArgsRejectsInvalidValuesBeforeLookup(t *testing.T) {
	tests := [][]string{
		{"stats", "--account", "x", "--level", "ad"},
		{"stats", "--account", "x", "--last", "7d"},
		{"stats", "--account", "x", "--last", "yesterday"},
		{"stats", "--account", "x", "--event", "lead"},
		{"stats", "--account", "x", "--json=false"},
		{"stats", "--account", "x", "extra"},
	}
	for _, args := range tests {
		if err := validateReaderArgs(args); err == nil {
			t.Errorf("validateReaderArgs(%q) succeeded", args)
		}
	}
}

func TestValidateReaderArgsRequiresCompleteKarajanContract(t *testing.T) {
	base := []string{
		"stats", "--root", "/workspace", "--account", "example",
		"--level", "campaign", "--last", "last_7d", "--event", "purchase", "--json",
	}
	for _, omitted := range []string{"--root", "--account", "--level", "--last", "--event", "--json"} {
		args := omitReaderFlag(base, omitted)
		if err := validateReaderArgs(args); err == nil {
			t.Errorf("validateReaderArgs() accepted contract without %s", omitted)
		}
	}
}

func omitReaderFlag(args []string, omitted string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != omitted {
			out = append(out, args[i])
			continue
		}
		if omitted != "--json" {
			i++
		}
	}
	return out
}
