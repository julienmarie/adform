package landing

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type renderContext struct {
	page        *PageFile
	assignments map[string]string
	site        *LoadedSite
	attribution AttributionData
	request     *http.Request
	state       BanditCounterStore
	serveOpts   ServeOptions
}

type devDashboardContext struct {
	site      *LoadedSite
	request   *http.Request
	serveOpts ServeOptions
	cards     []devAdCard
}

type devAdCard struct {
	AdKey         string
	AdName        string
	AdStatus      string
	CampaignKey   string
	CampaignName  string
	AdsetKey      string
	AdsetName     string
	CreativeKey   string
	CreativeName  string
	PrimaryText   string
	Headline      string
	Description   string
	CTAType       string
	Destination   string
	DestinationIs string
	LandingSlug   string
	MediaURL      string
	MediaType     string
}

func renderPageHTML(ctx renderContext) string {
	site := &ctx.site.Site
	page := ctx.page
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"")
	b.WriteString(html.EscapeString(normalizeHTMLLang(site.Defaults.Locale)))
	b.WriteString("\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	if page.Page.SEO.Title != "" {
		b.WriteString("<title>")
		b.WriteString(html.EscapeString(page.Page.SEO.Title))
		b.WriteString("</title>")
	}
	if page.Page.SEO.Description != "" {
		b.WriteString("<meta name=\"description\" content=\"")
		b.WriteString(html.EscapeString(page.Page.SEO.Description))
		b.WriteString("\">")
	}
	b.WriteString(`<link rel="icon" href="/assets/favicon.svg" type="image/svg+xml">`)
	b.WriteString(renderInlineThemeCSS(ctx))
	b.WriteString(renderMenuSystemFontCSS())
	if heroPreload := firstHeroImageURL(ctx); heroPreload != "" {
		b.WriteString(`<link rel="preload" as="image" fetchpriority="high" href="`)
		b.WriteString(html.EscapeString(heroPreload))
		b.WriteString(`">`)
	}
	if strings.EqualFold(strings.TrimSpace(ctx.serveOpts.Env), "prod") && site.MetaPixel.Enabled && strings.TrimSpace(site.MetaPixel.PixelID) != "" {
		b.WriteString(metaPixelSnippet(strings.TrimSpace(site.MetaPixel.PixelID)))
	}
	b.WriteString("</head><body>")
	b.WriteString(renderTopNav(ctx))
	b.WriteString("<main class=\"lp\">")
	for i := range page.Blocks {
		block := page.Blocks[i]
		arm := ""
		if block.Type == "hero" && block.Hero != nil && block.Hero.Bandit != nil && block.Hero.Bandit.Enabled {
			slot := strings.TrimSpace(block.Hero.Bandit.Slot)
			if slot == "" {
				slot = "hero"
			}
			arm = ctx.assignments[slot]
		}
		switch block.Type {
		case "spacer":
			b.WriteString(renderSpacerBlock(block))
		case "hero":
			b.WriteString(renderHeroBlock(ctx, block, arm))
		case "media_split":
			b.WriteString(renderMediaSplitBlock(ctx, block, arm))
		case "product_grid":
			b.WriteString(renderProductGridBlock(ctx, block, arm))
		case "columns":
			b.WriteString(renderColumnsBlock(ctx, block, arm))
		case "trust_strip":
			b.WriteString(renderTrustStripBlock(ctx, block, arm))
		case "faq":
			b.WriteString(renderFAQBlock(ctx, block, arm))
		case "pairings":
			b.WriteString(renderPairingsBlock(ctx, block, arm))
		}
	}
	b.WriteString("</main>")
	b.WriteString(renderFooter(ctx))
	b.WriteString(renderDevPageSwitcher(ctx))
	b.WriteString(renderGlobalScripts(ctx))
	b.WriteString("</body></html>")
	return b.String()
}

