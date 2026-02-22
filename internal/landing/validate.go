package landing

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func validateLoaded(loaded *LoadedSite, opts ServeOptions) {
	errorf := func(format string, args ...any) {
		loaded.ValidationErr = append(loaded.ValidationErr, fmt.Sprintf(format, args...))
	}
	warnf := func(format string, args ...any) {
		loaded.ValidationWarn = append(loaded.ValidationWarn, fmt.Sprintf(format, args...))
	}

	site := &loaded.Site
	if site.Version != 1 {
		errorf("landing/site.yml: version must be 1")
	}
	if !strings.HasPrefix(site.Tracking.CookieDomain, ".") {
		errorf("landing/site.yml: tracking.cookie_domain must start with '.'")
	}
	if strings.TrimSpace(site.Tracking.AttributionCookie) == "" {
		errorf("landing/site.yml: tracking.attribution_cookie must be set")
	}
	if strings.TrimSpace(site.Tracking.VariantCookie) == "" {
		errorf("landing/site.yml: tracking.variant_cookie must be set")
	}
	if site.PostHog.Enabled {
		if strings.TrimSpace(site.PostHog.APIKeyEnv) == "" {
			errorf("landing/site.yml: posthog.api_key_env must be set when posthog.enabled=true")
		}
	}
	for i, raw := range site.Scripts.URLs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || strings.TrimSpace(u.Host) == "" {
			errorf("landing/site.yml: scripts.urls[%d] must be an absolute http/https URL", i)
		}
	}
	if site.Bandit.Enabled {
		if site.Bandit.MinImpressionsPerArm <= 0 {
			errorf("landing/site.yml: bandit.min_impressions_per_arm must be > 0")
		}
		if site.Bandit.ControlMinShare < 0 || site.Bandit.ControlMinShare > 1 {
			errorf("landing/site.yml: bandit.control_min_share must be between 0 and 1")
		}
		switch strings.ToLower(strings.TrimSpace(site.Bandit.Storage.Type)) {
		case "", "sqlite":
		case "redis":
			if strings.TrimSpace(site.Bandit.Storage.Redis.Addr) == "" {
				errorf("landing/site.yml: bandit.storage.redis.addr is required when bandit.storage.type=redis")
			}
		default:
			errorf("landing/site.yml: bandit.storage.type must be sqlite|redis")
		}
	}

	slugSeen := map[string]bool{}
	for _, page := range loaded.Pages {
		if page.Version != 1 {
			errorf("landing/pages/%s.yml: version must be 1", page.Page.Key)
		}
		if !strings.HasPrefix(page.Page.Slug, "/") {
			errorf("landing/pages/%s.yml: page.slug must start with '/'", page.Page.Key)
		}
		if slugSeen[page.Page.Slug] {
			errorf("landing/pages/%s.yml: duplicate slug %s", page.Page.Key, page.Page.Slug)
		}
		slugSeen[page.Page.Slug] = true

		blockKeys := map[string]bool{}
		for i := range page.Blocks {
			block := &page.Blocks[i]
			if block.Key == "" {
				errorf("landing/pages/%s.yml: block[%d] key is required", page.Page.Key, i)
				continue
			}
			if blockKeys[block.Key] {
				errorf("landing/pages/%s.yml: block key %q must be unique", page.Page.Key, block.Key)
			}
			blockKeys[block.Key] = true
			validateBlock(loaded, page, block, errorf, warnf, opts)
		}
	}
}

