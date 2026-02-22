package landing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"adform/internal/config"
	appstate "adform/internal/state"
	"adform/internal/workspace"
)

type Server struct {
	opts    ServeOptions
	store   BanditCounterStore
	posthog *PostHogClient
	capi    *MetaCAPIClient

	mu            sync.RWMutex
	loaded        *LoadedSite
	banditRecByID map[string]string
}

const feedRefreshInterval = 30 * time.Minute

func NewServer(ctx context.Context, opts ServeOptions) (*Server, error) {
	loaded, err := Load(opts.Root, opts.Account, opts)
	if err != nil {
		return nil, err
	}
	store, err := openBanditCounterStore(opts, loaded.Site)
	if err != nil {
		return nil, err
	}
	posthog := (*PostHogClient)(nil)
	if loaded.Site.PostHog.Enabled {
		apiKey := strings.TrimSpace(os.Getenv(strings.TrimSpace(loaded.Site.PostHog.APIKeyEnv)))
		if apiKey == "" {
			return nil, fmt.Errorf("missing PostHog API key env %s", loaded.Site.PostHog.APIKeyEnv)
		}
		posthog = NewPostHogClient(loaded.Site.PostHog.Host, apiKey)
	}
	capi := (*MetaCAPIClient)(nil)
	if loaded.Site.MetaPixel.Enabled && strings.TrimSpace(loaded.Site.MetaPixel.CAPIAccessTokenEnv) != "" && strings.TrimSpace(loaded.Site.MetaPixel.PixelID) != "" {
		token := strings.TrimSpace(os.Getenv(strings.TrimSpace(loaded.Site.MetaPixel.CAPIAccessTokenEnv)))
		if token != "" {
			capi = NewMetaCAPIClient(strings.TrimSpace(loaded.Site.MetaPixel.PixelID), token)
		}
	}

	s := &Server{
		opts:          opts,
		store:         store,
		posthog:       posthog,
		capi:          capi,
		loaded:        loaded,
		banditRecByID: map[string]string{},
	}
	s.restoreFeedIndexFromCache()
	go s.refreshFeedIndex(context.Background())
	if loaded.Site.Bandit.Enabled {
		go s.refreshBanditRecommendations(context.Background())
	}
	if len(loaded.ValidationWarn) > 0 {
		for _, w := range loaded.ValidationWarn {
			log.Printf("[landing][warn] %s", w)
		}
	}
	_ = ctx
	return s, nil
}

func (s *Server) Close() error {
	if s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *Server) current() *LoadedSite {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

func (s *Server) setLoaded(loaded *LoadedSite) {
	s.mu.Lock()
	s.loaded = loaded
	s.mu.Unlock()
}

func (s *Server) Run(ctx context.Context) error {
	if s.opts.HotReload {
		go s.reloadLoop(ctx)
	}
	go s.feedRefreshLoop(ctx)
	if s.current().Site.Bandit.Enabled && s.current().Site.Bandit.UpdateIntervalMinutes > 0 {
		go s.banditUpdateLoop(ctx)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/theme.css", s.handleTheme)
	mux.Handle("/assets/", s.assetHandler())
	mux.Handle("/meta-assets/", s.metaAssetHandler())
	mux.HandleFunc("/r", s.handleRedirect)
	mux.HandleFunc("/", s.handlePage)

	srv := &http.Server{Addr: s.current().Site.Runtime.Bind, Handler: s.wrap(mux)}
	log.Printf("[landing] serving on %s env=%s hot_reload=%t", srv.Addr, s.opts.Env, s.opts.HotReload)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/assets/favicon.svg", http.StatusTemporaryRedirect)
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	loaded := s.current()
	if strings.EqualFold(s.opts.Env, "prod") {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	http.ServeContent(w, r, "theme.css", loaded.LoadedAt, strings.NewReader(loaded.ThemeCSS))
}

func (s *Server) assetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaded := s.current()
		root := filepath.Join(loaded.LandingDir, "assets")
		fs := http.FileServer(http.Dir(root))
		if strings.EqualFold(s.opts.Env, "prod") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		http.StripPrefix("/assets/", fs).ServeHTTP(w, r)
	})
}

