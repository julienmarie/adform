package gsc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultBaseURL = "https://www.googleapis.com/webmasters/v3"
	readonlyScope  = "https://www.googleapis.com/auth/webmasters.readonly"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

type QueryRequest struct {
	StartDate             string                 `json:"startDate"`
	EndDate               string                 `json:"endDate"`
	Dimensions            []string               `json:"dimensions,omitempty"`
	Type                  string                 `json:"type,omitempty"`
	DimensionFilterGroups []DimensionFilterGroup `json:"dimensionFilterGroups,omitempty"`
	AggregationType       string                 `json:"aggregationType,omitempty"`
	DataState             string                 `json:"dataState,omitempty"`
	RowLimit              int                    `json:"rowLimit,omitempty"`
	StartRow              int                    `json:"startRow,omitempty"`
}

type DimensionFilterGroup struct {
	GroupType string            `json:"groupType,omitempty"`
	Filters   []DimensionFilter `json:"filters,omitempty"`
}

type DimensionFilter struct {
	Dimension  string `json:"dimension"`
	Operator   string `json:"operator,omitempty"`
	Expression string `json:"expression"`
}

type QueryResponse struct {
	Rows                    []QueryRow `json:"rows,omitempty"`
	ResponseAggregationType string     `json:"responseAggregationType,omitempty"`
}

type QueryRow struct {
	Keys        []string `json:"keys,omitempty"`
	Clicks      float64  `json:"clicks,omitempty"`
	Impressions float64  `json:"impressions,omitempty"`
	CTR         float64  `json:"ctr,omitempty"`
	Position    float64  `json:"position,omitempty"`
}

func NewClientFromCredentialsJSON(ctx context.Context, credentialsJSON []byte) (*Client, error) {
	if strings.TrimSpace(string(credentialsJSON)) == "" {
		return nil, fmt.Errorf("credentials JSON is empty")
	}
	creds, err := google.CredentialsFromJSON(ctx, credentialsJSON, readonlyScope)
	if err != nil {
		return nil, fmt.Errorf("google credentials from json: %w", err)
	}
	httpClient := oauth2.NewClient(ctx, creds.TokenSource)
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: httpClient,
	}, nil
}

func (c *Client) Query(ctx context.Context, siteURL string, req QueryRequest) (QueryResponse, error) {
	if strings.TrimSpace(siteURL) == "" {
		return QueryResponse{}, fmt.Errorf("site URL is required")
	}
	endpoint := strings.TrimRight(c.baseURL(), "/") + "/sites/" + url.PathEscape(strings.TrimSpace(siteURL)) + "/searchAnalytics/query"

	body, err := json.Marshal(req)
	if err != nil {
		return QueryResponse{}, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return QueryResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return QueryResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return QueryResponse{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return QueryResponse{}, fmt.Errorf("gsc query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out QueryResponse
	if len(respBody) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return QueryResponse{}, fmt.Errorf("parse response: %w", err)
	}
	return out, nil
}

func (c *Client) baseURL() string {
	if strings.TrimSpace(c.BaseURL) != "" {
		return strings.TrimSpace(c.BaseURL)
	}
	return defaultBaseURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}
