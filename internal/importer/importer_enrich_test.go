package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"adform/internal/state"
)

type fakeDetails struct{}

func (f fakeDetails) GetNode(id string, _ ...string) (map[string]any, error) {
	switch id {
	case "c1":
		return map[string]any{"id": "c1", "name": "Campaign PH", "objective": "SALES", "status": "PAUSED"}, nil
	case "as1":
		return map[string]any{
			"id": "as1", "name": "Adset Manila", "status": "PAUSED", "campaign_id": "c1",
			"daily_budget": "250000", "bid_strategy": "LOWEST_COST_WITHOUT_CAP", "optimization_goal": "OFFSITE_CONVERSIONS", "billing_event": "IMPRESSIONS",
			"promoted_object": map[string]any{"pixel_id": "2455611864710842", "custom_event_type": "PURCHASE"},
			"targeting": map[string]any{
				"age_min": 18.0,
				"age_max": 55.0,
				"genders": []any{1.0, 2.0},
				"geo_locations": map[string]any{
					"countries":      []any{"PH"},
					"location_types": []any{"home", "recent"},
					"cities":         []any{map[string]any{"key": "2425566", "name": "Manila", "radius": 25.0, "distance_unit": "mile"}},
				},
				"flexible_spec":        []any{map[string]any{"behaviors": []any{map[string]any{"id": "1", "name": "High-value"}}}},
				"targeting_automation": map[string]any{"advantage_audience": 1.0},
			},
		}, nil
	case "ad1":
		return map[string]any{"id": "ad1", "name": "Ad 1", "status": "PAUSED", "adset_id": "as1", "creative": map[string]any{"id": "cr1"}}, nil
	case "cr1":
		return map[string]any{
			"id":   "cr1",
			"name": "Creative 1",
			"object_story_spec": map[string]any{
				"page_id":              "123456789",
				"instagram_actor_id":   "987654321",
				"link_data":            map[string]any{"message": "Hello", "link": "https://example.com/x", "name": "Headline", "description": "Desc", "image_hash": "img_hash_1", "call_to_action": map[string]any{"type": "SHOP_NOW"}},
				"template_data":        map[string]any{"message": "Template message"},
				"video_data":           map[string]any{"message": "Video message"},
				"unused_reference_key": "unused",
			},
			"asset_feed_spec": map[string]any{
				"ad_formats":           []any{"AUTOMATIC_FORMAT"},
				"call_to_action_types": []any{"SHOP_NOW"},
				"bodies":               []any{map[string]any{"text": "Hello"}, map[string]any{"text": "Hello 2"}},
			},
			"degrees_of_freedom_spec": map[string]any{
				"creative_features_spec": map[string]any{
					"standard_enhancements": map[string]any{"enroll_status": "OPT_IN"},
				},
			},
		}, nil
	case "aud1":
		return map[string]any{
			"id":                 "aud1",
			"name":               "Past Buyers 180d",
			"subtype":            "WEBSITE",
			"description":        "buyers",
			"retention_days":     180.0,
			"pixel_id":           "2455611864710842",
			"source_audience_id": "src1",
			"rule":               "{\"inclusions\":{\"operator\":\"or\"}}",
			"data_source":        map[string]any{"type": "EVENT_BASED"},
			"operation_status":   map[string]any{"code": 200.0, "description": "Normal"},
		}, nil
	default:
		return map[string]any{"id": id}, nil
	}
}

