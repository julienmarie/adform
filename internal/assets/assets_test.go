package assets

import (
	"os"
	"path/filepath"
	"testing"

	"adform/internal/state"
)

func TestUploadAndVerify(t *testing.T) {
	root := t.TempDir()
	account := "acc"
	manifestPath := filepath.Join(root, "meta", account, "assets.yml")
	assetPath := filepath.Join(root, "meta", account, "assets", "images", "hero.txt")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "assets:\n" +
		"  - key: hero\n" +
		"    type: image\n" +
		"    file: \"meta/acc/assets/images/hero.txt\"\n" +
		"    sha256: null\n" +
		"    meta:\n" +
		"      image_hash: null\n" +
		"      origin: local\n"
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := state.Open(filepath.Join(root, ".adform", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := Upload(st, UploadOptions{Root: root, Account: account, Type: "image"})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if res.Uploaded != 1 {
		t.Fatalf("expected 1 uploaded, got %d", res.Uploaded)
	}

	res2, err := Upload(st, UploadOptions{Root: root, Account: account, Type: "image"})
	if err != nil {
		t.Fatalf("upload2: %v", err)
	}
	if res2.Deduped != 1 {
		t.Fatalf("expected 1 deduped, got %d", res2.Deduped)
	}

	verify, err := Verify(root, account, st)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verify.Failed != 0 {
		t.Fatalf("expected 0 verify failures, got %d", verify.Failed)
	}
}

func TestGCDryRunAndDelete(t *testing.T) {
	root := t.TempDir()
	account := "acc"
	manifestPath := filepath.Join(root, "meta", account, "assets.yml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("assets: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := state.Open(filepath.Join(root, ".adform", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "asset_image", LogicalKey: "orphan", MetaID: "m", LastAppliedHash: "h"}); err != nil {
		t.Fatal(err)
	}

	dry, err := GC(root, account, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Items) != 1 || dry.Deleted != 0 {
		t.Fatalf("unexpected dry gc result: %+v", dry)
	}

	real, err := GC(root, account, st, false)
	if err != nil {
		t.Fatal(err)
	}
	if real.Deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", real.Deleted)
	}
	row, err := st.GetResource(account, "asset_image", "orphan")
	if err != nil {
		t.Fatal(err)
	}
	if row != nil {
		t.Fatalf("expected row deleted")
	}
}
