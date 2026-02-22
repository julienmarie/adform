package apply

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"adform/internal/config"
	"adform/internal/plan"
	"adform/internal/state"
)

var ErrPolicyBlocked = errors.New("blocked by policy")

type MetaAPI interface {
	CreateObject(edge string, params url.Values) (string, error)
	UpdateObject(id string, params url.Values) error
	PauseObject(id string) error
	UploadImage(adAccountID, filePath string) (string, error)
	UploadVideo(adAccountID, filePath string) (string, error)
}

type Options struct {
	MaxOps int
	DryRun bool
	Delete bool
	Actor  string

	Root   string
	Bundle config.Bundle
	Meta   MetaAPI
}

type OperationResult struct {
	ID      string             `json:"id"`
	Action  plan.OperationType `json:"action"`
	Success bool               `json:"success"`
	Message string             `json:"message"`
}

type Result struct {
	AppliedAt  string            `json:"applied_at"`
	DryRun     bool              `json:"dry_run"`
	Success    bool              `json:"success"`
	Operations []OperationResult `json:"operations"`
	AppliedOps int               `json:"applied_ops"`
	SkippedOps int               `json:"skipped_ops"`
	FailedOps  int               `json:"failed_ops"`
	BlockedOps int               `json:"blocked_ops"`
}

func Execute(account string, st *state.Store, pl plan.Plan, opts Options) (Result, error) {
	if opts.MaxOps <= 0 {
		opts.MaxOps = 200
	}
	actionable := actionableOperationCount(pl.Operations)
	if actionable > opts.MaxOps {
		return Result{}, fmt.Errorf("plan has %d actionable operations (%d total), exceeds --max-ops=%d", actionable, len(pl.Operations), opts.MaxOps)
	}
	if opts.Actor == "" {
		if u, err := user.Current(); err == nil {
			opts.Actor = u.Username
		} else {
			opts.Actor = "unknown"
		}
	}
	if !opts.DryRun {
		if opts.Meta == nil {
			return Result{}, fmt.Errorf("meta client is required for non-dry-run apply")
		}
		if err := st.AcquireLock(account, opts.Actor); err != nil {
			return Result{}, err
		}
		defer st.ReleaseLock(account)
	}

	idx, err := loadStateIndex(account, st)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		AppliedAt:  time.Now().UTC().Format(time.RFC3339),
		DryRun:     opts.DryRun,
		Success:    true,
		Operations: make([]OperationResult, 0, len(pl.Operations)),
	}

	for _, op := range pl.Operations {
		res := OperationResult{ID: op.ID, Action: op.Action}
		switch op.Action {
		case plan.OpNoop, plan.OpDriftOnly:
			res.Success = true
			res.Message = string(op.Action)
			result.SkippedOps++
			result.Operations = append(result.Operations, res)
			continue
		case plan.OpPauseOrphan:
			if opts.DryRun {
				res.Success = true
				if opts.Delete {
					res.Message = "would archive orphan"
				} else {
					res.Message = "would pause orphan"
				}
				result.AppliedOps++
				result.Operations = append(result.Operations, res)
				continue
			}
			err := pauseOrphan(account, st, idx, opts, op)
			if err != nil {
				res.Success = false
				res.Message = err.Error()
				result.FailedOps++
				result.Success = false
				result.Operations = append(result.Operations, res)
				continue
			}
			res.Success = true
			if opts.Delete {
				res.Message = "orphan archived"
			} else {
				res.Message = "orphan paused"
			}
			result.AppliedOps++
			result.Operations = append(result.Operations, res)
			continue
		default:
			if opts.DryRun {
				res.Success = true
				res.Message = "would apply remote"
				result.AppliedOps++
				result.Operations = append(result.Operations, res)
				continue
			}

			metaID, msg, err := applyResource(account, st, idx, opts, op)
			if err != nil {
				res.Success = false
				res.Message = err.Error()
				result.FailedOps++
				result.Success = false
				result.Operations = append(result.Operations, res)
				continue
			}
			if err := upsertManagedResource(account, st, idx, op.Kind, op.Key, metaID, op.Hash, op.Hash); err != nil {
				res.Success = false
				res.Message = err.Error()
				result.FailedOps++
				result.Success = false
				result.Operations = append(result.Operations, res)
				continue
			}
			res.Success = true
			res.Message = msg
			result.AppliedOps++
			result.Operations = append(result.Operations, res)
		}
	}

	return result, nil
}

