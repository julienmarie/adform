package meta

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListEdgeDoesNotLeakAccessTokenInLogs(t *testing.T) {
	const token = "super-secret-access-token"
	var logs strings.Builder
	client := &Client{
		Token: token,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(&logs, format, args...)
		},
	}
	req := httptest.NewRequest(http.MethodGet, "https://graph.example/v22.0/act_123/insights?access_token="+token, nil)
	client.logRetryResponse(http.MethodGet, requestTarget(req), http.StatusTooManyRequests, &APIError{Message: "rate limited for " + token, Code: 4}, nil, 0, 1, 2)
	if strings.Contains(logs.String(), token) {
		t.Fatalf("retry logs leaked access token: %s", logs.String())
	}
}

func TestListEdgeRejectsPaginationBeyondLimit(t *testing.T) {
	requests := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		body := fmt.Sprintf(`{"data":[{"id":"%d"}],"paging":{"next":%q}}`, requests, r.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	client := &Client{
		BaseURL:    "https://graph.example/v22.0",
		Token:      "test-token",
		HTTPClient: &http.Client{Transport: transport},
	}
	rows, err := client.ListEdge("act_123/insights", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "pagination limit") {
		t.Fatalf("ListEdge() error = %v, want pagination limit error", err)
	}
	if requests != maxPaginationPages {
		t.Fatalf("requests = %d, want %d", requests, maxPaginationPages)
	}
	if rows != nil {
		t.Fatalf("rows = %#v, want nil on pagination limit", rows)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
