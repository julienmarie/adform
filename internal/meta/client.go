package meta

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIVersion    = "v26.0"
	productionBaseURL    = "https://graph.facebook.com/" + defaultAPIVersion
	maxPaginationPages   = 100
	maxResponseBodyBytes = 8 << 20
)

type APIError struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == 0 {
		return e.Message
	}
	return fmt.Sprintf("meta api error (%d): %s", e.Code, e.Message)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Code == 100 || apiErr.Code == 803 {
		return true
	}
	msg := strings.ToLower(apiErr.Message)
	return strings.Contains(msg, "unsupported get request") || strings.Contains(msg, "does not exist")
}

func IsRateLimited(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case 4, 17, 32, 613, 80004:
		return true
	}
	msg := strings.ToLower(apiErr.Message)
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "user request limit reached")
}

type Client struct {
	baseURL            string
	token              string
	httpClientOverride *http.Client
	logFunc            func(format string, args ...any)
	urlPolicy          func(*url.URL) error
}

func FromEnv() (*Client, error) {
	token := strings.TrimSpace(os.Getenv("META_ACCESS_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("META_ACCESS_TOKEN is required")
	}
	return FromToken(token), nil
}

func FromToken(token string) *Client {
	return &Client{
		baseURL:            productionBaseURL,
		token:              token,
		httpClientOverride: &http.Client{Timeout: 45 * time.Second},
		logFunc: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		},
	}
}

