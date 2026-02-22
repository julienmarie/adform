package landing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PostHogClient struct {
	host       string
	apiKey     string
	httpClient *http.Client
}

func NewPostHogClient(host, apiKey string) *PostHogClient {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		host = "https://app.posthog.com"
	}
	return &PostHogClient{
		host:       host,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 6 * time.Second},
	}
}

func (c *PostHogClient) Enabled() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

func (c *PostHogClient) Capture(ctx context.Context, event string, distinctID string, props map[string]any) error {
	if !c.Enabled() || strings.TrimSpace(event) == "" {
		return nil
	}
	if props == nil {
		props = map[string]any{}
	}
	if strings.TrimSpace(distinctID) == "" {
		distinctID = "anonymous"
	}
	if _, ok := props["distinct_id"]; !ok {
		props["distinct_id"] = distinctID
	}
	payload := map[string]any{
		"api_key":     c.apiKey,
		"event":       event,
		"distinct_id": distinctID,
		"properties":  props,
	}
	b, _ := json.Marshal(payload)
	url := c.host + "/capture/"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("posthog capture status=%d", resp.StatusCode)
	}
	return nil
}
