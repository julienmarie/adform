package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"adform/internal/assets"
	"adform/internal/config"
	"adform/internal/state"
)

func runAssets(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: adform assets <upload|list|verify|gc> [flags]")
		return 1
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "upload":
		return runAssetsUpload(subArgs, stdout, stderr)
	case "list":
		return runAssetsList(subArgs, stdout, stderr)
	case "verify":
		return runAssetsVerify(subArgs, stdout, stderr)
	case "gc":
		return runAssetsGC(subArgs, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown assets subcommand: %s\n", sub)
		return 1
	}
}

func runAssetsUpload(args []string, stdout, stderr io.Writer) int {
	opts := commonOptions{}
	var typ string
	var pathGlob string
	fs := flag.NewFlagSet("assets upload", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts)
	fs.StringVar(&typ, "type", "", "image|video")
	fs.StringVar(&pathGlob, "path", "", "Glob path filter")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	var uploader assets.Uploader
	client, _, err := metaClientForAccount(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	uploader = client

	result, err := assets.Upload(st, assets.UploadOptions{
		Root:        opts.Root,
		Account:     opts.Account,
		AdAccountID: bundle.AccountCfg.Meta.AdAccountID,
		Type:        strings.TrimSpace(typ),
		Path:        strings.TrimSpace(pathGlob),
		Uploader:    uploader,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if opts.JSON {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "assets upload completed\n")
		fmt.Fprintf(stdout, "- uploaded: %d\n", result.Uploaded)
		fmt.Fprintf(stdout, "- deduped: %d\n", result.Deduped)
		fmt.Fprintf(stdout, "- skipped: %d\n", result.Skipped)
		fmt.Fprintf(stdout, "- errors: %d\n", result.Errors)
		for _, item := range result.Items {
			fmt.Fprintf(stdout, "- %s (%s): %s\n", item.Key, item.Kind, item.Message)
		}
	}
	if result.Errors > 0 {
		return 1
	}
	return 0
}

func runAssetsList(args []string, stdout, stderr io.Writer) int {
	opts := commonOptions{}
	fs := flag.NewFlagSet("assets list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	items, err := assets.List(opts.Root, opts.Account, st)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.JSON {
		b, _ := json.MarshalIndent(items, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		for _, item := range items {
			fmt.Fprintf(stdout, "- %s [%s] path=%s sha=%s meta=%s origin=%s\n", item.Key, item.Kind, item.Path, item.SHA256, item.MetaID, item.Message)
		}
	}
	return 0
}

func runAssetsVerify(args []string, stdout, stderr io.Writer) int {
	opts := commonOptions{}
	fs := flag.NewFlagSet("assets verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	result, err := assets.Verify(opts.Root, opts.Account, st)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.JSON {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "assets verify\n- checked: %d\n- failed: %d\n", result.Checked, result.Failed)
		for _, item := range result.Items {
			fmt.Fprintf(stdout, "- %s: %s\n", item.Key, item.Message)
		}
	}
	if result.Failed > 0 {
		return 1
	}
	return 0
}

func runAssetsGC(args []string, stdout, stderr io.Writer) int {
	opts := commonOptions{}
	dryRun := true
	fs := flag.NewFlagSet("assets gc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts)
	fs.BoolVar(&dryRun, "dry-run", true, "Only show what would be deleted")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	result, err := assets.GC(opts.Root, opts.Account, st, dryRun)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.JSON {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "assets gc (dry-run=%t)\n- candidates: %d\n- deleted: %d\n", result.DryRun, len(result.Items), result.Deleted)
		for _, item := range result.Items {
			fmt.Fprintf(stdout, "- %s:%s\n", item.Kind, item.Key)
		}
	}
	return 0
}
