package landing

import (
	"strings"
	"testing"
)

func TestRenderPageHTMLDevPageSwitcher(t *testing.T) {
	loaded := &LoadedSite{
		Site: SiteConfig{
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		Pages: []*PageFile{
			{Page: PageMeta{Key: "one", Slug: "/one"}},
			{Page: PageMeta{Key: "two", Slug: "/two"}},
		},
	}
	page := &PageFile{Page: PageMeta{Key: "one", Slug: "/one"}}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "dev"},
	})

	if !strings.Contains(html, "adform-dev-pages") {
		t.Fatalf("expected dev page switcher in html")
	}
	if !strings.Contains(html, "href=\"/one\"") || !strings.Contains(html, "href=\"/two\"") {
		t.Fatalf("expected page links in switcher")
	}
	if !strings.Contains(html, "is-current") {
		t.Fatalf("expected current page highlight in switcher")
	}
}

func TestRenderPageHTMLProdHidesPageSwitcher(t *testing.T) {
	loaded := &LoadedSite{
		Site: SiteConfig{
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		Pages: []*PageFile{
			{Page: PageMeta{Key: "one", Slug: "/one"}},
		},
	}
	page := &PageFile{Page: PageMeta{Key: "one", Slug: "/one"}}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "prod"},
	})

	if strings.Contains(html, "adform-dev-pages") {
		t.Fatalf("did not expect dev page switcher in prod html")
	}
}

func TestRenderPageHTMLInjectsGlobalScripts(t *testing.T) {
	loaded := &LoadedSite{
		Site: SiteConfig{
			Scripts: struct {
				URLs   []string "yaml:\"urls\""
				Inline []string "yaml:\"inline\""
			}{
				URLs:   []string{"https://cdn.example.com/analytics.js"},
				Inline: []string{"window.__landing_test = true;"},
			},
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		Pages: []*PageFile{
			{Page: PageMeta{Key: "one", Slug: "/one"}},
		},
	}
	page := &PageFile{Page: PageMeta{Key: "one", Slug: "/one"}}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "prod"},
	})

	if !strings.Contains(html, `src="https://cdn.example.com/analytics.js"`) {
		t.Fatalf("expected external script URL to be rendered")
	}
	if !strings.Contains(html, "window.__landing_test = true;") {
		t.Fatalf("expected inline script to be rendered")
	}
}

func TestRenderPageHTMLInlinesThemeCSS(t *testing.T) {
	loaded := &LoadedSite{
		Site: SiteConfig{
			Scripts: struct {
				URLs   []string "yaml:\"urls\""
				Inline []string "yaml:\"inline\""
			}{
				URLs: []string{
					"https://fonts.googleapis.com/css2?family=Inter:wght@400;700&display=swap",
					"https://cdn.example.com/app.js",
				},
			},
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		Pages:    []*PageFile{{Page: PageMeta{Key: "one", Slug: "/one"}}},
		ThemeCSS: "@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;700&display=swap');\n.lp{color:#111;} .lp-nav-links a{font-family:'CustomFont';}",
	}
	page := &PageFile{Page: PageMeta{Key: "one", Slug: "/one"}}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "prod"},
	})

	if !strings.Contains(html, `id="adform-theme-inline"`) {
		t.Fatalf("expected inline theme style tag")
	}
	if strings.Contains(html, `rel="stylesheet" href="/theme.css"`) {
		t.Fatalf("did not expect external /theme.css link when inlining theme")
	}
	if !strings.Contains(html, `id="adform-menu-system-fonts"`) {
		t.Fatalf("expected system font override style tag for menu")
	}
	if strings.Contains(html, "fonts.googleapis.com") || strings.Contains(html, "fonts.gstatic.com") {
		t.Fatalf("did not expect google fonts host in rendered html")
	}
	if !strings.Contains(html, `src="https://cdn.example.com/app.js"`) {
		t.Fatalf("expected non-google script URL to still render")
	}
}

func TestNormalizeHTMLLangFallback(t *testing.T) {
	if got := normalizeHTMLLang(""); got != "en" {
		t.Fatalf("expected en fallback, got %q", got)
	}
	if got := normalizeHTMLLang("EN_ph"); got != "en-PH" {
		t.Fatalf("expected normalized en-PH, got %q", got)
	}
}

