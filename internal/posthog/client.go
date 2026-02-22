package posthog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

type QueryResponse struct {
	Columns []string `json:"columns"`
	Results []any    `json:"results"`
	Types   []any    `json:"types,omitempty"`
}

func NewClient(baseURL, apiKey string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = "https://app.posthog.com"
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Query(projectID, sql string) (QueryResponse, error) {
	if strings.TrimSpace(projectID) == "" {
		return QueryResponse{}, fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return QueryResponse{}, fmt.Errorf("api key is required")
	}
	query := strings.TrimSpace(sql)
	if query == "" {
		return QueryResponse{}, fmt.Errorf("sql query is required")
	}

	endpoint := fmt.Sprintf("%s/api/projects/%s/query/", c.BaseURL, url.PathEscape(strings.TrimSpace(projectID)))
	payload := map[string]any{
		"query": map[string]any{
			"kind":  "HogQLQuery",
			"query": query,
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return QueryResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return QueryResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return QueryResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return QueryResponse{}, fmt.Errorf("posthog query failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var out QueryResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return QueryResponse{}, fmt.Errorf("parse posthog response: %w", err)
	}
	return out, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}
