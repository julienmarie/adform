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

	"adform/internal/gsc"
	"adform/internal/workspace"
)

type gscOptions struct {
	commonOptions
	Since           string
	Until           string
	SiteURL         string
	Type            string
	Dimensions      string
	Country         string
	Platform        string
	QueryContains   string
	PageContains    string
	AggregationType string
	DataState       string
	Limit           int
	RowLimit        int
	Export          string
}

type gscOutputRow struct {
	Dimensions  map[string]string `json:"dimensions,omitempty"`
	Clicks      float64           `json:"clicks"`
	Impressions float64           `json:"impressions"`
	CTR         float64           `json:"ctr"`
	Position    float64           `json:"position"`
}

func runGSC(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts := gscOptions{}
	fs := flag.NewFlagSet("gsc", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Since, "since", "", "Start date/time (YYYY-MM-DD or RFC3339). Default: last 30 days.")
	fs.StringVar(&opts.Until, "until", "", "End date/time (YYYY-MM-DD or RFC3339). Default: now UTC.")
	fs.StringVar(&opts.SiteURL, "site-url", "", "GSC property site URL override (e.g. sc-domain:example.com or https://example.com/)")
	fs.StringVar(&opts.Type, "type", "web", "Search type: web|image|video|news|discover|googleNews")
	fs.StringVar(&opts.Dimensions, "dimensions", "query", "Comma-separated dimensions: query,page,country,platform,date,search_appearance")
	fs.StringVar(&opts.Country, "country", "", "Optional country filter (e.g. USA, PHL)")
	fs.StringVar(&opts.Platform, "platform", "", "Optional platform filter: desktop|mobile|tablet")
	fs.StringVar(&opts.QueryContains, "query", "", "Optional query substring filter")
	fs.StringVar(&opts.PageContains, "page", "", "Optional page substring filter")
	fs.StringVar(&opts.AggregationType, "aggregation", "", "Optional aggregation type: auto|byPage|byProperty")
	fs.StringVar(&opts.DataState, "data-state", "", "Optional data state: all|final")
	fs.IntVar(&opts.Limit, "limit", 1000, "Max rows in output (0 = all)")
	fs.IntVar(&opts.RowLimit, "row-limit", 25000, "API page size per request (1-25000)")
	fs.StringVar(&opts.Export, "export", "", "Write JSON payload to path")
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
	if opts.RowLimit <= 0 || opts.RowLimit > 25000 {
		fmt.Fprintln(stderr, "error: --row-limit must be within 1..25000")
		return 1
	}

	since, until, err := resolveGSCRange(opts.Since, opts.Until)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if since.After(until) {
		fmt.Fprintln(stderr, "error: --since must be <= --until")
		return 1
	}

	cfg, err := workspace.LoadAccountsConfig(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	siteURL := strings.TrimSpace(opts.SiteURL)
	if siteURL == "" && cfg != nil {
		siteURL = strings.TrimSpace(cfg.Platforms.GoogleSearchConsole.SiteURL)
	}
	if siteURL == "" {
		fmt.Fprintln(stderr, "error: GSC site URL is required (set platforms.google_search_console.site_url in accounts.yml or --site-url)")
		return 1
	}

	credsJSON, credsSource, err := workspace.ResolveGSCredentialsJSON(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	client, err := gsc.NewClientFromCredentialsJSON(ctx, credsJSON)
	if err != nil {
		fmt.Fprintf(stderr, "error: init GSC client: %v\n", err)
		return 1
	}

	requestDims, outputDims, err := parseGSCDimensions(opts.Dimensions)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --dimensions: %v\n", err)
		return 1
	}
	searchType, err := normalizeGSCType(opts.Type)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --type: %v\n", err)
		return 1
	}
	aggregationType, err := normalizeGSCAggregationType(opts.AggregationType)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --aggregation: %v\n", err)
		return 1
	}
	dataState, err := normalizeGSCDataState(opts.DataState)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid --data-state: %v\n", err)
		return 1
	}
	filters, err := buildGSCFilters(opts.Country, opts.Platform, opts.QueryContains, opts.PageContains)
	if err != nil {
		fmt.Fprintf(stderr, "error: invalid filter: %v\n", err)
		return 1
	}

	baseReq := gsc.QueryRequest{
		StartDate:  since.Format("2006-01-02"),
		EndDate:    until.Format("2006-01-02"),
		Dimensions: requestDims,
		Type:       searchType,
	}
	if len(filters) > 0 {
		baseReq.DimensionFilterGroups = []gsc.DimensionFilterGroup{
			{
				GroupType: "and",
				Filters:   filters,
			},
		}
	}
	if aggregationType != "" {
		baseReq.AggregationType = aggregationType
	}
	if dataState != "" {
		baseReq.DataState = dataState
	}

	rows := make([]gsc.QueryRow, 0, minInt(opts.Limit, 1000))
	startRow := 0
	remaining := opts.Limit
	for {
		req := baseReq
		req.StartRow = startRow
		req.RowLimit = opts.RowLimit
		if remaining > 0 && remaining < req.RowLimit {
			req.RowLimit = remaining
		}

		resp, err := client.Query(ctx, siteURL, req)
		if err != nil {
			fmt.Fprintf(stderr, "error: query gsc: %v\n", err)
			return 1
		}
		if len(resp.Rows) == 0 {
			break
		}

		rows = append(rows, resp.Rows...)
		fetched := len(resp.Rows)
		startRow += fetched
		if remaining > 0 {
			remaining -= fetched
			if remaining <= 0 {
				break
			}
		}
		if fetched < req.RowLimit {
			break
		}
	}

	outputRows := make([]gscOutputRow, 0, len(rows))
	for _, row := range rows {
		dimVals := map[string]string{}
		for idx, dim := range outputDims {
			val := ""
			if idx < len(row.Keys) {
				val = strings.TrimSpace(row.Keys[idx])
			}
			dimVals[dim] = val
		}
		outputRows = append(outputRows, gscOutputRow{
			Dimensions:  dimVals,
			Clicks:      row.Clicks,
			Impressions: row.Impressions,
			CTR:         row.CTR,
			Position:    row.Position,
		})
	}

	sort.SliceStable(outputRows, func(i, j int) bool {
		if outputRows[i].Clicks != outputRows[j].Clicks {
			return outputRows[i].Clicks > outputRows[j].Clicks
		}
		return outputRows[i].Impressions > outputRows[j].Impressions
	})
	summary := summarizeGSCRows(outputRows)

	payload := map[string]any{
		"account":            opts.Account,
		"site_url":           siteURL,
		"credentials_source": credsSource,
		"since":              since.Format("2006-01-02"),
		"until":              until.Format("2006-01-02"),
		"type":               searchType,
		"dimensions":         outputDims,
		"aggregation_type":   aggregationType,
		"data_state":         dataState,
		"filters":            filters,
		"rows":               outputRows,
		"summary":            summary,
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

	printGSCTable(stdout, outputRows, outputDims, summary)
	if opts.Export != "" {
		fmt.Fprintf(stdout, "\nexported: %s\n", opts.Export)
	}
	return 0
}