func renderDevDashboardHTML(ctx devDashboardContext) string {
	if ctx.site == nil {
		return "<!doctype html><html><head><meta charset=\"utf-8\"><title>Ad Preview</title></head><body>no landing config loaded</body></html>"
	}
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"")
	b.WriteString(html.EscapeString(normalizeHTMLLang(ctx.site.Site.Defaults.Locale)))
	b.WriteString("\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>Adform Dev Preview</title>")
	b.WriteString(renderInlineThemeCSS(renderContext{site: ctx.site}))
	b.WriteString(renderMenuSystemFontCSS())
	b.WriteString(`<style id="adform-dev-dashboard">`)
	b.WriteString(`body{margin:0;background:#eef1f6;color:#0f172a;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}`)
	b.WriteString(`.adform-dev-shell{padding:16px 20px 32px;max-width:1400px;margin:0 auto}`)
	b.WriteString(`.adform-dev-head{margin:8px 0 14px;display:flex;justify-content:space-between;gap:12px;align-items:flex-end;flex-wrap:wrap}`)
	b.WriteString(`.adform-dev-head h1{margin:0;font-size:20px;line-height:1.2}`)
	b.WriteString(`.adform-dev-head p{margin:6px 0 0;color:#475569;font-size:13px}`)
	b.WriteString(`.adform-dev-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:14px}`)
	b.WriteString(`.ad-preview-card{background:#fff;border:1px solid #d9e2ec;border-radius:14px;overflow:hidden;box-shadow:0 8px 20px rgba(2,6,23,.06);display:flex;flex-direction:column}`)
	b.WriteString(`.ad-preview-media{position:relative;background:linear-gradient(160deg,#c7d2fe,#bfdbfe);aspect-ratio:1/1;display:flex;align-items:center;justify-content:center;color:#1e293b}`)
	b.WriteString(`.ad-preview-media img,.ad-preview-media video{width:100%;height:100%;object-fit:cover;display:block}`)
	b.WriteString(`.ad-preview-media .media-badge{position:absolute;top:8px;left:8px;background:rgba(15,23,42,.78);color:#fff;border-radius:999px;padding:2px 8px;font-size:11px;letter-spacing:.02em}`)
	b.WriteString(`.ad-preview-media .media-empty{font-size:12px;opacity:.88;text-align:center;padding:0 16px}`)
	b.WriteString(`.ad-preview-body{padding:10px 12px 12px;display:grid;gap:8px}`)
	b.WriteString(`.ad-preview-top{display:flex;justify-content:space-between;gap:8px;align-items:flex-start}`)
	b.WriteString(`.ad-preview-title{margin:0;font-size:13px;font-weight:700;line-height:1.35;color:#0f172a}`)
	b.WriteString(`.ad-preview-meta{font-size:11px;color:#64748b;line-height:1.35}`)
	b.WriteString(`.ad-preview-copy{font-size:12px;line-height:1.45;color:#1f2937;display:grid;gap:4px}`)
	b.WriteString(`.ad-preview-copy p{margin:0}`)
	b.WriteString(`.ad-preview-copy .muted{color:#6b7280}`)
	b.WriteString(`.ad-preview-tags{display:flex;flex-wrap:wrap;gap:6px}`)
	b.WriteString(`.ad-preview-tag{display:inline-flex;align-items:center;border:1px solid #cbd5e1;background:#f8fafc;color:#334155;padding:2px 8px;border-radius:999px;font-size:11px}`)
	b.WriteString(`.ad-preview-tag.status-active{background:#dcfce7;border-color:#bbf7d0;color:#166534}`)
	b.WriteString(`.ad-preview-tag.status-paused{background:#fef3c7;border-color:#fde68a;color:#92400e}`)
	b.WriteString(`.ad-preview-actions{display:flex;gap:8px;align-items:center;justify-content:space-between}`)
	b.WriteString(`.ad-preview-link{display:inline-flex;align-items:center;justify-content:center;padding:7px 11px;border-radius:8px;background:#1877f2;color:#fff;text-decoration:none;font-size:12px;font-weight:700}`)
	b.WriteString(`.ad-preview-link:hover{background:#0b65da}`)
	b.WriteString(`.ad-preview-dest{font-size:11px;color:#475569;word-break:break-all;max-width:58%}`)
	b.WriteString(`.ad-preview-dest a{color:inherit}`)
	b.WriteString(`@media (max-width:900px){.adform-dev-shell{padding:12px}.adform-dev-grid{grid-template-columns:1fr}}`)
	b.WriteString(`</style></head><body>`)
	b.WriteString(`<main class="adform-dev-shell">`)
	b.WriteString(`<section class="adform-dev-head">`)
	b.WriteString(`<div><h1>Ad Preview Board</h1><p>`)
	b.WriteString(strconv.Itoa(len(ctx.cards)))
	b.WriteString(` ads loaded for account `)
	b.WriteString(html.EscapeString(ctx.serveOpts.Account))
	b.WriteString(`. Click a card CTA to open its landing or destination URL.</p></div>`)
	b.WriteString(`</section>`)
	b.WriteString(`<section class="adform-dev-grid" aria-label="Ads preview grid">`)
	for _, card := range ctx.cards {
		b.WriteString(renderDevAdCard(card))
	}
	if len(ctx.cards) == 0 {
		b.WriteString(`<article class="ad-preview-card"><div class="ad-preview-body"><p class="ad-preview-title">No ads found</p><p class="ad-preview-meta">Import or generate ads, then refresh this page.</p></div></article>`)
	}
	b.WriteString(`</section></main>`)
	b.WriteString(renderDevPageNavigator(ctx.site, ""))
	b.WriteString(renderGlobalScripts(renderContext{
		site:      ctx.site,
		request:   ctx.request,
		serveOpts: ctx.serveOpts,
	}))
	b.WriteString(`</body></html>`)
	return b.String()
}

