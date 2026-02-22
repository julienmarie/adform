package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"adform/internal/config"
	"adform/internal/landing"
	"adform/internal/workspace"
)

func runValidate(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := struct {
		commonOptions
		Rules          bool
		StrictWarnings bool
	}{}
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.BoolVar(&opts.Rules, "rules", false, "Print full validation rule catalog")
	fs.BoolVar(&opts.StrictWarnings, "strict-warnings", false, "Return non-zero when warnings are present")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	finalizeCommon(&opts.commonOptions)
	rules := config.ValidationRules()
	if opts.Rules && opts.Account == "" {
		if opts.JSON {
			payload := map[string]any{
				"rules": rules,
			}
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Fprintln(stdout, string(b))
		} else {
			printValidationRules(stdout, rules)
		}
		return 0
	}
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	result := config.Validate(bundle)
	landingDir := workspace.ResolveLandingDir(opts.Root, opts.Account)
	if fileExists(filepath.Join(landingDir, "site.yml")) {
		loaded, err := landing.Load(opts.Root, opts.Account, landing.ServeOptions{
			Root:    opts.Root,
			Account: opts.Account,
			Env:     "dev",
		})
		if err != nil {
			result.Errors = append(result.Errors, config.ValidationError{
				Rule:    "landing.validation",
				Field:   "landing",
				Message: err.Error(),
			})
		} else {
			for _, w := range loaded.ValidationWarn {
				result.Warnings = append(result.Warnings, config.ValidationWarning{
					Rule:    "landing.warning",
					Field:   "landing",
					Message: w,
				})
			}
		}
	}

	if opts.JSON {
		payload := map[string]any{
			"ok":       result.OK(),
			"errors":   result.Errors,
			"warnings": result.Warnings,
		}
		if opts.Rules {
			payload["rules"] = rules
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		if opts.Rules {
			printValidationRules(stdout, rules)
			fmt.Fprintln(stdout)
		}

		if result.OK() && !result.HasWarnings() {
			fmt.Fprintln(stdout, "validation: ok (0 errors, 0 warnings)")
		} else {
			fmt.Fprintf(stdout, "validation: %d error(s), %d warning(s)\n", len(result.Errors), len(result.Warnings))
			for _, e := range sortValidationErrors(result.Errors) {
				fmt.Fprintf(stdout, "- [error] %s (%s): %s\n", e.Field, e.Rule, e.Message)
			}
			for _, w := range sortValidationWarnings(result.Warnings) {
				fmt.Fprintf(stdout, "- [warn ] %s (%s): %s\n", w.Field, w.Rule, w.Message)
			}
		}
	}

	if result.OK() && !(opts.StrictWarnings && result.HasWarnings()) {
		return 0
	}
	return 1
}

func printValidationRules(w io.Writer, rules []config.ValidationRule) {
	fmt.Fprintf(w, "validation rules: %d\n", len(rules))
	for _, r := range rules {
		fmt.Fprintf(w, "- [%s] %-7s %-10s %s\n", r.ID, r.Severity, r.Scope, r.Description)
	}
}

func sortValidationErrors(in []config.ValidationError) []config.ValidationError {
	out := append([]config.ValidationError{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func sortValidationWarnings(in []config.ValidationWarning) []config.ValidationWarning {
	out := append([]config.ValidationWarning{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Message < out[j].Message
	})
	return out
}