func resolveGSCRange(sinceRaw, untilRaw string) (time.Time, time.Time, error) {
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

func parseGSCDimensions(raw string) ([]string, []string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "query"
	}
	parts := strings.Split(raw, ",")
	requestDims := make([]string, 0, len(parts))
	outputDims := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}
		req := ""
		out := ""
		switch token {
		case "query":
			req, out = "query", "query"
		case "page":
			req, out = "page", "page"
		case "country":
			req, out = "country", "country"
		case "platform", "device":
			req, out = "device", "platform"
		case "date":
			req, out = "date", "date"
		case "searchappearance", "search_appearance":
			req, out = "searchAppearance", "search_appearance"
		default:
			return nil, nil, fmt.Errorf("unsupported dimension %q", token)
		}
		if seen[out] {
			continue
		}
		seen[out] = true
		requestDims = append(requestDims, req)
		outputDims = append(outputDims, out)
	}
	if len(requestDims) == 0 {
		return nil, nil, fmt.Errorf("no dimensions provided")
	}
	return requestDims, outputDims, nil
}

func normalizeGSCType(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return "web", nil
	}
	switch v {
	case "web", "image", "video", "news", "discover":
		return v, nil
	case "googlenews", "google_news", "google-news":
		return "googleNews", nil
	default:
		return "", fmt.Errorf("supported values: web,image,video,news,discover,googleNews")
	}
}