func renderDevAdCard(card devAdCard) string {
	var b strings.Builder
	b.WriteString(`<article class="ad-preview-card">`)
	b.WriteString(`<div class="ad-preview-media">`)
	if strings.TrimSpace(card.MediaURL) != "" {
		if strings.EqualFold(strings.TrimSpace(card.MediaType), "video") {
			b.WriteString(`<video src="`)
			b.WriteString(html.EscapeString(card.MediaURL))
			b.WriteString(`" muted playsinline preload="metadata"></video>`)
		} else {
			b.WriteString(`<img loading="lazy" decoding="async" src="`)
			b.WriteString(html.EscapeString(card.MediaURL))
			b.WriteString(`" alt="`)
			b.WriteString(html.EscapeString(card.CreativeName))
			b.WriteString(`">`)
		}
		b.WriteString(`<span class="media-badge">`)
		b.WriteString(html.EscapeString(strings.ToUpper(strings.TrimSpace(card.MediaType))))
		b.WriteString(`</span>`)
	} else {
		b.WriteString(`<div class="media-empty">No media preview available<br>`)
		b.WriteString(html.EscapeString(card.CreativeKey))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="ad-preview-body">`)
	b.WriteString(`<div class="ad-preview-top"><div>`)
	b.WriteString(`<h2 class="ad-preview-title">`)
	b.WriteString(html.EscapeString(firstFilled(card.Headline, card.AdName, "Untitled ad")))
	b.WriteString(`</h2>`)
	b.WriteString(`<p class="ad-preview-meta">`)
	b.WriteString(html.EscapeString(firstFilled(card.CampaignName, card.CampaignKey, "campaign")))
	if v := strings.TrimSpace(firstFilled(card.AdsetName, card.AdsetKey, "")); v != "" {
		b.WriteString(` · `)
		b.WriteString(html.EscapeString(v))
	}
	b.WriteString(`</p></div></div>`)
	b.WriteString(`<div class="ad-preview-copy">`)
	if t := strings.TrimSpace(card.PrimaryText); t != "" {
		b.WriteString(`<p>`)
		b.WriteString(html.EscapeString(clampText(t, 190)))
		b.WriteString(`</p>`)
	}
	if d := strings.TrimSpace(card.Description); d != "" {
		b.WriteString(`<p class="muted">`)
		b.WriteString(html.EscapeString(clampText(d, 110)))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="ad-preview-tags">`)
	status := strings.ToLower(strings.TrimSpace(card.AdStatus))
	statusClass := "ad-preview-tag"
	if status == "active" {
		statusClass += " status-active"
	}
	if status == "paused" {
		statusClass += " status-paused"
	}
	b.WriteString(`<span class="`)
	b.WriteString(statusClass)
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(strings.ToUpper(firstFilled(card.AdStatus, "UNKNOWN"))))
	b.WriteString(`</span>`)
	if cta := strings.TrimSpace(card.CTAType); cta != "" {
		b.WriteString(`<span class="ad-preview-tag">`)
		b.WriteString(html.EscapeString(strings.ReplaceAll(cta, "_", " ")))
		b.WriteString(`</span>`)
	}
	if strings.TrimSpace(card.DestinationIs) == "landing" {
		b.WriteString(`<span class="ad-preview-tag">LANDING</span>`)
	}
	b.WriteString(`</div>`)
	linkTarget := ""
	linkRel := ""
	if !strings.HasPrefix(strings.TrimSpace(card.Destination), "/") {
		linkTarget = ` target="_blank"`
		linkRel = ` rel="noopener noreferrer"`
	}
	b.WriteString(`<div class="ad-preview-actions"><a class="ad-preview-link" href="`)
	b.WriteString(html.EscapeString(card.Destination))
	b.WriteString(`"`)
	b.WriteString(linkTarget)
	b.WriteString(linkRel)
	b.WriteString(`>Open</a><span class="ad-preview-dest">`)
	b.WriteString(html.EscapeString(clampText(card.Destination, 72)))
	b.WriteString(`</span></div>`)
	b.WriteString(`</div></article>`)
	return b.String()
}

func renderInlineThemeCSS(ctx renderContext) string {
	if ctx.site == nil {
		return ""
	}
	css := strings.TrimSpace(ctx.site.ThemeCSS)
	if css == "" {
		return ""
	}
	css = stripGoogleFontImports(css)
	css = strings.ReplaceAll(css, "</style", "<\\/style")
	return "<style id=\"adform-theme-inline\">\n" + css + "\n</style>"
}

func renderMenuSystemFontCSS() string {
	return `<style id="adform-menu-system-fonts">.lp-brand,.lp-nav-links a{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif !important;}</style>`
}

func normalizeHTMLLang(raw string) string {
	v := strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))
	if v == "" {
		return "en"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range v {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' && !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "en"
	}
	parts := strings.Split(out, "-")
	if len(parts) > 0 {
		parts[0] = strings.ToLower(parts[0])
	}
	if len(parts) > 1 {
		parts[1] = strings.ToUpper(parts[1])
	}
	return strings.Join(parts, "-")
}

