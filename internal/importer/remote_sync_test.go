package importer

import (
	"net/url"
	"path/filepath"
	"testing"

	"adform/internal/state"
)

type fakeRemote struct{}

func (f fakeRemote) ListEdge(edge string, _ []string, _ url.Values) ([]map[string]any, error) {
	switch edge {
	case "act_123/campaigns":
		return []map[string]any{{"id": "111111", "name": "My Campaign", "status": "PAUSED"}}, nil
	case "act_123/adsets":
		return []map[string]any{{"id": "222222", "name": "My Adset", "status": "PAUSED", "campaign_id": "111111", "promoted_object": map[string]any{"product_catalog_id": "777777"}}}, nil
	case "act_123/ads":
		return []map[string]any{{"id": "333333", "name": "My Ad", "status": "PAUSED", "adset_id": "222222", "creative": map[string]any{"id": "444444"}}}, nil
	case "act_123/customaudiences":
		return []map[string]any{{"id": "555555", "name": "Recent Purchasers"}}, nil
	case "act_123/adimages":
		return []map[string]any{{"hash": "aa11bb22cc33", "name": "Hero Image"}}, nil
	case "act_123/advideos":
		return []map[string]any{{"id": "666666", "title": "Promo Video"}}, nil
	default:
		return nil, nil
	}
}

func (f fakeRemote) GetNode(id string, _ ...string) (map[string]any, error) {
	if id == "444444" {
		return map[string]any{"id": "444444", "name": "My Creative"}, nil
	}
	if id == "777777" {
		return map[string]any{"id": "777777", "name": "Main Catalog"}, nil
	}
	return map[string]any{"id": id}, nil
}

