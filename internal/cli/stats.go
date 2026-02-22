package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"adform/internal/config"
	"adform/internal/state"
)

type statsOptions struct {
	commonOptions
	Level        string
	Last         string
	Compare      string
	Breakdown    string
	Event        string
	Export       string
	SaveSnapshot bool
	Limit        int
}

type statsRow struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Spend           float64        `json:"spend"`
	Impressions     int64          `json:"impressions"`
	Clicks          int64          `json:"clicks"`
	CTR             float64        `json:"ctr"`
	CPC             float64        `json:"cpc"`
	Conversions     float64        `json:"conversions"`
	ConversionValue float64        `json:"conversion_value"`
	CPA             float64        `json:"cpa,omitempty"`
	ROAS            float64        `json:"roas,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
}

func runStats(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := statsOptions{}
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Level, "level", "campaign", "Insights level: campaign|adset|ad")
	fs.StringVar(&opts.Last, "last", "7d", "Time window (e.g. 7d, 30d, last_7d, this_month)")
	fs.StringVar(&opts.Compare, "compare", "", "Optional comparison window label (informational)")
	fs.StringVar(&opts.Breakdown, "breakdown", "", "Comma-separated breakdowns (e.g. age,gender,platform_position)")
	fs.StringVar(&opts.Event, "event", "purchase", "Conversion action type (purchase alias supported)")
	fs.StringVar(&opts.Export, "export", "", "Write stats JSON payload to path")
	fs.BoolVar(&opts.SaveSnapshot, "save-snapshot", false, "Persist stats JSON into SQLite snapshots table")
	fs.IntVar(&opts.Limit, "limit", 500, "Max rows per page for insights fetch")
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

	level := strings.ToLower(strings.TrimSpace(opts.Level))
	switch level {
	case "campaign", "adset", "ad":
	default:
		fmt.Fprintf(stderr, "error: invalid --level %q (expected campaign|adset|ad)\n", opts.Level)
		return 1
	}

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to load account config: %v\n", err)
		return 1
	}
	adAccountID := strings.TrimSpace(bundle.AccountCfg.Meta.AdAccountID)
	if adAccountID == "" {
		fmt.Fprintln(stderr, "error: account.meta.ad_account_id is required")
		return 1
	}
	client, _, err := metaClientForAccount(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	params, err := buildStatsParams(opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	rows, err := client.ListEdge(normalizeActID(adAccountID)+"/insights", statsFields(level), params)
	if err != nil {
		fmt.Fprintf(stderr, "error: fetch insights: %v\n", err)
		return 1
	}

	parsed := make([]statsRow, 0, len(rows))
	for _, raw := range rows {
		r := buildStatsRow(level, raw, opts.Event)
		if opts.Verbose {
			r.Raw = raw
		}
		parsed = append(parsed, r)
	}
	sort.SliceStable(parsed, func(i, j int) bool { return parsed[i].Spend > parsed[j].Spend })

	summary := summarizeStats(parsed)
	payload := map[string]any{
		"account":    opts.Account,
		"ad_account": normalizeActID(adAccountID),
		"level":      level,
		"last":       opts.Last,
		"compare":    opts.Compare,
		"event":      opts.Event,
		"rows":       parsed,
		"summary":    summary,
	}

	if opts.Export != "" || opts.SaveSnapshot || opts.JSON {
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
		if opts.SaveSnapshot {
			st, err := state.Open(opts.StatePath)
			if err != nil {
				fmt.Fprintf(stderr, "error: open state: %v\n", err)
				return 1
			}
			defer st.Close()
			if err := st.InsertSnapshot(opts.Account, string(b)); err != nil {
				fmt.Fprintf(stderr, "error: save snapshot: %v\n", err)
				return 1
			}
		}
		if opts.JSON {
			fmt.Fprintln(stdout, string(b))
			return 0
		}
	}

	printStatsTable(stdout, parsed, summary)
	if opts.Export != "" {
		fmt.Fprintf(stdout, "\nexported: %s\n", opts.Export)
	}
	if opts.SaveSnapshot {
		fmt.Fprintf(stdout, "snapshot: saved to %s\n", opts.StatePath)
	}
	return 0
}

func buildStatsParams(opts statsOptions) (url.Values, error) {
	params := url.Values{}
	params.Set("level", strings.ToLower(strings.TrimSpace(opts.Level)))
	if opts.Limit <= 0 {
		opts.Limit = 500
	}
	params.Set("limit", strconv.Itoa(opts.Limit))
	if b := strings.TrimSpace(opts.Breakdown); b != "" {
		params.Set("breakdowns", b)
	}

	last := strings.ToLower(strings.TrimSpace(opts.Last))
	switch last {
	case "today", "yesterday", "this_month", "last_month", "last_3d", "last_7d", "last_14d", "last_28d", "last_30d", "last_90d", "lifetime", "maximum":
		params.Set("date_preset", last)
		return params, nil
	}
	if strings.HasPrefix(last, "last_") {
		params.Set("date_preset", last)
		return params, nil
	}
	if strings.HasSuffix(last, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(last, "d"))
		if err != nil || days <= 0 {
			return nil, fmt.Errorf("invalid --last %q", opts.Last)
		}
		switch days {
		case 3, 7, 14, 28, 30, 90:
			params.Set("date_preset", fmt.Sprintf("last_%dd", days))
		default:
			until := time.Now().UTC()
			since := until.AddDate(0, 0, -days)
			timeRange, _ := json.Marshal(map[string]string{
				"since": since.Format("2006-01-02"),
				"until": until.Format("2006-01-02"),
			})
			params.Set("time_range", string(timeRange))
		}
		return params, nil
	}
	return nil, fmt.Errorf("invalid --last %q", opts.Last)
}

func statsFields(level string) []string {
	fields := []string{"spend", "impressions", "clicks", "ctr", "cpc", "actions", "action_values"}
	switch level {
	case "campaign":
		return append([]string{"campaign_id", "campaign_name"}, fields...)
	case "adset":
		return append([]string{"adset_id", "adset_name", "campaign_name"}, fields...)
	case "ad":
		return append([]string{"ad_id", "ad_name", "adset_name", "campaign_name"}, fields...)
	default:
		return append([]string{"campaign_id", "campaign_name"}, fields...)
	}
}

func buildStatsRow(level string, raw map[string]any, event string) statsRow {
	id, name := rowIdentity(level, raw)
	spend, _ := parseNumber(raw["spend"])
	impressions := parseInt(raw["impressions"])
	clicks := parseInt(raw["clicks"])
	ctr, _ := parseNumber(raw["ctr"])
	cpc, _ := parseNumber(raw["cpc"])
	conversions := sumActionMetric(raw["actions"], event)
	conversionValue := sumActionMetric(raw["action_values"], event)
	row := statsRow{
		ID:              id,
		Name:            name,
		Spend:           spend,
		Impressions:     impressions,
		Clicks:          clicks,
		CTR:             ctr,
		CPC:             cpc,
		Conversions:     conversions,
		ConversionValue: conversionValue,
	}
	if conversions > 0 {
		row.CPA = spend / conversions
	}
	if spend > 0 {
		row.ROAS = conversionValue / spend
	}
	return row
}

func rowIdentity(level string, raw map[string]any) (string, string) {
	switch level {
	case "campaign":
		return stringAny(raw["campaign_id"]), stringAny(raw["campaign_name"])
	case "adset":
		return stringAny(raw["adset_id"]), stringAny(raw["adset_name"])
	case "ad":
		return stringAny(raw["ad_id"]), stringAny(raw["ad_name"])
	default:
		return stringAny(raw["campaign_id"]), stringAny(raw["campaign_name"])
	}
}

func stringAny(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func parseNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func parseInt(v any) int64 {
	if f, ok := parseNumber(v); ok {
		return int64(f)
	}
	return 0
}

func sumActionMetric(actionsRaw any, event string) float64 {
	totals := actionTotals(actionsRaw)
	if len(totals) == 0 {
		return 0
	}
	event = normalizeEvent(event)
	if event == "" || event == "all" || event == "*" {
		total := 0.0
		for _, v := range totals {
			total += v
		}
		return total
	}

	if strings.Contains(event, ",") {
		total := 0.0
		for _, part := range strings.Split(event, ",") {
			ev := normalizeEvent(part)
			if ev == "" {
				continue
			}
			if v, ok := totals[ev]; ok {
				total += v
			}
		}
		return total
	}

	// "purchase" is ambiguous in Meta actions. Prefer a canonical source
	// instead of summing all purchase-like rows (which double-counts).
	if event == "purchase" {
		for _, key := range []string{
			"offsite_conversion.fb_pixel_purchase",
			"omni_purchase",
			"onsite_conversion.purchase",
			"app_custom_event.fb_mobile_purchase",
			"purchase",
		} {
			if v, ok := totals[key]; ok {
				return v
			}
		}
		// Fallback: choose the largest purchase-like metric if only variants are present.
		best := 0.0
		found := false
		for key, value := range totals {
			if strings.HasSuffix(key, ".purchase") || strings.HasSuffix(key, ":purchase") || key == "purchase" || strings.HasSuffix(key, "_purchase") {
				if !found || value > best {
					best = value
					found = true
				}
			}
		}
		if found {
			return best
		}
		return 0
	}

	if v, ok := totals[event]; ok {
		return v
	}
	for key, v := range totals {
		if strings.HasSuffix(key, "."+event) || strings.HasSuffix(key, ":"+event) {
			return v
		}
	}
	return 0
}

func actionTotals(actionsRaw any) map[string]float64 {
	actions, ok := actionsRaw.([]any)
	if !ok {
		return nil
	}
	out := map[string]float64{}
	for _, item := range actions {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		actionType := normalizeEvent(stringAny(m["action_type"]))
		if actionType == "" {
			continue
		}
		if v, ok := parseNumber(m["value"]); ok {
			out[actionType] += v
		}
	}
	return out
}

func normalizeEvent(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func summarizeStats(rows []statsRow) map[string]any {
	var spend, conv, convValue float64
	var impressions, clicks int64
	for _, r := range rows {
		spend += r.Spend
		impressions += r.Impressions
		clicks += r.Clicks
		conv += r.Conversions
		convValue += r.ConversionValue
	}
	out := map[string]any{
		"rows":             len(rows),
		"spend":            spend,
		"impressions":      impressions,
		"clicks":           clicks,
		"conversions":      conv,
		"conversion_value": convValue,
	}
	if impressions > 0 {
		out["ctr"] = (float64(clicks) * 100) / float64(impressions)
	}
	if clicks > 0 {
		out["cpc"] = spend / float64(clicks)
	}
	if conv > 0 {
		out["cpa"] = spend / conv
	}
	if spend > 0 {
		out["roas"] = convValue / spend
	}
	return out
}

func printStatsTable(w io.Writer, rows []statsRow, summary map[string]any) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSPEND\tIMPRESSIONS\tCLICKS\tCTR%\tCPC\tCONV\tCONV_VALUE\tCPA\tROAS")
	for _, r := range rows {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%.2f\t%d\t%d\t%.2f\t%.2f\t%.2f\t%.2f\t%s\t%s\n",
			r.ID,
			r.Name,
			r.Spend,
			r.Impressions,
			r.Clicks,
			r.CTR,
			r.CPC,
			r.Conversions,
			r.ConversionValue,
			formatOptional(r.CPA),
			formatOptional(r.ROAS),
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(
		w,
		"summary: spend=%.2f impressions=%d clicks=%d conversions=%.2f conversion_value=%.2f ctr=%.2f cpc=%.2f cpa=%s roas=%s rows=%d\n",
		floatOrZero(summary["spend"]),
		intOrZero(summary["impressions"]),
		intOrZero(summary["clicks"]),
		floatOrZero(summary["conversions"]),
		floatOrZero(summary["conversion_value"]),
		floatOrZero(summary["ctr"]),
		floatOrZero(summary["cpc"]),
		formatOptional(floatOrZero(summary["cpa"])),
		formatOptional(floatOrZero(summary["roas"])),
		int(intOrZero(summary["rows"])),
	)
}

func formatOptional(v float64) string {
	if v <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", v)
}

func floatOrZero(v any) float64 {
	f, _ := parseNumber(v)
	return f
}

func intOrZero(v any) int64 {
	return parseInt(v)
}

func normalizeActID(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "act_") {
		return v
	}
	return "act_" + v
}
