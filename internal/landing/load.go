package landing

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"adform/internal/workspace"

	"gopkg.in/yaml.v3"
)

func Load(root, account string, opts ServeOptions) (*LoadedSite, error) {
	landingDir := workspace.ResolveLandingDir(root, account)
	sitePath := filepath.Join(landingDir, "site.yml")
	themePath := filepath.Join(landingDir, "theme.css")
	pagesDir := filepath.Join(landingDir, "pages")

	siteBytes, err := os.ReadFile(sitePath)
	if err != nil {
		return nil, fmt.Errorf("read landing/site.yml: %w", err)
	}
	var site SiteConfig
	if err := yaml.Unmarshal(siteBytes, &site); err != nil {
		return nil, fmt.Errorf("parse landing/site.yml: %w", err)
	}

	if strings.TrimSpace(opts.Bind) != "" {
		site.Runtime.Bind = strings.TrimSpace(opts.Bind)
	}
	if strings.TrimSpace(opts.PublicBaseOverride) != "" {
		site.Runtime.PublicBaseURL = strings.TrimSpace(opts.PublicBaseOverride)
	}
	if strings.TrimSpace(opts.MainSiteBaseOverride) != "" {
		site.Runtime.MainSiteBaseURL = strings.TrimSpace(opts.MainSiteBaseOverride)
	}
	applySiteDefaults(&site)

	themeBytes, err := os.ReadFile(themePath)
	if err != nil {
		return nil, fmt.Errorf("read landing/theme.css: %w", err)
	}

	entries, err := os.ReadDir(pagesDir)
	if err != nil {
		return nil, fmt.Errorf("read landing/pages: %w", err)
	}
	pages := make([]*PageFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		path := filepath.Join(pagesDir, entry.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		page, err := parsePage(b)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if strings.TrimSpace(page.Page.Key) == "" {
			base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			page.Page.Key = base
		}
		pages = append(pages, page)
	}
	sort.SliceStable(pages, func(i, j int) bool {
		return pages[i].Page.Slug < pages[j].Page.Slug
	})

	pageBySlug := map[string]*PageFile{}
	for _, page := range pages {
		pageBySlug[strings.TrimSpace(page.Page.Slug)] = page
	}

	assetIndex, err := buildAssetIndex(filepath.Join(landingDir, "assets"))
	if err != nil {
		return nil, fmt.Errorf("build landing assets index: %w", err)
	}

	out := &LoadedSite{
		Root:       root,
		LandingDir: landingDir,
		SitePath:   sitePath,
		ThemePath:  themePath,
		Site:       site,
		Pages:      pages,
		PageBySlug: pageBySlug,
		AssetIndex: assetIndex,
		FeedByID:   map[string]FeedProduct{},
		ThemeCSS:   string(themeBytes),
		LoadedAt:   time.Now().UTC(),
	}
	validateLoaded(out, opts)
	if len(out.ValidationErr) > 0 {
		return out, fmt.Errorf("landing validation failed: %s", strings.Join(out.ValidationErr, "; "))
	}
	return out, nil
}

func applySiteDefaults(site *SiteConfig) {
	if site.Runtime.Bind == "" {
		site.Runtime.Bind = "0.0.0.0:8080"
	}
	if site.Tracking.AttributionCookie == "" {
		site.Tracking.AttributionCookie = "tbd_attr"
	}
	if site.Tracking.VariantCookie == "" {
		site.Tracking.VariantCookie = "tbd_lp"
	}
	if site.Tracking.AttributionTTLDay <= 0 {
		site.Tracking.AttributionTTLDay = 30
	}
	if site.Tracking.VariantTTLDays <= 0 {
		site.Tracking.VariantTTLDays = 7
	}
	if len(site.Tracking.CaptureQueryParam) == 0 {
		site.Tracking.CaptureQueryParam = []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "fbclid"}
	}
	if len(site.Tracking.UTMPassthrough.Allowlist) == 0 {
		site.Tracking.UTMPassthrough.Allowlist = []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "fbclid"}
	}
	if site.PostHog.Events.Impression == "" {
		site.PostHog.Events.Impression = "lp_impression"
	}
	if site.PostHog.Events.CTAClick == "" {
		site.PostHog.Events.CTAClick = "lp_cta_click"
	}
	if site.PostHog.Events.ProductClick == "" {
		site.PostHog.Events.ProductClick = "lp_product_click"
	}
	if site.Bandit.Algorithm == "" {
		site.Bandit.Algorithm = "thompson_beta"
	}
	site.Bandit.Storage.Type = strings.ToLower(strings.TrimSpace(site.Bandit.Storage.Type))
	if site.Bandit.Storage.Type == "" {
		site.Bandit.Storage.Type = "sqlite"
	}
	if site.Bandit.Storage.Redis.KeyPrefix == "" {
		site.Bandit.Storage.Redis.KeyPrefix = "adform:landing:bandit"
	}
	if site.Bandit.MinImpressionsPerArm <= 0 {
		site.Bandit.MinImpressionsPerArm = 200
	}
	if site.Bandit.ControlMinShare <= 0 {
		site.Bandit.ControlMinShare = 0.15
	}
	if site.Bandit.ControlMinShare > 1 {
		site.Bandit.ControlMinShare = 1
	}
	if site.Bandit.Objective.Primary == "" {
		site.Bandit.Objective.Primary = "cta_click"
	}
}