func normalizeGSCAggregationType(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	switch v {
	case "auto", "byPage", "byProperty":
		return v, nil
	default:
		return "", fmt.Errorf("supported values: auto,byPage,byProperty")
	}
}

func normalizeGSCDataState(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return "", nil
	}
	switch v {
	case "all", "final":
		return v, nil
	default:
		return "", fmt.Errorf("supported values: all,final")
	}
}

func normalizeGSCPlatform(raw string) (string, error) {
	v := strings.TrimSpace(strings.ToUpper(raw))
	if v == "" {
		return "", nil
	}
	switch v {
	case "DESKTOP", "MOBILE", "TABLET":
		return v, nil
	default:
		return "", fmt.Errorf("supported values: desktop,mobile,tablet")
	}
}

func buildGSCFilters(country, platform, queryContains, pageContains string) ([]gsc.DimensionFilter, error) {
	filters := []gsc.DimensionFilter{}
	if c := strings.TrimSpace(strings.ToUpper(country)); c != "" {
		filters = append(filters, gsc.DimensionFilter{
			Dimension:  "country",
			Operator:   "equals",
			Expression: c,
		})
	}
	if p, err := normalizeGSCPlatform(platform); err != nil {
		return nil, err
	} else if p != "" {
		filters = append(filters, gsc.DimensionFilter{
			Dimension:  "device",
			Operator:   "equals",
			Expression: p,
		})
	}
	if q := strings.TrimSpace(queryContains); q != "" {
		filters = append(filters, gsc.DimensionFilter{
			Dimension:  "query",
			Operator:   "contains",
			Expression: q,
		})
	}
	if p := strings.TrimSpace(pageContains); p != "" {
		filters = append(filters, gsc.DimensionFilter{
			Dimension:  "page",
			Operator:   "contains",
			Expression: p,
		})
	}
	return filters, nil
}

func summarizeGSCRows(rows []gscOutputRow) map[string]any {
	totalClicks := 0.0
	totalImpressions := 0.0
	positionWeightedSum := 0.0
	for _, row := range rows {
		totalClicks += row.Clicks
		totalImpressions += row.Impressions
		positionWeightedSum += row.Position * row.Impressions
	}
	ctr := 0.0
	position := 0.0
	if totalImpressions > 0 {
		ctr = totalClicks / totalImpressions
		position = positionWeightedSum / totalImpressions
	}
	return map[string]any{
		"rows":        len(rows),
		"clicks":      totalClicks,
		"impressions": totalImpressions,
		"ctr":         ctr,
		"position":    position,
	}
}

func printGSCTable(w io.Writer, rows []gscOutputRow, dimensions []string, summary map[string]any) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := make([]string, 0, len(dimensions)+4)
	for _, dim := range dimensions {
		header = append(header, strings.ToUpper(dim))
	}
	header = append(header, "CLICKS", "IMPRESSIONS", "CTR%", "POSITION")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, row := range rows {
		values := make([]string, 0, len(dimensions)+4)
		for _, dim := range dimensions {
			values = append(values, shorten(coalesce(row.Dimensions[dim], "-"), 80))
		}
		values = append(
			values,
			fmt.Sprintf("%.0f", row.Clicks),
			fmt.Sprintf("%.0f", row.Impressions),
			fmt.Sprintf("%.2f", row.CTR*100),
			fmt.Sprintf("%.2f", row.Position),
		)
		fmt.Fprintln(tw, strings.Join(values, "\t"))
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(
		w,
		"summary: rows=%d clicks=%.0f impressions=%.0f ctr=%.2f position=%.2f\n",
		intOrZero(summary["rows"]),
		floatOrZero(summary["clicks"]),
		floatOrZero(summary["impressions"]),
		floatOrZero(summary["ctr"])*100,
		floatOrZero(summary["position"]),
	)
}

func minInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if a < b {
		return a
	}
	return b
}
