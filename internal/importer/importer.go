package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"adform/internal/config"
	"adform/internal/state"
	"adform/internal/workspace"
	"gopkg.in/yaml.v3"
)

type RemoteDetailClient interface {
	GetNode(id string, fields ...string) (map[string]any, error)
}

type Options struct {
	Root               string
	Account            string
	Out                string
	DryRun             bool
	PreserveStatus     bool
	Force              bool
	CreatePlaceholders bool
	AccountConfig      *config.AccountConfig
	Remote             RemoteDetailClient
	PrefetchedDetails  map[string]map[string]map[string]any
	Progress           ProgressFunc
}

type Result struct {
	OutPath       string   `json:"out_path"`
	DryRun        bool     `json:"dry_run"`
	ResourcesSeen int      `json:"resources_seen"`
	FilesWritten  int      `json:"files_written"`
	FilesPlanned  int      `json:"files_planned"`
	Warnings      []string `json:"warnings"`
}

func ImportFromState(st *state.Store, opts Options) (Result, error) {
	out := opts.Out
	if out == "" {
		out = workspace.ResolveMetaDir(opts.Root, opts.Account)
	} else if !filepath.IsAbs(out) {
		out = filepath.Join(opts.Root, out)
	}

	rows, err := st.ListResources(opts.Account)
	if err != nil {
		return Result{}, err
	}

	result := Result{OutPath: out, DryRun: opts.DryRun, ResourcesSeen: len(rows)}
	byKind := map[string][]state.ResourceRow{}
	for _, row := range rows {
		byKind[row.Kind] = append(byKind[row.Kind], row)
	}

	details := opts.PrefetchedDetails
	if details == nil {
		details = map[string]map[string]map[string]any{}
	}
	if opts.Remote != nil {
		fetched, warnings := fetchRemoteDetails(byKind, opts.Remote)
		mergeDetails(details, fetched)
		result.Warnings = append(result.Warnings, warnings...)
		emitProgress(opts.Progress, ProgressEvent{Stage: "details", Current: 1, Total: 1, Done: true, Message: "remote detail fetch complete"})
	} else {
		emitProgress(opts.Progress, ProgressEvent{Stage: "details", Current: 1, Total: 1, Done: true, Message: "using prefetched details"})
	}

	campaignKeys := extractKeys(byKind["campaign"])
	if len(campaignKeys) == 0 && opts.CreatePlaceholders {
		campaignKeys = []string{"imported_campaign"}
		result.Warnings = append(result.Warnings, "no campaigns in state; created placeholder imported_campaign")
	}
	adsetKeys := extractKeys(byKind["adset"])
	if len(adsetKeys) == 0 && opts.CreatePlaceholders {
		adsetKeys = []string{"imported_adset"}
		result.Warnings = append(result.Warnings, "no adsets in state; created placeholder imported_adset")
	}
	creativeKeys := extractKeys(byKind["creative"])
	if len(creativeKeys) == 0 && opts.CreatePlaceholders {
		creativeKeys = []string{"imported_creative"}
		result.Warnings = append(result.Warnings, "no creatives in state; created placeholder imported_creative")
	}

	campaignByMeta := metaToKeyMap(byKind["campaign"])
	adsetByMeta := metaToKeyMap(byKind["adset"])
	creativeByMeta := metaToKeyMap(byKind["creative"])
	audienceByMeta := metaToKeyMap(byKind["audience"])
	catalogByMeta := metaToKeyMap(byKind["catalog"])
	assetImageByMeta := metaToKeyMap(byKind["asset_image"])
	assetVideoByMeta := metaToKeyMap(byKind["asset_video"])

	files := map[string]string{}
	accountPath := filepath.Join(out, "account.yml")
	if _, err := os.Stat(accountPath); err != nil {
		files[accountPath] = defaultAccount(opts.Account, opts.AccountConfig)
	}
	files[filepath.Join(out, "assets.yml")] = renderAssets(byKind)
	files = merge(files, renderAudiences(out, byKind["audience"], details["audience"]))
	files = merge(files, renderCatalogs(out, byKind["catalog"], details["catalog"]))
	files = merge(files, renderCreatives(out, byKind["creative"], details["creative"], assetImageByMeta, assetVideoByMeta))
	files = merge(files, renderCampaigns(out, byKind["campaign"], details["campaign"], opts.PreserveStatus))

	fallbackCampaign := ""
	if len(campaignKeys) > 0 {
		fallbackCampaign = campaignKeys[0]
	}
	adsetFiles, adsetToCampaign := renderAdsets(out, byKind["adset"], details["adset"], campaignByMeta, audienceByMeta, catalogByMeta, fallbackCampaign, opts.PreserveStatus)
	files = merge(files, adsetFiles)

	fallbackAdset := ""
	if len(adsetKeys) > 0 {
		fallbackAdset = adsetKeys[0]
	}
	fallbackCreative := ""
	if len(creativeKeys) > 0 {
		fallbackCreative = creativeKeys[0]
	}
	files = merge(files, renderAds(out, byKind["ad"], details["ad"], adsetByMeta, creativeByMeta, adsetToCampaign, fallbackCampaign, fallbackAdset, fallbackCreative, opts.PreserveStatus))

	if len(byKind["campaign"]) == 0 && opts.CreatePlaceholders {
		files[filepath.Join(out, "campaigns", "imported_campaign", "campaign.yml")] = defaultCampaign("imported_campaign")
	}
	if len(byKind["adset"]) == 0 && opts.CreatePlaceholders && fallbackCampaign != "" {
		files[filepath.Join(out, "campaigns", fallbackCampaign, "adsets", "imported_adset", "adset.yml")] = defaultAdset("imported_adset", fallbackCampaign)
	}
	if len(byKind["creative"]) == 0 && opts.CreatePlaceholders {
		files[filepath.Join(out, "creatives", "imported_creative.yml")] = defaultCreative("imported_creative")
	}

	sortedPaths := make([]string, 0, len(files))
	for p := range files {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)
	result.FilesPlanned = len(sortedPaths)
	emitProgress(opts.Progress, ProgressEvent{Stage: "scaffold", Current: len(files), Total: len(files), Done: true, Message: "yaml graph built"})

	if opts.DryRun {
		emitProgress(opts.Progress, ProgressEvent{Stage: "write-files", Current: 0, Total: len(sortedPaths), Done: true, Message: "dry-run: no writes"})
		return result, nil
	}
	emitProgress(opts.Progress, ProgressEvent{Stage: "write-files", Current: 0, Total: len(sortedPaths), Message: "writing files"})
	for _, path := range sortedPaths {
		content := files[path]
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				emitProgress(opts.Progress, ProgressEvent{Stage: "write-files", Current: result.FilesWritten, Total: len(sortedPaths), Message: "skipping existing"})
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return result, fmt.Errorf("create import dir: %w", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return result, fmt.Errorf("write import file %s: %w", path, err)
		}
		result.FilesWritten++
		emitProgress(opts.Progress, ProgressEvent{Stage: "write-files", Current: result.FilesWritten, Total: len(sortedPaths)})
	}
	emitProgress(opts.Progress, ProgressEvent{Stage: "write-files", Current: len(sortedPaths), Total: len(sortedPaths), Done: true})

	reportPath := filepath.Join(opts.Root, "reports", "import-"+time.Now().UTC().Format("20060102-1504")+".md")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err == nil {
		_ = os.WriteFile(reportPath, []byte(renderReport(result)), 0o644)
	}
	return result, nil
}

