package apply

import (
	"path/filepath"
	"testing"

	"adform/internal/plan"
	"adform/internal/state"
)

func TestExecuteMaxOpsIgnoresNoopAndDrift(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ops := make([]plan.Operation, 0, 300)
	for i := 0; i < 200; i++ {
		ops = append(ops, plan.Operation{ID: "noop", Action: plan.OpNoop})
	}
	for i := 0; i < 100; i++ {
		ops = append(ops, plan.Operation{ID: "drift", Action: plan.OpDriftOnly})
	}

	res, err := Execute("acc", st, plan.Plan{Operations: ops}, Options{DryRun: true, MaxOps: 1})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.SkippedOps != 300 {
		t.Fatalf("expected 300 skipped ops, got %d", res.SkippedOps)
	}
}

func TestExecuteMaxOpsCountsActionable(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ops := []plan.Operation{
		{ID: "noop-1", Action: plan.OpNoop},
		{ID: "update-1", Action: plan.OpUpdate},
		{ID: "update-2", Action: plan.OpUpdate},
	}

	_, err = Execute("acc", st, plan.Plan{Operations: ops}, Options{DryRun: true, MaxOps: 1})
	if err == nil {
		t.Fatal("expected max-ops error, got nil")
	}
}
