package config

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"adform/internal/workspace"
	"gopkg.in/yaml.v3"
)

func Load(root, account string) (Bundle, error) {
	bundle := NewBundle(root, account)
	base := workspace.ResolveMetaDir(root, account)

	accountPath := filepath.Join(base, "account.yml")
	if err := decodeYAMLFile(accountPath, &bundle.AccountCfg); err != nil {
		if os.IsNotExist(err) {
			for _, cand := range workspace.MetaDirCandidates(root, account) {
				if cand == base {
					continue
				}
				candPath := filepath.Join(cand, "account.yml")
				if candErr := decodeYAMLFile(candPath, &bundle.AccountCfg); candErr == nil {
					base = cand
					goto accountLoaded
				}
			}
		}
		return bundle, fmt.Errorf("load account config: %w", err)
	}
accountLoaded:

	assetsPath := filepath.Join(base, "assets.yml")
	if _, err := os.Stat(assetsPath); err == nil {
		var manifest AssetManifest
		if err := decodeYAMLFile(assetsPath, &manifest); err != nil {
			return bundle, fmt.Errorf("load assets.yml: %w", err)
		}
		for _, asset := range manifest.Assets {
			bundle.Assets[asset.Key] = asset
		}
	}

	if err := loadDirYAML(filepath.Join(base, "audiences"), &bundle, "audience"); err != nil {
		return bundle, err
	}
	if err := loadDirYAML(filepath.Join(base, "catalogs"), &bundle, "catalog"); err != nil {
		return bundle, err
	}
	if err := loadDirYAML(filepath.Join(base, "creatives"), &bundle, "creative"); err != nil {
		return bundle, err
	}
	if err := loadCampaignTree(filepath.Join(base, "campaigns"), &bundle); err != nil {
		return bundle, err
	}

	return bundle, nil
}

func loadDirYAML(dir string, bundle *Bundle, kind string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s dir: %w", kind, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		switch kind {
		case "audience":
			var v Audience
			if err := decodeYAMLFile(path, &v); err != nil {
				return fmt.Errorf("load audience %s: %w", path, err)
			}
			bundle.Audiences[v.Key] = v
		case "catalog":
			var v Catalog
			if err := decodeYAMLFile(path, &v); err != nil {
				return fmt.Errorf("load catalog %s: %w", path, err)
			}
			bundle.Catalogs[v.Key] = v
		case "creative":
			var v Creative
			if err := decodeYAMLFile(path, &v); err != nil {
				return fmt.Errorf("load creative %s: %w", path, err)
			}
			bundle.Creatives[v.Key] = v
		}
	}
	return nil
}

func loadCampaignTree(dir string, bundle *Bundle) error {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat campaigns dir: %w", err)
	}

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(d.Name()) {
			return nil
		}

		switch d.Name() {
		case "campaign.yml":
			var c Campaign
			if err := decodeYAMLFile(path, &c); err != nil {
				return fmt.Errorf("load campaign %s: %w", path, err)
			}
			bundle.Campaigns[c.Key] = c
		case "adset.yml":
			var a AdSet
			if err := decodeYAMLFile(path, &a); err != nil {
				return fmt.Errorf("load adset %s: %w", path, err)
			}
			bundle.Adsets[a.Key] = a
		default:
			if strings.Contains(path, string(filepath.Separator)+"ads"+string(filepath.Separator)) {
				var ad Ad
				if err := decodeYAMLFile(path, &ad); err != nil {
					return fmt.Errorf("load ad %s: %w", path, err)
				}
				bundle.Ads[ad.Key] = ad
			}
		}
		return nil
	})
}

func decodeYAMLFile(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}
