package meta

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testMaxResponseBodyBytes = 8 << 20

func TestFromTokenPinsProductionEndpointAndIgnoresEnvironment(t *testing.T) {
	t.Setenv("META_API_VERSION", "v99.0")
	t.Setenv("META_API_BASE_URL", "https://evil.example/v99.0")
	client := FromToken("test-token")
	if client.baseURL != "https://graph.facebook.com/v26.0" {
		t.Fatalf("baseURL = %q, want pinned production endpoint", client.baseURL)
	}
}

func TestGetNodeUsesBearerHeaderWithoutCredentialURL(t *testing.T) {
	const token = "secret token/+"
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		if strings.Contains(r.URL.RawQuery, "access_token") || strings.Contains(r.URL.String(), url.QueryEscape(token)) {
			t.Fatalf("credential leaked into URL: %s", r.URL)
		}
		return jsonResponse(http.StatusOK, `{"id":"123"}`), nil
	})
	client := testClient(token, transport)
	if _, err := client.GetNode("123", "id"); err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
}

func TestMultipartUsesBearerHeaderWithoutCredentialField(t *testing.T) {
	const token = "multipart-secret"
	file := filepath.Join(t.TempDir(), "asset.bin")
	if err := os.WriteFile(file, []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		call func(*Client) error
		body string
	}{
		{name: "image", call: func(c *Client) error { _, err := c.UploadImage("123", file); return err }, body: `{"images":{"asset":{"hash":"hash"}}}`},
		{name: "video", call: func(c *Client) error { _, err := c.UploadVideo("123", file); return err }, body: `{"id":"video"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer "+token {
					t.Fatalf("Authorization = %q", got)
				}
				if bytes.Contains(body, []byte(token)) || bytes.Contains(body, []byte("access_token")) {
					t.Fatalf("credential present in multipart body")
				}
				return jsonResponse(http.StatusOK, tc.body), nil
			})
			client := testClient(token, transport)
			if err := tc.call(client); err != nil {
				t.Fatalf("upload error = %v", err)
			}
		})
	}
}

func TestListEdgeRejectsUnsafePagingURLsBeforeRequest(t *testing.T) {
	for _, next := range []string{
		"https://evil.example/v26.0/next",
		"http://graph.facebook.com/v26.0/next",
		"https://graph.facebook.com/v25.0/next",
		"https://graph.facebook.com/v26.0/next?access_token=leaked",
	} {
		t.Run(next, func(t *testing.T) {
			requests := 0
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				return jsonResponse(http.StatusOK, fmt.Sprintf(`{"data":[],"paging":{"next":%q}}`, next)), nil
			})
			client := testClient("test-token", transport)
			if _, err := client.ListEdge("act_123/insights", nil, nil); err == nil {
				t.Fatal("ListEdge() succeeded for unsafe paging URL")
			}
			if requests != 1 {
				t.Fatalf("requests = %d, unsafe paging URL was requested", requests)
			}
		})
	}
}

func TestClientRejectsUnsafeInitialURLBeforeRequest(t *testing.T) {
	for _, baseURL := range []string{
		"https://evil.example/v26.0",
		"http://graph.facebook.com/v26.0",
		"https://graph.facebook.com/v25.0",
		"https://graph.facebook.com/v26.0?access_token=leaked",
	} {
		t.Run(baseURL, func(t *testing.T) {
			requests := 0
			transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				return jsonResponse(http.StatusOK, `{}`), nil
			})
			client := testClient("test-token", transport)
			client.baseURL = baseURL
			if _, err := client.GetNode("123"); err == nil {
				t.Fatal("GetNode() succeeded for unsafe initial URL")
			}
			if requests != 0 {
				t.Fatalf("requests = %d, want validation before transport", requests)
			}
		})
	}
}

func TestClientRejectsCredentialBearingForm(t *testing.T) {
	requests := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return jsonResponse(http.StatusOK, `{"success":true}`), nil
	})
	client := testClient("test-token", transport)
	err := client.UpdateObject("123", url.Values{"access_token": {"forbidden"}})
	if err == nil || !strings.Contains(err.Error(), "credentials are forbidden") {
		t.Fatalf("UpdateObject() error = %v, want credential-field rejection", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want rejection before transport", requests)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	requests := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			resp := jsonResponse(http.StatusFound, "redirect")
			resp.Header.Set("Location", "https://graph.facebook.com/v26.0/redirected")
			return resp, nil
		}
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	client := testClient("test-token", transport)
	if _, err := client.GetNode("123"); err == nil {
		t.Fatal("GetNode() succeeded on redirect")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, redirect was followed", requests)
	}
}

func TestClientBoundsResponseBodies(t *testing.T) {
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, strings.Repeat("x", testMaxResponseBodyBytes+1)), nil
	})
	client := testClient("test-token", transport)
	if _, err := client.GetNode("123"); err == nil || !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("GetNode() error = %v, want bounded-body error", err)
	}
}

func TestClientSanitizesReturnedTransportErrors(t *testing.T) {
	const token = "transport-secret"
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("request failed with " + token)
	})
	client := testClient(token, transport)
	err := client.UpdateObject("123", nil)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("UpdateObject() error = %v, want sanitized error", err)
	}
}

func TestListEdgeDoesNotLeakAccessTokenInLogs(t *testing.T) {
	const token = "super-secret-access-token"
	var logs strings.Builder
	client := &Client{
		token: token,
		logFunc: func(format string, args ...any) {
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

	client := injectedTestClient("https://meta.test/v26.0", "test-token", transport)
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

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testClient(token string, transport http.RoundTripper) *Client {
	client := FromToken(token)
	client.httpClientOverride = &http.Client{Transport: transport}
	client.logFunc = nil
	return client
}

// injectedTestClient is intentionally test-only; production constructors never
// accept an endpoint or transport from environment or caller input.
func injectedTestClient(baseURL, token string, transport http.RoundTripper) *Client {
	client := testClient(token, transport)
	client.baseURL = baseURL
	base, err := url.Parse(baseURL)
	if err != nil {
		panic(err)
	}
	client.urlPolicy = func(target *url.URL) error {
		if target.Scheme != base.Scheme || target.Host != base.Host {
			return errors.New("test endpoint origin mismatch")
		}
		if target.Path != "/"+defaultAPIVersion && !strings.HasPrefix(target.Path, "/"+defaultAPIVersion+"/") {
			return errors.New("test endpoint API version mismatch")
		}
		return nil
	}
	return client
}