func TestRenderDevDashboardHTML(t *testing.T) {
	loaded := &LoadedSite{
		Site: SiteConfig{
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		Pages: []*PageFile{
			{Page: PageMeta{Key: "bordier", Slug: "/bordier"}},
			{Page: PageMeta{Key: "caviar", Slug: "/caviar-de-neuvic"}},
		},
	}

	html := renderDevDashboardHTML(devDashboardContext{
		site:      loaded,
		serveOpts: ServeOptions{Env: "dev", Account: "btd_main"},
		cards: []devAdCard{
			{
				AdKey:         "ad-1",
				AdName:        "Ad One",
				AdStatus:      "PAUSED",
				CampaignName:  "Campaign A",
				AdsetName:     "Adset A",
				Headline:      "Hero Product",
				PrimaryText:   "Premium imported product",
				CTAType:       "SHOP_NOW",
				Destination:   "/bordier",
				DestinationIs: "landing",
			},
		},
	})

	if !strings.Contains(html, "Ad Preview Board") {
		t.Fatalf("expected dev dashboard title")
	}
	if !strings.Contains(html, "ad-preview-card") {
		t.Fatalf("expected ad preview card markup")
	}
	if !strings.Contains(html, `href="/bordier"`) {
		t.Fatalf("expected card destination link")
	}
	if !strings.Contains(html, "adform-dev-pages") {
		t.Fatalf("expected page navigator in dashboard")
	}
	if !strings.Contains(html, `href="/"`) {
		t.Fatalf("expected preview board link in navigator")
	}
}

func TestRenderProductGridOmitsDescriptionFromCard(t *testing.T) {
	page := &PageFile{
		Page: PageMeta{Key: "catalog", Slug: "/catalog"},
		Blocks: []Block{
			{
				Type: "product_grid",
				Key:  "products",
				ProductGrid: &ProductGridBlock{
					Query: struct {
						Mode       string   "yaml:\"mode\""
						ProductIDs []int64  "yaml:\"product_ids\""
						Brand      string   "yaml:\"brand\""
						Tags       []string "yaml:\"tags\""
					}{
						Mode:       "explicit",
						ProductIDs: []int64{1},
					},
					Stock: struct {
						ShowOOS     bool   "yaml:\"show_oos\""
						OOSBehavior string "yaml:\"oos_behavior\""
					}{
						ShowOOS: true,
					},
				},
			},
		},
	}
	loaded := &LoadedSite{
		Site: SiteConfig{
			Runtime: struct {
				Bind            string "yaml:\"bind\""
				PublicBaseURL   string "yaml:\"public_base_url\""
				MainSiteBaseURL string "yaml:\"main_site_base_url\""
			}{
				MainSiteBaseURL: "https://example.com",
			},
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		FeedByID: map[string]FeedProduct{
			"1": {
				ID:          "1",
				Title:       "Bordier Butter",
				Description: "Small-batch churned butter from Brittany.",
				Price:       "PHP 1,290",
				URL:         "https://example.com/products/1",
			},
		},
	}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "prod"},
	})

	titleIdx := strings.Index(html, "<h3>Bordier Butter</h3>")
	priceIdx := strings.Index(html, "PHP 1,290")
	if titleIdx < 0 || priceIdx < 0 {
		t.Fatalf("expected title and price in product card html")
	}
	if strings.Contains(html, "product-description") {
		t.Fatalf("did not expect product-description in product card html")
	}
	if !(titleIdx < priceIdx) {
		t.Fatalf("expected title to render before price")
	}
}

func TestRenderProductGridHidesDescriptionWhenTastingNoteExists(t *testing.T) {
	page := &PageFile{
		Page: PageMeta{Key: "catalog", Slug: "/catalog"},
		Blocks: []Block{
			{
				Type: "product_grid",
				Key:  "products",
				ProductGrid: &ProductGridBlock{
					Query: struct {
						Mode       string   "yaml:\"mode\""
						ProductIDs []int64  "yaml:\"product_ids\""
						Brand      string   "yaml:\"brand\""
						Tags       []string "yaml:\"tags\""
					}{
						Mode:       "explicit",
						ProductIDs: []int64{1},
					},
					TastingNotes: map[int64]string{
						1: "Nutty finish with cultured cream depth.",
					},
					Stock: struct {
						ShowOOS     bool   "yaml:\"show_oos\""
						OOSBehavior string "yaml:\"oos_behavior\""
					}{
						ShowOOS: true,
					},
				},
			},
		},
	}
	loaded := &LoadedSite{
		Site: SiteConfig{
			Runtime: struct {
				Bind            string "yaml:\"bind\""
				PublicBaseURL   string "yaml:\"public_base_url\""
				MainSiteBaseURL string "yaml:\"main_site_base_url\""
			}{
				MainSiteBaseURL: "https://example.com",
			},
			Defaults: struct {
				Locale     string      "yaml:\"locale\""
				Currency   string      "yaml:\"currency\""
				TrustItems []TrustItem "yaml:\"trust_items\""
			}{Locale: "en-PH"},
		},
		FeedByID: map[string]FeedProduct{
			"1": {
				ID:          "1",
				Title:       "Bordier Butter",
				Description: "This description should be hidden when note exists.",
				Price:       "PHP 1,290",
				URL:         "https://example.com/products/1",
			},
		},
	}

	html := renderPageHTML(renderContext{
		page:      page,
		site:      loaded,
		serveOpts: ServeOptions{Env: "prod"},
	})

	if strings.Contains(html, "product-description") {
		t.Fatalf("did not expect product-description when tasting-note exists")
	}
	if !strings.Contains(html, "tasting-note") {
		t.Fatalf("expected tasting-note to render")
	}
}