func (c *Client) GetNode(id string, fields ...string) (map[string]any, error) {
	q := url.Values{}
	if len(fields) > 0 {
		q.Set("fields", strings.Join(fields, ","))
	}
	var out map[string]any
	if err := c.doJSON(http.MethodGet, "/"+id, q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateObject(edge string, params url.Values) (string, error) {
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(http.MethodPost, normalizePath(edge), nil, params, &resp); err != nil {
		return "", err
	}
	if resp.ID == "" {
		return "", fmt.Errorf("meta create returned empty id")
	}
	return resp.ID, nil
}

func (c *Client) ListEdge(edge string, fields []string, params url.Values) ([]map[string]any, error) {
	if params == nil {
		params = url.Values{}
	}
	if len(fields) > 0 {
		params.Set("fields", strings.Join(fields, ","))
	}
	nextURL := c.baseURL + normalizePath(edge) + "?" + params.Encode()
	out := make([]map[string]any, 0)
	for page := 0; nextURL != ""; page++ {
		if page >= maxPaginationPages {
			return nil, fmt.Errorf("meta pagination limit exceeded (%d pages)", maxPaginationPages)
		}
		payload, err := c.doWithRetry(http.MethodGet, func() (*http.Request, error) {
			return http.NewRequest(http.MethodGet, nextURL, nil)
		})
		if err != nil {
			return nil, err
		}

		var page struct {
			Data   []map[string]any `json:"data"`
			Paging struct {
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := json.Unmarshal(payload, &page); err != nil {
			return nil, fmt.Errorf("parse list response: %w", err)
		}
		out = append(out, page.Data...)
		nextURL = page.Paging.Next
	}
	return out, nil
}

func (c *Client) UpdateObject(id string, params url.Values) error {
	if params == nil {
		params = url.Values{}
	}
	var resp struct {
		Success bool `json:"success"`
	}
	if err := c.doJSON(http.MethodPost, "/"+id, nil, params, &resp); err != nil {
		return err
	}
	if !resp.Success {
		// Some endpoints return id instead of success on update; accept if no error.
		return nil
	}
	return nil
}

func (c *Client) PauseObject(id string) error {
	return c.UpdateObject(id, url.Values{"status": {"PAUSED"}})
}

func (c *Client) UploadImage(adAccountID, filePath string) (string, error) {
	if !strings.HasPrefix(adAccountID, "act_") {
		adAccountID = "act_" + adAccountID
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("filename", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := c.baseURL + "/" + adAccountID + "/adimages"
	contentType := writer.FormDataContentType()
	payload, err := c.doWithRetry(http.MethodPost, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Images map[string]struct {
			Hash string `json:"hash"`
		} `json:"images"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", fmt.Errorf("parse image upload response: %w", err)
	}
	for _, img := range out.Images {
		if img.Hash != "" {
			return img.Hash, nil
		}
	}
	return "", fmt.Errorf("image upload returned no hash")
}

func (c *Client) UploadVideo(adAccountID, filePath string) (string, error) {
	if !strings.HasPrefix(adAccountID, "act_") {
		adAccountID = "act_" + adAccountID
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("source", filepath.Base(filePath))
	if err != nil {
		return "", err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := c.baseURL + "/" + adAccountID + "/advideos"
	contentType := writer.FormDataContentType()
	payload, err := c.doWithRetry(http.MethodPost, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", fmt.Errorf("parse video upload response: %w", err)
	}
	if out.ID == "" {
		return "", fmt.Errorf("video upload returned no id")
	}
	return out.ID, nil
}

func (c *Client) doJSON(method, path string, query url.Values, form url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	if hasCredentialField(query) || hasCredentialField(form) {
		return errors.New("meta request credentials are forbidden outside Authorization header")
	}
	endpoint := c.baseURL + normalizePath(path)
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	var bodyStr string
	headers := map[string]string{}
	if form != nil {
		bodyStr = form.Encode()
		headers["Content-Type"] = "application/x-www-form-urlencoded"
	}

	payload, err := c.doWithRetry(method, func() (*http.Request, error) {
		var body io.Reader
		if bodyStr != "" {
			body = strings.NewReader(bodyStr)
		}
		req, err := http.NewRequest(method, endpoint, body)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	})
	if err != nil {
		return err
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

func hasCredentialField(values url.Values) bool {
	for key := range values {
		if strings.EqualFold(key, "access_token") {
			return true
		}
	}
	return false
}

func (c *Client) decodeAPIError(payload []byte) error {
	var wrapped struct {
		Error APIError `json:"error"`
	}
	if err := json.Unmarshal(payload, &wrapped); err == nil && wrapped.Error.Message != "" {
		wrapped.Error.Message = c.redactToken(wrapped.Error.Message)
		return &wrapped.Error
	}
	return &APIError{Message: "meta api returned an invalid error response"}
}

func (c *Client) httpClient() *http.Client {
	base := c.httpClientOverride
	if base == nil {
		base = &http.Client{Timeout: 45 * time.Second}
	}
	client := *base
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func (c *Client) doWithRetry(method string, buildReq func() (*http.Request, error)) ([]byte, error) {
	const maxAttempts = 6
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := buildReq()
		if err != nil {
			return nil, c.sanitizeError(err)
		}
		if err := c.prepareRequest(req); err != nil {
			return nil, err
		}
		resp, err := c.httpClient().Do(req)
		if err != nil {
			if shouldRetryTransport(method, err) && attempt < maxAttempts-1 {
				delay := retryDelay(attempt, 0, "", 0)
				c.logRetryTransport(method, requestTarget(req), err, delay, attempt+1, maxAttempts)
				time.Sleep(delay)
				continue
			}
			return nil, c.sanitizeError(err)
		}
		payload, readErr := readBounded(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			if errors.Is(readErr, errResponseBodyTooLarge) {
				return nil, readErr
			}
			if attempt < maxAttempts-1 {
				delay := retryDelay(attempt, resp.StatusCode, resp.Header.Get("Retry-After"), suggestedDelayFromHeaders(resp.Header))
				c.logRetryRead(method, requestTarget(req), resp.StatusCode, readErr, resp.Header, delay, attempt+1, maxAttempts)
				time.Sleep(delay)
				continue
			}
			return nil, c.sanitizeError(readErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if d := proactiveDelayFromHeaders(resp.Header); d > 0 {
				c.logProactiveThrottle(method, requestTarget(req), resp.Header, d)
				time.Sleep(d)
			}
			return payload, nil
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			return nil, fmt.Errorf("meta redirect response rejected (status %d)", resp.StatusCode)
		}
		apiErr := c.decodeAPIError(payload)
		if shouldRetryResponse(method, resp.StatusCode, apiErr) && attempt < maxAttempts-1 {
			delay := retryDelay(attempt, resp.StatusCode, resp.Header.Get("Retry-After"), suggestedDelayFromHeaders(resp.Header))
			c.logRetryResponse(method, requestTarget(req), resp.StatusCode, apiErr, resp.Header, delay, attempt+1, maxAttempts)
			time.Sleep(delay)
			continue
		}
		return nil, apiErr
	}
	return nil, fmt.Errorf("request failed after retries")
}

func (c *Client) logf(format string, args ...any) {
	if c == nil || c.logFunc == nil {
		return
	}
	c.logFunc(format, args...)
}

func (c *Client) logRetryTransport(method, target string, err error, delay time.Duration, attempt, maxAttempts int) {
	c.logf(
		"[meta] transport retry: %s %s failed (%s); waiting %s before retry %d/%d",
		method,
		target,
		c.redactToken(err.Error()),
		formatWait(delay),
		attempt,
		maxAttempts-1,
	)
}

func (c *Client) logRetryRead(method, target string, status int, err error, headers http.Header, delay time.Duration, attempt, maxAttempts int) {
	c.logf(
		"[meta] response read retry: %s %s status=%d (%v); waiting %s before retry %d/%d (%s)",
		method,
		target,
		status,
		c.redactToken(err.Error()),
		formatWait(delay),
		attempt,
		maxAttempts-1,
		summarizeUsage(headers),
	)
}

func (c *Client) logRetryResponse(method, target string, status int, err error, headers http.Header, delay time.Duration, attempt, maxAttempts int) {
	c.logf(
		"[meta] throttled/retry: %s %s status=%d error=%s; waiting %s before retry %d/%d (%s)",
		method,
		target,
		status,
		c.redactToken(compactError(err)),
		formatWait(delay),
		attempt,
		maxAttempts-1,
		summarizeUsage(headers),
	)
}

func (c *Client) redactToken(message string) string {
	if c == nil || c.token == "" {
		return message
	}
	message = strings.ReplaceAll(message, c.token, "[REDACTED]")
	if encoded := url.QueryEscape(c.token); encoded != c.token {
		message = strings.ReplaceAll(message, encoded, "[REDACTED]")
	}
	return message
}

var errResponseBodyTooLarge = errors.New("meta response body exceeds 8 MiB limit")

func readBounded(body io.Reader) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(body, maxResponseBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxResponseBodyBytes {
		return nil, errResponseBodyTooLarge
	}
	return payload, nil
}

func (c *Client) prepareRequest(req *http.Request) error {
	if req == nil || req.URL == nil {
		return errors.New("meta request URL is required")
	}
	policy := validateProductionURL
	if c.urlPolicy != nil {
		policy = c.urlPolicy
	}
	if err := policy(req.URL); err != nil {
		return err
	}
	if req.URL.User != nil {
		return errors.New("meta request URL credentials are forbidden")
	}
	for key := range req.URL.Query() {
		if strings.EqualFold(key, "access_token") {
			return errors.New("meta request URL credentials are forbidden")
		}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return nil
}

func validateProductionURL(target *url.URL) error {
	if target.Scheme != "https" {
		return errors.New("meta request rejected: HTTPS is required")
	}
	if target.Host != "graph.facebook.com" {
		return errors.New("meta request rejected: unexpected host")
	}
	if target.Path != "/"+defaultAPIVersion && !strings.HasPrefix(target.Path, "/"+defaultAPIVersion+"/") {
		return errors.New("meta request rejected: unexpected API version")
	}
	return nil
}

func (c *Client) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(c.redactToken(err.Error()))
}

func (c *Client) logProactiveThrottle(method, target string, headers http.Header, delay time.Duration) {
	c.logf(
		"[meta] proactive throttle: %s %s usage high; waiting %s (%s)",
		method,
		target,
		formatWait(delay),
		summarizeUsage(headers),
	)
}

func shouldRetryTransport(method string, err error) bool {
	var nerr net.Error
	if errors.As(err, &nerr) && (nerr.Timeout() || nerr.Temporary()) {
		return true
	}
	// Be conservative for network flakes on GET.
	return method == http.MethodGet
}

func shouldRetryResponse(method string, statusCode int, err error) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	if IsRateLimited(err) {
		return true
	}
	if statusCode >= 500 && method == http.MethodGet {
		return true
	}
	return false
}

func retryDelay(attempt int, statusCode int, retryAfter string, suggested time.Duration) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	base := 1500 * time.Millisecond
	d := base * time.Duration(1<<attempt)
	max := 45 * time.Second
	if statusCode == http.StatusTooManyRequests && d < 5*time.Second {
		d = 5 * time.Second
	}
	if suggested > d {
		d = suggested
	}
	if d > max {
		return max
	}
	return d
}

func proactiveDelayFromHeaders(h http.Header) time.Duration {
	callCount, _ := extractPercentMetric(h.Get("X-App-Usage"), "call_count")
	totalTime, _ := extractPercentMetric(h.Get("X-App-Usage"), "total_time")
	totalCPU, _ := extractPercentMetric(h.Get("X-App-Usage"), "total_cputime")
	accUtil, _ := extractPercentMetric(h.Get("X-Ad-Account-Usage"), "acc_id_util_pct")

	maxPct := maxFloat(callCount, totalTime, totalCPU, accUtil)
	switch {
	case maxPct >= 98:
		return 5 * time.Second
	case maxPct >= 95:
		return 3 * time.Second
	case maxPct >= 90:
		return 1200 * time.Millisecond
	default:
		return 0
	}
}

func suggestedDelayFromHeaders(h http.Header) time.Duration {
	var d time.Duration
	if secs, ok := extractNumber(h.Get("X-Ad-Account-Usage"), "reset_time_duration"); ok && secs > 0 {
		d = maxDuration(d, time.Duration(secs)*time.Second)
	}
	if secs, ok := extractNumber(h.Get("X-Business-Use-Case-Usage"), "estimated_time_to_regain_access"); ok && secs > 0 {
		d = maxDuration(d, time.Duration(secs)*time.Second)
	}
	return d
}

func extractPercentMetric(headerVal string, key string) (float64, bool) {
	n, ok := extractNumber(headerVal, key)
	if !ok {
		return 0, false
	}
	if n < 0 {
		n = 0
	}
	if n > 100 {
		n = 100
	}
	return n, true
}

func extractNumber(headerVal string, key string) (float64, bool) {
	headerVal = strings.TrimSpace(headerVal)
	if headerVal == "" {
		return 0, false
	}
	normalized := strings.ReplaceAll(headerVal, "'", "\"")
	var v any
	if err := json.Unmarshal([]byte(normalized), &v); err != nil {
		return 0, false
	}
	return findNumber(v, key)
}

func findNumber(v any, key string) (float64, bool) {
	switch t := v.(type) {
	case map[string]any:
		if raw, ok := t[key]; ok {
			if n, ok := toFloat(raw); ok {
				return n, true
			}
		}
		for _, child := range t {
			if n, ok := findNumber(child, key); ok {
				return n, true
			}
		}
	case []any:
		for _, child := range t {
			if n, ok := findNumber(child, key); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func toFloat(v any) (float64, bool) {
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

func maxFloat(vals ...float64) float64 {
	max := 0.0
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	return max
}

func maxDuration(a, b time.Duration) time.Duration {
	if b > a {
		return b
	}
	return a
}

func requestTarget(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "<unknown>"
	}
	if req.URL.Path == "" {
		return "/"
	}
	return req.URL.Path
}

func compactError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > 180 {
		return msg[:177] + "..."
	}
	return msg
}

func formatWait(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(100 * time.Millisecond).String()
}

func summarizeUsage(h http.Header) string {
	parts := make([]string, 0, 6)
	if v, ok := extractPercentMetric(h.Get("X-App-Usage"), "call_count"); ok {
		parts = append(parts, fmt.Sprintf("app_call=%.0f%%", v))
	}
	if v, ok := extractPercentMetric(h.Get("X-App-Usage"), "total_time"); ok {
		parts = append(parts, fmt.Sprintf("app_time=%.0f%%", v))
	}
	if v, ok := extractPercentMetric(h.Get("X-App-Usage"), "total_cputime"); ok {
		parts = append(parts, fmt.Sprintf("app_cpu=%.0f%%", v))
	}
	if v, ok := extractPercentMetric(h.Get("X-Ad-Account-Usage"), "acc_id_util_pct"); ok {
		parts = append(parts, fmt.Sprintf("acct=%.1f%%", v))
	}
	if secs, ok := extractNumber(h.Get("X-Ad-Account-Usage"), "reset_time_duration"); ok && secs > 0 {
		parts = append(parts, fmt.Sprintf("reset=%ds", int(secs+0.5)))
	}
	if secs, ok := extractNumber(h.Get("X-Business-Use-Case-Usage"), "estimated_time_to_regain_access"); ok && secs > 0 {
		parts = append(parts, fmt.Sprintf("regain=%ds", int(secs+0.5)))
	}
	if len(parts) == 0 {
		return "usage=unknown"
	}
	return strings.Join(parts, ", ")
}