func validateBlock(loaded *LoadedSite, page *PageFile, block *Block, errorf, warnf func(string, ...any), opts ServeOptions) {
	validateCTA := func(field string, cta *CTA) {
		if cta == nil {
			return
		}
		if strings.TrimSpace(cta.Href) == "" {
			return
		}
		u, err := url.Parse(strings.TrimSpace(cta.Href))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			errorf("landing/pages/%s.yml: block %s %s href must be absolute http/https URL", page.Page.Key, block.Key, field)
		}
	}
	mustHaveAsset := func(field string, assetKey string) {
		if strings.TrimSpace(assetKey) == "" {
			return
		}
		if _, ok := loaded.AssetIndex[normalizeKey(assetKey)]; !ok {
			msg := fmt.Sprintf("landing/pages/%s.yml: block %s %s asset %q not found in landing/assets", page.Page.Key, block.Key, field, assetKey)
			if strings.EqualFold(opts.Env, "prod") {
				errorf(msg)
			} else {
				warnf(msg)
			}
		}
	}

	switch block.Type {
	case "spacer":
		if block.Spacer == nil {
			errorf("landing/pages/%s.yml: block %s spacer payload missing", page.Page.Key, block.Key)
			return
		}
		s := strings.ToLower(strings.TrimSpace(block.Spacer.Size))
		if s != "sm" && s != "md" && s != "lg" && s != "xl" {
			errorf("landing/pages/%s.yml: block %s spacer.size must be sm|md|lg|xl", page.Page.Key, block.Key)
		}
	case "hero":
		if block.Hero == nil {
			errorf("landing/pages/%s.yml: block %s hero payload missing", page.Page.Key, block.Key)
			return
		}
		validateCTA("primary_cta", block.Hero.PrimaryCTA)
		validateCTA("secondary_cta", block.Hero.SecondaryCTA)
		mustHaveAsset("bg_image_asset_key", block.Hero.BGImageAssetKey)
		if block.Hero.Bandit != nil && block.Hero.Bandit.Enabled {
			if strings.TrimSpace(block.Hero.Bandit.Slot) == "" {
				errorf("landing/pages/%s.yml: block %s hero.bandit.slot required", page.Page.Key, block.Key)
			}
			if len(block.Hero.Bandit.Arms) < 2 {
				errorf("landing/pages/%s.yml: block %s hero.bandit.arms requires at least 2 arms", page.Page.Key, block.Key)
			}
			for _, arm := range block.Hero.Bandit.Arms {
				if strings.TrimSpace(arm.Key) == "" {
					errorf("landing/pages/%s.yml: block %s hero.bandit arm key required", page.Page.Key, block.Key)
				}
				mustHaveAsset("hero.bandit.arm.bg_image_asset_key", arm.BGImageAssetKey)
				validateCTA("hero.bandit.arm.primary_cta", arm.PrimaryCTA)
				validateCTA("hero.bandit.arm.secondary_cta", arm.SecondaryCTA)
			}
		}
	case "media_split":
		if block.MediaSplit == nil {
			errorf("landing/pages/%s.yml: block %s media_split payload missing", page.Page.Key, block.Key)
			return
		}
		mustHaveAsset("media.image_asset_key", block.MediaSplit.Media.ImageAssetKey)
		validateCTA("content.cta", block.MediaSplit.Content.CTA)
	case "product_grid":
		if block.ProductGrid == nil {
			errorf("landing/pages/%s.yml: block %s product_grid payload missing", page.Page.Key, block.Key)
			return
		}
		validateCTA("cta", block.ProductGrid.CTA)
		mode := strings.ToLower(strings.TrimSpace(block.ProductGrid.Query.Mode))
		if mode == "" {
			mode = "explicit"
		}
		if mode != "explicit" && mode != "feed_filter" {
			errorf("landing/pages/%s.yml: block %s product_grid.query.mode must be explicit|feed_filter", page.Page.Key, block.Key)
		}
		if mode == "explicit" {
			for _, id := range block.ProductGrid.Query.ProductIDs {
				if id <= 0 {
					errorf("landing/pages/%s.yml: block %s product_grid.query.product_ids must be positive ints", page.Page.Key, block.Key)
				}
			}
		}
	case "columns":
		if block.Columns == nil {
			errorf("landing/pages/%s.yml: block %s columns payload missing", page.Page.Key, block.Key)
			return
		}
		if block.Columns.Columns != 2 && block.Columns.Columns != 3 {
			errorf("landing/pages/%s.yml: block %s columns.columns must be 2 or 3", page.Page.Key, block.Key)
		}
		for _, item := range block.Columns.Items {
			if strings.TrimSpace(item.Href) != "" {
				validateCTA("columns.items.href", &CTA{Href: item.Href})
			}
		}
	case "trust_strip":
		if block.TrustStrip == nil {
			errorf("landing/pages/%s.yml: block %s trust_strip payload missing", page.Page.Key, block.Key)
		}
	case "faq":
		if block.FAQ == nil {
			errorf("landing/pages/%s.yml: block %s faq payload missing", page.Page.Key, block.Key)
		}
	case "pairings":
		if block.Pairings == nil {
			errorf("landing/pages/%s.yml: block %s pairings payload missing", page.Page.Key, block.Key)
			return
		}
		for _, item := range block.Pairings.Items {
			if strings.TrimSpace(item.Href) != "" {
				validateCTA("pairings.items.href", &CTA{Href: item.Href})
			}
		}
	default:
		errorf("landing/pages/%s.yml: block %s unsupported type %q", page.Page.Key, block.Key, block.Type)
	}

	if expected := strings.TrimSuffix(filepath.Base(page.Page.Key+".yml"), ".yml"); expected != "" {
		_ = expected
	}
}