func (s *Server) metaAssetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(strings.TrimSpace(s.opts.Env), "dev") {
			http.NotFound(w, r)
			return
		}
		root := filepath.Join(workspace.ResolveMetaDir(s.opts.Root, s.opts.Account), "assets")
		fs := http.FileServer(http.Dir(root))
		w.Header().Set("Cache-Control", "no-store")
		http.StripPrefix("/meta-assets/", fs).ServeHTTP(w, r)
	})
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	loaded := s.current()
	if strings.EqualFold(strings.TrimSpace(s.opts.Env), "dev") && strings.TrimSpace(r.URL.Path) == "/" {
		s.handleDevDashboard(w, r, loaded)
		return
	}
	page := loaded.PageBySlug[r.URL.Path]
	if page == nil {
		http.NotFound(w, r)
		return
	}

	attr := readAttributionCookie(r, loaded.Site.Tracking.AttributionCookie)
	attr, changedAttr := captureAttributionQuery(attr, r.URL.Query(), loaded.Site.Tracking.CaptureQueryParam, page.Page.Key, page.Page.Slug)
	if changedAttr {
		writeAttributionCookie(w, loaded.Site, attr)
	}

	variant := readVariantCookie(r, loaded.Site.Tracking.VariantCookie)
	assignments := renderArmsForPage(page, variant)
	changedVariant := false

	for _, block := range page.Blocks {
		if block.Type != "hero" || block.Hero == nil || block.Hero.Bandit == nil || !block.Hero.Bandit.Enabled {
			continue
		}
		slot := strings.TrimSpace(block.Hero.Bandit.Slot)
		if slot == "" {
			slot = "hero"
		}
		arm := strings.TrimSpace(assignments[slot])
		validArm := false
		armKeys := make([]string, 0, len(block.Hero.Bandit.Arms))
		for _, candidate := range block.Hero.Bandit.Arms {
			k := strings.TrimSpace(candidate.Key)
			if k != "" {
				armKeys = append(armKeys, k)
			}
			if k == arm {
				validArm = true
			}
		}
		if !validArm {
			recID := banditRecKey(page.Page.Key, block.Key, slot)
			if rec := strings.TrimSpace(s.banditRecommendation(recID)); rec != "" {
				arm = rec
			}
			if strings.TrimSpace(arm) == "" {
				stats, err := s.store.Stats(s.opts.Account, page.Page.Key, block.Key, slot, armKeys)
				if err != nil {
					log.Printf("[landing][warn] bandit stats failed page=%s block=%s slot=%s err=%v", page.Page.Key, block.Key, slot, err)
				}
				arm = ChooseArm(block.Hero.Bandit.Arms, stats, loaded.Site.Bandit.MinImpressionsPerArm, loaded.Site.Bandit.ControlMinShare)
			}
			if variant[page.Page.Key] == nil {
				variant[page.Page.Key] = map[string]string{}
			}
			variant[page.Page.Key][slot] = arm
			assignments[slot] = arm
			changedVariant = true
		}
		if arm != "" {
			if err := s.store.IncrementImpression(s.opts.Account, page.Page.Key, block.Key, slot, arm); err != nil {
				log.Printf("[landing][warn] bandit impression failed page=%s block=%s slot=%s arm=%s err=%v", page.Page.Key, block.Key, slot, arm, err)
			}
		}
	}
	if changedVariant {
		writeVariantCookie(w, loaded.Site, variant)
	}

	if s.posthog != nil {
		props := attributionProperties(attr)
		props["page_key"] = page.Page.Key
		props["slug"] = page.Page.Slug
		props["referrer"] = r.Referer()
		props["user_agent"] = r.UserAgent()
		props["path"] = r.URL.Path
		if len(assignments) > 0 {
			props["arms"] = assignments
		}
		if err := s.posthog.Capture(r.Context(), loaded.Site.PostHog.Events.Impression, attr.AnonID, props); err != nil {
			log.Printf("[landing][warn] posthog impression failed: %v", err)
		}
	}
	if s.capi != nil {
		props := map[string]any{
			"page_key": page.Page.Key,
			"slug":     page.Page.Slug,
			"arms":     assignments,
		}
		if err := s.capi.Capture(r.Context(), MetaCAPIEvent{
			EventName:  "PageView",
			EventID:    randomID(),
			EventTime:  time.Now().UTC(),
			EventURL:   absoluteURLForRequest(loaded.Site.Runtime.PublicBaseURL, r),
			UserAgent:  r.UserAgent(),
			ClientIP:   requestClientIP(r, s.opts.TrustProxy),
			FBC:        deriveFBC(r, attr),
			FBP:        deriveFBP(r),
			ExternalID: attr.AnonID,
			CustomData: props,
		}); err != nil {
			log.Printf("[landing][warn] meta capi pageview failed: %v", err)
		}
	}

	html := renderPageHTML(renderContext{
		page:        page,
		assignments: assignments,
		site:        loaded,
		attribution: attr,
		request:     r,
		state:       s.store,
		serveOpts:   s.opts,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if strings.EqualFold(s.opts.Env, "prod") {
		w.Header().Set("Cache-Control", "public, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleDevDashboard(w http.ResponseWriter, r *http.Request, loaded *LoadedSite) {
	cards, err := s.loadDevAdCards(loaded)
	if err != nil {
		log.Printf("[landing][warn] dev dashboard data load failed: %v", err)
	}
	html := renderDevDashboardHTML(devDashboardContext{
		site:      loaded,
		request:   r,
		serveOpts: s.opts,
		cards:     cards,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(html))
}

func (s *Server) loadDevAdCards(loaded *LoadedSite) ([]devAdCard, error) {
	bundle, err := config.Load(s.opts.Root, s.opts.Account)
	if err != nil {
		return nil, err
	}
	metaDir := workspace.ResolveMetaDir(s.opts.Root, s.opts.Account)
	imageByKey := buildLocalMetaAssetURLIndex(filepath.Join(metaDir, "assets", "images"), "/meta-assets/images")
	videoByKey := buildLocalMetaAssetURLIndex(filepath.Join(metaDir, "assets", "videos"), "/meta-assets/videos")

	adKeys := make([]string, 0, len(bundle.Ads))
	for k := range bundle.Ads {
		adKeys = append(adKeys, k)
	}
	sort.Strings(adKeys)

	cards := make([]devAdCard, 0, len(adKeys))
	for _, adKey := range adKeys {
		ad := bundle.Ads[adKey]
		adset := bundle.Adsets[ad.AdsetKey]
		campaign := bundle.Campaigns[adset.CampaignKey]
		creative := bundle.Creatives[ad.CreativeKey]

		destinationRaw := firstNonEmpty(
			creative.Link.URL,
			findFirstNestedStringByKey(creative.ObjectStorySpec, "link"),
			strings.TrimSpace(loaded.Site.Runtime.MainSiteBaseURL),
		)
		targetURL, targetKind, landingSlug := classifyPreviewDestination(destinationRaw, loaded)
		if strings.TrimSpace(targetURL) == "" {
			targetURL = "/"
		}

		mediaURL, mediaType := resolveCreativePreviewMedia(creative, imageByKey, videoByKey)
		primary := firstNonEmpty(
			creative.Link.Message,
			firstListValue(creative.BodyVariants),
			findFirstNestedStringByKey(creative.ObjectStorySpec, "message"),
			findFirstNestedStringByKey(creative.AssetFeedSpec, "text"),
		)
		headline := firstNonEmpty(
			creative.Link.Headline,
			firstListValue(creative.HeadlineVariants),
			findFirstNestedStringByKey(creative.ObjectStorySpec, "name"),
			findFirstNestedStringByKey(creative.ObjectStorySpec, "title"),
			creative.Name,
		)
		description := firstNonEmpty(
			creative.Link.Description,
			firstListValue(creative.DescriptionVariants),
			findFirstNestedStringByKey(creative.ObjectStorySpec, "description"),
		)
		cta := firstNonEmpty(
			creative.Link.CallToActionType,
			findCallToActionType(creative.ObjectStorySpec),
			"LEARN_MORE",
		)

		cards = append(cards, devAdCard{
			AdKey:         ad.Key,
			AdName:        ad.Name,
			AdStatus:      ad.Status,
			CampaignKey:   campaign.Key,
			CampaignName:  campaign.Name,
			AdsetKey:      adset.Key,
			AdsetName:     adset.Name,
			CreativeKey:   creative.Key,
			CreativeName:  creative.Name,
			PrimaryText:   primary,
			Headline:      headline,
			Description:   description,
			CTAType:       strings.ToUpper(strings.TrimSpace(cta)),
			Destination:   targetURL,
			DestinationIs: targetKind,
			LandingSlug:   landingSlug,
			MediaURL:      mediaURL,
			MediaType:     mediaType,
		})
	}
	return cards, nil
}

func buildLocalMetaAssetURLIndex(dir string, publicPrefix string) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		key := normalizeKey(base)
		if key == "" {
			continue
		}
		out[key] = strings.TrimRight(publicPrefix, "/") + "/" + url.PathEscape(name)
	}
	return out
}

func resolveCreativePreviewMedia(creative config.Creative, imageByKey, videoByKey map[string]string) (string, string) {
	keys := make([]string, 0, len(creative.LinkedAssetKeys)+1)
	if strings.TrimSpace(creative.Link.ImageAssetKey) != "" {
		keys = append(keys, creative.Link.ImageAssetKey)
	}
	keys = append(keys, creative.LinkedAssetKeys...)
	for _, raw := range keys {
		key := normalizeKey(raw)
		if key == "" {
			continue
		}
		if u := strings.TrimSpace(imageByKey[key]); u != "" {
			return u, "image"
		}
		if u := strings.TrimSpace(videoByKey[key]); u != "" {
			return u, "video"
		}
	}
	if u := firstNonEmpty(
		findFirstNestedStringByKey(creative.ObjectStorySpec, "image_url"),
		findFirstNestedStringByKey(creative.ObjectStorySpec, "picture"),
		findFirstNestedStringByKey(creative.ObjectStorySpec, "thumbnail_url"),
	); u != "" {
		return u, "image"
	}
	if u := firstNonEmpty(
		findFirstNestedStringByKey(creative.ObjectStorySpec, "video_url"),
		findFirstNestedStringByKey(creative.ObjectStorySpec, "source"),
	); u != "" {
		return u, "video"
	}
	return "", ""
}

func classifyPreviewDestination(raw string, loaded *LoadedSite) (string, string, string) {
	dest := strings.TrimSpace(raw)
	if dest == "" {
		return "", "unknown", ""
	}
	if strings.HasPrefix(dest, "/") {
		if loaded != nil && loaded.PageBySlug[strings.TrimSpace(dest)] != nil {
			return dest, "landing", strings.TrimSpace(dest)
		}
		return dest, "website", ""
	}
	u, err := url.Parse(dest)
	if err != nil {
		return dest, "website", ""
	}
	slug := strings.TrimSpace(u.Path)
	if loaded != nil && loaded.PageBySlug[slug] != nil {
		publicHost := hostFromBaseURL(loaded.Site.Runtime.PublicBaseURL)
		mainHost := hostFromBaseURL(loaded.Site.Runtime.MainSiteBaseURL)
		destHost := strings.ToLower(strings.TrimSpace(u.Hostname()))
		if destHost == "" || destHost == publicHost {
			return pathWithQueryAndFragment(u), "landing", slug
		}
		if destHost == mainHost && publicHost != "" {
			u.Scheme = ""
			u.Host = ""
			return pathWithQueryAndFragment(u), "landing", slug
		}
	}
	return dest, "website", ""
}

func hostFromBaseURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Hostname()))
}