func fetchRemoteDetails(byKind map[string][]state.ResourceRow, remote RemoteDetailClient) (map[string]map[string]map[string]any, []string) {
	details := map[string]map[string]map[string]any{}
	if remote == nil {
		return details, nil
	}
	fields := map[string][]string{
		"campaign":    {"id", "name", "objective", "status", "special_ad_categories"},
		"adset":       {"id", "name", "status", "daily_budget", "bid_strategy", "optimization_goal", "billing_event", "start_time", "campaign_id", "targeting", "promoted_object"},
		"ad":          {"id", "name", "status", "adset_id", "creative"},
		"creative":    creativeDetailFields(),
		"audience":    audienceDetailFields(),
		"catalog":     {"id", "name"},
		"asset_image": {"hash", "name"},
		"asset_video": {"id", "title"},
	}

	warnings := make([]string, 0)
	for kind, rows := range byKind {
		kfields, ok := fields[kind]
		if !ok {
			continue
		}
		details[kind] = map[string]map[string]any{}
		for _, row := range rows {
			if row.MetaID == "" {
				continue
			}
			obj, err := remote.GetNode(row.MetaID, kfields...)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("remote detail skipped %s:%s: %v", kind, row.LogicalKey, err))
				continue
			}
			details[kind][row.LogicalKey] = obj
		}
	}
	return details, warnings
}

func renderAssets(byKind map[string][]state.ResourceRow) string {
	rows := append([]state.ResourceRow{}, byKind["asset_image"]...)
	rows = append(rows, byKind["asset_video"]...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].LogicalKey < rows[j].LogicalKey
	})
	manifest := config.AssetManifest{Assets: make([]config.Asset, 0, len(rows))}
	for _, row := range rows {
		a := config.Asset{Key: row.LogicalKey, Type: "image"}
		a.File = nil
		a.SHA256 = nil
		a.Meta.Origin = "imported"
		if row.Kind == "asset_video" {
			a.Type = "video"
			a.Meta.VideoID = ptr(row.MetaID)
		} else {
			a.Meta.ImageHash = ptr(row.MetaID)
		}
		manifest.Assets = append(manifest.Assets, a)
	}
	if len(manifest.Assets) == 0 {
		return "assets: []\n"
	}
	return mustYAML(manifest)
}

