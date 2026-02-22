package importer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"adform/internal/state"
)

type RemoteSyncClient interface {
	ListEdge(edge string, fields []string, params url.Values) ([]map[string]any, error)
	GetNode(id string, fields ...string) (map[string]any, error)
}

type RemoteSyncOptions struct {
	CampaignFilter string
	StatusFilter   string
	Progress       ProgressFunc
}

type RemoteSyncResult struct {
	Campaigns   int
	Adsets      int
	Ads         int
	Creatives   int
	Audiences   int
	AssetImages int
	AssetVideos int
	Catalogs    int
	Warnings    []string
	Details     map[string]map[string]map[string]any `json:"-"`
}

func SyncStateFromRemote(st *state.Store, account, adAccountID string, client RemoteSyncClient, opts RemoteSyncOptions) (RemoteSyncResult, error) {
	if adAccountID == "" {
		return RemoteSyncResult{}, fmt.Errorf("ad account id is required for remote import")
	}
	edge := normalizeActID(adAccountID)

	statusParams := buildStatusParams(opts.StatusFilter)
	details := map[string]map[string]map[string]any{
		"campaign":    {},
		"adset":       {},
		"ad":          {},
		"creative":    {},
		"audience":    {},
		"catalog":     {},
		"asset_image": {},
		"asset_video": {},
	}

	campaigns, err := client.ListEdge(edge+"/campaigns", []string{"id", "name", "status", "objective", "special_ad_categories"}, statusParams)
	if err != nil {
		return RemoteSyncResult{}, fmt.Errorf("list campaigns: %w", err)
	}
	emitProgress(opts.Progress, ProgressEvent{Stage: "campaigns", Current: 0, Total: len(campaigns), Message: "fetched"})
	used := map[string]struct{}{}
	filterByCampaign := strings.TrimSpace(opts.CampaignFilter) != ""
	campaignIDToKey := map[string]string{}
	for i, c := range campaigns {
		name := stringField(c, "name")
		status := strings.ToUpper(stringField(c, "status"))
		if opts.CampaignFilter != "" && !strings.Contains(name, opts.CampaignFilter) {
			continue
		}
		if opts.StatusFilter != "" && status != strings.ToUpper(opts.StatusFilter) {
			continue
		}
		id := stringField(c, "id")
		if id == "" {
			continue
		}
		key := stableKey(name, id, used)
		campaignIDToKey[id] = key
		if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "campaign", LogicalKey: key, MetaID: id}); err != nil {
			return RemoteSyncResult{}, err
		}
		details["campaign"][key] = c
		emitProgress(opts.Progress, ProgressEvent{Stage: "campaigns", Current: i + 1, Total: len(campaigns)})
	}
	adsets, err := client.ListEdge(edge+"/adsets", []string{"id", "name", "status", "campaign_id", "daily_budget", "bid_strategy", "optimization_goal", "billing_event", "start_time", "targeting", "promoted_object"}, statusParams)
	if err != nil {
		return RemoteSyncResult{}, fmt.Errorf("list adsets: %w", err)
	}
	emitProgress(opts.Progress, ProgressEvent{Stage: "adsets", Current: 0, Total: len(adsets), Message: "fetched"})
	adsetIDToKey := map[string]string{}
	for i, a := range adsets {
		id := stringField(a, "id")
		if id == "" {
			continue
		}
		campID := stringField(a, "campaign_id")
		if filterByCampaign {
			if _, ok := campaignIDToKey[campID]; !ok {
				continue
			}
		}
		status := strings.ToUpper(stringField(a, "status"))
		if opts.StatusFilter != "" && status != strings.ToUpper(opts.StatusFilter) {
			continue
		}
		key := stableKey(stringField(a, "name"), id, used)
		adsetIDToKey[id] = key
		if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "adset", LogicalKey: key, MetaID: id}); err != nil {
			return RemoteSyncResult{}, err
		}
		details["adset"][key] = a
		emitProgress(opts.Progress, ProgressEvent{Stage: "adsets", Current: i + 1, Total: len(adsets)})
	}
	ads, err := client.ListEdge(edge+"/ads", []string{"id", "name", "status", "adset_id", "creative"}, statusParams)
	if err != nil {
		return RemoteSyncResult{}, fmt.Errorf("list ads: %w", err)
	}
	emitProgress(opts.Progress, ProgressEvent{Stage: "ads", Current: 0, Total: len(ads), Message: "fetched"})
	creativeIDs := map[string]struct{}{}
	for i, a := range ads {
		id := stringField(a, "id")
		if id == "" {
			continue
		}
		adsetID := stringField(a, "adset_id")
		if filterByCampaign {
			if _, ok := adsetIDToKey[adsetID]; !ok {
				continue
			}
		}
		status := strings.ToUpper(stringField(a, "status"))
		if opts.StatusFilter != "" && status != strings.ToUpper(opts.StatusFilter) {
			continue
		}
		key := stableKey(stringField(a, "name"), id, used)
		if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "ad", LogicalKey: key, MetaID: id}); err != nil {
			return RemoteSyncResult{}, err
		}
		details["ad"][key] = a
		if creativeID := creativeIDFromAd(a); creativeID != "" {
			creativeIDs[creativeID] = struct{}{}
		}
		emitProgress(opts.Progress, ProgressEvent{Stage: "ads", Current: i + 1, Total: len(ads)})
	}
	creativeList := make([]string, 0, len(creativeIDs))
	for id := range creativeIDs {
		creativeList = append(creativeList, id)
	}
	sort.Strings(creativeList)
	creativeFields := creativeDetailFields()
	creativeFallback := false
	var warnings []string
	emitProgress(opts.Progress, ProgressEvent{Stage: "creatives", Current: 0, Total: len(creativeList), Message: "resolving"})
	for i, id := range creativeList {
		obj, err := client.GetNode(id, creativeFields...)
		if err != nil && !creativeFallback && isInvalidFieldStyleError(err) {
			creativeFields = []string{"id", "name"}
			creativeFallback = true
			warnings = append(warnings, "creative detail fields partially unsupported by API; falling back to id/name only")
			obj, err = client.GetNode(id, creativeFields...)
		}
		if err != nil {
			return RemoteSyncResult{}, fmt.Errorf("get creative %s: %w", id, err)
		}
		key := stableKey(stringField(obj, "name"), id, used)
		if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "creative", LogicalKey: key, MetaID: id}); err != nil {
			return RemoteSyncResult{}, err
		}
		details["creative"][key] = obj
		emitProgress(opts.Progress, ProgressEvent{Stage: "creatives", Current: i + 1, Total: len(creativeList)})
	}
	psWarnings := enrichAdsetsWithCatalogFromProductSets(details["adset"], client)
	warnings = append(warnings, psWarnings...)
	inferWarnings := inferCatalogForDynamicAdsets(details["adset"], details["ad"], details["creative"])
	warnings = append(warnings, inferWarnings...)

	audienceFields := audienceDetailFields()
	audiences, err := client.ListEdge(edge+"/customaudiences", audienceFields, nil)
	if err != nil && isInvalidFieldStyleError(err) {
		audienceFields = []string{"id", "name", "subtype"}
		warnings = append(warnings, "audience detail fields partially unsupported by API; falling back to id/name/subtype")
		audiences, err = client.ListEdge(edge+"/customaudiences", audienceFields, nil)
	}
	if err == nil {
		emitProgress(opts.Progress, ProgressEvent{Stage: "audiences", Current: 0, Total: len(audiences), Message: "fetched"})
		for i, aud := range audiences {
			id := stringField(aud, "id")
			if id == "" {
				continue
			}
			key := stableKey(stringField(aud, "name"), id, used)
			if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "audience", LogicalKey: key, MetaID: id}); err != nil {
				return RemoteSyncResult{}, err
			}
			details["audience"][key] = aud
			emitProgress(opts.Progress, ProgressEvent{Stage: "audiences", Current: i + 1, Total: len(audiences)})
		}
	} else {
		warnings = append(warnings, "customaudiences import skipped: "+err.Error())
	}
	if images, err := client.ListEdge(edge+"/adimages", []string{"hash", "name"}, nil); err == nil {
		emitProgress(opts.Progress, ProgressEvent{Stage: "assets-image", Current: 0, Total: len(images), Message: "fetched"})
		for i, img := range images {
			hash := stringField(img, "hash")
			if hash == "" {
				continue
			}
			key := stableKey(stringField(img, "name"), hash, used)
			if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "asset_image", LogicalKey: key, MetaID: hash}); err != nil {
				return RemoteSyncResult{}, err
			}
			details["asset_image"][key] = img
			emitProgress(opts.Progress, ProgressEvent{Stage: "assets-image", Current: i + 1, Total: len(images)})
		}
	} else {
		warnings = append(warnings, "adimages import skipped: "+err.Error())
	}

	if videos, err := client.ListEdge(edge+"/advideos", []string{"id", "title"}, nil); err == nil {
		emitProgress(opts.Progress, ProgressEvent{Stage: "assets-video", Current: 0, Total: len(videos), Message: "fetched"})
		for i, v := range videos {
			id := stringField(v, "id")
			if id == "" {
				continue
			}
			key := stableKey(stringField(v, "title"), id, used)
			if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "asset_video", LogicalKey: key, MetaID: id}); err != nil {
				return RemoteSyncResult{}, err
			}
			details["asset_video"][key] = v
			emitProgress(opts.Progress, ProgressEvent{Stage: "assets-video", Current: i + 1, Total: len(videos)})
		}
	} else {
		warnings = append(warnings, "advideos import skipped: "+err.Error())
	}

	catalogByID := map[string]map[string]any{}
	if catalogs, err := client.ListEdge(edge+"/product_catalogs", []string{"id", "name"}, nil); err == nil {
		for _, cat := range catalogs {
			id := strings.TrimSpace(stringField(cat, "id"))
			if id == "" {
				continue
			}
			catalogByID[id] = cat
		}
	} else if !isInvalidFieldStyleError(err) {
		warnings = append(warnings, "product_catalogs list skipped: "+err.Error())
	}
	if ids, err := collectCatalogIDsFromAccountProductSets(client, edge); err == nil {
		for _, id := range ids {
			if _, ok := catalogByID[id]; ok {
				continue
			}
			catalogByID[id] = map[string]any{"id": id}
		}
	} else {
		warnings = append(warnings, "product_sets catalog scan skipped: "+err.Error())
	}
	for _, catalogID := range collectCatalogIDsFromAdsets(details["adset"]) {
		if _, ok := catalogByID[catalogID]; ok {
			continue
		}
		obj, err := client.GetNode(catalogID, "id", "name")
		if err != nil {
			warnings = append(warnings, "catalog detail skipped "+catalogID+": "+err.Error())
			continue
		}
		catalogByID[catalogID] = obj
	}
	catalogIDs := make([]string, 0, len(catalogByID))
	for id := range catalogByID {
		catalogIDs = append(catalogIDs, id)
	}
	sort.Strings(catalogIDs)
	emitProgress(opts.Progress, ProgressEvent{Stage: "catalogs", Current: 0, Total: len(catalogIDs), Message: "syncing"})
	for i, catalogID := range catalogIDs {
		obj := catalogByID[catalogID]
		key := stableKey(stringField(obj, "name"), catalogID, used)
		if err := st.UpsertResource(state.ResourceRow{AccountName: account, Kind: "catalog", LogicalKey: key, MetaID: catalogID}); err != nil {
			return RemoteSyncResult{}, err
		}
		details["catalog"][key] = obj
		emitProgress(opts.Progress, ProgressEvent{Stage: "catalogs", Current: i + 1, Total: len(catalogIDs)})
	}

	rows, err := st.ListResources(account)
	if err != nil {
		return RemoteSyncResult{}, err
	}
	result := RemoteSyncResult{Warnings: warnings, Details: details}
	for _, row := range rows {
		switch row.Kind {
		case "campaign":
			result.Campaigns++
		case "adset":
			result.Adsets++
		case "ad":
			result.Ads++
		case "creative":
			result.Creatives++
		case "audience":
			result.Audiences++
		case "asset_image":
			result.AssetImages++
		case "asset_video":
			result.AssetVideos++
		case "catalog":
			result.Catalogs++
		}
	}
	return result, nil
}

