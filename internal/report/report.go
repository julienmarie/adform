package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"adform/internal/apply"
	"adform/internal/plan"
)

func WritePlanJSON(path string, pl plan.Plan) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create plan dir: %w", err)
	}
	b, err := json.MarshalIndent(pl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write plan file: %w", err)
	}
	return nil
}

func ReadPlanJSON(path string) (plan.Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("read plan file: %w", err)
	}
	var pl plan.Plan
	if err := json.Unmarshal(b, &pl); err != nil {
		return plan.Plan{}, fmt.Errorf("parse plan file: %w", err)
	}
	return pl, nil
}

func WriteApplyReports(root string, pl plan.Plan, result apply.Result) (string, string, error) {
	reportsDir := filepath.Join(root, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create reports dir: %w", err)
	}

	ts := time.Now().UTC().Format("20060102-1504")
	mdPath := filepath.Join(reportsDir, "apply-"+ts+".md")
	jsonPath := filepath.Join(reportsDir, "apply-"+ts+".json")

	md := strings.Builder{}
	md.WriteString("# adform apply report\n\n")
	md.WriteString("- account: `" + pl.Account + "`\n")
	md.WriteString("- generated_at: `" + pl.GeneratedAt + "`\n")
	md.WriteString("- applied_at: `" + result.AppliedAt + "`\n")
	md.WriteString("- dry_run: `")
	md.WriteString(fmt.Sprintf("%t", result.DryRun))
	md.WriteString("`\n")
	md.WriteString("\n## Summary\n\n")
	md.WriteString(fmt.Sprintf("- applied: %d\n- skipped: %d\n- failed: %d\n", result.AppliedOps, result.SkippedOps, result.FailedOps))
	md.WriteString("\n## Operations\n\n")
	for _, op := range result.Operations {
		md.WriteString(fmt.Sprintf("- `%s` %s: %s\n", op.ID, op.Action, op.Message))
	}

	if err := os.WriteFile(mdPath, []byte(md.String()), 0o644); err != nil {
		return "", "", fmt.Errorf("write apply markdown: %w", err)
	}
	payload := struct {
		Plan   plan.Plan    `json:"plan"`
		Result apply.Result `json:"result"`
	}{Plan: pl, Result: result}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("marshal apply json: %w", err)
	}
	if err := os.WriteFile(jsonPath, b, 0o644); err != nil {
		return "", "", fmt.Errorf("write apply json: %w", err)
	}
	return mdPath, jsonPath, nil
}
