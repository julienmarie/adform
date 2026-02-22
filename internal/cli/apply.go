package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"adform/internal/apply"
	"adform/internal/config"
	"adform/internal/meta"
	"adform/internal/plan"
	"adform/internal/report"
	"adform/internal/state"
)

type applyOptions struct {
	commonOptions
	Planfile       string
	MaxOps         int
	Activate       bool
	BudgetCapDelta float64
	DryRun         bool
	Refresh        bool
	Delete         bool
}

func runApply(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := applyOptions{}
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Planfile, "planfile", "", "Path to previously generated plan JSON")
	fs.IntVar(&opts.MaxOps, "max-ops", 200, "Maximum operations in a single apply")
	fs.BoolVar(&opts.Activate, "activate", false, "Allow ACTIVE resources in apply")
	fs.Float64Var(&opts.BudgetCapDelta, "budget-cap-delta", 0.2, "Budget cap override (reserved)")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Evaluate apply without changing state")
	fs.BoolVar(&opts.Refresh, "refresh", true, "Refresh managed resources from Meta before apply (when no --planfile)")
	fs.BoolVar(&opts.Delete, "delete", false, "Archive orphan remote resources missing from config (dangerous)")
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

	if !bundle.AccountCfg.Policies.AllowActivate && opts.Activate {
		fmt.Fprintln(stderr, "policy blocked: --activate provided but policies.allow_activate=false")
		return 2
	}
	if bundle.AccountCfg.Policies.NoDelete && opts.Delete {
		fmt.Fprintln(stderr, "policy blocked: --delete provided but policies.no_delete=true")
		return 2
	}
	if !opts.Activate && anyActive(bundle) {
		fmt.Fprintln(stderr, "policy blocked: ACTIVE resources present; rerun with --activate")
		return 2
	}

	st, err := state.Open(opts.StatePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	defer st.Close()

	var pl plan.Plan
	if opts.Planfile != "" {
		if opts.Verbose {
			fmt.Fprintf(stderr, "[apply] loading planfile %s\n", opts.Planfile)
		}
		pl, err = report.ReadPlanJSON(opts.Planfile)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	} else {
		if opts.Verbose {
			fmt.Fprintln(stderr, "[apply] building plan from local config/state")
		}
		pl, err = plan.Build(opts.Account, bundle, st)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	var metaClient *meta.Client
	if !opts.DryRun {
		metaClient, _, err = metaClientForAccount(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if opts.Planfile == "" && opts.Refresh {
			if opts.Verbose {
				fmt.Fprintln(stderr, "[apply] refreshing plan from remote Meta state")
			}
			pl, err = plan.RefreshFromRemote(opts.Account, bundle, st, pl, metaClient, true)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}
		} else if opts.Verbose && opts.Planfile == "" && !opts.Refresh {
			fmt.Fprintln(stderr, "[apply] skipping remote refresh (--refresh=false)")
		}
	}

	if opts.Verbose {
		fmt.Fprintf(stderr, "[apply] executing %d operations (dry-run=%t)\n", len(pl.Operations), opts.DryRun)
	}
	res, err := apply.Execute(opts.Account, st, pl, apply.Options{
		MaxOps: opts.MaxOps,
		DryRun: opts.DryRun,
		Delete: opts.Delete,
		Root:   opts.Root,
		Bundle: bundle,
		Meta:   metaClient,
	})
	if err != nil {
		if err == state.ErrLocked || strings.Contains(err.Error(), "locked") {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	mdPath, jsonPath, err := report.WriteApplyReports(opts.Root, pl, res)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	planJSON, _ := json.Marshal(pl)
	resultJSON, _ := json.Marshal(res)
	_ = st.InsertApplyLog(opts.Account, "adform", string(planJSON), string(resultJSON))

	if opts.JSON {
		payload := map[string]any{"plan": pl, "result": res, "reports": map[string]string{"markdown": mdPath, "json": jsonPath}}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "apply completed (dry-run=%t)\n", opts.DryRun)
		fmt.Fprintf(stdout, "- applied: %d\n", res.AppliedOps)
		fmt.Fprintf(stdout, "- skipped: %d\n", res.SkippedOps)
		fmt.Fprintf(stdout, "- failed: %d\n", res.FailedOps)
		fmt.Fprintf(stdout, "reports:\n- %s\n- %s\n", mdPath, jsonPath)
	}

	if res.Success {
		return 0
	}
	return 1
}

func anyActive(bundle config.Bundle) bool {
	for _, c := range bundle.Campaigns {
		if strings.EqualFold(c.Status, "ACTIVE") {
			return true
		}
	}
	for _, a := range bundle.Adsets {
		if strings.EqualFold(a.Status, "ACTIVE") {
			return true
		}
	}
	for _, a := range bundle.Ads {
		if strings.EqualFold(a.Status, "ACTIVE") {
			return true
		}
	}
	return false
}
