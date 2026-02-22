package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"adform/internal/config"
)

type feedOptions struct {
	commonOptions
	URL    string
	Format string
	Limit  int
	Export string
}

type feedProduct struct {
	ID           string `json:"id,omitempty"`
	Title        string `json:"title,omitempty"`
	URL          string `json:"url,omitempty"`
	Price        string `json:"price,omitempty"`
	SalePrice    string `json:"sale_price,omitempty"`
	Availability string `json:"availability,omitempty"`
	InStock      *bool  `json:"in_stock,omitempty"`
}

func runFeed(_ context.Context, args []string, stdout, stderr io.Writer) int {
	opts := feedOptions{}
	fs := flag.NewFlagSet("feed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bindCommonFlags(fs, &opts.commonOptions)
	fs.StringVar(&opts.URL, "url", "", "Override product feed URL (defaults to account.meta.product_feed_url)")
	fs.StringVar(&opts.Format, "format", "auto", "Feed format: auto|xml|csv")
	fs.IntVar(&opts.Limit, "limit", 0, "Limit output rows (0 = all)")
	fs.StringVar(&opts.Export, "export", "", "Write feed JSON payload to path")
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
	opts.Format = strings.ToLower(strings.TrimSpace(opts.Format))
	if opts.Format == "" {
		opts.Format = "auto"
	}
	if opts.Format != "auto" && opts.Format != "xml" && opts.Format != "csv" {
		fmt.Fprintf(stderr, "error: invalid --format %q (expected auto|xml|csv)\n", opts.Format)
		return 1
	}
	if opts.Limit < 0 {
		fmt.Fprintln(stderr, "error: --limit must be >= 0")
		return 1
	}

	bundle, err := config.Load(opts.Root, opts.Account)
	if err != nil {
		fmt.Fprintf(stderr, "error: unable to load account config: %v\n", err)
		return 1
	}
	feedURL := strings.TrimSpace(opts.URL)
	if feedURL == "" {
		feedURL = strings.TrimSpace(bundle.AccountCfg.Meta.ProductFeedURL)
	}
	if feedURL == "" {
		fmt.Fprintln(stderr, "error: missing feed URL; set account.meta.product_feed_url or pass --url")
		return 1
	}

	data, contentType, err := fetchFeed(feedURL)
	if err != nil {
		fmt.Fprintf(stderr, "error: fetch feed: %v\n", err)
		return 1
	}
	products, detectedFormat, err := parseFeedProducts(data, opts.Format, feedURL, contentType)
	if err != nil {
		fmt.Fprintf(stderr, "error: parse feed: %v\n", err)
		return 1
	}

	for i := range products {
		finalizeProduct(&products[i])
	}
	sort.SliceStable(products, func(i, j int) bool {
		if products[i].ID != products[j].ID {
			return products[i].ID < products[j].ID
		}
		return products[i].URL < products[j].URL
	})

	total := len(products)
	if opts.Limit > 0 && len(products) > opts.Limit {
		products = products[:opts.Limit]
	}

	summary := summarizeFeed(products, total)
	payload := map[string]any{
		"account":         opts.Account,
		"feed_url":        feedURL,
		"detected_format": detectedFormat,
		"fetched_at":      time.Now().UTC().Format(time.RFC3339),
		"products":        products,
		"summary":         summary,
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

	printFeedTable(stdout, products, summary)
	if opts.Export != "" {
		fmt.Fprintf(stdout, "\nexported: %s\n", opts.Export)
	}
	return 0
}

func fetchFeed(feedURL string) ([]byte, string, error) {
	if strings.HasPrefix(feedURL, "file://") {
		u, err := neturl.Parse(feedURL)
		if err != nil {
			return nil, "", err
		}
		b, err := os.ReadFile(u.Path)
		return b, "", err
	}

	u, err := neturl.Parse(feedURL)
	if err != nil {
		return nil, "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", fmt.Errorf("unsupported scheme %q (expected http/https)", u.Scheme)
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "adform-feed/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return b, resp.Header.Get("Content-Type"), nil
}

func parseFeedProducts(data []byte, format, feedURL, contentType string) ([]feedProduct, string, error) {
	detected := detectFeedFormat(format, data, feedURL, contentType)
	switch detected {
	case "xml":
		out, err := parseXMLFeed(data)
		return out, detected, err
	case "csv":
		out, err := parseCSVFeed(data)
		return out, detected, err
	default:
		return nil, detected, fmt.Errorf("unable to detect feed format")
	}
}

func detectFeedFormat(format string, data []byte, feedURL, contentType string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "xml" || format == "csv" {
		return format
	}

	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "xml") || strings.Contains(ct, "rss") || strings.Contains(ct, "atom") {
		return "xml"
	}
	if strings.Contains(ct, "csv") || strings.Contains(ct, "tab-separated") || strings.Contains(ct, "tsv") {
		return "csv"
	}

	trimmed := bytes.TrimSpace(data)
	trimmed = bytes.TrimPrefix(trimmed, []byte("\xef\xbb\xbf"))
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return "xml"
	}

	lowerURL := strings.ToLower(feedURL)
	if strings.HasSuffix(lowerURL, ".xml") || strings.HasSuffix(lowerURL, ".rss") || strings.HasSuffix(lowerURL, ".atom") {
		return "xml"
	}
	if strings.HasSuffix(lowerURL, ".csv") || strings.HasSuffix(lowerURL, ".tsv") || strings.HasSuffix(lowerURL, ".txt") {
		return "csv"
	}
	return "csv"
}

func parseXMLFeed(data []byte) ([]feedProduct, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	products := make([]feedProduct, 0, 128)

	inItem := false
	cur := feedProduct{}
	stack := make([]string, 0, 16)
	var charBuf strings.Builder

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			stack = append(stack, name)
			if name == "item" || name == "entry" || name == "product" {
				inItem = true
				cur = feedProduct{}
			}
			if inItem && name == "link" && cur.URL == "" {
				for _, a := range t.Attr {
					if strings.EqualFold(a.Name.Local, "href") && strings.TrimSpace(a.Value) != "" {
						cur.URL = strings.TrimSpace(a.Value)
						break
					}
				}
			}
			charBuf.Reset()

		case xml.CharData:
			if inItem {
				charBuf.Write([]byte(t))
			}

		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if inItem {
				text := strings.TrimSpace(charBuf.String())
				assignXMLField(&cur, stack, text)
				charBuf.Reset()
				if name == "item" || name == "entry" || name == "product" {
					finalizeProduct(&cur)
					if hasFeedProductData(cur) {
						products = append(products, cur)
					}
					inItem = false
				}
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	return products, nil
}

func assignXMLField(p *feedProduct, stack []string, text string) {
	if len(stack) == 0 || text == "" {
		return
	}
	leaf := stack[len(stack)-1]
	switch leaf {
	case "id", "item_id", "retailer_id", "sku":
		if p.ID == "" || looksLikeURL(p.ID) {
			p.ID = text
		}
	case "title", "name":
		if p.Title == "" {
			p.Title = text
		}
	case "link", "url":
		if p.URL == "" && looksLikeURL(text) {
			p.URL = text
		}
	case "price":
		if containsPathToken(stack, "shipping") {
			return
		}
		if p.Price == "" {
			p.Price = text
		}
	case "sale_price", "saleprice":
		if p.SalePrice == "" {
			p.SalePrice = text
		}
	case "availability", "stock_status", "stockstatus", "in_stock", "instock":
		if p.Availability == "" {
			p.Availability = text
		}
	}
}

func containsPathToken(path []string, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	for _, part := range path {
		if strings.ToLower(strings.TrimSpace(part)) == token {
			return true
		}
	}
	return false
}

func parseCSVFeed(data []byte) ([]feedProduct, error) {
	comma := detectCSVDelimiter(data)
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []feedProduct{}, nil
	}

	header := map[string]int{}
	for idx, cell := range rows[0] {
		key := normalizeCSVHeader(cell)
		if key != "" {
			header[key] = idx
		}
	}
	get := func(row []string, keys ...string) string {
		for _, k := range keys {
			if idx, ok := header[k]; ok && idx < len(row) {
				val := strings.TrimSpace(row[idx])
				if val != "" {
					return val
				}
			}
		}
		return ""
	}

	products := make([]feedProduct, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		p := feedProduct{
			ID:           get(row, "id", "item_id", "retailer_id", "sku", "offer_id"),
			Title:        get(row, "title", "name", "product_name"),
			URL:          get(row, "link", "url", "product_url", "landing_page"),
			Price:        get(row, "price", "current_price", "regular_price"),
			SalePrice:    get(row, "sale_price", "discount_price", "promo_price"),
			Availability: get(row, "availability", "stock_status", "in_stock", "instock", "stock"),
		}
		finalizeProduct(&p)
		if hasFeedProductData(p) {
			products = append(products, p)
		}
	}
	return products, nil
}

func detectCSVDelimiter(data []byte) rune {
	firstLine := string(data)
	if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
		firstLine = firstLine[:i]
	}
	candidates := []rune{',', ';', '\t'}
	best := ','
	bestCount := -1
	for _, c := range candidates {
		count := strings.Count(firstLine, string(c))
		if count > bestCount {
			best = c
			bestCount = count
		}
	}
	return best
}

