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
