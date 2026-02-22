package landing

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const landingFeedFetchTimeout = 2 * time.Minute

func loadFeedIndex(feedURL string) (map[string]FeedProduct, error) {
	data, contentType, err := fetchFeed(feedURL)
	if err != nil {
		return nil, err
	}
	format := detectFeedFormat("auto", data, feedURL, contentType)
	products := []FeedProduct{}
	switch format {
	case "xml":
		products, err = parseXMLFeed(data)
	case "csv":
		products, err = parseCSVFeed(data)
	default:
		return nil, fmt.Errorf("unsupported feed format")
	}
	if err != nil {
		return nil, err
	}
	out := map[string]FeedProduct{}
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		out[strings.TrimSpace(p.ID)] = p
	}
	return out, nil
}

func queryFeedProducts(index map[string]FeedProduct, q ProductGridBlock) []FeedProduct {
	mode := strings.ToLower(strings.TrimSpace(q.Query.Mode))
	if mode == "" {
		mode = "explicit"
	}
	out := make([]FeedProduct, 0)
	switch mode {
	case "explicit":
		for _, id := range q.Query.ProductIDs {
			if p, ok := index[strconv.FormatInt(id, 10)]; ok {
				if !q.Stock.ShowOOS && p.InStock != nil && !*p.InStock {
					continue
				}
				out = append(out, p)
			}
		}
	case "feed_filter":
		brand := strings.ToLower(strings.TrimSpace(q.Query.Brand))
		for _, p := range index {
			if !q.Stock.ShowOOS && p.InStock != nil && !*p.InStock {
				continue
			}
			if brand != "" {
				cand := strings.ToLower(strings.TrimSpace(p.Brand + " " + p.Title))
				if !strings.Contains(cand, brand) {
					continue
				}
			}
			if len(q.Query.Tags) > 0 {
				tagHay := strings.ToLower(p.Tags)
				matched := true
				for _, tag := range q.Query.Tags {
					t := strings.ToLower(strings.TrimSpace(tag))
					if t == "" {
						continue
					}
					if !strings.Contains(tagHay, t) {
						matched = false
						break
					}
				}
				if !matched {
					continue
				}
			}
			out = append(out, p)
		}
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Title < out[j].Title
		})
	}
	return out
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
		return nil, "", fmt.Errorf("unsupported feed scheme %q", u.Scheme)
	}
	client := &http.Client{Timeout: landingFeedFetchTimeout}
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "adform-landing/1.0")
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
	return "csv"
}

func parseXMLFeed(data []byte) ([]FeedProduct, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	products := make([]FeedProduct, 0, 128)

	inItem := false
	cur := FeedProduct{}
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
				cur = FeedProduct{}
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

func assignXMLField(p *FeedProduct, stack []string, text string) {
	if len(stack) == 0 || text == "" {
		return
	}
	leaf := stack[len(stack)-1]
	switch leaf {
	case "id", "item_id", "retailer_id", "sku", "offer_id":
		if p.ID == "" || looksLikeURL(p.ID) {
			p.ID = text
		}
	case "title", "name":
		if p.Title == "" {
			p.Title = text
		}
	case "description", "summary", "short_description":
		if p.Description == "" && !containsPathToken(stack, "shipping") {
			p.Description = text
		}
	case "link", "url":
		if p.URL == "" && looksLikeURL(text) {
			p.URL = text
		}
	case "price":
		if !containsPathToken(stack, "shipping") {
			if p.Price == "" {
				p.Price = text
			}
		}
	case "sale_price", "saleprice":
		if p.SalePrice == "" {
			p.SalePrice = text
		}
	case "availability", "stock_status", "stockstatus", "in_stock", "instock":
		if p.Availability == "" {
			p.Availability = text
		}
	case "image_link", "image", "image_url":
		if p.ImageURL == "" && looksLikeURL(text) {
			p.ImageURL = text
		}
	case "brand":
		if p.Brand == "" {
			p.Brand = text
		}
	case "product_type", "tags":
		if p.Tags == "" {
			p.Tags = text
		}
	}
}

func parseCSVFeed(data []byte) ([]FeedProduct, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = detectCSVDelimiter(data)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []FeedProduct{}, nil
	}
	header := map[string]int{}
	for idx, cell := range rows[0] {
		k := normalizeCSVHeader(cell)
		if k != "" {
			header[k] = idx
		}
	}
	get := func(row []string, keys ...string) string {
		for _, k := range keys {
			if idx, ok := header[k]; ok && idx < len(row) {
				v := strings.TrimSpace(row[idx])
				if v != "" {
					return v
				}
			}
		}
		return ""
	}
	products := make([]FeedProduct, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		p := FeedProduct{
			ID:           get(row, "id", "item_id", "retailer_id", "sku", "offer_id"),
			Title:        get(row, "title", "name", "product_name"),
			Description:  get(row, "description", "product_description", "short_description", "summary", "body"),
			URL:          get(row, "link", "url", "product_url", "landing_page"),
			Price:        get(row, "price", "current_price", "regular_price"),
			SalePrice:    get(row, "sale_price", "discount_price", "promo_price"),
			Availability: get(row, "availability", "stock_status", "in_stock", "instock", "stock"),
			ImageURL:     get(row, "image_link", "image", "image_url"),
			Brand:        get(row, "brand"),
			Tags:         get(row, "tags", "product_type"),
		}
		finalizeProduct(&p)
		if hasFeedProductData(p) {
			products = append(products, p)
		}
	}
	return products, nil
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
	repl := strings.NewReplacer(" ", "_", "-", "_", "/", "_", ".", "_", ":", "_")
	raw = repl.Replace(raw)
	return strings.Trim(raw, "_")
}

func finalizeProduct(p *FeedProduct) {
	p.ID = strings.TrimSpace(p.ID)
	p.Title = strings.TrimSpace(p.Title)
	p.Description = strings.TrimSpace(p.Description)
	p.URL = strings.TrimSpace(p.URL)
	p.Price = strings.TrimSpace(p.Price)
	p.SalePrice = strings.TrimSpace(p.SalePrice)
	p.Availability = strings.TrimSpace(p.Availability)
	p.ImageURL = strings.TrimSpace(p.ImageURL)
	p.Brand = strings.TrimSpace(p.Brand)
	p.Tags = strings.TrimSpace(p.Tags)
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

func hasFeedProductData(p FeedProduct) bool {
	return p.ID != "" || p.Title != "" || p.URL != "" || p.Price != "" || p.SalePrice != "" || p.Availability != ""
}

func looksLikeURL(v string) bool {
	u, err := neturl.Parse(strings.TrimSpace(v))
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