func pathWithQueryAndFragment(u *url.URL) string {
	if u == nil {
		return ""
	}
	out := strings.TrimSpace(u.Path)
	if out == "" {
		out = "/"
	}
	if strings.TrimSpace(u.RawQuery) != "" {
		out += "?" + u.RawQuery
	}
	if strings.TrimSpace(u.Fragment) != "" {
		out += "#" + u.Fragment
	}
	return out
}

func findCallToActionType(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if strings.EqualFold(strings.TrimSpace(k), "call_to_action") {
				if m, ok := vv.(map[string]any); ok {
					if ctaType := strings.TrimSpace(asString(m["type"])); ctaType != "" {
						return ctaType
					}
				}
			}
		}
		for _, vv := range t {
			if out := findCallToActionType(vv); out != "" {
				return out
			}
		}
	case []any:
		for _, item := range t {
			if out := findCallToActionType(item); out != "" {
				return out
			}
		}
	}
	return ""
}

func findFirstNestedStringByKey(v any, targetKey string) string {
	target := strings.ToLower(strings.TrimSpace(targetKey))
	if target == "" {
		return ""
	}
	switch t := v.(type) {
	case map[string]any:
		for k, vv := range t {
			if strings.ToLower(strings.TrimSpace(k)) == target {
				if s := strings.TrimSpace(asString(vv)); s != "" {
					return s
				}
			}
		}
		for _, vv := range t {
			if s := findFirstNestedStringByKey(vv, target); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range t {
			if s := findFirstNestedStringByKey(item, target); s != "" {
				return s
			}
		}
	}
	return ""
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func firstListValue(values []string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	loaded := s.current()
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if to == "" {
		http.Error(w, "missing to", http.StatusBadRequest)
		return
	}
	dest, err := url.Parse(to)
	if err != nil || (dest.Scheme != "http" && dest.Scheme != "https") {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}
	pageKey := strings.TrimSpace(r.URL.Query().Get("page"))
	blockKey := strings.TrimSpace(r.URL.Query().Get("block"))
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	arm := strings.TrimSpace(r.URL.Query().Get("arm"))

	attr := readAttributionCookie(r, loaded.Site.Tracking.AttributionCookie)
	finalDest := withUTMPassthrough(to, attr, loaded.Site)

	if arm != "" && pageKey != "" && blockKey != "" {
		slot := "hero"
		for _, page := range loaded.Pages {
			if page.Page.Key != pageKey {
				continue
			}
			for _, block := range page.Blocks {
				if block.Key == blockKey && block.Hero != nil && block.Hero.Bandit != nil {
					if strings.TrimSpace(block.Hero.Bandit.Slot) != "" {
						slot = strings.TrimSpace(block.Hero.Bandit.Slot)
					}
				}
			}
		}
		if err := s.store.IncrementClick(s.opts.Account, pageKey, blockKey, slot, arm); err != nil {
			log.Printf("[landing][warn] bandit click failed page=%s block=%s slot=%s arm=%s err=%v", pageKey, blockKey, slot, arm, err)
		}
	}

	if s.posthog != nil {
		event := loaded.Site.PostHog.Events.CTAClick
		if strings.EqualFold(kind, "product") {
			event = loaded.Site.PostHog.Events.ProductClick
		}
		props := attributionProperties(attr)
		props["page_key"] = pageKey
		props["block_key"] = blockKey
		props["kind"] = kind
		props["arm"] = arm
		props["to"] = finalDest
		props["referrer"] = r.Referer()
		if err := s.posthog.Capture(r.Context(), event, attr.AnonID, props); err != nil {
			log.Printf("[landing][warn] posthog click failed: %v", err)
		}
	}
	if s.capi != nil {
		name := "Lead"
		if strings.EqualFold(kind, "product") {
			name = "ViewContent"
		}
		props := map[string]any{
			"page_key":  pageKey,
			"block_key": blockKey,
			"kind":      kind,
			"arm":       arm,
			"to":        finalDest,
		}
		if err := s.capi.Capture(r.Context(), MetaCAPIEvent{
			EventName:  name,
			EventID:    randomID(),
			EventTime:  time.Now().UTC(),
			EventURL:   absoluteURLForRequest(loaded.Site.Runtime.PublicBaseURL, r),
			UserAgent:  r.UserAgent(),
			ClientIP:   requestClientIP(r, s.opts.TrustProxy),
			FBC:        deriveFBC(r, attr),
			FBP:        deriveFBP(r),
			ExternalID: attr.AnonID,
			CustomData: props,
		}); err != nil {
			log.Printf("[landing][warn] meta capi click failed: %v", err)
		}
	}

	http.Redirect(w, r, finalDest, http.StatusFound)
}

func (s *Server) reloadLoop(ctx context.Context) {
	lastSig := ""
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loaded := s.current()
			if loaded == nil {
				continue
			}
			sig := landingSignature(loaded.LandingDir)
			if sig == "" || sig == lastSig {
				continue
			}
			loaded, err := Load(s.opts.Root, s.opts.Account, s.opts)
			if err != nil {
				log.Printf("[landing][reload] invalid config, keeping previous: %v", err)
				continue
			}
			s.setLoaded(loaded)
			s.restoreFeedIndexFromCache()
			go s.refreshFeedIndex(ctx)
			if loaded.Site.Bandit.Enabled {
				go s.refreshBanditRecommendations(ctx)
			}
			lastSig = sig
			if len(loaded.ValidationWarn) > 0 {
				for _, w := range loaded.ValidationWarn {
					log.Printf("[landing][reload][warn] %s", w)
				}
			}
			log.Printf("[landing][reload] applied")
		}
	}
}