func renderGlobalScripts(ctx renderContext) string {
	if ctx.site == nil {
		return ""
	}
	var b strings.Builder
	for _, raw := range ctx.site.Site.Scripts.URLs {
		src := strings.TrimSpace(raw)
		if src == "" {
			continue
		}
		if isGoogleFontsURL(src) {
			continue
		}
		b.WriteString(`<script src="`)
		b.WriteString(html.EscapeString(src))
		b.WriteString(`" defer></script>`)
	}
	for _, raw := range ctx.site.Site.Scripts.Inline {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		b.WriteString("<script>\n")
		b.WriteString(code)
		b.WriteString("\n</script>")
	}
	return b.String()
}

func isGoogleFontsURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return host == "fonts.googleapis.com" || host == "fonts.gstatic.com"
}

func stripGoogleFontImports(css string) string {
	lines := strings.Split(css, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "fonts.googleapis.com") || strings.Contains(lower, "fonts.gstatic.com") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func renderDevPageSwitcher(ctx renderContext) string {
	if !strings.EqualFold(strings.TrimSpace(ctx.serveOpts.Env), "dev") {
		return ""
	}
	if ctx.site == nil {
		return ""
	}
	currentSlug := ""
	if ctx.page != nil {
		currentSlug = strings.TrimSpace(ctx.page.Page.Slug)
	}
	return renderDevPageNavigator(ctx.site, currentSlug)
}

func renderDevPageNavigator(site *LoadedSite, currentSlug string) string {
	if site == nil || len(site.Pages) == 0 {
		return ""
	}
	type pageLink struct {
		Key  string
		Slug string
	}
	links := make([]pageLink, 0, len(site.Pages)+1)
	links = append(links, pageLink{Key: "ad-preview-board", Slug: "/"})
	for _, p := range site.Pages {
		if p == nil {
			continue
		}
		slug := strings.TrimSpace(p.Page.Slug)
		if slug == "" {
			continue
		}
		links = append(links, pageLink{Key: strings.TrimSpace(p.Page.Key), Slug: slug})
	}
	if len(links) == 0 {
		return ""
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Slug != links[j].Slug {
			return links[i].Slug < links[j].Slug
		}
		return links[i].Key < links[j].Key
	})
	var b strings.Builder
	b.WriteString(`<aside class="adform-dev-pages" aria-label="Landing pages navigator">`)
	b.WriteString(`<style>`)
	b.WriteString(`.adform-dev-pages{position:fixed;top:12px;right:12px;z-index:2147483647;width:min(360px,34vw);max-height:calc(100vh - 24px);overflow:auto;background:rgba(17,24,39,.92);color:#fff;border:1px solid rgba(255,255,255,.2);border-radius:12px;padding:10px 12px;font:12px/1.4 ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;box-shadow:0 8px 24px rgba(0,0,0,.35)}`)
	b.WriteString(`.adform-dev-pages h3{margin:0 0 8px;font-size:12px;letter-spacing:.02em;text-transform:uppercase;color:#e5e7eb}`)
	b.WriteString(`.adform-dev-pages ul{list-style:none;margin:0;padding:0;display:grid;gap:6px}`)
	b.WriteString(`.adform-dev-pages a{display:block;padding:6px 8px;border-radius:8px;color:#dbeafe;text-decoration:none;border:1px solid transparent}`)
	b.WriteString(`.adform-dev-pages a:hover{background:rgba(255,255,255,.08);border-color:rgba(255,255,255,.22)}`)
	b.WriteString(`.adform-dev-pages a.is-current{background:rgba(37,99,235,.35);border-color:rgba(147,197,253,.75);color:#fff}`)
	b.WriteString(`.adform-dev-pages .slug{display:block}`)
	b.WriteString(`.adform-dev-pages .key{display:block;color:#cbd5e1;font-size:11px;opacity:.9}`)
	b.WriteString(`@media (max-width:900px){.adform-dev-pages{left:8px;right:8px;top:auto;bottom:8px;width:auto;max-height:38vh}}`)
	b.WriteString(`</style>`)
	b.WriteString(`<h3>Pages</h3><ul>`)
	for _, link := range links {
		className := ""
		if link.Slug == currentSlug {
			className = " class=\"is-current\""
		}
		b.WriteString(`<li><a`)
		b.WriteString(className)
		b.WriteString(` href="`)
		b.WriteString(html.EscapeString(link.Slug))
		b.WriteString(`"><span class="slug">`)
		b.WriteString(html.EscapeString(link.Slug))
		b.WriteString(`</span>`)
		if link.Key != "" {
			b.WriteString(`<span class="key">`)
			b.WriteString(html.EscapeString(link.Key))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</a></li>`)
	}
	b.WriteString(`</ul></aside>`)
	return b.String()
}

func clampText(v string, limit int) string {
	s := strings.TrimSpace(v)
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func firstFilled(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func renderSpacerBlock(block Block) string {
	size := "md"
	if block.Spacer != nil && strings.TrimSpace(block.Spacer.Size) != "" {
		size = strings.ToLower(strings.TrimSpace(block.Spacer.Size))
	}
	return fmt.Sprintf("<div id=\"%s\" class=\"block block-spacer spacer-%s\" data-block=\"%s\"></div>", html.EscapeString(blockAnchor(block.Key)), html.EscapeString(size), html.EscapeString(block.Key))
}

func renderHeroBlock(ctx renderContext, block Block, armKey string) string {
	hero := block.Hero
	if hero == nil {
		return ""
	}
	selected := HeroArm{}
	if hero.Bandit != nil && hero.Bandit.Enabled {
		for _, arm := range hero.Bandit.Arms {
			if strings.TrimSpace(arm.Key) == strings.TrimSpace(armKey) {
				selected = arm
				break
			}
		}
	}
	h1 := chooseString(selected.H1, hero.H1)
	subhead := chooseString(selected.Subhead, hero.Subhead)
	body := chooseString(selected.Body, hero.Body)
	assetKey := chooseString(selected.BGImageAssetKey, hero.BGImageAssetKey)
	overlay := hero.Overlay.Opacity
	if selected.Overlay.Opacity > 0 {
		overlay = selected.Overlay.Opacity
	}
	primary := hero.PrimaryCTA
	if selected.PrimaryCTA != nil {
		primary = selected.PrimaryCTA
	}
	secondary := hero.SecondaryCTA
	if selected.SecondaryCTA != nil {
		secondary = selected.SecondaryCTA
	}

	assetURL := ctx.site.AssetIndex[normalizeKey(assetKey)]
	style := ""
	if assetURL != "" {
		style = " style=\"background-image:url('" + html.EscapeString(assetURL) + "')\""
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-hero\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\" data-arm=\"")
	b.WriteString(html.EscapeString(armKey))
	b.WriteString("\"" + style + ">")
	b.WriteString("<div class=\"hero-overlay\" style=\"opacity:")
	b.WriteString(strconv.FormatFloat(overlay, 'f', 2, 64))
	b.WriteString("\"></div><div class=\"hero-content\">")
	if h1 != "" {
		b.WriteString("<h1>")
		b.WriteString(html.EscapeString(h1))
		b.WriteString("</h1>")
	}
	if subhead != "" {
		b.WriteString("<p class=\"subhead\">")
		b.WriteString(html.EscapeString(subhead))
		b.WriteString("</p>")
	}
	if body != "" {
		b.WriteString("<p class=\"body\">")
		b.WriteString(html.EscapeString(body))
		b.WriteString("</p>")
	}
	if primary != nil {
		b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, armKey, "cta", "hero-primary", primary))
	}
	if secondary != nil {
		b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, armKey, "cta", "hero-secondary", secondary))
	}
	b.WriteString("</div></section>")
	return b.String()
}

func renderMediaSplitBlock(ctx renderContext, block Block, arm string) string {
	m := block.MediaSplit
	if m == nil {
		return ""
	}
	mediaURL := ctx.site.AssetIndex[normalizeKey(m.Media.ImageAssetKey)]
	if mediaURL == "" {
		mediaURL = "/assets/"
	}
	side := strings.ToLower(strings.TrimSpace(m.Layout.MediaSide))
	if side != "left" && side != "right" {
		side = "left"
	}
	align := strings.ToLower(strings.TrimSpace(m.Layout.Align))
	if align != "top" && align != "center" {
		align = "center"
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-media-split side-")
	b.WriteString(html.EscapeString(side))
	b.WriteString(" align-")
	b.WriteString(html.EscapeString(align))
	b.WriteString("\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\"><div class=\"media\"><img loading=\"lazy\" decoding=\"async\" fetchpriority=\"low\" src=\"")
	b.WriteString(html.EscapeString(mediaURL))
	b.WriteString("\" alt=\"")
	b.WriteString(html.EscapeString(m.Media.Alt))
	b.WriteString(`" width="1200" height="800"></div><div class="content">`)
	if m.Content.Eyebrow != "" {
		b.WriteString("<p class=\"eyebrow\">" + html.EscapeString(m.Content.Eyebrow) + "</p>")
	}
	if m.Content.Title != "" {
		b.WriteString("<h2>" + html.EscapeString(m.Content.Title) + "</h2>")
	}
	if m.Content.Body != "" {
		b.WriteString("<p>" + html.EscapeString(m.Content.Body) + "</p>")
	}
	if m.Content.CTA != nil {
		b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, arm, "cta", "media-cta", m.Content.CTA))
	}
	b.WriteString("</div></section>")
	return b.String()
}