func renderAudiences(out string, rows []state.ResourceRow, detail map[string]map[string]any) map[string]string {
	files := map[string]string{}
	for _, row := range rows {
		aud := config.Audience{Key: row.LogicalKey, Type: "custom", MetaID: row.MetaID}
		if d, ok := detail[row.LogicalKey]; ok {
			if v := stringAny(d["name"]); v != "" {
				aud.Name = v
			}
			if v := stringAny(d["subtype"]); v != "" {
				aud.Subtype = v
			}
			if v := stringAny(d["description"]); v != "" {
				aud.Description = v
			}
			if v := intAny(d["retention_days"]); v > 0 {
				aud.RetentionDays = v
			}
			if v := stringAny(d["pixel_id"]); v != "" {
				aud.PixelID = v
			}
			if v := stringAny(d["source_audience_id"]); v != "" {
				aud.SourceAudienceID = v
			}
			if m, ok := mapAny(d["rule"]); ok {
				aud.Rule = m
			}
			if m, ok := mapAny(d["lookalike_spec"]); ok {
				aud.LookalikeSpec = m
			}
			if m, ok := mapAny(d["operation_status"]); ok {
				aud.OperationStatus = m
			}
			if m, ok := mapAny(d["data_source"]); ok {
				aud.DataSource = m
			}
		}
		p := filepath.Join(out, "audiences", row.LogicalKey+".yml")
		files[p] = mustYAML(aud)
	}
	return files
}

func renderCatalogs(out string, rows []state.ResourceRow, detail map[string]map[string]any) map[string]string {
	files := map[string]string{}
	for _, row := range rows {
		cat := config.Catalog{Key: row.LogicalKey, Type: "catalog", MetaID: row.MetaID, Name: "Imported Catalog"}
		if d, ok := detail[row.LogicalKey]; ok {
			if name := stringAny(d["name"]); name != "" {
				cat.Name = name
			}
		}
		p := filepath.Join(out, "catalogs", row.LogicalKey+".yml")
		files[p] = mustYAML(cat)
	}
	return files
}

func renderCreatives(out string, rows []state.ResourceRow, detail map[string]map[string]any, assetImageByMeta map[string]string, assetVideoByMeta map[string]string) map[string]string {
	files := map[string]string{}
	for _, row := range rows {
		c := config.Creative{Key: row.LogicalKey, Type: "reference", MetaID: row.MetaID, Name: "Imported Creative"}
		if d, ok := detail[row.LogicalKey]; ok {
			if name := stringAny(d["name"]); name != "" {
				c.Name = name
			}
			if v := stringAny(d["object_type"]); v != "" {
				c.ObjectType = v
			}
			if spec, ok := asMap(d["object_story_spec"]); ok {
				c.ObjectStorySpec = normalizeMapAny(spec)
				if v := stringAny(spec["page_id"]); v != "" {
					c.PageIDRef = v
				}
				if v := stringAny(spec["instagram_actor_id"]); v != "" {
					c.InstagramActorIDRef = v
				} else if v := stringAny(spec["instagram_user_id"]); v != "" {
					c.InstagramActorIDRef = v
				}
				fillCreativeFromStorySpec(&c, spec, assetImageByMeta)
			}
			if feed, ok := asMap(d["asset_feed_spec"]); ok {
				c.AssetFeedSpec = normalizeMapAny(feed)
				fillCreativeFromAssetFeedSpec(&c, feed, assetImageByMeta)
			}
			if dof, ok := asMap(d["degrees_of_freedom_spec"]); ok {
				c.DegreesOfFreedomSpec = normalizeMapAny(dof)
			}

			linked := collectCreativeAssetKeys(d, assetImageByMeta, assetVideoByMeta)
			if c.Link.ImageAssetKey != "" {
				linked = append(linked, c.Link.ImageAssetKey)
			}
			c.LinkedAssetKeys = uniqueStrings(linked)
			if len(c.LinkedAssetKeys) > 0 {
				c.AssetSource = "static_assets"
				if c.Link.ImageAssetKey == "" {
					c.Link.ImageAssetKey = c.LinkedAssetKeys[0]
				}
			} else if isCatalogDynamicCreative(d) {
				c.AssetSource = "catalog_dynamic"
			}
		}
		p := filepath.Join(out, "creatives", row.LogicalKey+".yml")
		files[p] = mustYAML(c)
	}
	return files
}

func renderCampaigns(out string, rows []state.ResourceRow, detail map[string]map[string]any, preserveStatus bool) map[string]string {
	files := map[string]string{}
	for _, row := range rows {
		c := config.Campaign{Key: row.LogicalKey, Name: "Imported Campaign", Objective: "SALES", Status: "PAUSED", SpecialAdCategories: []string{}}
		if d, ok := detail[row.LogicalKey]; ok {
			if v := stringAny(d["name"]); v != "" {
				c.Name = v
			}
			if v := stringAny(d["objective"]); v != "" {
				c.Objective = v
			}
			if preserveStatus {
				if v := strings.ToUpper(stringAny(d["status"])); v != "" {
					c.Status = v
				}
			}
			if arr := stringSliceAny(d["special_ad_categories"]); len(arr) > 0 {
				c.SpecialAdCategories = arr
			}
		}
		p := filepath.Join(out, "campaigns", row.LogicalKey, "campaign.yml")
		files[p] = mustYAML(c)
	}
	return files
}