func normalizeCSVHeader(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "\ufeff")
	if raw == "" {
		return ""
	}
	repl := strings.NewReplacer(" ", "_", "-", "_", "/", "_", ".", "_", ":", "_")
	raw = repl.Replace(raw)
	raw = strings.Trim(raw, "_")
	return raw
}

func finalizeProduct(p *feedProduct) {
	p.ID = strings.TrimSpace(p.ID)
	p.Title = strings.TrimSpace(p.Title)
	p.URL = strings.TrimSpace(p.URL)
	p.Price = strings.TrimSpace(p.Price)
	p.SalePrice = strings.TrimSpace(p.SalePrice)
	p.Availability = strings.TrimSpace(p.Availability)

	if p.InStock == nil && p.Availability != "" {
		p.InStock = parseInStock(p.Availability)
	}
}

func parseInStock(v string) *bool {
	norm := strings.ToLower(strings.TrimSpace(v))
	norm = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(norm)
	switch norm {
	case "instock", "available", "true", "1", "yes", "y":
		b := true
		return &b
	case "outofstock", "oos", "unavailable", "false", "0", "no", "n", "preorder", "backorder", "discontinued":
		b := false
		return &b
	}
	if strings.Contains(norm, "instock") || strings.Contains(norm, "available") {
		b := true
		return &b
	}
	if strings.Contains(norm, "outofstock") || strings.Contains(norm, "unavailable") {
		b := false
		return &b
	}
	return nil
}

