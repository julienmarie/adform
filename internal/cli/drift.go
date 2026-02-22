package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"adform/internal/config"
	"adform/internal/plan"
	"adform/internal/state"
)

type driftItem struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Action   string `json:"action"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

type driftResult struct {
	Account string      `json:"account"`
	Count   int         `json:"count"`
	Items   []driftItem `json:"items"`
	Note    string      `json:"note"`
}

func runDrift(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := commonOptions{}
	refresh := true
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts)
	fs.BoolVar(&refresh, "refresh", true, "Refresh remote objects")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts)
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
	if refresh {
		client, _, err := metaClientForAccount(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		pl, err = plan.RefreshFromRemote(opts.Account, bundle, st, pl, client, true)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	note := "local drift only (state vs desired)"
	if refresh {
		note = "includes remote refresh checks for managed resources"
	}
	result := driftResult{Account: opts.Account, Note: note}
	for _, op := range pl.Operations {
		if op.Action == plan.OpNoop {
			continue
		}
		result.Items = append(result.Items, driftItem{
			ID:       op.ID,
			Kind:     op.Kind,
			Action:   string(op.Action),
			Severity: driftSeverity(op.Action),
			Reason:   op.Reason,
		})
	}
	result.Count = len(result.Items)

	if opts.JSON {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprintf(stdout, "drift for account %q\n", result.Account)
		fmt.Fprintf(stdout, "- count: %d\n", result.Count)
		for _, item := range result.Items {
			fmt.Fprintf(stdout, "- %s [%s] severity=%s reason=%s\n", item.ID, item.Action, item.Severity, item.Reason)
		}
		fmt.Fprintf(stdout, "note: %s\n", result.Note)
	}
	return 0
}

func driftSeverity(action plan.OperationType) string {
	switch action {
	case plan.OpReplace, plan.OpPauseOrphan:
		return "high"
	case plan.OpUpdate, plan.OpCreate:
		return "medium"
	default:
		return "low"
	}
}