func normalizeActID(v string) string {
	if strings.HasPrefix(v, "act_") {
		return v
	}
	return "act_" + v
}

func creativeIDFromAd(ad map[string]any) string {
	v, ok := ad["creative"]
	if !ok {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if id := stringField(m, "id"); id != "" {
		return id
	}
	return stringField(m, "creative_id")
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return stringValue(v)
}

func stringValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return strings.TrimSpace(t.String())
	case int:
		return strconv.Itoa(t)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case uint8:
		return strconv.FormatUint(uint64(t), 10)
	case uint16:
		return strconv.FormatUint(uint64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	case float32:
		f := float64(t)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(f, 'f', -1, 64))
	default:
		return ""
	}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func stableKey(name, metaID string, used map[string]struct{}) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = slugRe.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "imported"
	}
	short := shortID(metaID, 6)
	key := base + "__" + short
	if _, ok := used[key]; !ok {
		used[key] = struct{}{}
		return key
	}
	for i := 7; i <= 12; i++ {
		candidate := base + "__" + shortID(metaID, i)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
	n := 2
	for {
		candidate := fmt.Sprintf("%s__%s-%d", base, short, n)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
		n++
	}
}

func shortID(v string, n int) string {
	v = strings.TrimSpace(v)
	if len(v) <= n {
		return v
	}
	return v[len(v)-n:]
}