func renderProductGridBlock(ctx renderContext, block Block, arm string) string {
	p := block.ProductGrid
	if p == nil {
		return ""
	}
	products := queryFeedProducts(ctx.site.FeedByID, *p)
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-product-grid\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\">")
	if p.Title != "" {
		b.WriteString("<h2>" + html.EscapeString(p.Title) + "</h2>")
	}
	b.WriteString("<div class=\"product-grid\">")
	for _, product := range products {
		to := product.URL
		if strings.TrimSpace(to) == "" {
			to = strings.TrimSpace(ctx.site.Site.Runtime.MainSiteBaseURL)
		}
		if to == "" {
			to = "#"
		}
		link := trackedURL(ctx.request, ctx.page.Page.Key, block.Key, "product", arm, to)
		cardClass := "product-card"
		b.WriteString("<article class=\"")
		b.WriteString(html.EscapeString(cardClass))
		b.WriteString("\"><a href=\"")
		b.WriteString(html.EscapeString(link))
		b.WriteString("\">")
		if product.ImageURL != "" {
			b.WriteString("<img loading=\"lazy\" src=\"")
			b.WriteString(html.EscapeString(product.ImageURL))
			b.WriteString("\" alt=\"")
			b.WriteString(html.EscapeString(product.Title))
			b.WriteString("\">")
		}
		b.WriteString("<h3>" + html.EscapeString(product.Title) + "</h3>")
		note := productTastingNote(*p, product)
		if note != "" {
			b.WriteString("<p class=\"tasting-note\">" + html.EscapeString(note) + "</p>")
		}
		if product.Price != "" {
			b.WriteString("<p class=\"price\">" + html.EscapeString(product.Price) + "</p>")
		}
		if product.InStock != nil && !*product.InStock {
			b.WriteString("<p class=\"stock oos\">Out of stock</p>")
		}
		b.WriteString("</a></article>")
	}
	b.WriteString("</div>")
	if p.CTA != nil {
		b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, arm, "cta", "grid-cta", p.CTA))
	}
	b.WriteString("</section>")
	return b.String()
}