func actionableOperationCount(ops []plan.Operation) int {
	count := 0
	for _, op := range ops {
		switch op.Action {
		case plan.OpNoop, plan.OpDriftOnly:
			continue
		default:
			count++
		}
	}
	return count
}

func pauseOrphan(account string, st *state.Store, idx *stateIndex, opts Options, op plan.Operation) error {
	row := idx.Get(op.Kind, op.Key)
	if row == nil {
		return nil
	}
	if row.MetaID != "" && canPauseRemote(op.Kind) {
		if opts.Delete {
			if err := opts.Meta.UpdateObject(row.MetaID, url.Values{"status": {"ARCHIVED"}}); err != nil {
				return fmt.Errorf("archive orphan remote %s: %w", op.ID, err)
			}
		} else {
			if err := opts.Meta.PauseObject(row.MetaID); err != nil {
				return fmt.Errorf("pause orphan remote %s: %w", op.ID, err)
			}
		}
	}
	return upsertManagedResource(account, st, idx, op.Kind, op.Key, row.MetaID, row.LastAppliedHash, row.LastSeenRemoteHash)
}

func applyResource(account string, st *state.Store, idx *stateIndex, opts Options, op plan.Operation) (string, string, error) {
	if opts.Meta == nil {
		return "", "", fmt.Errorf("meta client is required")
	}
	adAccountID := opts.Bundle.AccountCfg.Meta.AdAccountID
	if adAccountID == "" {
		return "", "", fmt.Errorf("account meta.ad_account_id is required")
	}

	existing := idx.Get(op.Kind, op.Key)
	existingID := ""
	if existing != nil {
		existingID = existing.MetaID
	}

	switch op.Kind {
	case "asset_image", "asset_video":
		asset, ok := opts.Bundle.Assets[op.Key]
		if !ok {
			return "", "", fmt.Errorf("asset %q not found in config", op.Key)
		}
		var metaID string
		if asset.File != nil && strings.TrimSpace(*asset.File) != "" {
			path := *asset.File
			if !filepath.IsAbs(path) {
				path = filepath.Join(opts.Root, path)
			}
			if _, err := os.Stat(path); err != nil {
				return "", "", fmt.Errorf("asset %q file missing: %w", op.Key, err)
			}
			var err error
			if op.Kind == "asset_image" {
				metaID, err = opts.Meta.UploadImage(adAccountID, path)
			} else {
				metaID, err = opts.Meta.UploadVideo(adAccountID, path)
			}
			if err != nil {
				return "", "", err
			}
		} else {
			if op.Kind == "asset_image" && asset.Meta.ImageHash != nil {
				metaID = *asset.Meta.ImageHash
			}
			if op.Kind == "asset_video" && asset.Meta.VideoID != nil {
				metaID = *asset.Meta.VideoID
			}
			if metaID == "" {
				return "", "", fmt.Errorf("asset %q has no file and no imported meta id", op.Key)
			}
		}
		return metaID, "asset synced", nil

	case "audience":
		aud, ok := opts.Bundle.Audiences[op.Key]
		if !ok {
			return "", "", fmt.Errorf("audience %q not found", op.Key)
		}
		if aud.MetaID == "" {
			return "", "", fmt.Errorf("audience %q is reference-only and requires meta_id", op.Key)
		}
		return aud.MetaID, "audience reference recorded", nil

	case "catalog":
		cat, ok := opts.Bundle.Catalogs[op.Key]
		if !ok {
			return "", "", fmt.Errorf("catalog %q not found", op.Key)
		}
		if cat.MetaID == "" {
			return "", "", fmt.Errorf("catalog %q requires meta_id", op.Key)
		}
		return cat.MetaID, "catalog reference recorded", nil

	case "campaign":
		obj, ok := opts.Bundle.Campaigns[op.Key]
		if !ok {
			return "", "", fmt.Errorf("campaign %q not found", op.Key)
		}
		params, err := campaignParams(obj)
		if err != nil {
			return "", "", err
		}
		if op.Action == plan.OpCreate || op.Action == plan.OpReplace || existingID == "" {
			newID, err := opts.Meta.CreateObject(normalizeActID(adAccountID)+"/campaigns", params)
			if err != nil {
				return "", "", err
			}
			if op.Action == plan.OpReplace && existingID != "" {
				_ = opts.Meta.PauseObject(existingID)
			}
			return newID, "campaign created", nil
		}
		if err := opts.Meta.UpdateObject(existingID, params); err != nil {
			return "", "", err
		}
		return existingID, "campaign updated", nil

	case "creative":
		obj, ok := opts.Bundle.Creatives[op.Key]
		if !ok {
			return "", "", fmt.Errorf("creative %q not found", op.Key)
		}
		if obj.Type == "reference" {
			if obj.MetaID == "" {
				return "", "", fmt.Errorf("creative %q reference requires meta_id", op.Key)
			}
			return obj.MetaID, "creative reference recorded", nil
		}
		params, err := creativeParams(obj, opts.Bundle, idx)
		if err != nil {
			return "", "", err
		}
		newID, err := opts.Meta.CreateObject(normalizeActID(adAccountID)+"/adcreatives", params)
		if err != nil {
			return "", "", err
		}
		return newID, "creative created", nil

	case "adset":
		obj, ok := opts.Bundle.Adsets[op.Key]
		if !ok {
			return "", "", fmt.Errorf("adset %q not found", op.Key)
		}
		params, err := adsetParams(obj, opts.Bundle, idx)
		if err != nil {
			return "", "", err
		}
		if op.Action == plan.OpCreate || op.Action == plan.OpReplace || existingID == "" {
			newID, err := opts.Meta.CreateObject(normalizeActID(adAccountID)+"/adsets", params)
			if err != nil {
				return "", "", err
			}
			if op.Action == plan.OpReplace && existingID != "" {
				_ = opts.Meta.PauseObject(existingID)
			}
			return newID, "adset created", nil
		}
		if err := opts.Meta.UpdateObject(existingID, params); err != nil {
			return "", "", err
		}
		return existingID, "adset updated", nil

	case "ad":
		obj, ok := opts.Bundle.Ads[op.Key]
		if !ok {
			return "", "", fmt.Errorf("ad %q not found", op.Key)
		}
		params, err := adParams(obj, idx)
		if err != nil {
			return "", "", err
		}
		if op.Action == plan.OpCreate || op.Action == plan.OpReplace || existingID == "" {
			newID, err := opts.Meta.CreateObject(normalizeActID(adAccountID)+"/ads", params)
			if err != nil {
				return "", "", err
			}
			if op.Action == plan.OpReplace && existingID != "" {
				_ = opts.Meta.PauseObject(existingID)
			}
			return newID, "ad created", nil
		}
		if err := opts.Meta.UpdateObject(existingID, params); err != nil {
			return "", "", err
		}
		return existingID, "ad updated", nil
	}

	return "", "", fmt.Errorf("unsupported kind %q", op.Kind)
}

