package workspace

import (
	"os"
	"path/filepath"
)

func AccountRoot(root, account string) string {
	return filepath.Join(root, account)
}

func AccountMetaDir(root, account string) string {
	return filepath.Join(AccountRoot(root, account), "meta")
}

func LegacyMetaDir(root, account string) string {
	return filepath.Join(root, "meta", account)
}

func AccountsYAMLPath(root, account string) string {
	return filepath.Join(AccountRoot(root, account), "accounts.yml")
}

func AccountLandingDir(root, account string) string {
	return filepath.Join(AccountRoot(root, account), "landing")
}

func LegacyLandingDir(root string) string {
	return filepath.Join(root, "landing")
}

func MetaDirCandidates(root, account string) []string {
	return []string{
		AccountMetaDir(root, account),
		LegacyMetaDir(root, account),
	}
}

func LandingDirCandidates(root, account string) []string {
	return []string{
		AccountLandingDir(root, account),
		LegacyLandingDir(root),
	}
}

// ResolveMetaDir prefers account-centric layout and gracefully falls back to legacy layout.
// If nothing exists yet, it returns the account-centric path.
func ResolveMetaDir(root, account string) string {
	accountCentric := AccountMetaDir(root, account)
	legacy := LegacyMetaDir(root, account)

	if fileExists(filepath.Join(accountCentric, "account.yml")) {
		return accountCentric
	}
	if fileExists(filepath.Join(legacy, "account.yml")) {
		return legacy
	}
	if dirExists(accountCentric) {
		return accountCentric
	}
	if dirExists(legacy) {
		return legacy
	}
	return accountCentric
}

// ResolveLandingDir prefers account-centric layout and gracefully falls back to legacy layout.
// If nothing exists yet, it returns the account-centric path.
func ResolveLandingDir(root, account string) string {
	accountCentric := AccountLandingDir(root, account)
	legacy := LegacyLandingDir(root)

	if fileExists(filepath.Join(accountCentric, "site.yml")) {
		return accountCentric
	}
	if fileExists(filepath.Join(legacy, "site.yml")) {
		return legacy
	}
	if dirExists(accountCentric) {
		return accountCentric
	}
	if dirExists(legacy) {
		return legacy
	}
	return accountCentric
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