func (s *Server) banditRecommendation(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.banditRecByID[id]
}

func (s *Server) setBanditRecommendations(recs map[string]string) {
	s.mu.Lock()
	s.banditRecByID = recs
	s.mu.Unlock()
}

func (s *Server) banditUpdateLoop(ctx context.Context) {
	interval := s.current().Site.Bandit.UpdateIntervalMinutes
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshBanditRecommendations(ctx)
		}
	}
}

func (s *Server) refreshBanditRecommendations(ctx context.Context) {
	loaded := s.current()
	if loaded == nil || !loaded.Site.Bandit.Enabled {
		return
	}
	recs := map[string]string{}
	for _, page := range loaded.Pages {
		for _, block := range page.Blocks {
			if block.Hero == nil || block.Hero.Bandit == nil || !block.Hero.Bandit.Enabled {
				continue
			}
			slot := strings.TrimSpace(block.Hero.Bandit.Slot)
			if slot == "" {
				slot = "hero"
			}
			armKeys := make([]string, 0, len(block.Hero.Bandit.Arms))
			for _, a := range block.Hero.Bandit.Arms {
				key := strings.TrimSpace(a.Key)
				if key == "" {
					continue
				}
				armKeys = append(armKeys, key)
				_ = s.store.EnsureArm(s.opts.Account, page.Page.Key, block.Key, slot, key)
			}
			stats, err := s.store.Stats(s.opts.Account, page.Page.Key, block.Key, slot, armKeys)
			if err != nil {
				log.Printf("[landing][warn] bandit update stats failed page=%s block=%s slot=%s err=%v", page.Page.Key, block.Key, slot, err)
				continue
			}
			arm := ChooseArm(block.Hero.Bandit.Arms, stats, loaded.Site.Bandit.MinImpressionsPerArm, loaded.Site.Bandit.ControlMinShare)
			if strings.TrimSpace(arm) != "" {
				recs[banditRecKey(page.Page.Key, block.Key, slot)] = arm
			}
		}
	}
	s.setBanditRecommendations(recs)
	log.Printf("[landing] bandit recommendations updated: %d slots", len(recs))
	_ = ctx
}