func campaignParams(c config.Campaign) (url.Values, error) {
	params := url.Values{}
	params.Set("name", c.Name)
	params.Set("objective", c.Objective)
	params.Set("status", strings.ToUpper(c.Status))
	if len(c.SpecialAdCategories) == 0 {
		params.Set("special_ad_categories", "[]")
	} else {
		b, err := json.Marshal(c.SpecialAdCategories)
		if err != nil {
			return nil, err
		}
		params.Set("special_ad_categories", string(b))
	}
	return params, nil
}

func adsetParams(a config.AdSet, bundle config.Bundle, idx *stateIndex) (url.Values, error) {
	camp := idx.Get("campaign", a.CampaignKey)
	if camp == nil || camp.MetaID == "" {
		return nil, fmt.Errorf("campaign %q has no meta id in state", a.CampaignKey)
	}
	params := url.Values{}
	params.Set("name", a.Name)
	params.Set("status", strings.ToUpper(a.Status))
	params.Set("campaign_id", camp.MetaID)
	params.Set("daily_budget", fmt.Sprintf("%d", budgetMinor(a.DailyBudget, bundle.AccountCfg.Budgets.Unit)))
	params.Set("bid_strategy", a.BidStrategy)
	params.Set("optimization_goal", a.OptimizationGoal)
	params.Set("billing_event", a.BillingEvent)
	if a.Schedule.StartTime != "" {
		params.Set("start_time", a.Schedule.StartTime)
	}

	targeting := cloneMapAny(a.TargetingRaw)
	if targeting == nil {
		targeting = map[string]any{}
	}
	if a.Targeting.AgeMin > 0 {
		targeting["age_min"] = a.Targeting.AgeMin
	}
	if a.Targeting.AgeMax > 0 {
		targeting["age_max"] = a.Targeting.AgeMax
	}
	if len(a.Targeting.Genders) > 0 {
		targeting["genders"] = a.Targeting.Genders
	}
	if len(a.Targeting.Locales) > 0 {
		targeting["locales"] = a.Targeting.Locales
	}
	if len(a.Targeting.Geo.Countries) > 0 {
		targeting["geo_locations"] = map[string]any{"countries": a.Targeting.Geo.Countries}
	}
	if len(a.Targeting.Geo.CountryGroups) > 0 {
		geo, _ := targeting["geo_locations"].(map[string]any)
		if geo == nil {
			geo = map[string]any{}
		}
		geo["country_groups"] = a.Targeting.Geo.CountryGroups
		targeting["geo_locations"] = geo
	}
	if len(a.Targeting.Geo.LocationTypes) > 0 {
		geo, _ := targeting["geo_locations"].(map[string]any)
		if geo == nil {
			geo = map[string]any{}
		}
		geo["location_types"] = a.Targeting.Geo.LocationTypes
		targeting["geo_locations"] = geo
	}
	if len(a.Targeting.Geo.Regions) > 0 {
		geo, _ := targeting["geo_locations"].(map[string]any)
		if geo == nil {
			geo = map[string]any{}
		}
		regions := make([]map[string]any, 0, len(a.Targeting.Geo.Regions))
		for _, r := range a.Targeting.Geo.Regions {
			region := map[string]any{}
			if strings.TrimSpace(r.Key) != "" {
				region["key"] = r.Key
			}
			if strings.TrimSpace(r.Name) != "" {
				region["name"] = r.Name
			}
			if len(region) > 0 {
				regions = append(regions, region)
			}
		}
		if len(regions) > 0 {
			geo["regions"] = regions
			targeting["geo_locations"] = geo
		}
	}
	if len(a.Targeting.Geo.Cities) > 0 {
		geo, _ := targeting["geo_locations"].(map[string]any)
		if geo == nil {
			geo = map[string]any{}
		}
		cities := make([]map[string]any, 0, len(a.Targeting.Geo.Cities))
		for _, c := range a.Targeting.Geo.Cities {
			city := map[string]any{}
			if strings.TrimSpace(c.Key) != "" {
				city["key"] = c.Key
			}
			if strings.TrimSpace(c.Name) != "" {
				city["name"] = c.Name
			}
			if c.Radius > 0 {
				city["radius"] = c.Radius
			}
			if strings.TrimSpace(c.DistanceUnit) != "" {
				city["distance_unit"] = c.DistanceUnit
			}
			if len(city) > 0 {
				cities = append(cities, city)
			}
		}
		if len(cities) > 0 {
			geo["cities"] = cities
			targeting["geo_locations"] = geo
		}
	}
	if len(a.Targeting.CustomAudiences) > 0 {
		auds := make([]map[string]string, 0, len(a.Targeting.CustomAudiences))
		for _, audKey := range a.Targeting.CustomAudiences {
			auds = append(auds, map[string]string{"id": audienceMetaID(bundle, audKey)})
		}
		targeting["custom_audiences"] = auds
	}
	if len(a.Targeting.ExcludedCustomAudiences) > 0 {
		auds := make([]map[string]string, 0, len(a.Targeting.ExcludedCustomAudiences))
		for _, audKey := range a.Targeting.ExcludedCustomAudiences {
			auds = append(auds, map[string]string{"id": audienceMetaID(bundle, audKey)})
		}
		targeting["excluded_custom_audiences"] = auds
	}
	if len(a.Targeting.PublisherPlatforms) > 0 {
		targeting["publisher_platforms"] = a.Targeting.PublisherPlatforms
	}
	if len(a.Targeting.FacebookPositions) > 0 {
		targeting["facebook_positions"] = a.Targeting.FacebookPositions
	}
	if len(a.Targeting.InstagramPositions) > 0 {
		targeting["instagram_positions"] = a.Targeting.InstagramPositions
	}
	if len(a.Targeting.AudienceNetworkPositions) > 0 {
		targeting["audience_network_positions"] = a.Targeting.AudienceNetworkPositions
	}
	if len(a.Targeting.MessengerPositions) > 0 {
		targeting["messenger_positions"] = a.Targeting.MessengerPositions
	}
	if len(a.Targeting.DevicePlatforms) > 0 {
		targeting["device_platforms"] = a.Targeting.DevicePlatforms
	}
	if v, ok := parseJSONAny(a.Targeting.FlexibleSpecJSON); ok {
		targeting["flexible_spec"] = v
	}
	if v, ok := parseJSONAny(a.Targeting.TargetingAutomationJSON); ok {
		targeting["targeting_automation"] = v
	}
	if len(targeting) > 0 {
		b, err := json.Marshal(targeting)
		if err != nil {
			return nil, err
		}
		params.Set("targeting", string(b))
	}

	promoted := cloneMapAny(a.PromotedObjectRaw)
	if promoted == nil {
		promoted = map[string]any{}
	}
	pixel := a.PromotedObject.PixelKey
	if pixel == "default" || pixel == "" {
		pixel = bundle.AccountCfg.Meta.PixelKeyDefault
	}
	if pixel != "" {
		promoted["pixel_id"] = pixel
	}
	if a.PromotedObject.EventType != "" {
		promoted["custom_event_type"] = a.PromotedObject.EventType
	}
	catalogID := strings.TrimSpace(a.PromotedObject.ProductCatalogID)
	if catalogID == "" && strings.TrimSpace(a.PromotedObject.CatalogKey) != "" {
		if cat, ok := bundle.Catalogs[a.PromotedObject.CatalogKey]; ok {
			catalogID = strings.TrimSpace(cat.MetaID)
		}
	}
	if catalogID != "" {
		promoted["product_catalog_id"] = catalogID
	}
	if strings.TrimSpace(a.PromotedObject.ProductSetID) != "" {
		promoted["product_set_id"] = strings.TrimSpace(a.PromotedObject.ProductSetID)
	}
	if len(promoted) > 0 {
		b, err := json.Marshal(promoted)
		if err != nil {
			return nil, err
		}
		params.Set("promoted_object", string(b))
	}
	return params, nil
}

