package cli

import "testing"

func TestSumActionMetricPurchasePrefersCanonicalType(t *testing.T) {
	actions := []any{
		map[string]any{"action_type": "omni_purchase", "value": "15"},
		map[string]any{"action_type": "offsite_conversion.fb_pixel_purchase", "value": "12"},
		map[string]any{"action_type": "purchase", "value": "27"},
	}
	got := sumActionMetric(actions, "purchase")
	if got != 12 {
		t.Fatalf("expected purchase alias to pick offsite pixel purchase (12), got %v", got)
	}
}

func TestSumActionMetricAllSumsEverything(t *testing.T) {
	actions := []any{
		map[string]any{"action_type": "a", "value": "2"},
		map[string]any{"action_type": "b", "value": "3"},
	}
	got := sumActionMetric(actions, "all")
	if got != 5 {
		t.Fatalf("expected all to sum to 5, got %v", got)
	}
}

func TestSumActionMetricCommaEventUsesExactMatches(t *testing.T) {
	actions := []any{
		map[string]any{"action_type": "lead", "value": "2"},
		map[string]any{"action_type": "purchase", "value": "3"},
		map[string]any{"action_type": "offsite_conversion.fb_pixel_purchase", "value": "4"},
	}
	got := sumActionMetric(actions, "lead,purchase")
	if got != 5 {
		t.Fatalf("expected lead+purchase exact sum to 5, got %v", got)
	}
}
