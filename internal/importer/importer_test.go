package importer

import (
	"os"
	"path/filepath"
	"testing"

	"adform/internal/state"
)

func TestImportFromStateDryRunAndWrite(t *testing.T) {
	root := t.TempDir()
	account := "acc"
	st, err := state.Open(filepath.Join(root, ".adform", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seed := []state.ResourceRow{
		{AccountName: account, Kind: "campaign", LogicalKey: "camp1", MetaID: "c1", LastAppliedHash: "h1"},
		{AccountName: account, Kind: "adset", LogicalKey: "adset1", MetaID: "a1", LastAppliedHash: "h2"},
		{AccountName: account, Kind: "creative", LogicalKey: "cre1", MetaID: "cr1", LastAppliedHash: "h3"},
		{AccountName: account, Kind: "ad", LogicalKey: "ad1", MetaID: "ad1", LastAppliedHash: "h4"},
	}
	for _, row := range seed {
		if err := st.UpsertResource(row); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(root, "meta", account+"_imported")
	dry, err := ImportFromState(st, Options{Root: root, Account: account, Out: out, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.FilesPlanned == 0 || dry.FilesWritten != 0 {
		t.Fatalf("unexpected dry result: %+v", dry)
	}

	real, err := ImportFromState(st, Options{Root: root, Account: account, Out: out, DryRun: false, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if real.FilesWritten == 0 {
		t.Fatalf("expected files written")
	}
	if _, err := os.Stat(filepath.Join(out, "account.yml")); err != nil {
		t.Fatalf("expected account.yml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "campaigns", "camp1", "campaign.yml")); err != nil {
		t.Fatalf("expected campaign scaffold: %v", err)
	}
}