func banditRecKey(pageKey, blockKey, slot string) string {
	return strings.TrimSpace(pageKey) + "|" + strings.TrimSpace(blockKey) + "|" + strings.TrimSpace(slot)
}

func openBanditCounterStore(opts ServeOptions, site SiteConfig) (BanditCounterStore, error) {
	storeType := strings.ToLower(strings.TrimSpace(site.Bandit.Storage.Type))
	if storeType == "" {
		storeType = "sqlite"
	}
	switch storeType {
	case "sqlite":
		path := strings.TrimSpace(site.Bandit.Storage.SQLitePath)
		if path == "" {
			path = strings.TrimSpace(opts.StatePath)
		}
		if path == "" {
			path = filepath.Join(opts.Root, ".adform", fmt.Sprintf("landing_state_%s.db", opts.Account))
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.Root, path)
		}
		return OpenSQLiteBanditStore(path)
	case "redis":
		password := ""
		if env := strings.TrimSpace(site.Bandit.Storage.Redis.PasswordEnv); env != "" {
			password = strings.TrimSpace(os.Getenv(env))
		}
		return OpenRedisBanditStore(RedisBanditConfig{
			Addr:      strings.TrimSpace(site.Bandit.Storage.Redis.Addr),
			Password:  password,
			DB:        site.Bandit.Storage.Redis.DB,
			KeyPrefix: strings.TrimSpace(site.Bandit.Storage.Redis.KeyPrefix),
		})
	default:
		return nil, fmt.Errorf("unsupported bandit storage type %q (expected sqlite|redis)", storeType)
	}
}

