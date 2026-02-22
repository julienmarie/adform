package cli

import (
	"strings"
	"testing"

	"adform/internal/posthog"
)

func TestRenderPostHogQueryInterpolatesVars(t *testing.T) {
	query := "SELECT * FROM events WHERE event='{{event_order_completed}}' AND timestamp >= toDateTime('{{since}}')"
	out := renderPostHogQuery(query, map[string]string{
		"event_order_completed": "Order Completed",
		"since":                 "2026-01-01 00:00:00",
	})
	if out == query {
		t.Fatalf("expected interpolation, got unchanged query")
	}
	if want := "Order Completed"; !strings.Contains(out, want) {
		t.Fatalf("expected query to contain %q, got %q", want, out)
	}
}

func TestMergePostHogMetricsFromColumnResults(t *testing.T) {
	sales := posthog.QueryResponse{
		Columns: []string{"product_id", "product_sku", "product_name", "product_variant", "total_qty", "purchase_count", "total_value"},
		Results: []any{
			[]any{"p1", "sku1", "Name 1", "v1", 3, 2, 100.5},
		},
	}
	added := posthog.QueryResponse{
		Columns: []string{"product_id", "add_to_cart_count"},
		Results: []any{
			[]any{"p1", 8},
		},
	}
	viewed := posthog.QueryResponse{
		Columns: []string{"product_id", "product_view_count"},
		Results: []any{
			[]any{"p1", 40},
		},
	}

	rows := mergePostHogMetrics(sales, added, viewed)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.ProductID != "p1" || r.PurchaseCount != 2 || r.AddToCartCount != 8 || r.ProductViewCount != 40 {
		t.Fatalf("unexpected merged row: %+v", r)
	}
}