func renderAdsets(
	out string,
	rows []state.ResourceRow,
	detail map[string]map[string]any,
	campaignByMeta map[string]string,
	audienceByMeta map[string]string,
	catalogByMeta map[string]string,
	fallbackCampaign string,
	preserveStatus bool,
) (map[string]string, map[string]string) {
	files := map[string]string{}
	adsetToCampaign := map[string]string{}

	for _, row := range rows {
		a := config.AdSet{}
		a.Key = row.LogicalKey
		a.CampaignKey = fallbackCampaign
		a.Name = "Imported Adset"
		a.Status = "PAUSED"
		a.DailyBudget = 100
		a.BidStrategy = "LOWEST_COST_WITHOUT_CAP"
		a.OptimizationGoal = "OFFSITE_CONVERSIONS"
		a.BillingEvent = "IMPRESSIONS"
		a.PromotedObject.PixelKey = "default"
		a.Targeting.CustomAudiences = []string{}

		if d, ok := detail[row.LogicalKey]; ok {
			if v := stringAny(d["campaign_id"]); v != "" {
				if key, ok := campaignByMeta[v]; ok {
					a.CampaignKey = key
				}
			}
			if v := stringAny(d["name"]); v != "" {
				a.Name = v
			}
			if preserveStatus {
				if v := strings.ToUpper(stringAny(d["status"])); v != "" {
					a.Status = v
				}
			}
			if v := minorToMajor(d["daily_budget"]); v > 0 {
				a.DailyBudget = v
			}
			if v := stringAny(d["bid_strategy"]); v != "" {
				a.BidStrategy = v
			}
			if v := stringAny(d["optimization_goal"]); v != "" {
				a.OptimizationGoal = v
			}
			if v := stringAny(d["billing_event"]); v != "" {
				a.BillingEvent = v
			}
			if v := stringAny(d["start_time"]); v != "" {
				a.Schedule.StartTime = v
			}

			if promoted, ok := asMap(d["promoted_object"]); ok {
				if v := stringAny(promoted["pixel_id"]); v != "" {
					a.PromotedObject.PixelKey = v
				}
				if v := stringAny(promoted["custom_event_type"]); v != "" {
					a.PromotedObject.EventType = v
				}
				catalogID := stringAny(promoted["product_catalog_id"])
				if catalogID != "" {
					a.PromotedObject.ProductCatalogID = catalogID
					if k, ok := catalogByMeta[catalogID]; ok {
						a.PromotedObject.CatalogKey = k
					}
				}
				if v := stringAny(promoted["product_set_id"]); v != "" {
					a.PromotedObject.ProductSetID = v
				}
			}

			if targeting, ok := asMap(d["targeting"]); ok {
				if v := intAny(targeting["age_min"]); v > 0 {
					a.Targeting.AgeMin = v
				}
				if v := intAny(targeting["age_max"]); v > 0 {
					a.Targeting.AgeMax = v
				}
				if a.Targeting.AgeMin == 0 || a.Targeting.AgeMax == 0 {
					if r := intSliceAny(targeting["age_range"]); len(r) >= 2 {
						if a.Targeting.AgeMin == 0 {
							a.Targeting.AgeMin = r[0]
						}
						if a.Targeting.AgeMax == 0 {
							a.Targeting.AgeMax = r[1]
						}
					}
				}
				if genders := intSliceAny(targeting["genders"]); len(genders) > 0 {
					a.Targeting.Genders = genders
				}
				if locales := intSliceAny(targeting["locales"]); len(locales) > 0 {
					a.Targeting.Locales = locales
				}
				if geo, ok := asMap(targeting["geo_locations"]); ok {
					if countries := stringSliceAny(geo["countries"]); len(countries) > 0 {
						a.Targeting.Geo.Countries = countries
					}
					if countryGroups := stringSliceAny(geo["country_groups"]); len(countryGroups) > 0 {
						a.Targeting.Geo.CountryGroups = countryGroups
					}
					if locationTypes := stringSliceAny(geo["location_types"]); len(locationTypes) > 0 {
						a.Targeting.Geo.LocationTypes = locationTypes
					}
					if regions := anySlice(geo["regions"]); len(regions) > 0 {
						parsed := make([]struct {
							Key  string `yaml:"key" json:"key"`
							Name string `yaml:"name" json:"name"`
						}, 0, len(regions))
						for _, r := range regions {
							m, ok := asMap(r)
							if !ok {
								continue
							}
							parsed = append(parsed, struct {
								Key  string `yaml:"key" json:"key"`
								Name string `yaml:"name" json:"name"`
							}{
								Key:  stringAny(m["key"]),
								Name: stringAny(m["name"]),
							})
						}
						if len(parsed) > 0 {
							a.Targeting.Geo.Regions = parsed
						}
					}
					if cities := anySlice(geo["cities"]); len(cities) > 0 {
						parsed := make([]struct {
							Key          string  `yaml:"key" json:"key"`
							Name         string  `yaml:"name" json:"name"`
							Radius       float64 `yaml:"radius,omitempty" json:"radius,omitempty"`
							DistanceUnit string  `yaml:"distance_unit,omitempty" json:"distance_unit,omitempty"`
						}, 0, len(cities))
						for _, c := range cities {
							m, ok := asMap(c)
							if !ok {
								continue
							}
							name := stringAny(m["name"])
							if name == "" {
								name = stringAny(m["city"])
							}
							parsed = append(parsed, struct {
								Key          string  `yaml:"key" json:"key"`
								Name         string  `yaml:"name" json:"name"`
								Radius       float64 `yaml:"radius,omitempty" json:"radius,omitempty"`
								DistanceUnit string  `yaml:"distance_unit,omitempty" json:"distance_unit,omitempty"`
							}{
								Key:          stringAny(m["key"]),
								Name:         name,
								Radius:       floatAny(m["radius"]),
								DistanceUnit: stringAny(m["distance_unit"]),
							})
						}
						if len(parsed) > 0 {
							a.Targeting.Geo.Cities = parsed
						}
					}
				}

				if ca := anySlice(targeting["custom_audiences"]); len(ca) > 0 {
					keys := make([]string, 0, len(ca))
					for _, v := range ca {
						if id := strings.TrimSpace(stringAny(v)); id != "" {
							if k, ok := audienceByMeta[id]; ok {
								keys = append(keys, k)
							} else {
								keys = append(keys, id)
							}
							continue
						}
						m, ok := asMap(v)
						if !ok {
							continue
						}
						id := stringAny(m["id"])
						if id == "" {
							continue
						}
						if k, ok := audienceByMeta[id]; ok {
							keys = append(keys, k)
						} else {
							keys = append(keys, id)
						}
					}
					a.Targeting.CustomAudiences = keys
				}
				if ca := anySlice(targeting["excluded_custom_audiences"]); len(ca) > 0 {
					keys := make([]string, 0, len(ca))
					for _, v := range ca {
						if id := strings.TrimSpace(stringAny(v)); id != "" {
							if k, ok := audienceByMeta[id]; ok {
								keys = append(keys, k)
							} else {
								keys = append(keys, id)
							}
							continue
						}
						m, ok := asMap(v)
						if !ok {
							continue
						}
						id := stringAny(m["id"])
						if id == "" {
							continue
						}
						if k, ok := audienceByMeta[id]; ok {
							keys = append(keys, k)
						} else {
							keys = append(keys, id)
						}
					}
					a.Targeting.ExcludedCustomAudiences = keys
				}
				if vals := stringSliceAny(targeting["publisher_platforms"]); len(vals) > 0 {
					a.Targeting.PublisherPlatforms = vals
				}
				if vals := stringSliceAny(targeting["facebook_positions"]); len(vals) > 0 {
					a.Targeting.FacebookPositions = vals
				}
				if vals := stringSliceAny(targeting["instagram_positions"]); len(vals) > 0 {
					a.Targeting.InstagramPositions = vals
				}
				if vals := stringSliceAny(targeting["audience_network_positions"]); len(vals) > 0 {
					a.Targeting.AudienceNetworkPositions = vals
				}
				if vals := stringSliceAny(targeting["messenger_positions"]); len(vals) > 0 {
					a.Targeting.MessengerPositions = vals
				}
				if vals := stringSliceAny(targeting["device_platforms"]); len(vals) > 0 {
					a.Targeting.DevicePlatforms = vals
				}
				if v := normalizeAny(targeting["flexible_spec"]); v != nil {
					a.Targeting.FlexibleSpec = v
				}
				if v := normalizeAny(targeting["targeting_automation"]); v != nil {
					a.Targeting.TargetingAutomation = v
				}

				if hasManualPlacements(targeting) {
					a.Targeting.Placements = "manual"
				} else if a.Targeting.Placements == "" {
					a.Targeting.Placements = "advantage_plus"
				}
			}
		}

		if a.CampaignKey == "" {
			a.CampaignKey = fallbackCampaign
		}
		if a.CampaignKey == "" {
			continue
		}

		adsetToCampaign[a.Key] = a.CampaignKey
		p := filepath.Join(out, "campaigns", a.CampaignKey, "adsets", row.LogicalKey, "adset.yml")
		files[p] = mustYAML(a)
	}
	return files, adsetToCampaign
}

