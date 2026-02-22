package cli

import (
	"os"
	"path/filepath"
	"testing"

	"adform/internal/state"
)

func TestBaselineImportedStateHashesUpdatesLastAppliedHash(t *testing.T) {
	root := t.TempDir()
	account := "acc"

	if err := os.MkdirAll(filepath.Join(root, "meta", account, "campaigns", "camp_1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "meta", account, "account.yml"), []byte(
		"account_name: acc\n"+
			"meta:\n"+
			"  ad_account_id: \"act_123\"\n"+
			"  currency: \"USD\"\n"+
			"  timezone: \"America/New_York\"\n"+
			"  page_id: \"\"\n"+
			"  instagram_actor_id: \"\"\n"+
			"  pixel_key_default: \"default\"\n"+
			"budgets:\n"+
			"  unit: major\n"+
			"policies:\n"+
			"  no_delete: true\n"+
			"  allow_activate: false\n"+
			"  orphan:\n"+
			"    on_missing_in_config: pause\n"+
			"  budget:\n"+
			"    max_increase_ratio: 0.20\n"+
			"    max_decrease_ratio: 0.50\n"+
			"    max_daily_budget_major: 20000\n"+
			"naming:\n"+
			"  campaign_prefix: \"\"\n"+
			"  adset_prefix: \"\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "meta", account, "campaigns", "camp_1", "campaign.yml"), []byte(
		"key: camp_1\nname: Campaign 1\nobjective: SALES\nstatus: PAUSED\nspecial_ad_categories: []\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := state.Open(filepath.Join(root, ".adform", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.UpsertResource(state.ResourceRow{
		AccountName: account,
		Kind:        "campaign",
		LogicalKey:  "camp_1",
		MetaID:      "123",
	}); err != nil {
		t.Fatal(err)
	}

	warn := baselineImportedStateHashes(importOptions{
		commonOptions: commonOptions{
			Account: account,
			Root:    root,
		},
	}, st)
	if warn == "" {
		t.Fatalf("expected baseline message, got empty")
	}

	row, err := st.GetResource(account, "campaign", "camp_1")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.LastAppliedHash == "" {
		t.Fatalf("expected last_applied_hash to be set")
	}
	if row.LastSeenRemoteHash == "" {
		t.Fatalf("expected last_seen_remote_hash to be set")
	}
}
