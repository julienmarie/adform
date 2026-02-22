package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"adform/internal/posthog"
	"adform/internal/workspace"
)

type posthogOptions struct {
	commonOptions
	Since         string
	Until         string
	Host          string
	ProjectID     string
	OrderEvent    string
	AddedEvent    string
	ViewedEvent   string
	SalesSQL      string
	AddedSQL      string
	ViewedSQL     string
	SalesSQLFile  string
	AddedSQLFile  string
	ViewedSQLFile string
	Limit         int
	Export        string
}

type posthogProductMetrics struct {
	ProductID        string  `json:"product_id,omitempty"`
	ProductSKU       string  `json:"product_sku,omitempty"`
	ProductName      string  `json:"product_name,omitempty"`
	ProductVariant   string  `json:"product_variant,omitempty"`
	TotalQty         float64 `json:"total_qty,omitempty"`
	PurchaseCount    float64 `json:"purchase_count,omitempty"`
	TotalValue       float64 `json:"total_value,omitempty"`
	AddToCartCount   float64 `json:"add_to_cart_count,omitempty"`
	ProductViewCount float64 `json:"product_view_count,omitempty"`
}

func runPostHog(_ context.Context, args []string, stdout, stderr io.Writer) int {
	sub := "sales"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = strings.ToLower(strings.TrimSpace(args[0]))
		args = args[1:]
	}
	switch sub {
	case "sales", "products":
		return runPostHogProducts(args, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown posthog subcommand: %s\n", sub)
		fmt.Fprintln(stderr, "usage: adform posthog sales [flags]")
		return 1
	}
}

