package landing

import "time"

type SiteConfig struct {
	Version int `yaml:"version"`
	Runtime struct {
		Bind            string `yaml:"bind"`
		PublicBaseURL   string `yaml:"public_base_url"`
		MainSiteBaseURL string `yaml:"main_site_base_url"`
	} `yaml:"runtime"`
	Scripts struct {
		URLs   []string `yaml:"urls"`
		Inline []string `yaml:"inline"`
	} `yaml:"scripts"`
	Tracking struct {
		CookieDomain      string   `yaml:"cookie_domain"`
		AttributionCookie string   `yaml:"attribution_cookie"`
		VariantCookie     string   `yaml:"variant_cookie"`
		AttributionTTLDay int      `yaml:"attribution_ttl_days"`
		VariantTTLDays    int      `yaml:"variant_ttl_days"`
		CaptureQueryParam []string `yaml:"capture_query_params"`
		UTMPassthrough    struct {
			Enabled   bool     `yaml:"enabled"`
			Allowlist []string `yaml:"allowlist"`
		} `yaml:"utm_passthrough"`
	} `yaml:"tracking"`
	PostHog struct {
		Enabled   bool   `yaml:"enabled"`
		Host      string `yaml:"host"`
		APIKeyEnv string `yaml:"api_key_env"`
		Events    struct {
			Impression   string `yaml:"impression"`
			CTAClick     string `yaml:"cta_click"`
			ProductClick string `yaml:"product_click"`
		} `yaml:"events"`
	} `yaml:"posthog"`
	MetaPixel struct {
		Enabled            bool   `yaml:"enabled"`
		PixelID            string `yaml:"pixel_id"`
		CAPIAccessTokenEnv string `yaml:"capi_access_token_env"`
	} `yaml:"meta_pixel"`
	Bandit struct {
		Enabled               bool    `yaml:"enabled"`
		Algorithm             string  `yaml:"algorithm"`
		UpdateIntervalMinutes int     `yaml:"update_interval_minutes"`
		MinImpressionsPerArm  int     `yaml:"min_impressions_per_arm"`
		ControlMinShare       float64 `yaml:"control_min_share"`
		Storage               struct {
			Type       string `yaml:"type"`
			SQLitePath string `yaml:"sqlite_path"`
			Redis      struct {
				Addr        string `yaml:"addr"`
				PasswordEnv string `yaml:"password_env"`
				DB          int    `yaml:"db"`
				KeyPrefix   string `yaml:"key_prefix"`
			} `yaml:"redis"`
		} `yaml:"storage"`
		Objective struct {
			Primary   string  `yaml:"primary"`
			Secondary *string `yaml:"secondary"`
		} `yaml:"objective"`
	} `yaml:"bandit"`
	Defaults struct {
		Locale     string      `yaml:"locale"`
		Currency   string      `yaml:"currency"`
		TrustItems []TrustItem `yaml:"trust_items"`
	} `yaml:"defaults"`
}

type TrustItem struct {
	Icon  string `yaml:"icon" json:"icon,omitempty"`
	Title string `yaml:"title" json:"title,omitempty"`
	Body  string `yaml:"body" json:"body,omitempty"`
}

type PageFile struct {
	Version int      `yaml:"version"`
	Page    PageMeta `yaml:"page"`
	Blocks  []Block  `yaml:"blocks"`
}

type PageMeta struct {
	Key  string `yaml:"key"`
	Slug string `yaml:"slug"`
	Type string `yaml:"type"`
	SEO  struct {
		Title       string `yaml:"title"`
		Description string `yaml:"description"`
	} `yaml:"seo"`
}

type Block struct {
	Type      string        `yaml:"type"`
	Key       string        `yaml:"key"`
	Analytics BlockAnalytic `yaml:"analytics"`

	Spacer      *SpacerBlock      `yaml:"-"`
	Hero        *HeroBlock        `yaml:"-"`
	MediaSplit  *MediaSplitBlock  `yaml:"-"`
	ProductGrid *ProductGridBlock `yaml:"-"`
	Columns     *ColumnsBlock     `yaml:"-"`
	TrustStrip  *TrustStripBlock  `yaml:"-"`
	FAQ         *FAQBlock         `yaml:"-"`
	Pairings    *PairingsBlock    `yaml:"-"`
}

type BlockAnalytic struct {
	Tag string `yaml:"tag"`
}

type CTA struct {
	Label string `yaml:"label" json:"label,omitempty"`
	Href  string `yaml:"href" json:"href,omitempty"`
}

type SpacerBlock struct {
	Size string `yaml:"size"`
}

type HeroBlock struct {
	H1              string      `yaml:"h1"`
	Subhead         string      `yaml:"subhead"`
	Body            string      `yaml:"body"`
	BGImageAssetKey string      `yaml:"bg_image_asset_key"`
	Overlay         HeroOverlay `yaml:"overlay"`
	PrimaryCTA      *CTA        `yaml:"primary_cta"`
	SecondaryCTA    *CTA        `yaml:"secondary_cta"`
	Bandit          *HeroBandit `yaml:"bandit"`
}

