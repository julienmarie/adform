package cli

import "fmt"

func defaultAccountYAML(account string) string {
	return fmt.Sprintf(`account_name: %s

meta:
  ad_account_id: "act_1234567890"
  currency: "USD"
  timezone: "America/New_York"
  page_id: ""
  instagram_actor_id: ""
  pixel_key_default: "default-pixel"
  product_feed_url: ""

budgets:
  unit: major

policies:
  no_delete: true
  allow_activate: false
  orphan:
    on_missing_in_config: pause
  budget:
    max_increase_ratio: 0.20
    max_decrease_ratio: 0.50
    max_daily_budget_major: 20000

naming:
  campaign_prefix: ""
  adset_prefix: ""
`, account)
}

func sampleAssetsYAML(account string) string {
	return fmt.Sprintf(`assets:
  - key: sample_image
    type: image
    file: "%s/meta/assets/images/sample.jpg"
    sha256: null
    meta:
      image_hash: null
      origin: local
`, account)
}

func defaultAccountsYAML(account string) string {
	return fmt.Sprintf(`property: %s

platforms:
  meta:
    config_dir: "meta"
    ad_account_id: "act_1234567890"
    meta_api_key: "get_env(META_API_KEY)"
  photoroom:
    api_key: "get_env(PHOTOROOM_API_KEY)"
  posthog:
    host: "https://app.posthog.com"
    project_id: ""
    api_key: "get_env(POSTHOG_API_KEY)"
    events:
      order_completed: "Order Completed"
      product_added: "Product Added"
      product_viewed: "Product Viewed"
    queries:
      product_sales: ""
      product_added: ""
      product_viewed: ""
  tiktok:
    advertiser_id: ""
  google_ads:
    customer_id: ""
  google_analytics:
    property_id: ""
  google_search_console:
    site_url: ""
    credentials_file: ""
    credentials_json: "get_env(GSC_CREDENTIALS_JSON)"
`, account)
}

func sampleAudienceYAML() string {
	return `key: stub_audience
type: custom
meta_id: "23850000000000000"
`
}

func sampleCatalogYAML() string {
	return `key: stub_catalog
type: catalog
meta_id: "123456789012345"
name: "Stub Catalog"
`
}

func sampleCampaignYAML() string {
	return `key: sample_campaign
name: "Sample Campaign"
objective: SALES
status: PAUSED
special_ad_categories: []
`
}

func sampleAdsetYAML() string {
	return `key: sample_adset
campaign_key: sample_campaign

name: "Sample Adset"
status: PAUSED

daily_budget: 100
bid_strategy: LOWEST_COST_WITHOUT_CAP
optimization_goal: OFFSITE_CONVERSIONS
billing_event: IMPRESSIONS

promoted_object:
  pixel_key: default-pixel
  event_type: PURCHASE

targeting:
  geo:
    countries: ["US"]
  custom_audiences: ["stub_audience"]
  placements: advantage_plus

schedule:
  start_time: "2026-01-01T00:00:00-05:00"
`
}

func sampleCreativeYAML() string {
	return `key: sample_creative
name: "Sample Creative"
type: link_ad

page_id_ref: default
instagram_actor_id_ref: default

link:
  url: "https://example.com"
  message: "Sample message"
  headline: "Sample headline"
  description: "Sample description"
  image_asset_key: sample_image
`
}

func sampleAdYAML() string {
	return `key: sample_ad
adset_key: sample_adset
creative_key: sample_creative

name: "Sample Ad"
status: PAUSED

tracking:
  utm:
    source: facebook
    medium: paid_social
    campaign: "{{campaign.key}}"
    content: "{{creative.key}}"
`
}

func defaultAgentsMD() string {
	return `# AGENTS.md

## Core Principles
- Git is the source of truth for desired state under ` + "`<account>/meta/`" + `.
- SQLite state is local only and must stay out of Git (` + "`.adform/state.db`" + `).
- All production changes should flow through pull requests.

## Common Commands
- ` + "`adform validate --account <account>`" + `
- ` + "`adform plan --account <account> --out reports/plan.json`" + `
- ` + "`adform stats --account <account> --level campaign --last 7d`" + `

## What Not To Do
- Do not commit '.adform/' or 'reports/' artifacts.
- Do not manually edit SQLite state unless recovering from a known issue.
- Do not run ` + "`adform apply`" + ` without reviewing plan output.

## Replace Semantics
- For immutable/unsafe changes, create new keys in YAML and pause old resources.
- Avoid in-place mutation for creatives.

## Credentials
- Prefer per-account secrets in ` + "`<account>/accounts.yml`" + ` (e.g. ` + "`meta_api_key: get_env(META_API_KEY_BTD_MAIN)`" + `).
- Fallback env vars are ` + "`META_ACCESS_TOKEN`" + ` and ` + "`META_API_KEY`" + `.
- Never store tokens in YAML or commit them.

## PR Output Format
- Include validation result.
- Include plan summary (create/update/replace/pause/noop).
- Include risks and rollback notes.
`
}

func ciPRWorkflow(account string) string {
	return fmt.Sprintf(`name: adform-pr
on:
  pull_request:
    paths:
      - '**/meta/**'
      - 'meta/**'
      - '.github/workflows/adform-pr.yml'

jobs:
  validate-plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go build ./cmd/adform
      - run: ./adform validate --account %s
      - run: ./adform plan --account %s --out reports/plan.json
      - uses: actions/upload-artifact@v4
        with:
          name: adform-plan
          path: reports/plan.json
`, account, account)
}

