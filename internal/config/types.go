package config

type AccountConfig struct {
	AccountName string `yaml:"account_name" json:"account_name"`
	Meta        struct {
		AdAccountID      string `yaml:"ad_account_id" json:"ad_account_id"`
		Currency         string `yaml:"currency" json:"currency"`
		Timezone         string `yaml:"timezone" json:"timezone"`
		PageID           string `yaml:"page_id" json:"page_id"`
		InstagramActorID string `yaml:"instagram_actor_id" json:"instagram_actor_id"`
		PixelKeyDefault  string `yaml:"pixel_key_default" json:"pixel_key_default"`
		ProductFeedURL   string `yaml:"product_feed_url" json:"product_feed_url"`
	} `yaml:"meta" json:"meta"`
	Budgets struct {
		Unit string `yaml:"unit" json:"unit"`
	} `yaml:"budgets" json:"budgets"`
	Policies struct {
		NoDelete      bool `yaml:"no_delete" json:"no_delete"`
		AllowActivate bool `yaml:"allow_activate" json:"allow_activate"`
		Orphan        struct {
			OnMissingInConfig string `yaml:"on_missing_in_config" json:"on_missing_in_config"`
		} `yaml:"orphan" json:"orphan"`
		Budget struct {
			MaxIncreaseRatio    float64 `yaml:"max_increase_ratio" json:"max_increase_ratio"`
			MaxDecreaseRatio    float64 `yaml:"max_decrease_ratio" json:"max_decrease_ratio"`
			MaxDailyBudgetMajor float64 `yaml:"max_daily_budget_major" json:"max_daily_budget_major"`
		} `yaml:"budget" json:"budget"`
	} `yaml:"policies" json:"policies"`
	Naming struct {
		CampaignPrefix string `yaml:"campaign_prefix" json:"campaign_prefix"`
		AdsetPrefix    string `yaml:"adset_prefix" json:"adset_prefix"`
	} `yaml:"naming" json:"naming"`
}

type AssetManifest struct {
	Assets []Asset `yaml:"assets" json:"assets"`
}

type Asset struct {
	Key    string  `yaml:"key" json:"key"`
	Type   string  `yaml:"type" json:"type"`
	File   *string `yaml:"file" json:"file,omitempty"`
	SHA256 *string `yaml:"sha256" json:"sha256,omitempty"`
	Meta   struct {
		ImageHash *string `yaml:"image_hash" json:"image_hash,omitempty"`
		VideoID   *string `yaml:"video_id" json:"video_id,omitempty"`
		Origin    string  `yaml:"origin" json:"origin"`
	} `yaml:"meta" json:"meta"`
}

