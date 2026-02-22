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
)

type logOptions struct {
	commonOptions
	Since  string
	Until  string
	Limit  int
	Export string
}

type activityLogEntry struct {
	EventTime       string         `json:"event_time,omitempty"`
	EventType       string         `json:"event_type,omitempty"`
	ActorName       string         `json:"actor_name,omitempty"`
	ObjectType      string         `json:"object_type,omitempty"`
	ObjectID        string         `json:"object_id,omitempty"`
	ObjectName      string         `json:"object_name,omitempty"`
	ApplicationName string         `json:"application_name,omitempty"`
	ExtraData       map[string]any `json:"extra_data,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
	SortUnix        int64          `json:"-"`
}

func runLog(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := logOptions{}
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.Since, "since", "", "Start time (YYYY-MM-DD, RFC3339, or unix seconds)")
	fs.StringVar(&opts.Until, "until", "", "End time (YYYY-MM-DD, RFC3339, or unix seconds)")
	fs.IntVar(&opts.Limit, "limit", 200, "Rows per page requested from Meta")
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

	params, sinceNorm, untilNorm, err := buildLogParams(opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	edge := normalizeActID(adAccountID) + "/activities"
	rawRows, err := client.ListEdge(edge, activityFields(), params)
	if err != nil && shouldFallbackActivityFields(err) {
		rawRows, err = client.ListEdge(edge, nil, params)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: fetch activities: %v\n", err)
		return 1
	}

	entries := make([]activityLogEntry, 0, len(rawRows))
	for _, row := range rawRows {
		entry := buildActivityEntry(row)
		if opts.Verbose {
			entry.Raw = row
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].SortUnix != entries[j].SortUnix {
			return entries[i].SortUnix > entries[j].SortUnix
		}
		if entries[i].EventTime != entries[j].EventTime {
			return entries[i].EventTime > entries[j].EventTime
		}
		return entries[i].ObjectID < entries[j].ObjectID
	})

	summary := summarizeActivityLogs(entries)
	payload := map[string]any{
		"account":    opts.Account,
		"ad_account": normalizeActID(adAccountID),
		"since":      sinceNorm,
		"until":      untilNorm,
		"entries":    entries,
		"summary":    summary,
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

	printActivityTable(stdout, entries)
	if opts.Export != "" {
		fmt.Fprintf(stdout, "\nexported: %s\n", opts.Export)
	}
	return 0
}

func buildLogParams(opts logOptions) (url.Values, string, string, error) {
	params := url.Values{}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	params.Set("limit", strconv.Itoa(limit))

	sinceAPI, sinceCmp, err := normalizeLogBound(strings.TrimSpace(opts.Since))
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid --since: %w", err)
	}
	if sinceAPI != "" {
		params.Set("since", sinceAPI)
	}

	untilAPI, untilCmp, err := normalizeLogBound(strings.TrimSpace(opts.Until))
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid --until: %w", err)
	}
	if untilAPI != "" {
		params.Set("until", untilAPI)
	}

	if sinceCmp > 0 && untilCmp > 0 && sinceCmp > untilCmp {
		return nil, "", "", fmt.Errorf("--since must be <= --until")
	}

	return params, sinceAPI, untilAPI, nil
}

func normalizeLogBound(raw string) (apiValue string, unixCmp int64, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, nil
	}
	if _, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
		n, _ := strconv.ParseInt(raw, 10, 64)
		return raw, n, nil
	}
	if t, parseErr := time.Parse(time.RFC3339, raw); parseErr == nil {
		return strconv.FormatInt(t.Unix(), 10), t.Unix(), nil
	}
	if t, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
		return strconv.FormatInt(t.Unix(), 10), t.Unix(), nil
	}
	if t, parseErr := time.Parse("2006-01-02", raw); parseErr == nil {
		t = t.UTC()
		return raw, t.Unix(), nil
	}
	return "", 0, fmt.Errorf("expected YYYY-MM-DD, RFC3339, or unix seconds")
}

func buildActivityEntry(raw map[string]any) activityLogEntry {
	eventTime := firstNonEmpty(raw["event_time"], raw["created_time"], raw["time"])
	entry := activityLogEntry{
		EventTime:       eventTime,
		EventType:       firstNonEmpty(raw["event_type"], raw["translated_event_type"]),
		ActorName:       firstNonEmpty(raw["actor_name"], raw["actor_id"]),
		ObjectType:      firstNonEmpty(raw["object_type"], raw["object_class"]),
		ObjectID:        firstNonEmpty(raw["object_id"], raw["object"]),
		ObjectName:      firstNonEmpty(raw["object_name"]),
		ApplicationName: firstNonEmpty(raw["application_name"], raw["application_id"]),
		ExtraData:       mapFromAny(raw["extra_data"]),
		SortUnix:        activityUnix(eventTime),
	}
	return entry
}

func activityFields() []string {
	return []string{
		"event_time",
		"event_type",
		"translated_event_type",
		"actor_name",
		"actor_id",
		"object_type",
		"object_id",
		"object_name",
		"application_name",
		"application_id",
		"extra_data",
	}
}

func shouldFallbackActivityFields(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nonexisting field") || strings.Contains(msg, "tried accessing nonexisting field") || strings.Contains(msg, "unsupported field")
}

func firstNonEmpty(values ...any) string {
	for _, v := range values {
		s := stringAny(v)
		if s != "" {
			return s
		}
	}
	return ""
}

func mapFromAny(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

func activityUnix(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func summarizeActivityLogs(entries []activityLogEntry) map[string]any {
	byType := map[string]int{}
	byObject := map[string]int{}
	for _, e := range entries {
		if e.EventType != "" {
			byType[e.EventType]++
		}
		if e.ObjectType != "" {
			byObject[e.ObjectType]++
		}
	}
	return map[string]any{
		"rows":           len(entries),
		"by_event_type":  byType,
		"by_object_type": byObject,
	}
}

func printActivityTable(w io.Writer, entries []activityLogEntry) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tEVENT\tOBJECT_TYPE\tOBJECT_ID\tOBJECT_NAME\tACTOR")
	for _, e := range entries {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			coalesce(e.EventTime, "-"),
			coalesce(e.EventType, "-"),
			coalesce(e.ObjectType, "-"),
			coalesce(e.ObjectID, "-"),
			coalesce(e.ObjectName, "-"),
			coalesce(e.ActorName, "-"),
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(w, "summary: rows=%d\n", len(entries))
}

func coalesce(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