func (s *Server) refreshFeedIndex(ctx context.Context) {
	bundle, err := config.Load(s.opts.Root, s.opts.Account)
	if err != nil {
		return
	}
	feedURL := strings.TrimSpace(bundle.AccountCfg.Meta.ProductFeedURL)
	if feedURL == "" {
		return
	}
	st, err := s.openAccountState()
	if err != nil {
		log.Printf("[landing][warn] feed cache state open failed: %v", err)
		return
	}
	defer st.Close()

	loadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	type result struct {
		products map[string]FeedProduct
		err      error
	}
	done := make(chan result, 1)
	go func() {
		products, err := loadFeedIndex(feedURL)
		if err != nil {
			done <- result{err: err}
			return
		}
		done <- result{products: products}
	}()
	select {
	case <-loadCtx.Done():
		log.Printf("[landing][warn] feed index refresh timeout")
		s.restoreFeedIndexFromCacheForURL(st, feedURL)
		return
	case res := <-done:
		if res.err != nil {
			log.Printf("[landing][warn] feed index refresh failed: %v", res.err)
			s.restoreFeedIndexFromCacheForURL(st, feedURL)
			return
		}
		if res.products == nil {
			s.restoreFeedIndexFromCacheForURL(st, feedURL)
			return
		}
		payload, err := json.Marshal(res.products)
		if err != nil {
			log.Printf("[landing][warn] feed cache serialize failed: %v", err)
		} else if err := st.UpsertFeedCache(appstate.FeedCacheRow{
			AccountName: s.opts.Account,
			FeedURL:     feedURL,
			FetchedAt:   time.Now().UTC().Format(time.RFC3339),
			PayloadJSON: string(payload),
		}); err != nil {
			log.Printf("[landing][warn] feed cache write failed: %v", err)
		}
		s.mu.Lock()
		if s.loaded != nil {
			s.loaded.FeedByID = res.products
		}
		s.mu.Unlock()
		log.Printf("[landing] feed index loaded: %d products", len(res.products))
	}
}