func renderAds(
	out string,
	rows []state.ResourceRow,
	detail map[string]map[string]any,
	adsetByMeta map[string]string,
	creativeByMeta map[string]string,
	adsetToCampaign map[string]string,
	fallbackCampaign string,
	fallbackAdset string,
	fallbackCreative string,
	preserveStatus bool,
) map[string]string {
	files := map[string]string{}
	for _, row := range rows {
		a := config.Ad{}
		a.Key = row.LogicalKey
		a.AdsetKey = fallbackAdset
		a.CreativeKey = fallbackCreative
		a.Name = "Imported Ad"
		a.Status = "PAUSED"
		a.Tracking.UTM.Source = "facebook"
		a.Tracking.UTM.Medium = "paid_social"
		a.Tracking.UTM.Campaign = "{{campaign.key}}"
		a.Tracking.UTM.Content = "{{creative.key}}"

		if d, ok := detail[row.LogicalKey]; ok {
			if v := stringAny(d["name"]); v != "" {
				a.Name = v
			}
			if preserveStatus {
				if v := strings.ToUpper(stringAny(d["status"])); v != "" {
					a.Status = v
				}
			}
			if adsetID := stringAny(d["adset_id"]); adsetID != "" {
				if key, ok := adsetByMeta[adsetID]; ok {
					a.AdsetKey = key
				}
			}
			if cm, ok := asMap(d["creative"]); ok {
				cid := stringAny(cm["id"])
				if cid == "" {
					cid = stringAny(cm["creative_id"])
				}
				if cid != "" {
					if key, ok := creativeByMeta[cid]; ok {
						a.CreativeKey = key
					}
				}
			}
		}
		if a.AdsetKey == "" || a.CreativeKey == "" {
			continue
		}
		campaignKey := adsetToCampaign[a.AdsetKey]
		if campaignKey == "" {
			campaignKey = fallbackCampaign
		}
		if campaignKey == "" {
			continue
		}
		p := filepath.Join(out, "campaigns", campaignKey, "adsets", a.AdsetKey, "ads", row.LogicalKey+".yml")
		files[p] = mustYAML(a)
	}
	return files
}