func TestImportFromStateRemoteEnrichesAdsetAndPaths(t *testing.T) {
	root := t.TempDir()
	account := "acc"
	st, err := state.Open(filepath.Join(root, ".adform", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	seed := []state.ResourceRow{
		{AccountName: account, Kind: "campaign", LogicalKey: "camp_key", MetaID: "c1", LastAppliedHash: "h1"},
		{AccountName: account, Kind: "adset", LogicalKey: "adset_key", MetaID: "as1", LastAppliedHash: "h2"},
		{AccountName: account, Kind: "creative", LogicalKey: "creative_key", MetaID: "cr1", LastAppliedHash: "h3"},
		{AccountName: account, Kind: "ad", LogicalKey: "ad_key", MetaID: "ad1", LastAppliedHash: "h4"},
		{AccountName: account, Kind: "asset_image", LogicalKey: "hero_image", MetaID: "img_hash_1", LastAppliedHash: "h5"},
		{AccountName: account, Kind: "audience", LogicalKey: "audience_key", MetaID: "aud1", LastAppliedHash: "h6"},
	}
	for _, row := range seed {
		if err := st.UpsertResource(row); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(root, "meta", account)
	_, err = ImportFromState(st, Options{Root: root, Account: account, Out: out, Force: true, Remote: fakeDetails{}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	adsetPath := filepath.Join(out, "campaigns", "camp_key", "adsets", "adset_key", "adset.yml")
	adsetYAML, err := os.ReadFile(adsetPath)
	if err != nil {
		t.Fatalf("read adset: %v", err)
	}
	text := string(adsetYAML)
	checks := []string{
		"name: Adset Manila",
		"pixel_key: \"2455611864710842\"",
		"countries:",
		"- PH",
		"name: Manila",
	}
	for _, want := range checks {
		if !strings.Contains(text, want) {
			t.Fatalf("expected adset yaml to contain %q, got:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "location_types:") {
		t.Fatalf("expected location_types in adset yaml, got:\n%s", text)
	}
	if !strings.Contains(text, "age_min: 18") || !strings.Contains(text, "age_max: 55") {
		t.Fatalf("expected age range fields in adset yaml, got:\n%s", text)
	}
	if !strings.Contains(text, "genders:") || !strings.Contains(text, "targeting_automation:") || !strings.Contains(text, "flexible_spec:") {
		t.Fatalf("expected enriched targeting fields in adset yaml, got:\n%s", text)
	}
	if strings.Contains(text, "imported_targeting_json:") || strings.Contains(text, "targeting_raw:") || strings.Contains(text, "promoted_object_raw:") {
		t.Fatalf("expected no duplicated raw/json targeting fields in adset yaml, got:\n%s", text)
	}

	adPath := filepath.Join(out, "campaigns", "camp_key", "adsets", "adset_key", "ads", "ad_key.yml")
	if _, err := os.Stat(adPath); err != nil {
		t.Fatalf("expected ad file at campaign/adset path: %v", err)
	}

	creativePath := filepath.Join(out, "creatives", "creative_key.yml")
	creativeYAML, err := os.ReadFile(creativePath)
	if err != nil {
		t.Fatalf("read creative: %v", err)
	}
	creativeText := string(creativeYAML)
	creativeChecks := []string{
		"name: Creative 1",
		"page_id_ref: ",
		"instagram_actor_id_ref: ",
		"url: https://example.com/x",
		"message: Hello",
		"headline: Headline",
		"description: Desc",
		"call_to_action_type: SHOP_NOW",
		"ad_formats:",
		"body_variants:",
		"image_asset_key: hero_image",
		"asset_source: static_assets",
		"linked_asset_keys:",
	}
	for _, want := range creativeChecks {
		if !strings.Contains(creativeText, want) {
			t.Fatalf("expected creative yaml to contain %q, got:\n%s", want, creativeText)
		}
	}
	if !strings.Contains(creativeText, "123456789") || !strings.Contains(creativeText, "987654321") {
		t.Fatalf("expected creative yaml to include page/ig ids, got:\n%s", creativeText)
	}
	if !strings.Contains(creativeText, "object_story_spec:") || !strings.Contains(creativeText, "asset_feed_spec:") || !strings.Contains(creativeText, "degrees_of_freedom_spec:") {
		t.Fatalf("expected creative yaml to include canonical spec blocks, got:\n%s", creativeText)
	}
	if strings.Contains(creativeText, "imported_json:") || strings.Contains(creativeText, "object_story_spec_raw:") || strings.Contains(creativeText, "asset_feed_spec_raw:") || strings.Contains(creativeText, "raw:") {
		t.Fatalf("expected no duplicated raw/json blocks in creative yaml, got:\n%s", creativeText)
	}

	audiencePath := filepath.Join(out, "audiences", "audience_key.yml")
	audienceYAML, err := os.ReadFile(audiencePath)
	if err != nil {
		t.Fatalf("read audience: %v", err)
	}
	audienceText := string(audienceYAML)
	audienceChecks := []string{
		"name: Past Buyers 180d",
		"retention_days: 180",
		"pixel_id: \"2455611864710842\"",
		"rule:",
		"data_source:",
		"operation_status:",
	}
	for _, want := range audienceChecks {
		if !strings.Contains(audienceText, want) {
			t.Fatalf("expected audience yaml to contain %q, got:\n%s", want, audienceText)
		}
	}
	if strings.Contains(audienceText, "rule_json:") || strings.Contains(audienceText, "data_source_json:") || strings.Contains(audienceText, "operation_status_json:") || strings.Contains(audienceText, "raw:") {
		t.Fatalf("expected no duplicated raw/json fields in audience yaml, got:\n%s", audienceText)
	}
}
