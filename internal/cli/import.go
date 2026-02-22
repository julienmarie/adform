package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"adform/internal/config"
	"adform/internal/importer"
	"adform/internal/plan"
	"adform/internal/state"
	"adform/internal/workspace"
)

type importOptions struct {
	commonOptions
	Since          string
	CampaignFilter string
	StatusFilter   string
	DryRun         bool
	PreserveStatus bool
	Force          bool
	Out            string
	Full           bool
	FromState      bool
}

func runImport(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := importOptions{}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Since, "since", "", "Filter window hint (currently informational)")
	fs.StringVar(&opts.CampaignFilter, "campaign", "", "Campaign name contains filter for remote import")
	fs.StringVar(&opts.StatusFilter, "status", "", "Status filter for remote import (e.g. ACTIVE, PAUSED)")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Plan import scaffold without writing files")
	fs.BoolVar(&opts.PreserveStatus, "preserve-status", false, "Preserve remote status if available")
	fs.BoolVar(&opts.Force, "force", false, "Overwrite existing files")
	fs.StringVar(&opts.Out, "out", "", "Output directory (default <account>/meta; legacy meta/<account> supported)")
	fs.BoolVar(&opts.Full, "full", false, "Best-effort deeper extraction (currently informational)")
	fs.BoolVar(&opts.FromState, "from-state", false, "Skip Meta API sync and scaffold from local SQLite state only")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	finalizeCommon(&opts.commonOptions)
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

	remoteSynced := false
	remoteSummary := importer.RemoteSyncResult{}
	var remoteClient importer.RemoteDetailClient
	var prefetchedDetails map[string]map[string]map[string]any
	var accountCfg *config.AccountConfig
	var progress importer.ProgressFunc
	var renderer *importProgressRenderer
	if !opts.JSON {
		renderer = newImportProgressRenderer(stdout)
		progress = renderer.Handle
		defer renderer.Close()
	}
	if opts.FromState {
		remoteSynced = false
	} else {
		bundle, err := config.Load(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: unable to load account config for remote import: %v\n", err)
			return 1
		}
		if bundle.AccountCfg.Meta.AdAccountID == "" {
			fmt.Fprintln(stderr, "error: account.meta.ad_account_id is required for remote import")
			return 1
		}
		if strings.EqualFold(strings.TrimSpace(bundle.AccountCfg.Meta.AdAccountID), "act_0000000000") {
			fmt.Fprintln(stderr, "error: account.meta.ad_account_id is still placeholder (act_0000000000); set your real ad account id in <account>/meta/account.yml")
			return 1
		}
		accountCfg = &bundle.AccountCfg
		client, _, err := metaClientForAccount(opts.Root, opts.Account)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		remoteClient = client
		remoteSummary, err = importer.SyncStateFromRemote(st, opts.Account, bundle.AccountCfg.Meta.AdAccountID, client, importer.RemoteSyncOptions{
			CampaignFilter: opts.CampaignFilter,
			StatusFilter:   opts.StatusFilter,
			Progress:       progress,
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		remoteSynced = true
		prefetchedDetails = remoteSummary.Details
		remoteClient = nil
	}

	res, err := importer.ImportFromState(st, importer.Options{
		Root:               opts.Root,
		Account:            opts.Account,
		Out:                opts.Out,
		DryRun:             opts.DryRun,
		PreserveStatus:     opts.PreserveStatus,
		Force:              opts.Force,
		CreatePlaceholders: opts.FromState,
		AccountConfig:      accountCfg,
		Remote:             remoteClient,
		PrefetchedDetails:  prefetchedDetails,
		Progress:           progress,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if !opts.DryRun {
		if warn := baselineImportedStateHashes(opts, st); warn != "" {
			res.Warnings = append(res.Warnings, warn)
		}
		if err := st.SetAccountMeta(opts.Account, "import_completed_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
			res.Warnings = append(res.Warnings, "import completion marker skipped: "+err.Error())
		}
	}

	if opts.JSON {
		payload := map[string]any{"result": res}
		if remoteSynced {
			payload["remote_sync"] = remoteSummary
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		if renderer != nil {
			renderer.Close()
		}
		fmt.Fprintf(stdout, "import scaffold complete (dry-run=%t)\n", res.DryRun)
		fmt.Fprintf(stdout, "- out: %s\n", res.OutPath)
		fmt.Fprintf(stdout, "- resources seen: %d\n", res.ResourcesSeen)
		fmt.Fprintf(stdout, "- files planned: %d\n", res.FilesPlanned)
		fmt.Fprintf(stdout, "- files written: %d\n", res.FilesWritten)
		if remoteSynced {
			fmt.Fprintf(
				stdout,
				"- remote synced: campaigns=%d adsets=%d ads=%d creatives=%d audiences=%d asset_images=%d asset_videos=%d catalogs=%d\n",
				remoteSummary.Campaigns,
				remoteSummary.Adsets,
				remoteSummary.Ads,
				remoteSummary.Creatives,
				remoteSummary.Audiences,
				remoteSummary.AssetImages,
				remoteSummary.AssetVideos,
				remoteSummary.Catalogs,
			)
			for _, w := range remoteSummary.Warnings {
				fmt.Fprintf(stdout, "- warning: %s\n", w)
			}
		}
		for _, w := range res.Warnings {
			fmt.Fprintf(stdout, "- warning: %s\n", w)
		}
	}
	return 0
}

func baselineImportedStateHashes(opts importOptions, st *state.Store) string {
	if st == nil {
		return "state hash baseline skipped: state unavailable"
	}

	defaultOut := workspace.ResolveMetaDir(opts.Root, opts.Account)
	if opts.Out != "" {
		resolved := opts.Out
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(opts.Root, resolved)
		}
		if filepath.Clean(resolved) != filepath.Clean(defaultOut) {
			return "state hash baseline skipped: custom --out path not supported"
		}
	}

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		return "state hash baseline skipped: load config failed: " + err.Error()
	}
	pl, err := plan.Build(opts.Account, bundle, st)
	if err != nil {
		return "state hash baseline skipped: build plan failed: " + err.Error()
	}

	applied := 0
	for _, op := range pl.Operations {
		if op.Action == plan.OpPauseOrphan {
			continue
		}
		row, err := st.GetResource(opts.Account, op.Kind, op.Key)
		if err != nil {
			return "state hash baseline skipped: read state failed: " + err.Error()
		}
		if row == nil {
			continue
		}
		if row.LastAppliedHash == op.Hash && row.LastSeenRemoteHash == op.Hash {
			continue
		}
		row.LastAppliedHash = op.Hash
		row.LastSeenRemoteHash = op.Hash
		if err := st.UpsertResource(*row); err != nil {
			return "state hash baseline skipped: write state failed: " + err.Error()
		}
		applied++
	}
	return fmt.Sprintf("state hash baseline updated: %d resources", applied)
}
