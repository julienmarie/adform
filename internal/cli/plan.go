package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"adform/internal/config"
	"adform/internal/plan"
	"adform/internal/report"
	"adform/internal/state"
)

type planOptions struct {
	commonOptions
	Only         string
	Refresh      bool
	IncludeDrift bool
	Out          string
}

func runPlan(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := planOptions{}
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Only, "only", "", "Filter kind(s), comma-separated")
	fs.BoolVar(&opts.Refresh, "refresh", true, "Refresh managed resources from Meta before diff output")
	fs.BoolVar(&opts.IncludeDrift, "include-drift", true, "Include drift-only operations from remote refresh")
	fs.StringVar(&opts.Out, "out", "", "Write plan JSON to path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts.commonOptions)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	validation := config.Validate(bundle)
	if !validation.OK() {
		fmt.Fprintf(stderr, "validation failed: %d error(s)\n", len(validation.Errors))
		for _, e := range validation.Errors {
			fmt.Fprintf(stderr, "- %s: %s\n", e.Field, e.Message)
		}
		return 1
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	pl, err := plan.Build(opts.Account, bundle, st)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.Refresh {
		client, _, err := metaClientForAccount(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		pl, err = plan.RefreshFromRemote(opts.Account, bundle, st, pl, client, opts.IncludeDrift)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}
	if opts.Only != "" {
		pl.Operations = filterByKind(pl.Operations, opts.Only)
		pl.Summary = plan.RecalculateSummary(pl.Operations)
	}

	if opts.Out != "" {
		if err := report.WritePlanJSON(opts.Out, pl); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	if opts.JSON {
		b, _ := json.MarshalIndent(pl, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "Plan for account %q\n", pl.Account)
		fmt.Fprintf(stdout, "- create: %d\n", pl.Summary.Create)
		fmt.Fprintf(stdout, "- update: %d\n", pl.Summary.Update)
		fmt.Fprintf(stdout, "- replace: %d\n", pl.Summary.Replace)
		fmt.Fprintf(stdout, "- pause-orphan: %d\n", pl.Summary.PauseOrphan)
		fmt.Fprintf(stdout, "- drift-only: %d\n", pl.Summary.DriftOnly)
		fmt.Fprintf(stdout, "- noop: %d\n", pl.Summary.Noop)
		printed := 0
		for _, op := range pl.Operations {
			if op.Action == plan.OpNoop && !opts.Verbose {
				continue
			}
			fmt.Fprintf(stdout, "- %s %s (%s)\n", op.Action, op.ID, op.Reason)
			printed++
		}
		if printed == 0 {
			fmt.Fprintln(stdout, "- no changes")
		}
	}

	return 0
}

func filterByKind(ops []plan.Operation, only string) []plan.Operation {
	parts := strings.Split(only, ",")
	allowed := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			allowed[p] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return ops
	}
	filtered := make([]plan.Operation, 0, len(ops))
	for _, op := range ops {
		if _, ok := allowed[op.Kind]; ok {
			filtered = append(filtered, op)
			continue
		}
		if op.Kind == "asset_image" || op.Kind == "asset_video" {
			if _, ok := allowed["asset"]; ok {
				filtered = append(filtered, op)
			}
		}
	}
	return filtered
}
