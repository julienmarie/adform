package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMetaDirPrefersAccountCentric(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"

	accountMeta := AccountMetaDir(root, account)
	legacyMeta := LegacyMetaDir(root, account)
	if err := os.MkdirAll(accountMeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyMeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountMeta, "account.yml"), []byte("account_name: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMeta, "account.yml"), []byte("account_name: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveMetaDir(root, account)
	if got != accountMeta {
		t.Fatalf("expected account-centric meta dir %q, got %q", accountMeta, got)
	}
}

func TestResolveMetaDirFallsBackLegacy(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	legacyMeta := LegacyMetaDir(root, account)
	if err := os.MkdirAll(legacyMeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyMeta, "account.yml"), []byte("account_name: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveMetaDir(root, account)
	if got != legacyMeta {
		t.Fatalf("expected legacy meta dir %q, got %q", legacyMeta, got)
	}
}

func TestResolveLandingDirPrefersAccountCentric(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"

	accountLanding := AccountLandingDir(root, account)
	legacyLanding := LegacyLandingDir(root)
	if err := os.MkdirAll(accountLanding, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacyLanding, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountLanding, "site.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyLanding, "site.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveLandingDir(root, account)
	if got != accountLanding {
		t.Fatalf("expected account-centric landing dir %q, got %q", accountLanding, got)
	}
}

func TestResolveLandingDirFallsBackLegacy(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	legacyLanding := LegacyLandingDir(root)
	if err := os.MkdirAll(legacyLanding, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyLanding, "site.yml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ResolveLandingDir(root, account)
	if got != legacyLanding {
		t.Fatalf("expected legacy landing dir %q, got %q", legacyLanding, got)
	}
}