func creativeParams(c config.Creative, bundle config.Bundle, idx *stateIndex) (url.Values, error) {
	if c.Type != "link_ad" {
		return nil, fmt.Errorf("unsupported creative type for creation: %s", c.Type)
	}
	pageID := c.PageIDRef
	if pageID == "" || pageID == "default" {
		pageID = bundle.AccountCfg.Meta.PageID
	}
	if pageID == "" {
		return nil, fmt.Errorf("creative %q requires page_id or page_id_ref=default with account.meta.page_id", c.Key)
	}
	igID := c.InstagramActorIDRef
	if igID == "default" {
		igID = bundle.AccountCfg.Meta.InstagramActorID
	}

	asset, ok := bundle.Assets[c.Link.ImageAssetKey]
	if !ok {
		return nil, fmt.Errorf("creative %q references unknown asset %q", c.Key, c.Link.ImageAssetKey)
	}
	imageHash := ""
	if row := idx.Get("asset_image", c.Link.ImageAssetKey); row != nil {
		imageHash = row.MetaID
	}
	if imageHash == "" && asset.Meta.ImageHash != nil {
		imageHash = *asset.Meta.ImageHash
	}
	if imageHash == "" {
		return nil, fmt.Errorf("creative %q image asset %q has no uploaded image hash", c.Key, c.Link.ImageAssetKey)
	}

	linkData := map[string]any{
		"link":        c.Link.URL,
		"message":     c.Link.Message,
		"name":        c.Link.Headline,
		"description": c.Link.Description,
		"image_hash":  imageHash,
	}
	storySpec := cloneMapAny(c.ObjectStorySpec)
	if storySpec == nil {
		storySpec = cloneMapAny(c.ObjectStorySpecRaw)
	}
	if storySpec == nil {
		storySpec = map[string]any{}
	}
	existingLinkData, _ := storySpec["link_data"].(map[string]any)
	if existingLinkData != nil {
		for k, v := range existingLinkData {
			linkData[k] = v
		}
	}
	linkData["link"] = c.Link.URL
	linkData["message"] = c.Link.Message
	linkData["name"] = c.Link.Headline
	linkData["description"] = c.Link.Description
	linkData["image_hash"] = imageHash
	if strings.TrimSpace(c.Link.CallToActionType) != "" {
		linkData["call_to_action"] = map[string]any{
			"type": strings.TrimSpace(c.Link.CallToActionType),
			"value": map[string]any{
				"link": c.Link.URL,
			},
		}
	}
	storySpec["page_id"] = pageID
	storySpec["link_data"] = linkData
	if igID != "" {
		storySpec["instagram_actor_id"] = igID
	}
	storyJSON, err := json.Marshal(storySpec)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("name", c.Name)
	params.Set("object_story_spec", string(storyJSON))
	assetFeedSpec := c.AssetFeedSpec
	if len(assetFeedSpec) == 0 {
		assetFeedSpec = c.AssetFeedSpecRaw
	}
	if len(assetFeedSpec) > 0 {
		if b, err := json.Marshal(assetFeedSpec); err == nil {
			params.Set("asset_feed_spec", string(b))
		}
	}
	degreesSpec := c.DegreesOfFreedomSpec
	if len(degreesSpec) == 0 {
		degreesSpec = c.DegreesOfFreedomSpecRaw
	}
	if len(degreesSpec) > 0 {
		if b, err := json.Marshal(degreesSpec); err == nil {
			params.Set("degrees_of_freedom_spec", string(b))
		}
	}
	if strings.TrimSpace(c.ObjectType) != "" {
		params.Set("object_type", strings.TrimSpace(c.ObjectType))
	}
	return params, nil
}