func TestSyncStateFromRemote(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := SyncStateFromRemote(st, "acc", "123", fakeRemote{}, RemoteSyncOptions{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Campaigns != 1 || res.Adsets != 1 || res.Ads != 1 || res.Creatives != 1 || res.Audiences != 1 || res.AssetImages != 1 || res.AssetVideos != 1 || res.Catalogs != 1 {
		t.Fatalf("unexpected summary: %+v", res)
	}

	rows, err := st.ListResources("acc")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 8 {
		t.Fatalf("expected 8 rows, got %d", len(rows))
	}
}

func TestStableKeyCollision(t *testing.T) {
	used := map[string]struct{}{}
	k1 := stableKey("Name", "123456789", used)
	k2 := stableKey("Name", "123456789", used)
	if k1 == k2 {
		t.Fatalf("expected collision handling to produce different keys")
	}
}

type fakeRemoteCatalogInference struct{}

func (f fakeRemoteCatalogInference) ListEdge(edge string, _ []string, _ url.Values) ([]map[string]any, error) {
	switch edge {
	case "act_123/campaigns":
		return []map[string]any{
			{"id": "111111", "name": "Catalog Campaign", "status": "PAUSED"},
			{"id": "111112", "name": "Catalog Campaign 2", "status": "PAUSED"},
		}, nil
	case "act_123/adsets":
		return []map[string]any{
			{
				"id":          "222222",
				"name":        "Known Product Set",
				"status":      "PAUSED",
				"campaign_id": "111111",
				"promoted_object": map[string]any{
					"product_set_id": "set_1",
				},
			},
			{
				"id":          "222223",
				"name":        "Missing Catalog",
				"status":      "PAUSED",
				"campaign_id": "111112",
				"promoted_object": map[string]any{
					"pixel_id": "2455611864710842",
				},
			},
		}, nil
	case "act_123/ads":
		return []map[string]any{
			{"id": "333331", "name": "Ad Dynamic", "status": "PAUSED", "adset_id": "222223", "creative": map[string]any{"id": "444441"}},
		}, nil
	case "act_123/customaudiences":
		return nil, nil
	case "act_123/adimages":
		return nil, nil
	case "act_123/advideos":
		return nil, nil
	default:
		return nil, nil
	}
}

func (f fakeRemoteCatalogInference) GetNode(id string, _ ...string) (map[string]any, error) {
	switch id {
	case "set_1":
		return map[string]any{
			"id": "set_1",
			"product_catalog": map[string]any{
				"id": "cat_1",
			},
		}, nil
	case "444441":
		return map[string]any{
			"id":   "444441",
			"name": "Dynamic Creative",
			"object_story_spec": map[string]any{
				"template_data": map[string]any{
					"name": "{{product.name}}",
				},
			},
		}, nil
	case "cat_1":
		return map[string]any{"id": "cat_1", "name": "Main Catalog"}, nil
	default:
		return map[string]any{"id": id}, nil
	}
}

func TestSyncStateFromRemote_InferCatalogFromProductSetAndDynamicCreative(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := SyncStateFromRemote(st, "acc", "123", fakeRemoteCatalogInference{}, RemoteSyncOptions{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Catalogs != 1 {
		t.Fatalf("expected 1 catalog, got %+v", res)
	}

	foundMissingAdset := false
	for key, adset := range res.Details["adset"] {
		if stringField(adset, "id") != "222223" {
			continue
		}
		foundMissingAdset = true
		promoted, ok := adset["promoted_object"].(map[string]any)
		if !ok {
			t.Fatalf("expected promoted_object map for adset %s", key)
		}
		if got := stringField(promoted, "product_catalog_id"); got != "cat_1" {
			t.Fatalf("expected inferred product_catalog_id=cat_1 for adset %s, got %q", key, got)
		}
	}
	if !foundMissingAdset {
		t.Fatalf("missing test adset in details")
	}
}

type fakeRemoteCatalogEdge struct{}

func (f fakeRemoteCatalogEdge) ListEdge(edge string, _ []string, _ url.Values) ([]map[string]any, error) {
	switch edge {
	case "act_123/campaigns":
		return []map[string]any{{"id": "111111", "name": "My Campaign", "status": "PAUSED"}}, nil
	case "act_123/adsets":
		return []map[string]any{{"id": "222222", "name": "My Adset", "status": "PAUSED", "campaign_id": "111111"}}, nil
	case "act_123/ads":
		return []map[string]any{{"id": "333333", "name": "My Ad", "status": "PAUSED", "adset_id": "222222", "creative": map[string]any{"id": "444444"}}}, nil
	case "act_123/customaudiences":
		return nil, nil
	case "act_123/adimages":
		return nil, nil
	case "act_123/advideos":
		return nil, nil
	case "act_123/product_catalogs":
		return []map[string]any{{"id": "cat_edge_1", "name": "Catalog From Edge"}}, nil
	default:
		return nil, nil
	}
}

func (f fakeRemoteCatalogEdge) GetNode(id string, _ ...string) (map[string]any, error) {
	if id == "444444" {
		return map[string]any{"id": "444444", "name": "My Creative"}, nil
	}
	return map[string]any{"id": id}, nil
}

func TestSyncStateFromRemote_CatalogsFromProductCatalogsEdge(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := SyncStateFromRemote(st, "acc", "123", fakeRemoteCatalogEdge{}, RemoteSyncOptions{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Catalogs != 1 {
		t.Fatalf("expected 1 catalog from product_catalogs edge, got %+v", res)
	}
	if len(res.Details["catalog"]) != 1 {
		t.Fatalf("expected 1 catalog detail, got %d", len(res.Details["catalog"]))
	}
}

type fakeRemoteProductSetsCatalog struct{}

func (f fakeRemoteProductSetsCatalog) ListEdge(edge string, _ []string, _ url.Values) ([]map[string]any, error) {
	switch edge {
	case "act_123/campaigns":
		return []map[string]any{{"id": "111111", "name": "My Campaign", "status": "PAUSED"}}, nil
	case "act_123/adsets":
		return []map[string]any{
			{
				"id":          "222222",
				"name":        "My Adset",
				"status":      "PAUSED",
				"campaign_id": "111111",
				"promoted_object": map[string]any{
					"product_set_id": "set_1",
				},
			},
		}, nil
	case "act_123/ads":
		return []map[string]any{{"id": "333333", "name": "My Ad", "status": "PAUSED", "adset_id": "222222", "creative": map[string]any{"id": "444444"}}}, nil
	case "act_123/customaudiences":
		return nil, nil
	case "act_123/adimages":
		return nil, nil
	case "act_123/advideos":
		return nil, nil
	case "act_123/product_catalogs":
		return nil, &fakeInvalidFieldErr{msg: "nonexisting field (product_catalogs) on node type (AdAccount)"}
	case "act_123/product_sets":
		return []map[string]any{
			{"id": "set_1", "product_catalog": map[string]any{"id": "cat_ps_1"}},
		}, nil
	default:
		return nil, nil
	}
}

func (f fakeRemoteProductSetsCatalog) GetNode(id string, _ ...string) (map[string]any, error) {
	switch id {
	case "444444":
		return map[string]any{"id": "444444", "name": "My Creative"}, nil
	case "set_1":
		return map[string]any{"id": "set_1", "product_catalog": map[string]any{"id": "cat_ps_1"}}, nil
	case "cat_ps_1":
		return map[string]any{"id": "cat_ps_1", "name": "Catalog From Product Sets"}, nil
	default:
		return map[string]any{"id": id}, nil
	}
}

type fakeInvalidFieldErr struct {
	msg string
}

func (e *fakeInvalidFieldErr) Error() string { return e.msg }

func TestSyncStateFromRemote_CatalogsFromProductSetsFallback(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	res, err := SyncStateFromRemote(st, "acc", "123", fakeRemoteProductSetsCatalog{}, RemoteSyncOptions{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Catalogs != 1 {
		t.Fatalf("expected 1 catalog from product_sets fallback, got %+v", res)
	}
}