func runPostHogProducts(args []string, stdout, stderr io.Writer) int {
	opts := posthogOptions{}
	fs := flag.NewFlagSet("posthog sales", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Since, "since", "", "Start date/time (YYYY-MM-DD or RFC3339). Default: last 30 days.")
	fs.StringVar(&opts.Until, "until", "", "End date/time (YYYY-MM-DD or RFC3339). Default: now UTC.")
	fs.StringVar(&opts.Host, "host", "", "PostHog host override (default from accounts.yml or https://app.posthog.com)")
	fs.StringVar(&opts.ProjectID, "project-id", "", "PostHog project id override")
	fs.StringVar(&opts.OrderEvent, "event-order-completed", "", "Order completed event name override")
	fs.StringVar(&opts.AddedEvent, "event-product-added", "", "Product added event name override")
	fs.StringVar(&opts.ViewedEvent, "event-product-viewed", "", "Product viewed event name override")
	fs.StringVar(&opts.SalesSQL, "sales-sql", "", "Sales HogQL override")
	fs.StringVar(&opts.AddedSQL, "added-sql", "", "Add-to-cart HogQL override")
	fs.StringVar(&opts.ViewedSQL, "viewed-sql", "", "Product viewed HogQL override")
	fs.StringVar(&opts.SalesSQLFile, "sales-sql-file", "", "Path to sales HogQL file override")
	fs.StringVar(&opts.AddedSQLFile, "added-sql-file", "", "Path to add-to-cart HogQL file override")
	fs.StringVar(&opts.ViewedSQLFile, "viewed-sql-file", "", "Path to product-viewed HogQL file override")
	fs.IntVar(&opts.Limit, "limit", 0, "Limit rows in output (0 = all)")
	fs.StringVar(&opts.Export, "export", "", "Write JSON payload to file")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	finalizeCommon(&opts.commonOptions)
	if err := ensureAccount(opts.Account); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if opts.Limit < 0 {
		fmt.Fprintln(stderr, "error: --limit must be >= 0")
		return 1
	}

	since, until, err := resolvePostHogRange(opts.Since, opts.Until)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if since.After(until) {
		fmt.Fprintln(stderr, "error: --since must be <= --until")
		return 1
	}

	accountsCfg, err := workspace.LoadAccountsConfig(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	token, tokenSource, err := workspace.ResolvePostHogToken(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	host := strings.TrimSpace(opts.Host)
	projectID := strings.TrimSpace(opts.ProjectID)
	orderEvent := strings.TrimSpace(opts.OrderEvent)
	addedEvent := strings.TrimSpace(opts.AddedEvent)
	viewedEvent := strings.TrimSpace(opts.ViewedEvent)
	salesQueryCfg := ""
	addedQueryCfg := ""
	viewedQueryCfg := ""
	if accountsCfg != nil {
		if host == "" {
			host = strings.TrimSpace(accountsCfg.Platforms.PostHog.Host)
		}
		if projectID == "" {
			projectID = strings.TrimSpace(accountsCfg.Platforms.PostHog.ProjectID)
		}
		if orderEvent == "" {
			orderEvent = strings.TrimSpace(accountsCfg.Platforms.PostHog.Events.OrderCompleted)
		}
		if addedEvent == "" {
			addedEvent = strings.TrimSpace(accountsCfg.Platforms.PostHog.Events.ProductAdded)
		}
		if viewedEvent == "" {
			viewedEvent = strings.TrimSpace(accountsCfg.Platforms.PostHog.Events.ProductViewed)
		}
		salesQueryCfg = strings.TrimSpace(accountsCfg.Platforms.PostHog.Queries.ProductSales)
		addedQueryCfg = strings.TrimSpace(accountsCfg.Platforms.PostHog.Queries.ProductAdded)
		viewedQueryCfg = strings.TrimSpace(accountsCfg.Platforms.PostHog.Queries.ProductViewed)
	}
	if projectID == "" {
		fmt.Fprintln(stderr, "error: PostHog project id is required (set platforms.posthog.project_id in accounts.yml or --project-id)")
		return 1
	}
	if orderEvent == "" {
		orderEvent = "Order Completed"
	}
	if addedEvent == "" {
		addedEvent = "Product Added"
	}
	if viewedEvent == "" {
		viewedEvent = "Product Viewed"
	}

	salesQuery, err := resolveQueryOverride(opts.Root, opts.SalesSQL, opts.SalesSQLFile, salesQueryCfg, defaultPostHogSalesQuery())
	if err != nil {
		fmt.Fprintf(stderr, "error: sales query: %v\n", err)
		return 1
	}
	addedQuery, err := resolveQueryOverride(opts.Root, opts.AddedSQL, opts.AddedSQLFile, addedQueryCfg, defaultPostHogAddedQuery())
	if err != nil {
		fmt.Fprintf(stderr, "error: added query: %v\n", err)
		return 1
	}
	viewedQuery, err := resolveQueryOverride(opts.Root, opts.ViewedSQL, opts.ViewedSQLFile, viewedQueryCfg, defaultPostHogViewedQuery())
	if err != nil {
		fmt.Fprintf(stderr, "error: viewed query: %v\n", err)
		return 1
	}

	vars := map[string]string{
		"since":                 since.Format("2006-01-02 15:04:05"),
		"until":                 until.Format("2006-01-02 15:04:05"),
		"event_order_completed": orderEvent,
		"event_product_added":   addedEvent,
		"event_product_viewed":  viewedEvent,
	}
	salesQuery = renderPostHogQuery(salesQuery, vars)
	addedQuery = renderPostHogQuery(addedQuery, vars)
	viewedQuery = renderPostHogQuery(viewedQuery, vars)

	client := posthog.NewClient(host, token)
	salesResp, err := client.Query(projectID, salesQuery)
	if err != nil {
		fmt.Fprintf(stderr, "error: run sales query: %v\n", err)
		return 1
	}
	addedResp, err := client.Query(projectID, addedQuery)
	if err != nil {
		fmt.Fprintf(stderr, "error: run add-to-cart query: %v\n", err)
		return 1
	}
	viewedResp, err := client.Query(projectID, viewedQuery)
	if err != nil {
		fmt.Fprintf(stderr, "error: run product-view query: %v\n", err)
		return 1
	}

	metrics := mergePostHogMetrics(salesResp, addedResp, viewedResp)
	sort.SliceStable(metrics, func(i, j int) bool {
		if metrics[i].TotalValue != metrics[j].TotalValue {
			return metrics[i].TotalValue > metrics[j].TotalValue
		}
		if metrics[i].PurchaseCount != metrics[j].PurchaseCount {
			return metrics[i].PurchaseCount > metrics[j].PurchaseCount
		}
		if metrics[i].ProductViewCount != metrics[j].ProductViewCount {
			return metrics[i].ProductViewCount > metrics[j].ProductViewCount
		}
		return metrics[i].ProductID < metrics[j].ProductID
	})
	totalRows := len(metrics)
	if opts.Limit > 0 && len(metrics) > opts.Limit {
		metrics = metrics[:opts.Limit]
	}

	summary := summarizePostHogMetrics(metrics, totalRows)
	payload := map[string]any{
		"account":      opts.Account,
		"host":         strings.TrimSpace(host),
		"project_id":   projectID,
		"token_source": tokenSource,
		"since":        since.Format(time.RFC3339),
		"until":        until.Format(time.RFC3339),
		"events":       vars,
		"rows":         metrics,
		"summary":      summary,
		"queries": map[string]string{
			"product_sales":  salesQuery,
			"product_added":  addedQuery,
			"product_viewed": viewedQuery,
		},
	}

	if opts.Export != "" || opts.JSON {
		b, _ := json.MarshalIndent(payload, "", "  ")
		if opts.Export != "" {
			exportPath := opts.Export
			if !filepath.IsAbs(exportPath) {
				exportPath = filepath.Join(opts.Root, exportPath)
			}
			if err := os.MkdirAll(filepath.Dir(exportPath), 0o755); err != nil {
				fmt.Fprintf(stderr, "error: create export dir: %v\n", err)
				return 1
			}
			if err := os.WriteFile(exportPath, b, 0o644); err != nil {
				fmt.Fprintf(stderr, "error: write export: %v\n", err)
				return 1
			}
		}
		if opts.JSON {
			fmt.Fprintln(stdout, string(b))
			return 0
		}
	}

	printPostHogProductsTable(stdout, metrics, summary)
	if opts.Export != "" {
		fmt.Fprintf(stdout, "\nexported: %s\n", opts.Export)
	}
	return 0
}

func resolvePostHogRange(sinceRaw, untilRaw string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	until := now
	if strings.TrimSpace(untilRaw) != "" {
		t, err := parseRangeTime(untilRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --until: %w", err)
		}
		until = t.UTC()
	}
	since := until.AddDate(0, 0, -30)
	if strings.TrimSpace(sinceRaw) != "" {
		t, err := parseRangeTime(sinceRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --since: %w", err)
		}
		since = t.UTC()
	}
	return since, until, nil
}

func parseRangeTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339")
}

func resolveQueryOverride(root, inline, filePath, cfgValue, fallback string) (string, error) {
	if strings.TrimSpace(inline) != "" {
		return strings.TrimSpace(inline), nil
	}
	if strings.TrimSpace(filePath) != "" {
		p := strings.TrimSpace(filePath)
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	if strings.TrimSpace(cfgValue) != "" {
		return strings.TrimSpace(cfgValue), nil
	}
	return fallback, nil
}

func renderPostHogQuery(tpl string, vars map[string]string) string {
	out := tpl
	for key, value := range vars {
		escaped := strings.ReplaceAll(value, "'", "''")
		out = strings.ReplaceAll(out, "{{"+key+"}}", escaped)
	}
	return out
}

func defaultPostHogSalesQuery() string {
	return `WITH lines AS (
    SELECT
        JSONExtractString(properties, 'order_id') AS order_id,
        arrayJoin(JSONExtractArrayRaw(properties, 'products')) AS item_raw
    FROM events
    WHERE timestamp >= toDateTime('{{since}}')
      AND timestamp < toDateTime('{{until}}')
      AND event = '{{event_order_completed}}'
)
SELECT
    JSONExtractString(item_raw, 'product_id')      AS product_id,
    JSONExtractString(item_raw, 'product_sku')     AS product_sku,
    JSONExtractString(item_raw, 'product_name')    AS product_name,
    JSONExtractString(item_raw, 'product_variant') AS product_variant,
    sum(toIntOrZero(JSONExtractString(item_raw, 'product_qty'))) AS total_qty,
    uniqExact(order_id) AS purchase_count,
    sum(
        JSONExtractFloat(item_raw, 'product_price')
        *
        toIntOrZero(JSONExtractString(item_raw, 'product_qty'))
    ) AS total_value
FROM lines
GROUP BY 1,2,3,4
ORDER BY total_value DESC`
}

func defaultPostHogAddedQuery() string {
	return `SELECT
    JSONExtractString(properties, 'product_id')      AS product_id,
    JSONExtractString(properties, 'product_sku')     AS product_sku,
    JSONExtractString(properties, 'product_name')    AS product_name,
    JSONExtractString(properties, 'product_variant') AS product_variant,
    count() AS add_to_cart_count
FROM events
WHERE timestamp >= toDateTime('{{since}}')
  AND timestamp < toDateTime('{{until}}')
  AND event = '{{event_product_added}}'
GROUP BY 1,2,3,4
ORDER BY add_to_cart_count DESC`
}

func defaultPostHogViewedQuery() string {
	return `SELECT
    JSONExtractString(properties, 'product_id')      AS product_id,
    JSONExtractString(properties, 'product_sku')     AS product_sku,
    JSONExtractString(properties, 'product_name')    AS product_name,
    JSONExtractString(properties, 'product_variant') AS product_variant,
    count() AS product_view_count
FROM events
WHERE timestamp >= toDateTime('{{since}}')
  AND timestamp < toDateTime('{{until}}')
  AND event = '{{event_product_viewed}}'
GROUP BY 1,2,3,4
ORDER BY product_view_count DESC`
}

func mergePostHogMetrics(sales, added, viewed posthog.QueryResponse) []posthogProductMetrics {
	merged := map[string]posthogProductMetrics{}

	upsert := func(row map[string]any, source string) {
		id := strings.TrimSpace(stringAny(firstAny(row, "product_id", "id")))
		sku := strings.TrimSpace(stringAny(firstAny(row, "product_sku", "sku")))
		name := strings.TrimSpace(stringAny(firstAny(row, "product_name", "name")))
		variant := strings.TrimSpace(stringAny(firstAny(row, "product_variant", "variant")))
		key := ""
		if id != "" {
			key = "id:" + id
		} else if sku != "" {
			key = "sku:" + sku
		} else {
			key = strings.Join([]string{name, variant}, "|")
		}
		if strings.Trim(key, "|") == "" {
			return
		}
		item := merged[key]
		if item.ProductID == "" {
			item.ProductID = id
		}
		if item.ProductSKU == "" {
			item.ProductSKU = sku
		}
		if item.ProductName == "" {
			item.ProductName = name
		}
		if item.ProductVariant == "" {
			item.ProductVariant = variant
		}
		switch source {
		case "sales":
			item.TotalQty = numberAny(firstAny(row, "total_qty", "qty", "quantity"))
			item.PurchaseCount = numberAny(firstAny(row, "purchase_count", "orders", "count"))
			item.TotalValue = numberAny(firstAny(row, "total_value", "revenue", "value"))
		case "added":
			item.AddToCartCount = numberAny(firstAny(row, "add_to_cart_count", "count", "events"))
		case "viewed":
			item.ProductViewCount = numberAny(firstAny(row, "product_view_count", "view_count", "count", "events"))
		}
		merged[key] = item
	}

	for _, row := range rowsFromPostHog(sales) {
		upsert(row, "sales")
	}
	for _, row := range rowsFromPostHog(added) {
		upsert(row, "added")
	}
	for _, row := range rowsFromPostHog(viewed) {
		upsert(row, "viewed")
	}

	out := make([]posthogProductMetrics, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}
	return out
}

func rowsFromPostHog(resp posthog.QueryResponse) []map[string]any {
	rows := make([]map[string]any, 0, len(resp.Results))
	for _, raw := range resp.Results {
		switch t := raw.(type) {
		case map[string]any:
			rows = append(rows, t)
		case []any:
			row := map[string]any{}
			for i := range t {
				key := fmt.Sprintf("col_%d", i)
				if i < len(resp.Columns) && strings.TrimSpace(resp.Columns[i]) != "" {
					key = strings.TrimSpace(resp.Columns[i])
				}
				row[key] = t[i]
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func firstAny(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := row[key]; ok {
			return v
		}
	}
	return nil
}

func numberAny(v any) float64 {
	n, _ := parseNumber(v)
	return n
}

func summarizePostHogMetrics(rows []posthogProductMetrics, totalRows int) map[string]any {
	var totalQty, totalPurchases, totalValue, totalAdded, totalViewed float64
	for _, r := range rows {
		totalQty += r.TotalQty
		totalPurchases += r.PurchaseCount
		totalValue += r.TotalValue
		totalAdded += r.AddToCartCount
		totalViewed += r.ProductViewCount
	}
	return map[string]any{
		"rows_total":         totalRows,
		"rows_output":        len(rows),
		"total_qty":          totalQty,
		"purchase_count":     totalPurchases,
		"total_value":        totalValue,
		"add_to_cart_count":  totalAdded,
		"product_view_count": totalViewed,
	}
}

func printPostHogProductsTable(w io.Writer, rows []posthogProductMetrics, summary map[string]any) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PRODUCT_ID\tSKU\tNAME\tVARIANT\tPURCHASES\tQTY\tVALUE\tADDED\tVIEWED")
	for _, r := range rows {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%.0f\t%.0f\t%.2f\t%.0f\t%.0f\n",
			coalesce(r.ProductID, "-"),
			coalesce(r.ProductSKU, "-"),
			shorten(r.ProductName, 40),
			shorten(r.ProductVariant, 24),
			r.PurchaseCount,
			r.TotalQty,
			r.TotalValue,
			r.AddToCartCount,
			r.ProductViewCount,
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(
		w,
		"summary: rows_total=%d rows_output=%d purchase_count=%.0f total_qty=%.0f total_value=%.2f add_to_cart_count=%.0f product_view_count=%.0f\n",
		intOrZero(summary["rows_total"]),
		intOrZero(summary["rows_output"]),
		numberAny(summary["purchase_count"]),
		numberAny(summary["total_qty"]),
		numberAny(summary["total_value"]),
		numberAny(summary["add_to_cart_count"]),
		numberAny(summary["product_view_count"]),
	)
}