func adParams(a config.Ad, idx *stateIndex) (url.Values, error) {
	adset := idx.Get("adset", a.AdsetKey)
	if adset == nil || adset.MetaID == "" {
		return nil, fmt.Errorf("adset %q has no meta id in state", a.AdsetKey)
	}
	creative := idx.Get("creative", a.CreativeKey)
	if creative == nil || creative.MetaID == "" {
		return nil, fmt.Errorf("creative %q has no meta id in state", a.CreativeKey)
	}

	creativeRef, err := json.Marshal(map[string]string{"creative_id": creative.MetaID})
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("name", a.Name)
	params.Set("status", strings.ToUpper(a.Status))
	params.Set("adset_id", adset.MetaID)
	params.Set("creative", string(creativeRef))
	return params, nil
}

func budgetMinor(v float64, unit string) int64 {
	if strings.EqualFold(unit, "minor") {
		return int64(math.Round(v))
	}
	return int64(math.Round(v * 100))
}

func audienceMetaID(bundle config.Bundle, audKey string) string {
	if aud, ok := bundle.Audiences[audKey]; ok && strings.TrimSpace(aud.MetaID) != "" {
		return strings.TrimSpace(aud.MetaID)
	}
	return audKey
}

func parseJSONAny(raw string) (any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false
	}
	return out, true
}