func renderColumnsBlock(ctx renderContext, block Block, arm string) string {
	c := block.Columns
	if c == nil {
		return ""
	}
	cols := c.Columns
	if cols != 2 && cols != 3 {
		cols = 3
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-columns cols-")
	b.WriteString(strconv.Itoa(cols))
	b.WriteString("\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\">")
	if c.Title != "" {
		b.WriteString("<h2>" + html.EscapeString(c.Title) + "</h2>")
	}
	b.WriteString("<div class=\"columns-grid\">")
	for _, item := range c.Items {
		b.WriteString("<article>")
		if item.Title != "" {
			b.WriteString("<h3>" + html.EscapeString(item.Title) + "</h3>")
		}
		if item.Body != "" {
			b.WriteString("<p>" + html.EscapeString(item.Body) + "</p>")
		}
		if strings.TrimSpace(item.Href) != "" {
			cta := &CTA{Label: "Learn more", Href: item.Href}
			b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, arm, "cta", "columns-item", cta))
		}
		b.WriteString("</article>")
	}
	b.WriteString("</div></section>")
	return b.String()
}

func renderTrustStripBlock(ctx renderContext, block Block, arm string) string {
	t := block.TrustStrip
	if t == nil {
		return ""
	}
	items := t.Items
	if len(items) == 0 {
		items = ctx.site.Site.Defaults.TrustItems
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-trust\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\"><h2 class=\"sr-only\">Service highlights</h2><div class=\"trust-items\">")
	for _, item := range items {
		b.WriteString("<article><h3>")
		b.WriteString(html.EscapeString(item.Title))
		b.WriteString("</h3><p>")
		b.WriteString(html.EscapeString(item.Body))
		b.WriteString("</p></article>")
	}
	b.WriteString("</div></section>")
	return b.String()
}

func renderFAQBlock(ctx renderContext, block Block, arm string) string {
	f := block.FAQ
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-faq\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\"><h2>FAQ</h2>")
	for _, item := range f.Items {
		b.WriteString("<details><summary>")
		b.WriteString(html.EscapeString(item.Q))
		b.WriteString("</summary><p>")
		b.WriteString(html.EscapeString(item.A))
		b.WriteString("</p></details>")
	}
	b.WriteString("</section>")
	return b.String()
}

func renderPairingsBlock(ctx renderContext, block Block, arm string) string {
	p := block.Pairings
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<section id=\"")
	b.WriteString(html.EscapeString(blockAnchor(block.Key)))
	b.WriteString("\" class=\"block block-pairings\" data-block=\"")
	b.WriteString(html.EscapeString(block.Key))
	b.WriteString("\">")
	if p.Title != "" {
		b.WriteString("<h2>" + html.EscapeString(p.Title) + "</h2>")
	}
	b.WriteString("<div class=\"pairings-grid\">")
	for _, item := range p.Items {
		b.WriteString("<article>")
		if item.Title != "" {
			b.WriteString("<h3>" + html.EscapeString(item.Title) + "</h3>")
		}
		if item.Body != "" {
			b.WriteString("<p>" + html.EscapeString(item.Body) + "</p>")
		}
		if strings.TrimSpace(item.Href) != "" {
			cta := &CTA{Label: "Explore", Href: item.Href}
			b.WriteString(renderCTA(ctx, ctx.page.Page.Key, block, arm, "cta", "pairings-item", cta))
		}
		b.WriteString("</article>")
	}
	b.WriteString("</div></section>")
	return b.String()
}

