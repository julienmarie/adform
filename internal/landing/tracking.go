package landing

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type variantAssignments map[string]map[string]string

func readAttributionCookie(r *http.Request, cookieName string) AttributionData {
	cookie, err := r.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return AttributionData{Params: map[string]string{}}
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return AttributionData{Params: map[string]string{}}
	}
	var out AttributionData
	if err := json.Unmarshal(raw, &out); err != nil {
		return AttributionData{Params: map[string]string{}}
	}
	if out.Params == nil {
		out.Params = map[string]string{}
	}
	return out
}

func writeAttributionCookie(w http.ResponseWriter, cfg SiteConfig, data AttributionData) {
	if data.Params == nil {
		data.Params = map[string]string{}
	}
	b, _ := json.Marshal(data)
	encoded := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Tracking.AttributionCookie,
		Value:    encoded,
		Path:     "/",
		Domain:   cfg.Tracking.CookieDomain,
		Expires:  time.Now().AddDate(0, 0, cfg.Tracking.AttributionTTLDay),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func captureAttributionQuery(data AttributionData, q url.Values, capture []string, pageKey, slug string) (AttributionData, bool) {
	changed := false
	if data.Params == nil {
		data.Params = map[string]string{}
		changed = true
	}
	if strings.TrimSpace(data.AnonID) == "" {
		data.AnonID = randomID()
		changed = true
	}
	if strings.TrimSpace(data.FirstSeen) == "" {
		data.FirstSeen = time.Now().UTC().Format(time.RFC3339)
		changed = true
	}
	if strings.TrimSpace(data.PageKey) == "" && pageKey != "" {
		data.PageKey = pageKey
		changed = true
	}
	if strings.TrimSpace(data.Slug) == "" && slug != "" {
		data.Slug = slug
		changed = true
	}
	for _, key := range capture {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		v := strings.TrimSpace(q.Get(k))
		if v == "" {
			continue
		}
		if _, exists := data.Params[k]; !exists {
			data.Params[k] = v
			changed = true
		}
	}
	return data, changed
}

func readVariantCookie(r *http.Request, cookieName string) variantAssignments {
	cookie, err := r.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return variantAssignments{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return variantAssignments{}
	}
	var out variantAssignments
	if err := json.Unmarshal(raw, &out); err != nil {
		return variantAssignments{}
	}
	if out == nil {
		out = variantAssignments{}
	}
	return out
}

func writeVariantCookie(w http.ResponseWriter, cfg SiteConfig, data variantAssignments) {
	if data == nil {
		data = variantAssignments{}
	}
	b, _ := json.Marshal(data)
	encoded := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Tracking.VariantCookie,
		Value:    encoded,
		Path:     "/",
		Domain:   cfg.Tracking.CookieDomain,
		Expires:  time.Now().AddDate(0, 0, cfg.Tracking.VariantTTLDays),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func withUTMPassthrough(dest string, attr AttributionData, cfg SiteConfig) string {
	if !cfg.Tracking.UTMPassthrough.Enabled {
		return dest
	}
	u, err := url.Parse(dest)
	if err != nil {
		return dest
	}
	q := u.Query()
	for _, key := range cfg.Tracking.UTMPassthrough.Allowlist {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if q.Get(k) != "" {
			continue
		}
		if v := strings.TrimSpace(attr.Params[k]); v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func attributionProperties(attr AttributionData) map[string]any {
	out := map[string]any{}
	keys := make([]string, 0, len(attr.Params))
	for k := range attr.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = attr.Params[k]
	}
	if attr.PageKey != "" {
		out["first_page_key"] = attr.PageKey
	}
	if attr.Slug != "" {
		out["first_slug"] = attr.Slug
	}
	if attr.FirstSeen != "" {
		out["first_seen"] = attr.FirstSeen
	}
	return out
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