func hasManualPlacements(targeting map[string]any) bool {
	for _, key := range []string{"publisher_platforms", "facebook_positions", "instagram_positions", "audience_network_positions", "device_platforms", "messenger_positions"} {
		if vals := anySlice(targeting[key]); len(vals) > 0 {
			return true
		}
	}
	return false
}

func defaultAccount(account string, cfg *config.AccountConfig) string {
	if cfg == nil {
		return "account_name: " + account + "\n\n" +
			"meta:\n" +
			"  ad_account_id: \"act_0000000000\"\n" +
			"  currency: \"USD\"\n" +
			"  timezone: \"America/New_York\"\n" +
			"  page_id: \"\"\n" +
			"  instagram_actor_id: \"\"\n" +
			"  pixel_key_default: \"default-pixel\"\n" +
			"  product_feed_url: \"\"\n\n" +
			"budgets:\n" +
			"  unit: major\n\n" +
			"policies:\n" +
			"  no_delete: true\n" +
			"  allow_activate: false\n" +
			"  orphan:\n" +
			"    on_missing_in_config: pause\n" +
			"  budget:\n" +
			"    max_increase_ratio: 0.20\n" +
			"    max_decrease_ratio: 0.50\n" +
			"    max_daily_budget_major: 20000\n\n" +
			"naming:\n" +
			"  campaign_prefix: \"\"\n" +
			"  adset_prefix: \"\"\n"
	}
	clone := *cfg
	if clone.AccountName == "" {
		clone.AccountName = account
	}
	return mustYAML(clone)
}

func defaultCampaign(key string) string {
	return mustYAML(config.Campaign{Key: key, Name: "Imported Campaign", Objective: "SALES", Status: "PAUSED", SpecialAdCategories: []string{}})
}

func defaultAdset(key, campaignKey string) string {
	a := config.AdSet{}
	a.Key = key
	a.CampaignKey = campaignKey
	a.Name = "Imported Adset"
	a.Status = "PAUSED"
	a.DailyBudget = 100
	a.BidStrategy = "LOWEST_COST_WITHOUT_CAP"
	a.OptimizationGoal = "OFFSITE_CONVERSIONS"
	a.BillingEvent = "IMPRESSIONS"
	a.PromotedObject.PixelKey = "default-pixel"
	a.PromotedObject.EventType = "PURCHASE"
	a.Targeting.Geo.Countries = []string{"US"}
	a.Targeting.CustomAudiences = []string{}
	a.Targeting.Placements = "advantage_plus"
	a.Schedule.StartTime = "2026-01-01T00:00:00-05:00"
	return mustYAML(a)
}

func defaultCreative(key string) string {
	return mustYAML(config.Creative{Key: key, Type: "reference", MetaID: "23850000000000000", Name: "Imported Creative"})
}

func renderReport(result Result) string {
	var b strings.Builder
	b.WriteString("# adform import report\n\n")
	b.WriteString("- out_path: `" + result.OutPath + "`\n")
	b.WriteString(fmt.Sprintf("- dry_run: `%t`\n", result.DryRun))
	b.WriteString(fmt.Sprintf("- resources_seen: `%d`\n", result.ResourcesSeen))
	b.WriteString(fmt.Sprintf("- files_planned: `%d`\n", result.FilesPlanned))
	b.WriteString(fmt.Sprintf("- files_written: `%d`\n", result.FilesWritten))
	if len(result.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range result.Warnings {
			b.WriteString("- " + w + "\n")
		}
	}
	return b.String()
}

func creativeDetailFields() []string {
	return []string{
		"id",
		"name",
		"object_story_spec",
		"asset_feed_spec",
		"degrees_of_freedom_spec",
		"url_tags",
		"template_url",
		"object_type",
	}
}

func audienceDetailFields() []string {
	return []string{
		"id",
		"name",
		"subtype",
		"description",
		"rule",
		"lookalike_spec",
		"time_created",
		"retention_days",
		"operation_status",
		"data_source",
		"pixel_id",
		"source_audience_id",
	}
}

