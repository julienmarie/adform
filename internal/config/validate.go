package config

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

type ValidationError struct {
	Rule    string `json:"rule"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationWarning struct {
	Rule    string `json:"rule"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationRule struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
}

type ValidationResult struct {
	Errors   []ValidationError   `json:"errors"`
	Warnings []ValidationWarning `json:"warnings,omitempty"`
}

func (r ValidationResult) OK() bool {
	return len(r.Errors) == 0
}

func (r ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

var adAccountIDRe = regexp.MustCompile(`^(act_)?\d+$`)

func ValidationRules() []ValidationRule {
	return []ValidationRule{
		{ID: "ACC_ACCOUNT_NAME_REQUIRED", Severity: "error", Scope: "account", Description: "account.account_name must be set"},
		{ID: "ACC_AD_ACCOUNT_ID_REQUIRED", Severity: "error", Scope: "account", Description: "account.meta.ad_account_id must be set"},
		{ID: "ACC_AD_ACCOUNT_ID_FORMAT", Severity: "error", Scope: "account", Description: "account.meta.ad_account_id must be numeric with optional act_ prefix"},
		{ID: "ACC_CURRENCY_FORMAT", Severity: "error", Scope: "account", Description: "account.meta.currency must be ISO-4217 3-letter uppercase"},
		{ID: "ACC_TIMEZONE_REQUIRED", Severity: "error", Scope: "account", Description: "account.meta.timezone must be set"},
		{ID: "ACC_PRODUCT_FEED_URL_FORMAT", Severity: "error", Scope: "account", Description: "account.meta.product_feed_url must be valid absolute http(s) URL when set"},
		{ID: "ACC_BUDGETS_UNIT", Severity: "error", Scope: "account", Description: "account.budgets.unit must be major or minor"},
		{ID: "ACC_ORPHAN_POLICY", Severity: "error", Scope: "account", Description: "account.policies.orphan.on_missing_in_config must be pause, ignore, or empty"},
		{ID: "ACC_BUDGET_INCREASE_RANGE", Severity: "error", Scope: "account", Description: "account.policies.budget.max_increase_ratio must be between 0 and 1"},
		{ID: "ACC_BUDGET_DECREASE_RANGE", Severity: "error", Scope: "account", Description: "account.policies.budget.max_decrease_ratio must be between 0 and 1"},
		{ID: "ASSET_TYPE_ALLOWED", Severity: "error", Scope: "asset", Description: "asset.type must be image or video"},
		{ID: "ASSET_ORIGIN_ALLOWED", Severity: "error", Scope: "asset", Description: "asset.meta.origin must be local or imported when set"},
		{ID: "ASSET_LOCAL_FILE_REQUIRED", Severity: "warning", Scope: "asset", Description: "local assets should have file path"},
		{ID: "ASSET_IMPORTED_META_RECOMMENDED", Severity: "warning", Scope: "asset", Description: "imported assets should have image_hash/video_id"},
		{ID: "AUDIENCE_KEY_REQUIRED", Severity: "error", Scope: "audience", Description: "audience key must be set"},
		{ID: "AUDIENCE_META_ID_REQUIRED", Severity: "error", Scope: "audience", Description: "audience.meta_id is required in v1 reference mode"},
		{ID: "CATALOG_KEY_REQUIRED", Severity: "error", Scope: "catalog", Description: "catalog key must be set"},
		{ID: "CATALOG_TYPE_ALLOWED", Severity: "error", Scope: "catalog", Description: "catalog.type must be catalog when set"},
		{ID: "CATALOG_META_ID_REQUIRED", Severity: "error", Scope: "catalog", Description: "catalog.meta_id is required"},
		{ID: "CAMPAIGN_KEY_REQUIRED", Severity: "error", Scope: "campaign", Description: "campaign key must be set"},
		{ID: "CAMPAIGN_NAME_REQUIRED", Severity: "error", Scope: "campaign", Description: "campaign.name must be set"},
		{ID: "CAMPAIGN_OBJECTIVE_REQUIRED", Severity: "error", Scope: "campaign", Description: "campaign.objective must be set"},
		{ID: "CAMPAIGN_STATUS_ALLOWED", Severity: "error", Scope: "campaign", Description: "campaign.status must be ACTIVE or PAUSED"},
		{ID: "CAMPAIGN_ACTIVE_POLICY", Severity: "error", Scope: "campaign", Description: "campaign ACTIVE is blocked when policies.allow_activate=false"},
		{ID: "ADSET_KEY_REQUIRED", Severity: "error", Scope: "adset", Description: "adset key must be set"},
		{ID: "ADSET_NAME_REQUIRED", Severity: "error", Scope: "adset", Description: "adset.name must be set"},
		{ID: "ADSET_CAMPAIGN_REF", Severity: "error", Scope: "adset", Description: "adset.campaign_key must reference existing campaign"},
		{ID: "ADSET_STATUS_ALLOWED", Severity: "error", Scope: "adset", Description: "adset.status must be ACTIVE or PAUSED"},
		{ID: "ADSET_ACTIVE_POLICY", Severity: "error", Scope: "adset", Description: "adset ACTIVE is blocked when policies.allow_activate=false"},
		{ID: "ADSET_DAILY_BUDGET_POSITIVE", Severity: "error", Scope: "adset", Description: "adset.daily_budget must be > 0"},
		{ID: "ADSET_DAILY_BUDGET_MAX", Severity: "error", Scope: "adset", Description: "adset.daily_budget must not exceed policies.budget.max_daily_budget_major"},
		{ID: "ADSET_CUSTOM_AUDIENCE_REF", Severity: "error", Scope: "adset", Description: "adset targeting custom audiences must reference known audience keys"},
		{ID: "ADSET_EXCL_AUDIENCE_REF", Severity: "error", Scope: "adset", Description: "adset targeting excluded audiences must reference known audience keys"},
		{ID: "ADSET_CATALOG_REF", Severity: "error", Scope: "adset", Description: "adset promoted_object.catalog_key must reference known catalog key"},
		{ID: "ADSET_PLACEMENTS_ALLOWED", Severity: "error", Scope: "adset", Description: "adset.targeting.placements must be advantage_plus or manual"},
		{ID: "ADSET_GEO_MINIMUM", Severity: "error", Scope: "adset", Description: "adset.targeting.geo must have at least one country/country_group/city/region"},
		{ID: "ADSET_START_TIME_RFC3339", Severity: "warning", Scope: "adset", Description: "adset.schedule.start_time should be RFC3339/RFC3339Nano"},
		{ID: "CREATIVE_KEY_REQUIRED", Severity: "error", Scope: "creative", Description: "creative key must be set"},
		{ID: "CREATIVE_TYPE_ALLOWED", Severity: "error", Scope: "creative", Description: "creative.type must be link_ad or reference"},
		{ID: "CREATIVE_REFERENCE_META_ID", Severity: "error", Scope: "creative", Description: "reference creatives require meta_id"},
		{ID: "CREATIVE_LINK_URL_REQUIRED", Severity: "error", Scope: "creative", Description: "link_ad creatives require link.url"},
		{ID: "CREATIVE_LINK_URL_FORMAT", Severity: "warning", Scope: "creative", Description: "link_ad link.url should be valid absolute URL"},
		{ID: "CREATIVE_IMAGE_ASSET_REQUIRED", Severity: "error", Scope: "creative", Description: "link_ad creatives require link.image_asset_key"},
		{ID: "CREATIVE_IMAGE_ASSET_REF", Severity: "error", Scope: "creative", Description: "link.image_asset_key must reference known image asset"},
		{ID: "CREATIVE_DEFAULT_PAGE_ID", Severity: "error", Scope: "creative", Description: "page_id_ref=default requires account.meta.page_id"},
		{ID: "CREATIVE_DEFAULT_IG_ID", Severity: "warning", Scope: "creative", Description: "instagram_actor_id_ref=default works best when account.meta.instagram_actor_id is set"},
		{ID: "AD_KEY_REQUIRED", Severity: "error", Scope: "ad", Description: "ad key must be set"},
		{ID: "AD_NAME_REQUIRED", Severity: "error", Scope: "ad", Description: "ad.name must be set"},
		{ID: "AD_ADSET_REF", Severity: "error", Scope: "ad", Description: "ad.adset_key must reference known adset key"},
		{ID: "AD_CREATIVE_REF", Severity: "error", Scope: "ad", Description: "ad.creative_key must reference known creative key"},
		{ID: "AD_STATUS_ALLOWED", Severity: "error", Scope: "ad", Description: "ad.status must be ACTIVE or PAUSED"},
		{ID: "AD_ACTIVE_POLICY", Severity: "error", Scope: "ad", Description: "ad ACTIVE is blocked when policies.allow_activate=false"},
		{ID: "AD_UTM_SOURCE_RECOMMENDED", Severity: "warning", Scope: "ad", Description: "tracking.utm.source should be set"},
		{ID: "AD_UTM_MEDIUM_RECOMMENDED", Severity: "warning", Scope: "ad", Description: "tracking.utm.medium should be set"},
	}
}

func Validate(bundle Bundle) ValidationResult {
	result := ValidationResult{}
	addErr := func(rule, field, msg string) {
		result.Errors = append(result.Errors, ValidationError{Rule: rule, Field: field, Message: msg})
	}
	addWarn := func(rule, field, msg string) {
		result.Warnings = append(result.Warnings, ValidationWarning{Rule: rule, Field: field, Message: msg})
	}

	if bundle.AccountCfg.AccountName == "" {
		addErr("ACC_ACCOUNT_NAME_REQUIRED", "account.account_name", "required")
	}
	if bundle.AccountCfg.Meta.AdAccountID == "" {
		addErr("ACC_AD_ACCOUNT_ID_REQUIRED", "account.meta.ad_account_id", "required")
	} else if !adAccountIDRe.MatchString(strings.TrimSpace(bundle.AccountCfg.Meta.AdAccountID)) {
		addErr("ACC_AD_ACCOUNT_ID_FORMAT", "account.meta.ad_account_id", "must be numeric with optional act_ prefix")
	}
	if c := strings.TrimSpace(bundle.AccountCfg.Meta.Currency); c == "" || len(c) != 3 || c != strings.ToUpper(c) {
		addErr("ACC_CURRENCY_FORMAT", "account.meta.currency", "must be 3-letter uppercase ISO currency")
	}
	if strings.TrimSpace(bundle.AccountCfg.Meta.Timezone) == "" {
		addErr("ACC_TIMEZONE_REQUIRED", "account.meta.timezone", "required")
	}
	if feedURL := strings.TrimSpace(bundle.AccountCfg.Meta.ProductFeedURL); feedURL != "" && !isValidAbsoluteURL(feedURL) {
		addErr("ACC_PRODUCT_FEED_URL_FORMAT", "account.meta.product_feed_url", "must be valid absolute http(s) URL")
	}
	if bundle.AccountCfg.Budgets.Unit != "major" && bundle.AccountCfg.Budgets.Unit != "minor" {
		addErr("ACC_BUDGETS_UNIT", "account.budgets.unit", "must be major or minor")
	}
	if bundle.AccountCfg.Policies.Orphan.OnMissingInConfig != "" && bundle.AccountCfg.Policies.Orphan.OnMissingInConfig != "pause" && bundle.AccountCfg.Policies.Orphan.OnMissingInConfig != "ignore" {
		addErr("ACC_ORPHAN_POLICY", "account.policies.orphan.on_missing_in_config", "must be pause or ignore")
	}
	if v := bundle.AccountCfg.Policies.Budget.MaxIncreaseRatio; v < 0 || v > 1 {
		addErr("ACC_BUDGET_INCREASE_RANGE", "account.policies.budget.max_increase_ratio", "must be between 0 and 1")
	}
	if v := bundle.AccountCfg.Policies.Budget.MaxDecreaseRatio; v < 0 || v > 1 {
		addErr("ACC_BUDGET_DECREASE_RANGE", "account.policies.budget.max_decrease_ratio", "must be between 0 and 1")
	}

	for key, asset := range bundle.Assets {
		if key == "" {
			addErr("ASSET_TYPE_ALLOWED", "assets[].key", "required")
		}
		if asset.Type != "image" && asset.Type != "video" {
			addErr("ASSET_TYPE_ALLOWED", fmt.Sprintf("assets[%s].type", key), "must be image or video")
		}
		if asset.Meta.Origin != "" && asset.Meta.Origin != "local" && asset.Meta.Origin != "imported" {
			addErr("ASSET_ORIGIN_ALLOWED", fmt.Sprintf("assets[%s].meta.origin", key), "must be local or imported")
		}
		if asset.Meta.Origin == "local" && (asset.File == nil || strings.TrimSpace(*asset.File) == "") {
			addWarn("ASSET_LOCAL_FILE_REQUIRED", fmt.Sprintf("assets[%s].file", key), "recommended for local origin")
		}
		if asset.Meta.Origin == "imported" {
			hasMeta := false
			if asset.Type == "image" && asset.Meta.ImageHash != nil && strings.TrimSpace(*asset.Meta.ImageHash) != "" {
				hasMeta = true
			}
			if asset.Type == "video" && asset.Meta.VideoID != nil && strings.TrimSpace(*asset.Meta.VideoID) != "" {
				hasMeta = true
			}
			if !hasMeta {
				addWarn("ASSET_IMPORTED_META_RECOMMENDED", fmt.Sprintf("assets[%s].meta", key), "imported asset should include image_hash/video_id")
			}
		}
	}

	for key, audience := range bundle.Audiences {
		if key == "" {
			addErr("AUDIENCE_KEY_REQUIRED", "audience.key", "required")
		}
		if audience.MetaID == "" {
			addErr("AUDIENCE_META_ID_REQUIRED", fmt.Sprintf("audiences[%s].meta_id", key), "required in v1 reference mode")
		}
	}
	for key, catalog := range bundle.Catalogs {
		if key == "" {
			addErr("CATALOG_KEY_REQUIRED", "catalog.key", "required")
		}
		if catalog.Type != "" && catalog.Type != "catalog" {
			addErr("CATALOG_TYPE_ALLOWED", fmt.Sprintf("catalogs[%s].type", key), "must be catalog")
		}
		if catalog.MetaID == "" {
			addErr("CATALOG_META_ID_REQUIRED", fmt.Sprintf("catalogs[%s].meta_id", key), "required")
		}
	}

	for key, campaign := range bundle.Campaigns {
		if key == "" {
			addErr("CAMPAIGN_KEY_REQUIRED", "campaign.key", "required")
		}
		if campaign.Name == "" {
			addErr("CAMPAIGN_NAME_REQUIRED", fmt.Sprintf("campaigns[%s].name", key), "required")
		}
		if strings.TrimSpace(campaign.Objective) == "" {
			addErr("CAMPAIGN_OBJECTIVE_REQUIRED", fmt.Sprintf("campaigns[%s].objective", key), "required")
		}
		if !isValidStatus(campaign.Status) {
			addErr("CAMPAIGN_STATUS_ALLOWED", fmt.Sprintf("campaigns[%s].status", key), "must be ACTIVE or PAUSED")
		}
		if !bundle.AccountCfg.Policies.AllowActivate && strings.EqualFold(campaign.Status, "ACTIVE") {
			addErr("CAMPAIGN_ACTIVE_POLICY", fmt.Sprintf("campaigns[%s].status", key), "ACTIVE not allowed by policies.allow_activate=false")
		}
	}

	for key, adset := range bundle.Adsets {
		if key == "" {
			addErr("ADSET_KEY_REQUIRED", "adset.key", "required")
		}
		if strings.TrimSpace(adset.Name) == "" {
			addErr("ADSET_NAME_REQUIRED", fmt.Sprintf("adsets[%s].name", key), "required")
		}
		if _, ok := bundle.Campaigns[adset.CampaignKey]; !ok {
			addErr("ADSET_CAMPAIGN_REF", fmt.Sprintf("adsets[%s].campaign_key", key), "references unknown campaign")
		}
		if !isValidStatus(adset.Status) {
			addErr("ADSET_STATUS_ALLOWED", fmt.Sprintf("adsets[%s].status", key), "must be ACTIVE or PAUSED")
		}
		if !bundle.AccountCfg.Policies.AllowActivate && strings.EqualFold(adset.Status, "ACTIVE") {
			addErr("ADSET_ACTIVE_POLICY", fmt.Sprintf("adsets[%s].status", key), "ACTIVE not allowed by policies.allow_activate=false")
		}
		if adset.DailyBudget <= 0 {
			addErr("ADSET_DAILY_BUDGET_POSITIVE", fmt.Sprintf("adsets[%s].daily_budget", key), "must be > 0")
		}
		if max := bundle.AccountCfg.Policies.Budget.MaxDailyBudgetMajor; max > 0 {
			dailyMajor := adset.DailyBudget
			if bundle.AccountCfg.Budgets.Unit == "minor" {
				dailyMajor = dailyMajor / 100
			}
			if dailyMajor > max {
				addErr("ADSET_DAILY_BUDGET_MAX", fmt.Sprintf("adsets[%s].daily_budget", key), "exceeds policies.budget.max_daily_budget_major")
			}
		}
		for _, audKey := range adset.Targeting.CustomAudiences {
			if _, ok := bundle.Audiences[audKey]; !ok {
				addErr("ADSET_CUSTOM_AUDIENCE_REF", fmt.Sprintf("adsets[%s].targeting.custom_audiences", key), "references unknown audience: "+audKey)
			}
		}
		for _, audKey := range adset.Targeting.ExcludedCustomAudiences {
			if _, ok := bundle.Audiences[audKey]; !ok {
				addErr("ADSET_EXCL_AUDIENCE_REF", fmt.Sprintf("adsets[%s].targeting.excluded_custom_audiences", key), "references unknown audience: "+audKey)
			}
		}
		if adset.PromotedObject.CatalogKey != "" {
			if _, ok := bundle.Catalogs[adset.PromotedObject.CatalogKey]; !ok {
				addErr("ADSET_CATALOG_REF", fmt.Sprintf("adsets[%s].promoted_object.catalog_key", key), "references unknown catalog")
			}
		}
		if adset.Targeting.Placements != "" && adset.Targeting.Placements != "advantage_plus" && adset.Targeting.Placements != "manual" {
			addErr("ADSET_PLACEMENTS_ALLOWED", fmt.Sprintf("adsets[%s].targeting.placements", key), "must be advantage_plus or manual")
		}
		if len(adset.Targeting.Geo.Countries) == 0 && len(adset.Targeting.Geo.CountryGroups) == 0 && len(adset.Targeting.Geo.Cities) == 0 && len(adset.Targeting.Geo.Regions) == 0 {
			addErr("ADSET_GEO_MINIMUM", fmt.Sprintf("adsets[%s].targeting.geo", key), "must include countries, country_groups, cities, or regions")
		}
		if t := strings.TrimSpace(adset.Schedule.StartTime); t != "" {
			if _, err := time.Parse(time.RFC3339, t); err != nil {
				if _, errN := time.Parse(time.RFC3339Nano, t); errN != nil {
					addWarn("ADSET_START_TIME_RFC3339", fmt.Sprintf("adsets[%s].schedule.start_time", key), "expected RFC3339 timestamp with timezone offset")
				}
			}
		}
	}

	for key, creative := range bundle.Creatives {
		if key == "" {
			addErr("CREATIVE_KEY_REQUIRED", "creative.key", "required")
		}
		if !slices.Contains([]string{"link_ad", "reference"}, creative.Type) {
			addErr("CREATIVE_TYPE_ALLOWED", fmt.Sprintf("creatives[%s].type", key), "must be link_ad or reference")
		}
		if creative.Type == "reference" && creative.MetaID == "" {
			addErr("CREATIVE_REFERENCE_META_ID", fmt.Sprintf("creatives[%s].meta_id", key), "required for reference creatives")
		}
		if creative.Type == "link_ad" {
			if creative.Link.URL == "" {
				addErr("CREATIVE_LINK_URL_REQUIRED", fmt.Sprintf("creatives[%s].link.url", key), "required")
			} else if !isValidAbsoluteURL(creative.Link.URL) {
				addWarn("CREATIVE_LINK_URL_FORMAT", fmt.Sprintf("creatives[%s].link.url", key), "expected absolute URL")
			}
			if creative.Link.ImageAssetKey == "" {
				addErr("CREATIVE_IMAGE_ASSET_REQUIRED", fmt.Sprintf("creatives[%s].link.image_asset_key", key), "required")
			} else if asset, ok := bundle.Assets[creative.Link.ImageAssetKey]; !ok {
				addErr("CREATIVE_IMAGE_ASSET_REF", fmt.Sprintf("creatives[%s].link.image_asset_key", key), "references unknown asset")
			} else if asset.Type != "image" {
				addErr("CREATIVE_IMAGE_ASSET_REF", fmt.Sprintf("creatives[%s].link.image_asset_key", key), "must reference an image asset")
			}
			if strings.EqualFold(strings.TrimSpace(creative.PageIDRef), "default") && strings.TrimSpace(bundle.AccountCfg.Meta.PageID) == "" {
				addErr("CREATIVE_DEFAULT_PAGE_ID", fmt.Sprintf("creatives[%s].page_id_ref", key), "default requires account.meta.page_id")
			}
			if strings.EqualFold(strings.TrimSpace(creative.InstagramActorIDRef), "default") && strings.TrimSpace(bundle.AccountCfg.Meta.InstagramActorID) == "" {
				addWarn("CREATIVE_DEFAULT_IG_ID", fmt.Sprintf("creatives[%s].instagram_actor_id_ref", key), "default uses empty account.meta.instagram_actor_id")
			}
		}
	}

	for key, ad := range bundle.Ads {
		if key == "" {
			addErr("AD_KEY_REQUIRED", "ad.key", "required")
		}
		if strings.TrimSpace(ad.Name) == "" {
			addErr("AD_NAME_REQUIRED", fmt.Sprintf("ads[%s].name", key), "required")
		}
		if _, ok := bundle.Adsets[ad.AdsetKey]; !ok {
			addErr("AD_ADSET_REF", fmt.Sprintf("ads[%s].adset_key", key), "references unknown adset")
		}
		if _, ok := bundle.Creatives[ad.CreativeKey]; !ok {
			addErr("AD_CREATIVE_REF", fmt.Sprintf("ads[%s].creative_key", key), "references unknown creative")
		}
		if !isValidStatus(ad.Status) {
			addErr("AD_STATUS_ALLOWED", fmt.Sprintf("ads[%s].status", key), "must be ACTIVE or PAUSED")
		}
		if !bundle.AccountCfg.Policies.AllowActivate && strings.EqualFold(ad.Status, "ACTIVE") {
			addErr("AD_ACTIVE_POLICY", fmt.Sprintf("ads[%s].status", key), "ACTIVE not allowed by policies.allow_activate=false")
		}
		if strings.TrimSpace(ad.Tracking.UTM.Source) == "" {
			addWarn("AD_UTM_SOURCE_RECOMMENDED", fmt.Sprintf("ads[%s].tracking.utm.source", key), "recommended")
		}
		if strings.TrimSpace(ad.Tracking.UTM.Medium) == "" {
			addWarn("AD_UTM_MEDIUM_RECOMMENDED", fmt.Sprintf("ads[%s].tracking.utm.medium", key), "recommended")
		}
	}

	return result
}

func isValidStatus(status string) bool {
	return strings.EqualFold(status, "ACTIVE") || strings.EqualFold(status, "PAUSED")
}

func isValidAbsoluteURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return false
	}
	host := parsed.Hostname()
	for _, r := range host {
		if r == '.' || r == '-' {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}
