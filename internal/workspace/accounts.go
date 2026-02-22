package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type AccountsConfig struct {
	Property  string `yaml:"property"`
	Platforms struct {
		Meta struct {
			ConfigDir   string `yaml:"config_dir"`
			AdAccountID string `yaml:"ad_account_id"`
			MetaAPIKey  string `yaml:"meta_api_key"`
		} `yaml:"meta"`
		PhotoRoom struct {
			APIKey string `yaml:"api_key"`
		} `yaml:"photoroom"`
		PostHog struct {
			Host      string `yaml:"host"`
			ProjectID string `yaml:"project_id"`
			APIKey    string `yaml:"api_key"`
			Events    struct {
				OrderCompleted string `yaml:"order_completed"`
				ProductAdded   string `yaml:"product_added"`
				ProductViewed  string `yaml:"product_viewed"`
			} `yaml:"events"`
			Queries struct {
				ProductSales  string `yaml:"product_sales"`
				ProductAdded  string `yaml:"product_added"`
				ProductViewed string `yaml:"product_viewed"`
			} `yaml:"queries"`
		} `yaml:"posthog"`
		GoogleSearchConsole struct {
			SiteURL         string `yaml:"site_url"`
			CredentialsFile string `yaml:"credentials_file"`
			CredentialsJSON string `yaml:"credentials_json"`
		} `yaml:"google_search_console"`
	} `yaml:"platforms"`
}

var getEnvExprRe = regexp.MustCompile(`^get_env\(([A-Za-z_][A-Za-z0-9_]*)\)$`)

func LoadAccountsConfig(root, account string) (*AccountsConfig, error) {
	path := AccountsYAMLPath(root, account)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read accounts.yml: %w", err)
	}
	var cfg AccountsConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse accounts.yml: %w", err)
	}
	return &cfg, nil
}

func ResolveMetaToken(root, account string) (string, string, error) {
	cfg, err := LoadAccountsConfig(root, account)
	if err != nil {
		return "", "", err
	}
	if cfg != nil {
		if raw := strings.TrimSpace(cfg.Platforms.Meta.MetaAPIKey); raw != "" {
			val, source, err := resolveSecretExpr(raw)
			if err != nil {
				return "", "", fmt.Errorf("accounts.yml platforms.meta.meta_api_key: %w", err)
			}
			if strings.TrimSpace(val) == "" {
				return "", "", fmt.Errorf("accounts.yml platforms.meta.meta_api_key resolved empty token")
			}
			return val, source, nil
		}
	}

	if v := strings.TrimSpace(os.Getenv("META_ACCESS_TOKEN")); v != "" {
		return v, "env:META_ACCESS_TOKEN", nil
	}
	if v := strings.TrimSpace(os.Getenv("META_API_KEY")); v != "" {
		return v, "env:META_API_KEY", nil
	}
	return "", "", fmt.Errorf("missing Meta token: set %s or %s, or configure %s", "META_ACCESS_TOKEN", "META_API_KEY", AccountsYAMLPath(root, account))
}

func ResolvePostHogToken(root, account string) (string, string, error) {
	cfg, err := LoadAccountsConfig(root, account)
	if err != nil {
		return "", "", err
	}
	if cfg != nil {
		if raw := strings.TrimSpace(cfg.Platforms.PostHog.APIKey); raw != "" {
			val, source, err := resolveSecretExpr(raw)
			if err != nil {
				return "", "", fmt.Errorf("accounts.yml platforms.posthog.api_key: %w", err)
			}
			if strings.TrimSpace(val) == "" {
				return "", "", fmt.Errorf("accounts.yml platforms.posthog.api_key resolved empty token")
			}
			return val, source, nil
		}
	}
	if v := strings.TrimSpace(os.Getenv("POSTHOG_API_KEY")); v != "" {
		return v, "env:POSTHOG_API_KEY", nil
	}
	return "", "", fmt.Errorf("missing PostHog token: set POSTHOG_API_KEY or configure %s", AccountsYAMLPath(root, account))
}