func parsePage(b []byte) (*PageFile, error) {
	var raw struct {
		Version int                      `yaml:"version"`
		Page    PageMeta                 `yaml:"page"`
		Blocks  []map[string]interface{} `yaml:"blocks"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := &PageFile{Version: raw.Version, Page: raw.Page}
	out.Blocks = make([]Block, 0, len(raw.Blocks))
	for i, blockRaw := range raw.Blocks {
		block, err := decodeBlock(blockRaw)
		if err != nil {
			return nil, fmt.Errorf("block[%d]: %w", i, err)
		}
		out.Blocks = append(out.Blocks, block)
	}
	return out, nil
}

type blockBase struct {
	Type      string        `yaml:"type"`
	Key       string        `yaml:"key"`
	Analytics BlockAnalytic `yaml:"analytics"`
}

func decodeBlock(raw map[string]interface{}) (Block, error) {
	base := blockBase{}
	if err := decodeMap(raw, &base); err != nil {
		return Block{}, err
	}
	base.Type = strings.ToLower(strings.TrimSpace(base.Type))
	out := Block{Type: base.Type, Key: strings.TrimSpace(base.Key), Analytics: base.Analytics}
	switch base.Type {
	case "spacer":
		var b struct {
			blockBase   `yaml:",inline"`
			SpacerBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.Spacer = &b.SpacerBlock
	case "hero":
		var b struct {
			blockBase `yaml:",inline"`
			HeroBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.Hero = &b.HeroBlock
	case "media_split":
		var b struct {
			blockBase       `yaml:",inline"`
			MediaSplitBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.MediaSplit = &b.MediaSplitBlock
	case "product_grid":
		var b struct {
			blockBase        `yaml:",inline"`
			ProductGridBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.ProductGrid = &b.ProductGridBlock
	case "columns":
		var b struct {
			blockBase    `yaml:",inline"`
			ColumnsBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.Columns = &b.ColumnsBlock
	case "trust_strip":
		var b struct {
			blockBase       `yaml:",inline"`
			TrustStripBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.TrustStrip = &b.TrustStripBlock
	case "faq":
		var b struct {
			blockBase `yaml:",inline"`
			FAQBlock  `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.FAQ = &b.FAQBlock
	case "pairings":
		var b struct {
			blockBase     `yaml:",inline"`
			PairingsBlock `yaml:",inline"`
		}
		if err := decodeMap(raw, &b); err != nil {
			return Block{}, err
		}
		out.Pairings = &b.PairingsBlock
	default:
		return Block{}, fmt.Errorf("unsupported type %q", base.Type)
	}
	return out, nil
}

func decodeMap(raw map[string]interface{}, out interface{}) error {
	b, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(false)
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func buildAssetIndex(assetsDir string) (map[string]string, error) {
	index := map[string]string{}
	err := filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg", ".mp4", ".mov", ".webm":
		default:
			return nil
		}
		stem := strings.TrimSuffix(d.Name(), ext)
		key := normalizeKey(stem)
		rel, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		index[key] = "/assets/" + filepath.ToSlash(rel)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return index, nil
}

func normalizeKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