func collectCatalogIDsFromAdsets(adsetDetails map[string]map[string]any) []string {
	ids := map[string]struct{}{}
	for _, adset := range adsetDetails {
		promoted, ok := asMap(adset["promoted_object"])
		if !ok {
			continue
		}
		id := strings.TrimSpace(stringField(promoted, "product_catalog_id"))
		if id == "" {
			continue
		}
		ids[id] = struct{}{}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func buildStatusParams(statusFilter string) url.Values {
	params := url.Values{}
	params.Set("limit", "500")
	statusFilter = strings.TrimSpace(strings.ToUpper(statusFilter))
	if statusFilter != "" {
		if b, err := json.Marshal([]string{statusFilter}); err == nil {
			params.Set("effective_status", string(b))
		}
	}
	return params
}

func isInvalidFieldStyleError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid parameter") || strings.Contains(msg, "tried accessing nonexisting field")
}

func ensurePromotedObject(adset map[string]any) map[string]any {
	if adset == nil {
		return nil
	}
	if existing, ok := asMap(adset["promoted_object"]); ok {
		return existing
	}
	promoted := map[string]any{}
	adset["promoted_object"] = promoted
	return promoted
}

func enrichAdsetsWithCatalogFromProductSets(adsetDetails map[string]map[string]any, client RemoteSyncClient) []string {
	if len(adsetDetails) == 0 || client == nil {
		return nil
	}

	productSetCatalog := map[string]string{}
	missingProductSets := map[string]struct{}{}
	for _, adset := range adsetDetails {
		promoted := ensurePromotedObject(adset)
		if promoted == nil {
			continue
		}
		productSetID := strings.TrimSpace(stringField(promoted, "product_set_id"))
		if productSetID == "" {
			continue
		}
		catalogID := strings.TrimSpace(stringField(promoted, "product_catalog_id"))
		if catalogID != "" {
			productSetCatalog[productSetID] = catalogID
			continue
		}
		missingProductSets[productSetID] = struct{}{}
	}

	warnings := make([]string, 0)
	for productSetID := range missingProductSets {
		if _, known := productSetCatalog[productSetID]; known {
			continue
		}
		catalogID, err := resolveCatalogIDFromProductSet(client, productSetID)
		if err != nil {
			warnings = append(warnings, "product set detail skipped "+productSetID+": "+err.Error())
			continue
		}
		if catalogID == "" {
			continue
		}
		productSetCatalog[productSetID] = catalogID
	}

	for _, adset := range adsetDetails {
		promoted := ensurePromotedObject(adset)
		if promoted == nil {
			continue
		}
		if strings.TrimSpace(stringField(promoted, "product_catalog_id")) != "" {
			continue
		}
		productSetID := strings.TrimSpace(stringField(promoted, "product_set_id"))
		if productSetID == "" {
			continue
		}
		if catalogID := strings.TrimSpace(productSetCatalog[productSetID]); catalogID != "" {
			promoted["product_catalog_id"] = catalogID
		}
	}

	return warnings
}

func inferCatalogForDynamicAdsets(adsetDetails, adDetails, creativeDetails map[string]map[string]any) []string {
	if len(adsetDetails) == 0 {
		return nil
	}

	campaignCatalogs := map[string]map[string]struct{}{}
	globalCatalogs := map[string]struct{}{}
	for _, adset := range adsetDetails {
		promoted := ensurePromotedObject(adset)
		if promoted == nil {
			continue
		}
		catalogID := strings.TrimSpace(stringField(promoted, "product_catalog_id"))
		if catalogID == "" {
			continue
		}
		campaignID := strings.TrimSpace(stringField(adset, "campaign_id"))
		if campaignID != "" {
			if campaignCatalogs[campaignID] == nil {
				campaignCatalogs[campaignID] = map[string]struct{}{}
			}
			campaignCatalogs[campaignID][catalogID] = struct{}{}
		}
		globalCatalogs[catalogID] = struct{}{}
	}
	if len(globalCatalogs) == 0 {
		return nil
	}

	creativeByID := map[string]map[string]any{}
	for _, creative := range creativeDetails {
		id := strings.TrimSpace(stringField(creative, "id"))
		if id == "" {
			continue
		}
		creativeByID[id] = creative
	}

	dynamicAdsetIDs := map[string]struct{}{}
	for _, ad := range adDetails {
		adsetID := strings.TrimSpace(stringField(ad, "adset_id"))
		if adsetID == "" {
			continue
		}
		creativeID := strings.TrimSpace(creativeIDFromAd(ad))
		if creativeID == "" {
			continue
		}
		creative, ok := creativeByID[creativeID]
		if !ok {
			continue
		}
		if isCatalogDynamicCreative(creative) {
			dynamicAdsetIDs[adsetID] = struct{}{}
		}
	}

	globalCatalogID := ""
	if len(globalCatalogs) == 1 {
		for id := range globalCatalogs {
			globalCatalogID = id
		}
	}
	warnings := make([]string, 0)
	for adsetKey, adset := range adsetDetails {
		adsetID := strings.TrimSpace(stringField(adset, "id"))
		if adsetID == "" {
			continue
		}
		if _, isDynamic := dynamicAdsetIDs[adsetID]; !isDynamic {
			continue
		}
		promoted := ensurePromotedObject(adset)
		if promoted == nil {
			continue
		}
		if strings.TrimSpace(stringField(promoted, "product_catalog_id")) != "" {
			continue
		}

		campaignID := strings.TrimSpace(stringField(adset, "campaign_id"))
		chosenCatalog := ""
		if cats := campaignCatalogs[campaignID]; len(cats) == 1 {
			for id := range cats {
				chosenCatalog = id
			}
		} else if globalCatalogID != "" {
			chosenCatalog = globalCatalogID
		}
		if chosenCatalog == "" {
			continue
		}
		promoted["product_catalog_id"] = chosenCatalog
		warnings = append(warnings, "catalog inferred for adset "+adsetKey+": "+chosenCatalog)
	}
	return warnings
}

func collectCatalogIDsFromAccountProductSets(client RemoteSyncClient, edge string) ([]string, error) {
	if client == nil || strings.TrimSpace(edge) == "" {
		return nil, nil
	}
	fieldSets := [][]string{
		{"id", "product_catalog{id,name}"},
		{"id", "product_catalog"},
	}
	var lastInvalid error
	for _, fields := range fieldSets {
		rows, err := client.ListEdge(edge+"/product_sets", fields, nil)
		if err != nil {
			if isInvalidFieldStyleError(err) {
				lastInvalid = err
				continue
			}
			return nil, err
		}
		ids := map[string]struct{}{}
		for _, row := range rows {
			if id := extractCatalogIDFromProductSet(row); id != "" {
				ids[id] = struct{}{}
			}
		}
		out := make([]string, 0, len(ids))
		for id := range ids {
			out = append(out, id)
		}
		sort.Strings(out)
		return out, nil
	}
	if lastInvalid != nil {
		return nil, lastInvalid
	}
	return nil, nil
}

func resolveCatalogIDFromProductSet(client RemoteSyncClient, productSetID string) (string, error) {
	productSetID = strings.TrimSpace(productSetID)
	if client == nil || productSetID == "" {
		return "", nil
	}
	fieldSets := [][]string{
		{"id", "product_catalog{id,name}"},
		{"id", "product_catalog"},
	}
	var lastInvalid error
	for _, fields := range fieldSets {
		obj, err := client.GetNode(productSetID, fields...)
		if err != nil {
			if isInvalidFieldStyleError(err) {
				lastInvalid = err
				continue
			}
			return "", err
		}
		return extractCatalogIDFromProductSet(obj), nil
	}
	if lastInvalid != nil {
		return "", lastInvalid
	}
	return "", nil
}

func extractCatalogIDFromProductSet(obj map[string]any) string {
	if len(obj) == 0 {
		return ""
	}
	if m, ok := asMap(obj["product_catalog"]); ok {
		if id := strings.TrimSpace(stringField(m, "id")); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(stringField(obj, "product_catalog")); id != "" {
		return id
	}
	if id := strings.TrimSpace(stringField(obj, "product_catalog_id")); id != "" {
		return id
	}
	if id := strings.TrimSpace(stringField(obj, "catalog_id")); id != "" {
		return id
	}
	return ""
}