func ResolvePhotoRoomToken(root, account string) (string, string, error) {
	if strings.TrimSpace(account) != "" {
		cfg, err := LoadAccountsConfig(root, account)
		if err != nil {
			return "", "", err
		}
		if cfg != nil {
			if raw := strings.TrimSpace(cfg.Platforms.PhotoRoom.APIKey); raw != "" {
				val, source, err := resolveSecretExpr(raw)
				if err != nil {
					return "", "", fmt.Errorf("accounts.yml platforms.photoroom.api_key: %w", err)
				}
				if strings.TrimSpace(val) == "" {
					return "", "", fmt.Errorf("accounts.yml platforms.photoroom.api_key resolved empty token")
				}
				return val, source, nil
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("PHOTOROOM_API_KEY")); v != "" {
		return v, "env:PHOTOROOM_API_KEY", nil
	}
	if strings.TrimSpace(account) != "" {
		return "", "", fmt.Errorf("missing PhotoRoom token: set PHOTOROOM_API_KEY or configure %s", AccountsYAMLPath(root, account))
	}
	return "", "", fmt.Errorf("missing PhotoRoom token: set PHOTOROOM_API_KEY (or pass --account to use <account>/accounts.yml)")
}

func ResolveGSCredentialsJSON(root, account string) ([]byte, string, error) {
	cfg, err := LoadAccountsConfig(root, account)
	if err != nil {
		return nil, "", err
	}
	if cfg != nil {
		if raw := strings.TrimSpace(cfg.Platforms.GoogleSearchConsole.CredentialsJSON); raw != "" {
			val, source, err := resolveSecretExpr(raw)
			if err != nil {
				return nil, "", fmt.Errorf("accounts.yml platforms.google_search_console.credentials_json: %w", err)
			}
			if strings.TrimSpace(val) == "" {
				return nil, "", fmt.Errorf("accounts.yml platforms.google_search_console.credentials_json resolved empty content")
			}
			return []byte(val), source, nil
		}

		if rawPath := strings.TrimSpace(cfg.Platforms.GoogleSearchConsole.CredentialsFile); rawPath != "" {
			path, b, err := readCredentialsFile(root, account, rawPath)
			if err != nil {
				return nil, "", fmt.Errorf("accounts.yml platforms.google_search_console.credentials_file: %w", err)
			}
			return b, "file:" + path, nil
		}
	}

	if raw := strings.TrimSpace(os.Getenv("GSC_CREDENTIALS_JSON")); raw != "" {
		return []byte(raw), "env:GSC_CREDENTIALS_JSON", nil
	}
	if rawPath := strings.TrimSpace(os.Getenv("GSC_CREDENTIALS_FILE")); rawPath != "" {
		path, b, err := readCredentialsFile(root, account, rawPath)
		if err != nil {
			return nil, "", fmt.Errorf("env GSC_CREDENTIALS_FILE: %w", err)
		}
		return b, "file:" + path, nil
	}
	return nil, "", fmt.Errorf(
		"missing GSC credentials: set platforms.google_search_console.credentials_file/credentials_json in %s, or set GSC_CREDENTIALS_FILE/GSC_CREDENTIALS_JSON",
		AccountsYAMLPath(root, account),
	)
}

func readCredentialsFile(root, account, rawPath string) (string, []byte, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", nil, fmt.Errorf("empty path")
	}
	candidates := []string{}
	if filepath.IsAbs(rawPath) {
		candidates = append(candidates, rawPath)
	} else {
		candidates = append(candidates, filepath.Join(AccountRoot(root, account), rawPath))
		candidates = append(candidates, filepath.Join(root, rawPath))
	}
	var firstErr error
	for _, path := range candidates {
		b, err := os.ReadFile(path)
		if err == nil {
			if strings.TrimSpace(string(b)) == "" {
				return "", nil, fmt.Errorf("file %s is empty", path)
			}
			return path, b, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if len(candidates) > 0 {
		return "", nil, fmt.Errorf("read %s: %w", candidates[0], firstErr)
	}
	return "", nil, fmt.Errorf("no credential path candidates")
}

func resolveSecretExpr(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	m := getEnvExprRe.FindStringSubmatch(raw)
	if len(m) == 2 {
		envName := m[1]
		val := strings.TrimSpace(os.Getenv(envName))
		if val == "" {
			return "", "", fmt.Errorf("environment variable %s is not set", envName)
		}
		return val, "env:" + envName, nil
	}
	return raw, "literal", nil
}