func cloneMapAny(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		fallback := make(map[string]any, len(m))
		for k, v := range m {
			fallback[k] = v
		}
		return fallback
	}
	return out
}

func normalizeActID(v string) string {
	if strings.HasPrefix(v, "act_") {
		return v
	}
	return "act_" + v
}

func canPauseRemote(kind string) bool {
	switch kind {
	case "campaign", "adset", "ad":
		return true
	default:
		return false
	}
}

type stateIndex struct {
	byKindKey map[string]state.ResourceRow
}

func loadStateIndex(account string, st *state.Store) (*stateIndex, error) {
	rows, err := st.ListResources(account)
	if err != nil {
		return nil, err
	}
	idx := &stateIndex{byKindKey: map[string]state.ResourceRow{}}
	for _, row := range rows {
		idx.byKindKey[stateLookupKey(row.Kind, row.LogicalKey)] = row
	}
	return idx, nil
}

func (s *stateIndex) Get(kind, key string) *state.ResourceRow {
	row, ok := s.byKindKey[stateLookupKey(kind, key)]
	if !ok {
		return nil
	}
	copy := row
	return &copy
}

func (s *stateIndex) Put(row state.ResourceRow) {
	s.byKindKey[stateLookupKey(row.Kind, row.LogicalKey)] = row
}

func stateLookupKey(kind, key string) string {
	return kind + ":" + key
}

func upsertManagedResource(account string, st *state.Store, idx *stateIndex, kind, key, metaID, appliedHash, remoteHash string) error {
	existing := idx.Get(kind, key)
	row := state.ResourceRow{
		AccountName:        account,
		Kind:               kind,
		LogicalKey:         key,
		MetaID:             metaID,
		LastAppliedHash:    appliedHash,
		LastSeenRemoteHash: remoteHash,
	}
	if existing != nil {
		row.CreatedAt = existing.CreatedAt
		if row.MetaID == "" {
			row.MetaID = existing.MetaID
		}
		if row.LastAppliedHash == "" {
			row.LastAppliedHash = existing.LastAppliedHash
		}
		if row.LastSeenRemoteHash == "" {
			row.LastSeenRemoteHash = existing.LastSeenRemoteHash
		}
	}
	if err := st.UpsertResource(row); err != nil {
		return err
	}
	idx.Put(row)
	return nil
}
