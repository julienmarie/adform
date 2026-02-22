package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMetaTokenFromAccountsGetEnv(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	if err := os.MkdirAll(filepath.Join(root, account), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, account, "accounts.yml"), []byte(
		"property: btd_main\n"+
			"platforms:\n"+
			"  meta:\n"+
			"    config_dir: meta\n"+
			"    ad_account_id: \"act_1\"\n"+
			"    meta_api_key: \"get_env(TEST_META_TOKEN)\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_META_TOKEN", "token_123")

	token, source, err := ResolveMetaToken(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token_123" {
		t.Fatalf("expected token_123, got %q", token)
	}
	if source != "env:TEST_META_TOKEN" {
		t.Fatalf("unexpected source %q", source)
	}
}

func TestResolveMetaTokenFallbackEnv(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	t.Setenv("META_ACCESS_TOKEN", "fallback_tok")
	token, source, err := ResolveMetaToken(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fallback_tok" || source != "env:META_ACCESS_TOKEN" {
		t.Fatalf("unexpected token/source: %q %q", token, source)
	}
}

func TestResolvePostHogTokenFromAccountsGetEnv(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	if err := os.MkdirAll(filepath.Join(root, account), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, account, "accounts.yml"), []byte(
		"property: btd_main\n"+
			"platforms:\n"+
			"  posthog:\n"+
			"    host: \"https://app.posthog.com\"\n"+
			"    project_id: \"12345\"\n"+
			"    api_key: \"get_env(TEST_POSTHOG_TOKEN)\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_POSTHOG_TOKEN", "ph_tok_1")
	token, source, err := ResolvePostHogToken(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ph_tok_1" || source != "env:TEST_POSTHOG_TOKEN" {
		t.Fatalf("unexpected token/source: %q %q", token, source)
	}
}

func TestResolvePhotoRoomTokenFromAccountsGetEnv(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	if err := os.MkdirAll(filepath.Join(root, account), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, account, "accounts.yml"), []byte(
		"property: btd_main\n"+
			"platforms:\n"+
			"  photoroom:\n"+
			"    api_key: \"get_env(TEST_PHOTOROOM_TOKEN)\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_PHOTOROOM_TOKEN", "phr_tok_1")
	token, source, err := ResolvePhotoRoomToken(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "phr_tok_1" || source != "env:TEST_PHOTOROOM_TOKEN" {
		t.Fatalf("unexpected token/source: %q %q", token, source)
	}
}

func TestResolvePhotoRoomTokenFallbackEnv(t *testing.T) {
	root := t.TempDir()
	account := ""
	t.Setenv("PHOTOROOM_API_KEY", "phr_fallback")
	token, source, err := ResolvePhotoRoomToken(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "phr_fallback" || source != "env:PHOTOROOM_API_KEY" {
		t.Fatalf("unexpected token/source: %q %q", token, source)
	}
}

func TestResolveGSCredentialsJSONFromFile(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	accountDir := filepath.Join(root, account)
	if err := os.MkdirAll(accountDir, 0o755); err != nil {
		t.Fatal(err)
	}
	creds := `{"type":"service_account","project_id":"demo"}`
	if err := os.WriteFile(filepath.Join(accountDir, "gsc-creds.json"), []byte(creds), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(accountDir, "accounts.yml"), []byte(
		"property: btd_main\n"+
			"platforms:\n"+
			"  google_search_console:\n"+
			"    site_url: \"sc-domain:example.com\"\n"+
			"    credentials_file: \"gsc-creds.json\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	got, source, err := ResolveGSCredentialsJSON(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != creds {
		t.Fatalf("unexpected credentials payload: %q", string(got))
	}
	if source == "" || source[:5] != "file:" {
		t.Fatalf("unexpected source: %q", source)
	}
}

func TestResolveGSCredentialsJSONFromGetEnv(t *testing.T) {
	root := t.TempDir()
	account := "btd_main"
	if err := os.MkdirAll(filepath.Join(root, account), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, account, "accounts.yml"), []byte(
		"property: btd_main\n"+
			"platforms:\n"+
			"  google_search_console:\n"+
			"    credentials_json: \"get_env(TEST_GSC_JSON)\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_GSC_JSON", `{"type":"authorized_user"}`)

	got, source, err := ResolveGSCredentialsJSON(root, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != `{"type":"authorized_user"}` {
		t.Fatalf("unexpected credentials payload: %q", string(got))
	}
	if source != "env:TEST_GSC_JSON" {
		t.Fatalf("unexpected source: %q", source)
	}
}
