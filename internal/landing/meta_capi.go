package landing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type MetaCAPIClient struct {
	pixelID     string
	accessToken string
	baseURL     string
	httpClient  *http.Client
}

type MetaCAPIEvent struct {
	EventName  string
	EventID    string
	EventTime  time.Time
	EventURL   string
	UserAgent  string
	ClientIP   string
	FBC        string
	FBP        string
	ExternalID string
	CustomData map[string]any
}

func NewMetaCAPIClient(pixelID, accessToken string) *MetaCAPIClient {
	return &MetaCAPIClient{
		pixelID:     strings.TrimSpace(pixelID),
		accessToken: strings.TrimSpace(accessToken),
		baseURL:     "https://graph.facebook.com/v22.0",
		httpClient:  &http.Client{Timeout: 6 * time.Second},
	}
}

func (c *MetaCAPIClient) Capture(ctx context.Context, evt MetaCAPIEvent) error {
	if strings.TrimSpace(c.pixelID) == "" || strings.TrimSpace(c.accessToken) == "" {
		return nil
	}
	if strings.TrimSpace(evt.EventName) == "" {
		return nil
	}
	if evt.EventTime.IsZero() {
		evt.EventTime = time.Now().UTC()
	}
	if strings.TrimSpace(evt.EventID) == "" {
		evt.EventID = randomID()
	}

	userData := map[string]any{}
	if v := strings.TrimSpace(evt.UserAgent); v != "" {
		userData["client_user_agent"] = v
	}
	if v := strings.TrimSpace(evt.ClientIP); v != "" {
		userData["client_ip_address"] = v
	}
	if v := strings.TrimSpace(evt.FBC); v != "" {
		userData["fbc"] = v
	}
	if v := strings.TrimSpace(evt.FBP); v != "" {
		userData["fbp"] = v
	}
	if v := strings.TrimSpace(evt.ExternalID); v != "" {
		hash := sha256.Sum256([]byte(v))
		userData["external_id"] = hex.EncodeToString(hash[:])
	}

	record := map[string]any{
		"event_name":    evt.EventName,
		"event_time":    evt.EventTime.Unix(),
		"event_id":      evt.EventID,
		"action_source": "website",
	}
	if strings.TrimSpace(evt.EventURL) != "" {
		record["event_source_url"] = strings.TrimSpace(evt.EventURL)
	}
	if len(userData) > 0 {
		record["user_data"] = userData
	}
	if len(evt.CustomData) > 0 {
		record["custom_data"] = evt.CustomData
	}
	payload := map[string]any{
		"data": []any{record},
	}

	b, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("%s/%s/events?access_token=%s", strings.TrimRight(c.baseURL, "/"), url.PathEscape(c.pixelID), url.QueryEscape(c.accessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
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
		return fmt.Errorf("capi status=%d", resp.StatusCode)
	}
	return nil
}

func requestClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && strings.TrimSpace(host) != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func absoluteURLForRequest(publicBase string, r *http.Request) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if base == "" {
		return r.URL.String()
	}
	if r == nil || r.URL == nil {
		return base
	}
	if strings.HasPrefix(r.URL.Path, "/") {
		return base + r.URL.RequestURI()
	}
	return base + "/" + r.URL.RequestURI()
}

func deriveFBC(r *http.Request, attr AttributionData) string {
	if attr.Params != nil {
		if raw := strings.TrimSpace(attr.Params["fbclid"]); raw != "" {
			return fmt.Sprintf("fb.1.%d.%s", time.Now().Unix(), raw)
		}
	}
	if r != nil && r.URL != nil {
		if raw := strings.TrimSpace(r.URL.Query().Get("fbclid")); raw != "" {
			return fmt.Sprintf("fb.1.%d.%s", time.Now().Unix(), raw)
		}
	}
	return ""
}

func deriveFBP(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie("_fbp")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}
