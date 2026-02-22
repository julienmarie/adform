package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"adform/internal/workspace"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}

	switch args[0] {
	case "init":
		return runInit(ctx, args[1:], stdout, stderr)
	case "validate":
		return runValidate(ctx, args[1:], stdout, stderr)
	case "plan":
		return runPlan(ctx, args[1:], stdout, stderr)
	case "apply":
		return runApply(ctx, args[1:], stdout, stderr)
	case "assets":
		return runAssets(ctx, args[1:], stdout, stderr)
	case "stats":
		return runStats(ctx, args[1:], stdout, stderr)
	case "feed":
		return runFeed(ctx, args[1:], stdout, stderr)
	case "photoedit":
		return runPhotoEdit(ctx, args[1:], stdout, stderr)
	case "k8s":
		return runK8s(ctx, args[1:], stdout, stderr)
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr)
	case "gsc":
		return runGSC(ctx, args[1:], stdout, stderr)
	case "posthog":
		return runPostHog(ctx, args[1:], stdout, stderr)
	case "log":
		return runLog(ctx, args[1:], stdout, stderr)
	case "drift":
		return runDrift(ctx, args[1:], stdout, stderr)
	case "import":
		return runImport(ctx, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		if len(args) > 1 {
			if printCommandHelp(stdout, args[1]) {
				return 0
			}
			fmt.Fprintf(stderr, "unknown help topic: %s\n", args[1])
			printUsage(stderr)
			return 1
		}
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	printGeneralHelp(w)
}

func mainErr(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

func defaultStatePath(root string) string {
	return root + "/.adform/state.db"
}

func ensureAccount(account string) error {
	if account == "" {
		return fmt.Errorf("--account is required")
	}
	return nil
}

func accountDir(root, account string) string {
	return workspace.ResolveMetaDir(root, account)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