type HeroOverlay struct {
	Opacity float64 `yaml:"opacity"`
}

type HeroBandit struct {
	Enabled bool      `yaml:"enabled"`
	Slot    string    `yaml:"slot"`
	Arms    []HeroArm `yaml:"arms"`
}

type HeroArm struct {
	Key             string      `yaml:"key"`
	H1              string      `yaml:"h1"`
	Subhead         string      `yaml:"subhead"`
	Body            string      `yaml:"body"`
	BGImageAssetKey string      `yaml:"bg_image_asset_key"`
	Overlay         HeroOverlay `yaml:"overlay"`
	PrimaryCTA      *CTA        `yaml:"primary_cta"`
	SecondaryCTA    *CTA        `yaml:"secondary_cta"`
}

type MediaSplitBlock struct {
	Layout struct {
		MediaSide string `yaml:"media_side"`
		Align     string `yaml:"align"`
	} `yaml:"layout"`
	Media struct {
		ImageAssetKey string `yaml:"image_asset_key"`
		Alt           string `yaml:"alt"`
	} `yaml:"media"`
	Content struct {
		Eyebrow string `yaml:"eyebrow"`
		Title   string `yaml:"title"`
		Body    string `yaml:"body"`
		CTA     *CTA   `yaml:"cta"`
	} `yaml:"content"`
}

type ProductGridBlock struct {
	Title string `yaml:"title"`
	Query struct {
		Mode       string   `yaml:"mode"`
		ProductIDs []int64  `yaml:"product_ids"`
		Brand      string   `yaml:"brand"`
		Tags       []string `yaml:"tags"`
	} `yaml:"query"`
	TastingNotes  map[int64]string `yaml:"tasting_notes"`
	FeaturedBadge *bool            `yaml:"featured_badge"`
	Grid          struct {
		ColumnsDesktop int `yaml:"columns_desktop"`
		ColumnsMobile  int `yaml:"columns_mobile"`
	} `yaml:"grid"`
	Stock struct {
		ShowOOS     bool   `yaml:"show_oos"`
		OOSBehavior string `yaml:"oos_behavior"`
	} `yaml:"stock"`
	CTA *CTA `yaml:"cta"`
}

type ColumnsBlock struct {
	Title   string        `yaml:"title"`
	Columns int           `yaml:"columns"`
	Items   []ColumnsItem `yaml:"items"`
}

type ColumnsItem struct {
	Title string `yaml:"title"`
	Body  string `yaml:"body"`
	Icon  string `yaml:"icon"`
	Href  string `yaml:"href"`
}

type TrustStripBlock struct {
	Items []TrustItem `yaml:"items"`
}

type FAQBlock struct {
	Items []FAQItem `yaml:"items"`
}

type FAQItem struct {
	Q string `yaml:"q"`
	A string `yaml:"a"`
}

type PairingsBlock struct {
	Title string        `yaml:"title"`
	Items []PairingItem `yaml:"items"`
}

type PairingItem struct {
	Title string `yaml:"title"`
	Body  string `yaml:"body"`
	Href  string `yaml:"href"`
}

type FeedProduct struct {
	ID           string `json:"id,omitempty"`
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	URL          string `json:"url,omitempty"`
	Price        string `json:"price,omitempty"`
	SalePrice    string `json:"sale_price,omitempty"`
	Availability string `json:"availability,omitempty"`
	InStock      *bool  `json:"in_stock,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
	Brand        string `json:"brand,omitempty"`
	Tags         string `json:"tags,omitempty"`
}

type LoadedSite struct {
	Root           string
	LandingDir     string
	SitePath       string
	ThemePath      string
	Site           SiteConfig
	Pages          []*PageFile
	PageBySlug     map[string]*PageFile
	AssetIndex     map[string]string
	FeedByID       map[string]FeedProduct
	ThemeCSS       string
	LoadedAt       time.Time
	ValidationErr  []string
	ValidationWarn []string
}

type ServeOptions struct {
	Root                 string
	Account              string
	Bind                 string
	StatePath            string
	Env                  string
	HotReload            bool
	LogLevel             string
	TrustProxy           bool
	PublicBaseOverride   string
	MainSiteBaseOverride string
}

type ArmStats struct {
	ArmKey      string
	Impressions int64
	Clicks      int64
}

type ChosenArm struct {
	Slot string
	Arm  string
}

type AttributionData struct {
	AnonID    string            `json:"anon_id,omitempty"`
	PageKey   string            `json:"page_key,omitempty"`
	Slug      string            `json:"slug,omitempty"`
	FirstSeen string            `json:"first_seen,omitempty"`
	Params    map[string]string `json:"params,omitempty"`
}