func fillCreativeFromStorySpec(c *config.Creative, spec map[string]any, assetImageByMeta map[string]string) {
	if linkData, ok := asMap(spec["link_data"]); ok {
		setIfEmpty(&c.Link.URL, stringAny(linkData["link"]))
		setIfEmpty(&c.Link.Message, stringAny(linkData["message"]))
		setIfEmpty(&c.Link.Headline, stringAny(linkData["name"]))
		setIfEmpty(&c.Link.Description, stringAny(linkData["description"]))
		if cta, ok := asMap(linkData["call_to_action"]); ok {
			setIfEmpty(&c.Link.CallToActionType, stringAny(cta["type"]))
		}
		if imageAsset := assetKeyFromCreativeMap(linkData, assetImageByMeta); imageAsset != "" {
			setIfEmpty(&c.Link.ImageAssetKey, imageAsset)
		}
		if c.Link.URL == "" {
			if cta, ok := asMap(linkData["call_to_action"]); ok {
				if value, ok := asMap(cta["value"]); ok {
					setIfEmpty(&c.Link.URL, stringAny(value["link"]))
				}
			}
		}
		if children := anySlice(linkData["child_attachments"]); len(children) > 0 {
			if first, ok := asMap(children[0]); ok {
				setIfEmpty(&c.Link.URL, stringAny(first["link"]))
				setIfEmpty(&c.Link.Headline, stringAny(first["name"]))
				setIfEmpty(&c.Link.Description, stringAny(first["description"]))
				if imageAsset := assetKeyFromCreativeMap(first, assetImageByMeta); imageAsset != "" {
					setIfEmpty(&c.Link.ImageAssetKey, imageAsset)
				}
			}
		}
	}
	if videoData, ok := asMap(spec["video_data"]); ok {
		setIfEmpty(&c.Link.Message, stringAny(videoData["message"]))
		if cta, ok := asMap(videoData["call_to_action"]); ok {
			setIfEmpty(&c.Link.CallToActionType, stringAny(cta["type"]))
			if value, ok := asMap(cta["value"]); ok {
				setIfEmpty(&c.Link.URL, stringAny(value["link"]))
			}
		}
	}
	if templateData, ok := asMap(spec["template_data"]); ok {
		setIfEmpty(&c.Link.URL, stringAny(templateData["link"]))
		setIfEmpty(&c.Link.Message, stringAny(templateData["message"]))
		setIfEmpty(&c.Link.Headline, stringAny(templateData["name"]))
		setIfEmpty(&c.Link.Description, stringAny(templateData["description"]))
		if imageAsset := assetKeyFromCreativeMap(templateData, assetImageByMeta); imageAsset != "" {
			setIfEmpty(&c.Link.ImageAssetKey, imageAsset)
		}
	}
}

func fillCreativeFromAssetFeedSpec(c *config.Creative, feed map[string]any, assetImageByMeta map[string]string) {
	if values := anySlice(feed["link_urls"]); len(values) > 0 {
		c.LinkURLVariants = uniqueStrings(extractTextField(values, "website_url", "url"))
		if first, ok := asMap(values[0]); ok {
			setIfEmpty(&c.Link.URL, stringAny(first["website_url"]))
			setIfEmpty(&c.Link.URL, stringAny(first["url"]))
		}
	}
	if values := anySlice(feed["bodies"]); len(values) > 0 {
		c.BodyVariants = uniqueStrings(extractTextField(values, "text"))
		if first, ok := asMap(values[0]); ok {
			setIfEmpty(&c.Link.Message, stringAny(first["text"]))
		}
	}
	if values := anySlice(feed["titles"]); len(values) > 0 {
		c.HeadlineVariants = uniqueStrings(extractTextField(values, "text"))
		if first, ok := asMap(values[0]); ok {
			setIfEmpty(&c.Link.Headline, stringAny(first["text"]))
		}
	}
	if values := anySlice(feed["descriptions"]); len(values) > 0 {
		c.DescriptionVariants = uniqueStrings(extractTextField(values, "text"))
		if first, ok := asMap(values[0]); ok {
			setIfEmpty(&c.Link.Description, stringAny(first["text"]))
		}
	}
	c.AdFormats = uniqueStrings(stringSliceAny(feed["ad_formats"]))
	if ctaTypes := stringSliceAny(feed["call_to_action_types"]); len(ctaTypes) > 0 {
		setIfEmpty(&c.Link.CallToActionType, ctaTypes[0])
	}
	if values := anySlice(feed["images"]); len(values) > 0 {
		if first, ok := asMap(values[0]); ok {
			if imageAsset := assetKeyFromCreativeMap(first, assetImageByMeta); imageAsset != "" {
				setIfEmpty(&c.Link.ImageAssetKey, imageAsset)
			}
		}
	}
}

func assetKeyFromCreativeMap(v map[string]any, assetImageByMeta map[string]string) string {
	hash := strings.TrimSpace(stringAny(v["image_hash"]))
	if hash == "" {
		hash = strings.TrimSpace(stringAny(v["hash"]))
	}
	if hash == "" {
		return ""
	}
	if key, ok := assetImageByMeta[hash]; ok {
		return key
	}
	return ""
}

