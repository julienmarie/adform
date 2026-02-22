package config

import "testing"

func TestValidateDetectsBadReferences(t *testing.T) {
	bundle := NewBundle(".", "test")
	bundle.AccountCfg.AccountName = "test"
	bundle.AccountCfg.Meta.AdAccountID = "act_1"
	bundle.AccountCfg.Meta.Currency = "USD"
	bundle.AccountCfg.Budgets.Unit = "major"
	bundle.AccountCfg.Policies.AllowActivate = false
	bundle.AccountCfg.Policies.Budget.MaxDailyBudgetMajor = 100

	bundle.Campaigns["camp"] = Campaign{Key: "camp", Name: "C", Objective: "SALES", Status: "PAUSED"}
	bundle.Adsets["adset"] = AdSet{Key: "adset", CampaignKey: "missing", Status: "ACTIVE", DailyBudget: 200}
	bundle.Ads["ad"] = Ad{Key: "ad", AdsetKey: "adset", CreativeKey: "missing", Status: "PAUSED"}

	res := Validate(bundle)
	if res.OK() {
		t.Fatalf("expected validation errors")
	}
	if len(res.Errors) < 3 {
		t.Fatalf("expected at least 3 errors, got %d", len(res.Errors))
	}
}

func TestValidateProductFeedURLFormat(t *testing.T) {
	bundle := NewBundle(".", "test")
	bundle.AccountCfg.AccountName = "test"
	bundle.AccountCfg.Meta.AdAccountID = "act_1"
	bundle.AccountCfg.Meta.Currency = "USD"
	bundle.AccountCfg.Meta.Timezone = "America/New_York"
	bundle.AccountCfg.Meta.ProductFeedURL = "not-a-url"
	bundle.AccountCfg.Budgets.Unit = "major"

	res := Validate(bundle)
	found := false
	for _, e := range res.Errors {
		if e.Rule == "ACC_PRODUCT_FEED_URL_FORMAT" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ACC_PRODUCT_FEED_URL_FORMAT error, got %+v", res.Errors)
	}
}