type Audience struct {
	Key                 string         `yaml:"key" json:"key"`
	Name                string         `yaml:"name,omitempty" json:"name,omitempty"`
	Type                string         `yaml:"type" json:"type"`
	Subtype             string         `yaml:"subtype,omitempty" json:"subtype,omitempty"`
	Description         string         `yaml:"description,omitempty" json:"description,omitempty"`
	RetentionDays       int            `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
	PixelID             string         `yaml:"pixel_id,omitempty" json:"pixel_id,omitempty"`
	SourceAudienceID    string         `yaml:"source_audience_id,omitempty" json:"source_audience_id,omitempty"`
	Rule                map[string]any `yaml:"rule,omitempty" json:"rule,omitempty"`
	LookalikeSpec       map[string]any `yaml:"lookalike_spec,omitempty" json:"lookalike_spec,omitempty"`
	OperationStatus     map[string]any `yaml:"operation_status,omitempty" json:"operation_status,omitempty"`
	DataSource          map[string]any `yaml:"data_source,omitempty" json:"data_source,omitempty"`
	RuleJSON            string         `yaml:"rule_json,omitempty" json:"rule_json,omitempty"`
	LookalikeSpecJSON   string         `yaml:"lookalike_spec_json,omitempty" json:"lookalike_spec_json,omitempty"`
	OperationStatusJSON string         `yaml:"operation_status_json,omitempty" json:"operation_status_json,omitempty"`
	DataSourceJSON      string         `yaml:"data_source_json,omitempty" json:"data_source_json,omitempty"`
	MetaID              string         `yaml:"meta_id" json:"meta_id"`
	ImportedJSON        string         `yaml:"imported_json,omitempty" json:"imported_json,omitempty"`
	Raw                 map[string]any `yaml:"raw,omitempty" json:"raw,omitempty"`
}

type Catalog struct {
	Key    string `yaml:"key" json:"key"`
	Type   string `yaml:"type" json:"type"`
	MetaID string `yaml:"meta_id" json:"meta_id"`
	Name   string `yaml:"name" json:"name"`
}

type Campaign struct {
	Key                 string   `yaml:"key" json:"key"`
	Name                string   `yaml:"name" json:"name"`
	Objective           string   `yaml:"objective" json:"objective"`
	Status              string   `yaml:"status" json:"status"`
	SpecialAdCategories []string `yaml:"special_ad_categories" json:"special_ad_categories"`
}

type AdSet struct {
	Key              string  `yaml:"key" json:"key"`
	CampaignKey      string  `yaml:"campaign_key" json:"campaign_key"`
	Name             string  `yaml:"name" json:"name"`
	Status           string  `yaml:"status" json:"status"`
	DailyBudget      float64 `yaml:"daily_budget" json:"daily_budget"`
	BidStrategy      string  `yaml:"bid_strategy" json:"bid_strategy"`
	OptimizationGoal string  `yaml:"optimization_goal" json:"optimization_goal"`
	BillingEvent     string  `yaml:"billing_event" json:"billing_event"`
	PromotedObject   struct {
		PixelKey         string `yaml:"pixel_key" json:"pixel_key"`
		EventType        string `yaml:"event_type" json:"event_type"`
		CatalogKey       string `yaml:"catalog_key" json:"catalog_key"`
		ProductSetID     string `yaml:"product_set_id" json:"product_set_id"`
		ProductCatalogID string `yaml:"product_catalog_id" json:"product_catalog_id"`
	} `yaml:"promoted_object" json:"promoted_object"`
	Targeting struct {
		AgeMin  int   `yaml:"age_min,omitempty" json:"age_min,omitempty"`
		AgeMax  int   `yaml:"age_max,omitempty" json:"age_max,omitempty"`
		Genders []int `yaml:"genders,omitempty" json:"genders,omitempty"`
		Locales []int `yaml:"locales,omitempty" json:"locales,omitempty"`
		Geo     struct {
			Countries     []string `yaml:"countries" json:"countries"`
			CountryGroups []string `yaml:"country_groups,omitempty" json:"country_groups,omitempty"`
			LocationTypes []string `yaml:"location_types,omitempty" json:"location_types,omitempty"`
			Regions       []struct {
				Key  string `yaml:"key" json:"key"`
				Name string `yaml:"name" json:"name"`
			} `yaml:"regions,omitempty" json:"regions,omitempty"`
			Cities []struct {
				Key          string  `yaml:"key" json:"key"`
				Name         string  `yaml:"name" json:"name"`
				Radius       float64 `yaml:"radius,omitempty" json:"radius,omitempty"`
				DistanceUnit string  `yaml:"distance_unit,omitempty" json:"distance_unit,omitempty"`
			} `yaml:"cities,omitempty" json:"cities,omitempty"`
		} `yaml:"geo" json:"geo"`
		CustomAudiences          []string `yaml:"custom_audiences" json:"custom_audiences"`
		ExcludedCustomAudiences  []string `yaml:"excluded_custom_audiences,omitempty" json:"excluded_custom_audiences,omitempty"`
		PublisherPlatforms       []string `yaml:"publisher_platforms,omitempty" json:"publisher_platforms,omitempty"`
		FacebookPositions        []string `yaml:"facebook_positions,omitempty" json:"facebook_positions,omitempty"`
		InstagramPositions       []string `yaml:"instagram_positions,omitempty" json:"instagram_positions,omitempty"`
		AudienceNetworkPositions []string `yaml:"audience_network_positions,omitempty" json:"audience_network_positions,omitempty"`
		MessengerPositions       []string `yaml:"messenger_positions,omitempty" json:"messenger_positions,omitempty"`
		DevicePlatforms          []string `yaml:"device_platforms,omitempty" json:"device_platforms,omitempty"`
		Placements               string   `yaml:"placements" json:"placements"`
		FlexibleSpec             any      `yaml:"flexible_spec,omitempty" json:"flexible_spec,omitempty"`
		TargetingAutomation      any      `yaml:"targeting_automation,omitempty" json:"targeting_automation,omitempty"`
		FlexibleSpecJSON         string   `yaml:"flexible_spec_json,omitempty" json:"flexible_spec_json,omitempty"`
		TargetingAutomationJSON  string   `yaml:"targeting_automation_json,omitempty" json:"targeting_automation_json,omitempty"`
	} `yaml:"targeting" json:"targeting"`
	Schedule struct {
		StartTime string `yaml:"start_time" json:"start_time"`
	} `yaml:"schedule" json:"schedule"`
	ImportedTargetingJSON      string         `yaml:"imported_targeting_json,omitempty" json:"imported_targeting_json,omitempty"`
	ImportedPromotedObjectJSON string         `yaml:"imported_promoted_object_json,omitempty" json:"imported_promoted_object_json,omitempty"`
	TargetingRaw               map[string]any `yaml:"targeting_raw,omitempty" json:"targeting_raw,omitempty"`
	PromotedObjectRaw          map[string]any `yaml:"promoted_object_raw,omitempty" json:"promoted_object_raw,omitempty"`
}

type Creative struct {
	Key                 string `yaml:"key" json:"key"`
	Name                string `yaml:"name" json:"name"`
	Type                string `yaml:"type" json:"type"`
	MetaID              string `yaml:"meta_id" json:"meta_id"`
	PageIDRef           string `yaml:"page_id_ref" json:"page_id_ref"`
	InstagramActorIDRef string `yaml:"instagram_actor_id_ref" json:"instagram_actor_id_ref"`
	Link                struct {
		URL              string `yaml:"url" json:"url"`
		Message          string `yaml:"message" json:"message"`
		Headline         string `yaml:"headline" json:"headline"`
		Description      string `yaml:"description" json:"description"`
		CallToActionType string `yaml:"call_to_action_type,omitempty" json:"call_to_action_type,omitempty"`
		ImageAssetKey    string `yaml:"image_asset_key" json:"image_asset_key"`
	} `yaml:"link" json:"link"`
	ObjectType              string         `yaml:"object_type,omitempty" json:"object_type,omitempty"`
	AssetSource             string         `yaml:"asset_source,omitempty" json:"asset_source,omitempty"`
	LinkedAssetKeys         []string       `yaml:"linked_asset_keys,omitempty" json:"linked_asset_keys,omitempty"`
	AdFormats               []string       `yaml:"ad_formats,omitempty" json:"ad_formats,omitempty"`
	BodyVariants            []string       `yaml:"body_variants,omitempty" json:"body_variants,omitempty"`
	HeadlineVariants        []string       `yaml:"headline_variants,omitempty" json:"headline_variants,omitempty"`
	DescriptionVariants     []string       `yaml:"description_variants,omitempty" json:"description_variants,omitempty"`
	LinkURLVariants         []string       `yaml:"link_url_variants,omitempty" json:"link_url_variants,omitempty"`
	ImportedJSON            string         `yaml:"imported_json,omitempty" json:"imported_json,omitempty"`
	ObjectStorySpec         map[string]any `yaml:"object_story_spec,omitempty" json:"object_story_spec,omitempty"`
	AssetFeedSpec           map[string]any `yaml:"asset_feed_spec,omitempty" json:"asset_feed_spec,omitempty"`
	DegreesOfFreedomSpec    map[string]any `yaml:"degrees_of_freedom_spec,omitempty" json:"degrees_of_freedom_spec,omitempty"`
	ObjectStorySpecRaw      map[string]any `yaml:"object_story_spec_raw,omitempty" json:"object_story_spec_raw,omitempty"`
	AssetFeedSpecRaw        map[string]any `yaml:"asset_feed_spec_raw,omitempty" json:"asset_feed_spec_raw,omitempty"`
	DegreesOfFreedomSpecRaw map[string]any `yaml:"degrees_of_freedom_spec_raw,omitempty" json:"degrees_of_freedom_spec_raw,omitempty"`
	Raw                     map[string]any `yaml:"raw,omitempty" json:"raw,omitempty"`
}

type Ad struct {
	Key         string `yaml:"key" json:"key"`
	AdsetKey    string `yaml:"adset_key" json:"adset_key"`
	CreativeKey string `yaml:"creative_key" json:"creative_key"`
	Name        string `yaml:"name" json:"name"`
	Status      string `yaml:"status" json:"status"`
	Tracking    struct {
		UTM struct {
			Source   string `yaml:"source" json:"source"`
			Medium   string `yaml:"medium" json:"medium"`
			Campaign string `yaml:"campaign" json:"campaign"`
			Content  string `yaml:"content" json:"content"`
		} `yaml:"utm" json:"utm"`
	} `yaml:"tracking" json:"tracking"`
}

type Bundle struct {
	Root       string        `json:"root"`
	Account    string        `json:"account"`
	AccountCfg AccountConfig `json:"account_cfg"`
	Assets     map[string]Asset
	Audiences  map[string]Audience
	Catalogs   map[string]Catalog
	Creatives  map[string]Creative
	Campaigns  map[string]Campaign
	Adsets     map[string]AdSet
	Ads        map[string]Ad
}

func NewBundle(root, account string) Bundle {
	return Bundle{
		Root:      root,
		Account:   account,
		Assets:    make(map[string]Asset),
		Audiences: make(map[string]Audience),
		Catalogs:  make(map[string]Catalog),
		Creatives: make(map[string]Creative),
		Campaigns: make(map[string]Campaign),
		Adsets:    make(map[string]AdSet),
		Ads:       make(map[string]Ad),
	}
}