func (s *Server) restoreFeedIndexFromCache() {
	bundle, err := config.Load(s.opts.Root, s.opts.Account)
	if err != nil {
		return
	}
	feedURL := strings.TrimSpace(bundle.AccountCfg.Meta.ProductFeedURL)
	if feedURL == "" {
		return
	}
	st, err := s.openAccountState()
	if err != nil {
		return
	}
	defer st.Close()
	s.restoreFeedIndexFromCacheForURL(st, feedURL)
}

func (s *Server) openAccountState() (*appstate.Store, error) {
	path := strings.TrimSpace(os.Getenv("ADFORM_STATE_PATH"))
	if path == "" {
		path = filepath.Join(s.opts.Root, ".adform", "state.db")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.opts.Root, path)
	}
	return appstate.Open(path)
}

func (s *Server) restoreFeedIndexFromCacheForURL(st *appstate.Store, feedURL string) bool {
	if st == nil {
		return false
	}
	row, err := st.GetFeedCache(s.opts.Account, feedURL)
	if err != nil {
		log.Printf("[landing][warn] read feed cache failed: %v", err)
		return false
	}
	if row == nil || strings.TrimSpace(row.PayloadJSON) == "" {
		return false
	}
	products := map[string]FeedProduct{}
	if err := json.Unmarshal([]byte(row.PayloadJSON), &products); err != nil {
		log.Printf("[landing][warn] feed cache decode failed: %v", err)
		return false
	}
	s.mu.Lock()
	if s.loaded != nil {
		s.loaded.FeedByID = products
	}
	s.mu.Unlock()
	log.Printf("[landing] feed index restored from cache: %d products", len(products))
	return true
}

func (s *Server) feedRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(feedRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshFeedIndex(ctx)
		}
	}
}

func landingSignature(root string) string {
	base := root
	paths := []string{
		filepath.Join(base, "site.yml"),
		filepath.Join(base, "theme.css"),
	}
	_ = filepath.WalkDir(filepath.Join(base, "pages"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".yml") || strings.HasSuffix(strings.ToLower(d.Name()), ".yaml") {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		parts = append(parts, p+":"+st.ModTime().UTC().Format(time.RFC3339Nano)+":"+fmt.Sprintf("%d", st.Size()))
	}
	return strings.Join(parts, "|")
}
