package landing

import (
	"net/http/httptest"
	"testing"
)

func TestParsePageBlockTypes(t *testing.T) {
	yml := []byte(`version: 1
page:
  key: sample
  slug: /sample
blocks:
  - type: hero
    key: h1
    h1: "Hello"
    primary_cta:
      label: "Go"
      href: "https://example.com"
  - type: spacer
    key: s1
    size: md
`)
	page, err := parsePage(yml)
	if err != nil {
		t.Fatalf("parsePage failed: %v", err)
	}
	if len(page.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(page.Blocks))
	}
	if page.Blocks[0].Hero == nil || page.Blocks[1].Spacer == nil {
		t.Fatalf("typed blocks not decoded: %+v", page.Blocks)
	}
}

func TestAttributionCookieRoundTrip(t *testing.T) {
	site := SiteConfig{}
	site.Tracking.AttributionCookie = "tbd_attr"
	site.Tracking.CookieDomain = ".example.com"
	site.Tracking.AttributionTTLDay = 30

	w := httptest.NewRecorder()
	original := AttributionData{AnonID: "abc", Params: map[string]string{"utm_source": "meta"}}
	writeAttributionCookie(w, site, original)
	res := w.Result()
	cookie := res.Cookies()[0]

	r := httptest.NewRequest("GET", "https://ads.example.com/sample", nil)
	r.AddCookie(cookie)
	read := readAttributionCookie(r, site.Tracking.AttributionCookie)
	if read.AnonID != "abc" || read.Params["utm_source"] != "meta" {
		t.Fatalf("unexpected read attribution: %+v", read)
	}
}

func TestChooseArmReturnsUnderexposedFirst(t *testing.T) {
	arms := []HeroArm{{Key: "control"}, {Key: "variant"}}
	stats := map[string]ArmStats{
		"control": {ArmKey: "control", Impressions: 300, Clicks: 30},
		"variant": {ArmKey: "variant", Impressions: 12, Clicks: 3},
	}
	picked := ChooseArm(arms, stats, 200, 0.15)
	if picked != "variant" {
		t.Fatalf("expected underexposed arm, got %q", picked)
	}
}

func TestApplySiteDefaultsBanditStorage(t *testing.T) {
	site := SiteConfig{}
	applySiteDefaults(&site)
	if site.Bandit.Storage.Type != "sqlite" {
		t.Fatalf("expected default bandit storage sqlite, got %q", site.Bandit.Storage.Type)
	}
	if site.Bandit.Storage.Redis.KeyPrefix == "" {
		t.Fatalf("expected default redis key prefix")
	}
}
