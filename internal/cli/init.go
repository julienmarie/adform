package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"adform/internal/workspace"
)

type initOptions struct {
	commonOptions
	Sample bool
	CI     bool
	Force  bool
}

func runInit(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := initOptions{}
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.BoolVar(&opts.Sample, "sample", false, "Create sample config")
	fs.BoolVar(&opts.CI, "ci", false, "Create CI workflow templates")
	fs.BoolVar(&opts.Force, "force", false, "Overwrite generated files")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts.commonOptions)
	if opts.Account == "" {
		opts.Account = "default"
	}

	if err := initRepo(opts); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "initialized account %q at %s\n", opts.Account, workspace.AccountRoot(opts.Root, opts.Account))
	return 0
}

func initRepo(opts initOptions) error {
	metaRoot := workspace.AccountMetaDir(opts.Root, opts.Account)
	landingRoot := workspace.AccountLandingDir(opts.Root, opts.Account)
	dirs := []string{
		filepath.Join(opts.Root, ".adform"),
		filepath.Join(opts.Root, "reports"),
		filepath.Join(landingRoot, "pages"),
		filepath.Join(landingRoot, "assets", "images"),
		filepath.Join(landingRoot, "assets", "videos"),
		filepath.Join(landingRoot, "build"),
		filepath.Join(metaRoot, "assets", "images"),
		filepath.Join(metaRoot, "assets", "videos"),
		filepath.Join(metaRoot, "audiences"),
		filepath.Join(metaRoot, "catalogs"),
		filepath.Join(metaRoot, "creatives"),
		filepath.Join(metaRoot, "campaigns"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	for _, keep := range []string{
		filepath.Join(metaRoot, "assets", "images", ".keep"),
		filepath.Join(metaRoot, "assets", "videos", ".keep"),
	} {
		if err := writeIfAllowed(keep, []byte{}, opts.Force); err != nil {
			return err
		}
	}

	if err := ensureGitignoreEntries(filepath.Join(opts.Root, ".gitignore"), []string{".adform/", "reports/"}); err != nil {
		return err
	}
	if err := ensureGitignoreEntries(filepath.Join(opts.Root, ".gitignore"), []string{"*/landing/build/"}); err != nil {
		return err
	}

	agentsPath := filepath.Join(opts.Root, "AGENTS.md")
	if err := writeIfAllowed(agentsPath, []byte(defaultAgentsMD()), opts.Force); err != nil {
		return err
	}

	if opts.Sample {
		if err := writeSampleFiles(opts.Root, opts.Account, opts.Force); err != nil {
			return err
		}
	} else {
		accountYAML := filepath.Join(metaRoot, "account.yml")
		if err := writeIfAllowed(accountYAML, []byte(defaultAccountYAML(opts.Account)), opts.Force); err != nil {
			return err
		}
	}
	if err := writeIfAllowed(workspace.AccountsYAMLPath(opts.Root, opts.Account), []byte(defaultAccountsYAML(opts.Account)), opts.Force); err != nil {
		return err
	}
	if err := writeIfAllowed(filepath.Join(landingRoot, "site.yml"), []byte(defaultLandingSiteYAML()), opts.Force); err != nil {
		return err
	}
	if err := writeIfAllowed(filepath.Join(landingRoot, "theme.css"), []byte(defaultLandingThemeCSS()), opts.Force); err != nil {
		return err
	}
	if err := writeIfAllowed(filepath.Join(landingRoot, "pages", "sample.yml"), []byte(defaultLandingSamplePageYAML()), opts.Force); err != nil {
		return err
	}

	if opts.CI {
		if err := writeCI(opts.Root, opts.Account, opts.Force); err != nil {
			return err
		}
	}
	return nil
}

func writeSampleFiles(root, account string, force bool) error {
	base := workspace.AccountMetaDir(root, account)
	files := map[string]string{
		filepath.Join(base, "account.yml"):                                                                    defaultAccountYAML(account),
		filepath.Join(base, "assets.yml"):                                                                     sampleAssetsYAML(account),
		filepath.Join(base, "audiences", "stub_audience.yml"):                                                 sampleAudienceYAML(),
		filepath.Join(base, "catalogs", "stub_catalog.yml"):                                                   sampleCatalogYAML(),
		filepath.Join(base, "creatives", "sample_creative.yml"):                                               sampleCreativeYAML(),
		filepath.Join(base, "campaigns", "sample_campaign", "campaign.yml"):                                   sampleCampaignYAML(),
		filepath.Join(base, "campaigns", "sample_campaign", "adsets", "sample_adset", "adset.yml"):            sampleAdsetYAML(),
		filepath.Join(base, "campaigns", "sample_campaign", "adsets", "sample_adset", "ads", "sample_ad.yml"): sampleAdYAML(),
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create sample dir: %w", err)
		}
		if err := writeIfAllowed(path, []byte(content), force); err != nil {
			return err
		}
	}
	return nil
}

func writeCI(root, account string, force bool) error {
	prWorkflow := filepath.Join(root, ".github", "workflows", "adform-pr.yml")
	mainWorkflow := filepath.Join(root, ".github", "workflows", "adform-main.yml")
	prTemplate := filepath.Join(root, ".github", "pull_request_template.md")

	if err := os.MkdirAll(filepath.Dir(prWorkflow), 0o755); err != nil {
		return err
	}
	if err := writeIfAllowed(prWorkflow, []byte(ciPRWorkflow(account)), force); err != nil {
		return err
	}
	if err := writeIfAllowed(mainWorkflow, []byte(ciMainWorkflow(account)), force); err != nil {
		return err
	}
	if err := writeIfAllowed(prTemplate, []byte(ciPRTemplate()), force); err != nil {
		return err
	}
	return nil
}

func writeIfAllowed(path string, content []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func ensureGitignoreEntries(path string, entries []string) error {
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	}
	lines := strings.Split(existing, "\n")
	set := map[string]bool{}
	for _, line := range lines {
		set[strings.TrimSpace(line)] = true
	}
	changed := false
	for _, entry := range entries {
		if !set[entry] {
			if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
				existing += "\n"
			}
			existing += entry + "\n"
			changed = true
		}
	}
	if changed || existing == "" {
		if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	}
	return nil
}