func renderCTA(ctx renderContext, pageKey string, block Block, arm, kind, className string, cta *CTA) string {
	if cta == nil {
		return ""
	}
	to := strings.TrimSpace(cta.Href)
	if to == "" {
		return ""
	}
	link := trackedURL(ctx.request, pageKey, block.Key, kind, arm, to)
	label := cta.Label
	if strings.TrimSpace(label) == "" {
		label = "Learn more"
	}
	return "<a class=\"cta " + html.EscapeString(className) + "\" href=\"" + html.EscapeString(link) + "\">" + html.EscapeString(label) + "</a>"
}

func trackedURL(r *http.Request, pageKey, blockKey, kind, arm, to string) string {
	q := url.Values{}
	q.Set("to", to)
	q.Set("page", pageKey)
	q.Set("block", blockKey)
	q.Set("kind", kind)
	if strings.TrimSpace(arm) != "" {
		q.Set("arm", arm)
	}
	base := "/r?" + q.Encode()
	if r != nil {
		if r.URL != nil {
			base = path.Clean("/r") + "?" + q.Encode()
		}
	}
	return base
}

func metaPixelSnippet(pixelID string) string {
	pid := html.EscapeString(pixelID)
	return `<script>
!function(f,b,e,v,n,t,s)
{if(f.fbq)return;n=f.fbq=function(){n.callMethod?
n.callMethod.apply(n,arguments):n.queue.push(arguments)};
if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version='2.0';
n.queue=[];t=b.createElement(e);t.async=!0;
t.src=v;s=b.getElementsByTagName(e)[0];
s.parentNode.insertBefore(t,s)}(window,document,'script',
'https://connect.facebook.net/en_US/fbevents.js');
fbq('init', '` + pid + `');
fbq('track', 'PageView');
</script>
<noscript><img height="1" width="1" style="display:none"
src="https://www.facebook.com/tr?id=` + pid + `&ev=PageView&noscript=1"
/></noscript>`
}

func chooseString(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func firstHeroImageURL(ctx renderContext) string {
	if ctx.page == nil || ctx.site == nil {
		return ""
	}
	for _, block := range ctx.page.Blocks {
		if block.Type != "hero" || block.Hero == nil {
			continue
		}
		key := normalizeKey(block.Hero.BGImageAssetKey)
		if key == "" {
			continue
		}
		if assetURL := ctx.site.AssetIndex[key]; assetURL != "" {
			return assetURL
		}
	}
	return ""
}

type navTargets struct {
	Story      string
	Collection string
	Sourcing   string
}

func renderTopNav(ctx renderContext) string {
	shopURL := strings.TrimSpace(ctx.site.Site.Runtime.MainSiteBaseURL)
	if shopURL == "" {
		shopURL = "/"
	}
	targets := deriveNavTargets(ctx.page)
	var b strings.Builder
	b.WriteString(`<header class="lp-nav"><div class="lp-nav-inner">`)
	b.WriteString(`<a class="lp-brand" href="`)
	b.WriteString(html.EscapeString(shopURL))
	b.WriteString(`">The Bow Tie Duck</a>`)
	b.WriteString(`<nav class="lp-nav-links">`)
	b.WriteString(`<a href="`)
	b.WriteString(html.EscapeString(targets.Story))
	b.WriteString(`">Origin</a>`)
	b.WriteString(`<a href="`)
	b.WriteString(html.EscapeString(targets.Collection))
	b.WriteString(`">Collection</a>`)
	b.WriteString(`<a href="`)
	b.WriteString(html.EscapeString(targets.Sourcing))
	b.WriteString(`">Delivery</a>`)
	b.WriteString(`<a class="lp-nav-shop" href="`)
	b.WriteString(html.EscapeString(shopURL))
	b.WriteString(`">Main Store</a></nav></div></header>`)
	return b.String()
}

func renderFooter(ctx renderContext) string {
	shopURL := strings.TrimSpace(ctx.site.Site.Runtime.MainSiteBaseURL)
	if shopURL == "" {
		shopURL = "/"
	}
	type pageLink struct {
		Key  string
		Slug string
	}
	links := make([]pageLink, 0, len(ctx.site.Pages))
	for _, p := range ctx.site.Pages {
		if p == nil {
			continue
		}
		if strings.TrimSpace(p.Page.Slug) == "" || strings.TrimSpace(p.Page.Key) == "" {
			continue
		}
		links = append(links, pageLink{
			Key:  strings.TrimSpace(p.Page.Key),
			Slug: strings.TrimSpace(p.Page.Slug),
		})
	}
	sort.SliceStable(links, func(i, j int) bool {
		return links[i].Key < links[j].Key
	})
	if len(links) > 6 {
		links = links[:6]
	}
	var b strings.Builder
	b.WriteString(`<footer class="lp-footer"><div class="lp-footer-inner">`)
	b.WriteString(`<section class="lp-footer-brand"><h2>The Bow Tie Duck</h2>`)
	b.WriteString(`<p>European producers, selected with care and delivered across the Philippines with timing clarity. Fine food without guesswork.</p>`)
	b.WriteString(`<div class="lp-social">`)
	b.WriteString(`<a href="https://www.instagram.com/bowtieduck" target="_blank" rel="noopener noreferrer">IG</a>`)
	b.WriteString(`<a href="https://www.facebook.com/bowtieduck" target="_blank" rel="noopener noreferrer">FB</a>`)
	b.WriteString(`<a href="mailto:concierge@bowtieduck.com">Email</a>`)
	b.WriteString(`</div></section>`)
	b.WriteString(`<section class="lp-footer-nav"><h3>Collections</h3><ul>`)
	for _, link := range links {
		b.WriteString(`<li><a href="`)
		b.WriteString(html.EscapeString(link.Slug))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(humanizeKey(link.Key)))
		b.WriteString(`</a></li>`)
	}
	b.WriteString(`</ul></section>`)
	b.WriteString(`<section class="lp-footer-nav"><h3>Service</h3><ul>`)
	b.WriteString(`<li><a href="`)
	b.WriteString(html.EscapeString(shopURL))
	b.WriteString(`">Main Store</a></li>`)
	b.WriteString(`<li><a href="`)
	b.WriteString(html.EscapeString(shopURL))
	b.WriteString(`">Delivery Coverage</a></li>`)
	b.WriteString(`<li><a href="mailto:concierge@bowtieduck.com">Concierge Desk</a></li>`)
	b.WriteString(`</ul></section>`)
	b.WriteString(`<section class="lp-footer-contact"><h3>Concierge</h3><p>Metro Manila, Philippines<br>Mon-Sat 09:00-18:00</p><a href="mailto:concierge@bowtieduck.com">concierge@bowtieduck.com</a></section>`)
	b.WriteString(`</div><div class="lp-footer-legal"><p>© The Bow Tie Duck</p><a href="`)
	b.WriteString(html.EscapeString(shopURL))
	b.WriteString(`">bowtieduck.com</a></div></footer>`)
	return b.String()
}