func collectCreativeAssetKeys(creative map[string]any, assetImageByMeta map[string]string, assetVideoByMeta map[string]string) []string {
	keys := make([]string, 0, 8)
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}

	for _, hash := range collectStringValuesByKey(creative, "image_hash", "hash") {
		if k, ok := assetImageByMeta[hash]; ok {
			add(k)
		}
	}
	for _, videoID := range collectStringValuesByKey(creative, "video_id") {
		if k, ok := assetVideoByMeta[videoID]; ok {
			add(k)
		}
	}
	return uniqueStrings(keys)
}

func collectStringValuesByKey(v any, keys ...string) []string {
	wanted := map[string]struct{}{}
	for _, key := range keys {
		wanted[key] = struct{}{}
	}
	out := []string{}
	var walk func(any)
	walk = func(node any) {
		switch t := node.(type) {
		case map[string]any:
			for k, child := range t {
				if _, ok := wanted[k]; ok {
					if s := strings.TrimSpace(stringAny(child)); s != "" {
						out = append(out, s)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(v)
	return uniqueStrings(out)
}

func isCatalogDynamicCreative(creative map[string]any) bool {
	if containsStringPattern(creative, "{{product.") {
		return true
	}
	if dof, ok := asMap(creative["degrees_of_freedom_spec"]); ok {
		if fs, ok := asMap(dof["creative_features_spec"]); ok {
			if _, ok := fs["dynamic_partner_content"]; ok {
				return true
			}
			if _, ok := fs["product_metadata_automation"]; ok {
				return true
			}
		}
	}
	if feed, ok := asMap(creative["asset_feed_spec"]); ok {
		formats := map[string]struct{}{}
		for _, f := range stringSliceAny(feed["ad_formats"]) {
			formats[strings.ToUpper(strings.TrimSpace(f))] = struct{}{}
		}
		_, isCarousel := formats["CAROUSEL"]
		_, isCollection := formats["COLLECTION"]
		if (isCarousel || isCollection) && len(anySlice(feed["images"])) == 0 && len(anySlice(feed["videos"])) == 0 {
			return true
		}
	}
	return false
}

func containsStringPattern(v any, needle string) bool {
	needle = strings.TrimSpace(strings.ToLower(needle))
	if needle == "" {
		return false
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch t := node.(type) {
		case string:
			return strings.Contains(strings.ToLower(t), needle)
		case map[string]any:
			for _, child := range t {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range t {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(v)
}

func setIfEmpty(dst *string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || *dst != "" {
		return
	}
	*dst = value
}

func extractTextField(values []any, keys ...string) []string {
	out := make([]string, 0, len(values))
	for _, raw := range values {
		m, ok := asMap(raw)
		if !ok {
			continue
		}
		for _, key := range keys {
			v := strings.TrimSpace(stringAny(m[key]))
			if v != "" {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func normalizeMapAny(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalizeAny(v)
	}
	return out
}

func normalizeAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return normalizeMapAny(t)
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, normalizeAny(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, normalizeAny(item))
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return t
		}
		if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) || (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
			var decoded any
			if err := json.Unmarshal([]byte(s), &decoded); err == nil {
				return normalizeAny(decoded)
			}
		}
		return t
	default:
		return v
	}
}

func merge(base, extra map[string]string) map[string]string {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

func extractKeys(rows []state.ResourceRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.LogicalKey)
	}
	sort.Strings(keys)
	return keys
}

func metaToKeyMap(rows []state.ResourceRow) map[string]string {
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.MetaID != "" {
			out[row.MetaID] = row.LogicalKey
		}
	}
	return out
}

func mergeDetails(dst, src map[string]map[string]map[string]any) {
	for kind, byKey := range src {
		if dst[kind] == nil {
			dst[kind] = map[string]map[string]any{}
		}
		for key, obj := range byKey {
			dst[kind][key] = obj
		}
	}
}

func mustYAML(v any) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func ptr(s string) *string { return &s }

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func mapAny(v any) (map[string]any, bool) {
	if m, ok := asMap(v); ok {
		return normalizeMapAny(m), true
	}
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err != nil {
			return nil, false
		}
		if m, ok := decoded.(map[string]any); ok {
			return normalizeMapAny(m), true
		}
	}
	return nil, false
}

func anySlice(v any) []any {
	s, ok := v.([]any)
	if ok {
		return s
	}
	return nil
}

func stringSliceAny(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if sarr, ok := v.([]string); ok {
			return sarr
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := stringAny(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func stringAny(v any) string {
	s, _ := v.(string)
	return s
}

func floatAny(v any) float64 {
	s := stringAny(v)
	if s != "" {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func intAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(x))
		return i
	default:
		return int(floatAny(v))
	}
}

func intSliceAny(v any) []int {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		if n := intAny(item); n != 0 {
			out = append(out, n)
		}
	}
	return out
}

func minorToMajor(v any) float64 {
	f := floatAny(v)
	if f <= 0 {
		return 0
	}
	return f / 100
}