func hasFeedProductData(p feedProduct) bool {
	return p.ID != "" || p.Title != "" || p.URL != "" || p.Price != "" || p.SalePrice != "" || p.Availability != ""
}

func summarizeFeed(products []feedProduct, totalRows int) map[string]any {
	inStock := 0
	outOfStock := 0
	unknown := 0
	withPrice := 0
	withSalePrice := 0
	withURL := 0

	for _, p := range products {
		if p.Price != "" {
			withPrice++
		}
		if p.SalePrice != "" {
			withSalePrice++
		}
		if p.URL != "" {
			withURL++
		}
		if p.InStock == nil {
			unknown++
			continue
		}
		if *p.InStock {
			inStock++
		} else {
			outOfStock++
		}
	}

	return map[string]any{
		"rows_total":          totalRows,
		"rows_output":         len(products),
		"in_stock":            inStock,
		"out_of_stock":        outOfStock,
		"stock_unknown":       unknown,
		"with_price":          withPrice,
		"with_sale_price":     withSalePrice,
		"with_product_url":    withURL,
		"without_product_url": len(products) - withURL,
	}
}

func printFeedTable(w io.Writer, products []feedProduct, summary map[string]any) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tPRICE\tSALE_PRICE\tURL\tIN_STOCK\tAVAILABILITY")
	for _, p := range products {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			coalesce(p.ID, "-"),
			shorten(p.Title, 48),
			coalesce(p.Price, "-"),
			coalesce(p.SalePrice, "-"),
			shorten(p.URL, 72),
			inStockLabel(p.InStock),
			coalesce(p.Availability, "-"),
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)

	fmt.Fprintf(
		w,
		"summary: rows_total=%d rows_output=%d in_stock=%d out_of_stock=%d stock_unknown=%d with_price=%d with_sale_price=%d with_product_url=%d\n",
		intOrZero(summary["rows_total"]),
		intOrZero(summary["rows_output"]),
		intOrZero(summary["in_stock"]),
		intOrZero(summary["out_of_stock"]),
		intOrZero(summary["stock_unknown"]),
		intOrZero(summary["with_price"]),
		intOrZero(summary["with_sale_price"]),
		intOrZero(summary["with_product_url"]),
	)
}

func inStockLabel(v *bool) string {
	if v == nil {
		return "unknown"
	}
	if *v {
		return "yes"
	}
	return "no"
}

func shorten(v string, max int) string {
	v = strings.TrimSpace(v)
	if max <= 0 || len(v) <= max {
		return v
	}
	if max <= 3 {
		return v[:max]
	}
	return v[:max-3] + "..."
}

func looksLikeURL(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	u, err := neturl.Parse(v)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
