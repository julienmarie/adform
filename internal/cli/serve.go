package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"adform/internal/landing"
	"os/signal"
)

type serveOptions struct {
	Account     string
	Root        string
	Bind        string
	StatePath   string
	NoHotReload bool
	LogLevel    string
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts := serveOptions{}
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.Account, "account", "", "Account/property name (required; fallback ADFORM_SERVER_ACCOUNT)")
	fs.StringVar(&opts.Root, "root", "", "Repo root (default . or ADFORM_SERVER_ROOT)")
	fs.StringVar(&opts.Bind, "bind", "", "Bind address override (e.g. 0.0.0.0:8080)")
	fs.StringVar(&opts.StatePath, "state", "", "Landing state DB path override")
	fs.BoolVar(&opts.NoHotReload, "no-hot-reload", false, "Disable hot reload even in dev")
	fs.StringVar(&opts.LogLevel, "log-level", "", "Log level: debug|info|warn|error")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	account := strings.TrimSpace(opts.Account)
	if account == "" {
		account = strings.TrimSpace(os.Getenv("ADFORM_SERVER_ACCOUNT"))
	}
	if account == "" {
		fmt.Fprintln(stderr, "error: --account is required (or set ADFORM_SERVER_ACCOUNT)")
		return 1
	}
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("ADFORM_SERVER_ROOT"))
	}
	if root == "" {
		root = "."
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ADFORM_SERVER_ENV")))
	if env == "" {
		env = "dev"
	}
	if env != "dev" && env != "prod" {
		fmt.Fprintf(stderr, "error: invalid ADFORM_SERVER_ENV %q (expected dev|prod)\n", env)
		return 1
	}

	hotReload := env == "dev" && !opts.NoHotReload
	bind := strings.TrimSpace(opts.Bind)
	if bind == "" {
		bind = strings.TrimSpace(os.Getenv("ADFORM_SERVER_BIND"))
	}
	statePath := strings.TrimSpace(opts.StatePath)
	if statePath == "" {
		statePath = strings.TrimSpace(os.Getenv("ADFORM_SERVER_STATE_PATH"))
	}
	if statePath == "" {
		statePath = filepath.Join(root, ".adform", fmt.Sprintf("landing_state_%s.db", account))
	}
	logLevel := strings.ToLower(strings.TrimSpace(opts.LogLevel))
	if logLevel == "" {
		logLevel = strings.ToLower(strings.TrimSpace(os.Getenv("ADFORM_SERVER_LOG_LEVEL")))
	}
	if logLevel == "" {
		if env == "dev" {
			logLevel = "debug"
		} else {
			logLevel = "info"
		}
	}

	trustProxy := strings.EqualFold(strings.TrimSpace(os.Getenv("ADFORM_SERVER_TRUST_PROXY")), "true")
	publicBaseOverride := strings.TrimSpace(os.Getenv("ADFORM_SERVER_PUBLIC_BASE_URL"))
	mainSiteBaseOverride := strings.TrimSpace(os.Getenv("ADFORM_SERVER_MAIN_SITE_BASE_URL"))

	srv, err := landing.NewServer(ctx, landing.ServeOptions{
		Root:                 root,
		Account:              account,
		Bind:                 bind,
		StatePath:            statePath,
		Env:                  env,
		HotReload:            hotReload,
		LogLevel:             logLevel,
		TrustProxy:           trustProxy,
		PublicBaseOverride:   publicBaseOverride,
		MainSiteBaseOverride: mainSiteBaseOverride,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer srv.Close()

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(runCtx); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	_, _ = io.WriteString(stdout, "server stopped\n")
	return 0
}