func ciMainWorkflow(account string) string {
	return fmt.Sprintf(`name: adform-main
on:
  push:
    branches: [main]
    paths:
      - '**/meta/**'
      - 'meta/**'

jobs:
  apply:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go build ./cmd/adform
      - run: ./adform apply --account %s
`, account)
}

func ciPRTemplate() string {
	return `## adform Change Summary

### Why
- 

### Evidence
- [ ] ` + "`adform validate --account <account>`" + ` output included
- [ ] ` + "`adform plan --account <account>`" + ` summary included

### Plan Summary
- create:
- update:
- replace:
- pause-orphan:
- noop:

### Risks
- 

### Rollback
- 
`
}

func defaultLandingSiteYAML() string {
	return `version: 1

runtime:
  bind: "0.0.0.0:8080"
  public_base_url: "https://ads.example.com"
  main_site_base_url: "https://example.com"

scripts:
  urls: []
  inline: []

tracking:
  cookie_domain: ".example.com"
  attribution_cookie: "tbd_attr"
  variant_cookie: "tbd_lp"
  attribution_ttl_days: 30
  variant_ttl_days: 7
  capture_query_params:
    - utm_source
    - utm_medium
    - utm_campaign
    - utm_content
    - utm_term
    - fbclid
  utm_passthrough:
    enabled: true
    allowlist:
      - utm_source
      - utm_medium
      - utm_campaign
      - utm_content
      - utm_term
      - fbclid

posthog:
  enabled: false
  host: "https://app.posthog.com"
  api_key_env: "POSTHOG_API_KEY"
  events:
    impression: "lp_impression"
    cta_click: "lp_cta_click"
    product_click: "lp_product_click"

meta_pixel:
  enabled: false
  pixel_id: ""
  capi_access_token_env: "META_CAPI_TOKEN"

bandit:
  enabled: true
  algorithm: "thompson_beta"
  update_interval_minutes: 30
  min_impressions_per_arm: 200
  control_min_share: 0.15
  storage:
    type: "sqlite"
    sqlite_path: ""
    redis:
      addr: "127.0.0.1:6379"
      password_env: "REDIS_PASSWORD"
      db: 0
      key_prefix: "adform:landing:bandit"
  objective:
    primary: "cta_click"
    secondary: null

defaults:
  locale: "en-PH"
  currency: "PHP"
  trust_items:
    - icon: cold_chain
      title: "Cold-chain delivery"
      body: "Packed for reliability."
`
}

func defaultLandingThemeCSS() string {
	return `:root {
  --bg: #f8f5ef;
  --fg: #1f1a17;
  --accent: #8f4d2e;
  --card: #ffffff;
  --muted: #6b635d;
}

* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: "Helvetica Neue", Helvetica, Arial, sans-serif;
  background: var(--bg);
  color: var(--fg);
}

.lp { display: grid; gap: 28px; padding-bottom: 48px; }
.block { width: min(1100px, 92vw); margin: 0 auto; }
.block-hero {
  min-height: 58vh;
  display: grid;
  place-items: center;
  position: relative;
  border-radius: 20px;
  overflow: hidden;
  background: #1a1512 center/cover no-repeat;
  color: #fff;
}
.hero-overlay { position: absolute; inset: 0; background: #000; }
.hero-content { position: relative; z-index: 1; max-width: 760px; padding: 40px 28px; }
.hero-content h1 { margin: 0 0 12px; font-size: clamp(2rem, 4vw, 3.4rem); line-height: 1.05; }
.cta { display: inline-block; margin-right: 12px; margin-top: 12px; padding: 10px 16px; border-radius: 999px; background: var(--accent); color: #fff; text-decoration: none; }

.block-media-split { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; align-items: center; }
.block-media-split img { width: 100%; border-radius: 16px; }
.block-media-split.side-right .media { order: 2; }
.block-media-split.side-right .content { order: 1; }

.product-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; }
.product-card { background: var(--card); border-radius: 14px; padding: 12px; }
.product-card img { width: 100%; border-radius: 10px; }
.product-card a { color: inherit; text-decoration: none; }

.columns-grid, .trust-items, .pairings-grid { display: grid; gap: 14px; }
.columns-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.trust-items, .pairings-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.columns-grid article, .trust-items article, .pairings-grid article {
  background: var(--card);
  border-radius: 14px;
  padding: 14px;
}

.block-faq details { background: var(--card); border-radius: 10px; padding: 12px 14px; margin-bottom: 10px; }

.spacer-sm { height: 16px; }
.spacer-md { height: 28px; }
.spacer-lg { height: 44px; }
.spacer-xl { height: 64px; }

@media (max-width: 900px) {
  .block-media-split { grid-template-columns: 1fr; }
  .product-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .columns-grid, .trust-items, .pairings-grid { grid-template-columns: 1fr; }
}
`
}

func defaultLandingSamplePageYAML() string {
	return `version: 1
page:
  key: sample
  slug: /sample
  type: brand
  seo:
    title: "Sample Landing"
    description: "Sample ad landing page."

blocks:
  - type: hero
    key: hero
    h1: "Sample Landing Hero"
    subhead: "High-converting landing page blocks"
    body: "Edit <account>/landing/pages/*.yml to customize this page."
    bg_image_asset_key: sample_hero
    overlay:
      opacity: 0.40
    primary_cta:
      label: "Shop now"
      href: "https://example.com"
  - type: spacer
    key: s1
    size: md
  - type: trust_strip
    key: trust
`
}