func deriveNavTargets(page *PageFile) navTargets {
	out := navTargets{Story: "#", Collection: "#", Sourcing: "#"}
	if page == nil {
		return out
	}
	for _, block := range page.Blocks {
		anchorKey := blockAnchor(block.Key)
		if anchorKey == "" {
			continue
		}
		anchor := "#" + anchorKey
		switch block.Type {
		case "media_split", "columns":
			if out.Story == "#" {
				out.Story = anchor
			}
		case "product_grid":
			if out.Collection == "#" {
				out.Collection = anchor
			}
		case "trust_strip", "pairings", "faq":
			if out.Sourcing == "#" {
				out.Sourcing = anchor
			}
		}
	}
	if out.Story == "#" {
		for _, block := range page.Blocks {
			if block.Type == "hero" || block.Type == "spacer" {
				continue
			}
			anchorKey := blockAnchor(block.Key)
			if anchorKey == "" {
				continue
			}
			out.Story = "#" + anchorKey
			break
		}
	}
	if out.Collection == "#" {
		out.Collection = out.Story
	}
	if out.Sourcing == "#" {
		out.Sourcing = out.Collection
	}
	return out
}

func shouldRenderFeaturedBadge(title string, products []FeedProduct) bool {
	if len(products) < 3 {
		return false
	}
	norm := strings.ToLower(strings.TrimSpace(title))
	return !strings.Contains(norm, "restock") && !strings.Contains(norm, "watch") && !strings.Contains(norm, "waitlist")
}

func productTastingNote(grid ProductGridBlock, product FeedProduct) string {
	if len(grid.TastingNotes) == 0 {
		return ""
	}
	id := strings.TrimSpace(product.ID)
	if id == "" {
		return ""
	}
	pid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(grid.TastingNotes[pid])
}

func blockAnchor(key string) string {
	return strings.TrimSpace(key)
}

func humanizeKey(key string) string {
	key = strings.ReplaceAll(strings.TrimSpace(key), "-", " ")
	parts := strings.Fields(key)
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	if len(parts) == 0 {
		return "Page"
	}
	return strings.Join(parts, " ")
}

func renderArmsForPage(page *PageFile, variant variantAssignments) map[string]string {
	if variant == nil {
		return map[string]string{}
	}
	if page == nil {
		return map[string]string{}
	}
	if v, ok := variant[page.Page.Key]; ok {
		out := map[string]string{}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out[k] = v[k]
		}
		return out
	}
	return map[string]string{}
}
