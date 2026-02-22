package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func printGeneralHelp(w io.Writer) {
	fmt.Fprintln(w, "adform - Meta Ads as code (deterministic plan/apply)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  adform <command> [flags]")
	fmt.Fprintln(w, "  adform help [command]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Core workflow:")
	fmt.Fprintln(w, "  1) adform validate --account <account>")
	fmt.Fprintln(w, "  2) adform plan --account <account> --refresh=false")
	fmt.Fprintln(w, "  3) adform apply --account <account> --dry-run --refresh=false")
	fmt.Fprintln(w, "  4) adform apply --account <account> --refresh=false")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init      Bootstrap repo/account tree")
	fmt.Fprintln(w, "  validate  Validate YAML graph and cross-resource references")
	fmt.Fprintln(w, "  plan      Compute deterministic desired-vs-state operations")
	fmt.Fprintln(w, "  apply     Execute plan with policy guardrails")
	fmt.Fprintln(w, "  assets    Upload/list/verify/gc assets")
	fmt.Fprintln(w, "  stats     Fetch Meta insights metrics")
	fmt.Fprintln(w, "  feed      Scrape product feed (price/url/stock)")
	fmt.Fprintln(w, "  photoedit Edit images with PhotoRoom prompt API")
	fmt.Fprintln(w, "  k8s       Generate landing Kubernetes manifests")
	fmt.Fprintln(w, "  serve     Run landing pages server (dev hot reload)")
	fmt.Fprintln(w, "  gsc       Query Google Search Console Search Analytics")
	fmt.Fprintln(w, "  posthog   Query PostHog product metrics via HogQL")
	fmt.Fprintln(w, "  log       Fetch ad account change activity log")
	fmt.Fprintln(w, "  drift     Compare local desired vs remote state")
	fmt.Fprintln(w, "  import    Scaffold YAML + state from existing Meta account")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Agent-discovery:")
	fmt.Fprintln(w, "  adform help <command>          # full behavior/flags/examples")
	fmt.Fprintln(w, "  adform validate --rules --json # machine-readable rule catalog")
}

func printCommandHelp(w io.Writer, topic string) bool {
	topic = strings.TrimSpace(strings.ToLower(topic))
	help := map[string]string{
		"init":      helpInit,
		"validate":  helpValidate,
		"plan":      helpPlan,
		"apply":     helpApply,
		"assets":    helpAssets,
		"stats":     helpStats,
		"feed":      helpFeed,
		"photoedit": helpPhotoEdit,
		"k8s":       helpK8s,
		"serve":     helpServe,
		"gsc":       helpGSC,
		"posthog":   helpPostHog,
		"log":       helpLog,
		"drift":     helpDrift,
		"import":    helpImport,
	}
	if topic == "all" {
		keys := make([]string, 0, len(help))
		for k := range help {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, strings.Repeat("-", 72))
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w, help[k])
		}
		return true
	}
	v, ok := help[topic]
	if !ok {
		return false
	}
	fmt.Fprintln(w, v)
	return true
}

const helpInit = `init: bootstrap repository structure

Usage:
  adform init --account <account> [--root .]

Behavior:
  - Creates account-centric tree: <account>/meta with account.yml and starter resources.
  - Creates landing scaffold under <account>/landing (legacy root landing remains supported for backward compatibility).
  - Also creates <account>/accounts.yml for multi-platform workspace metadata.
  - accounts.yml supports per-account Meta auth: platforms.meta.meta_api_key: get_env(...)
  - Ensures .adform/ and reports/ exist and are gitignored.
  - Safe to rerun; only writes missing scaffold files.
`

const helpValidate = `validate: YAML graph validation and policy checks

Usage:
  adform validate --account <account> [--json] [--rules] [--strict-warnings]

Key flags:
  --rules            Print full validation rule catalog (IDs + descriptions).
  --strict-warnings  Exit non-zero if warnings exist.
  --json             Machine-readable output (errors, warnings, optional rules).

Behavior:
  - Loads full bundle from <account>/meta (legacy meta/<account> is still supported).
  - Validates account policy settings, resource schemas, references and key constraints.
  - If <account>/landing/site.yml exists (legacy landing/site.yml supported), also validates landing pages/block schemas and asset references.
  - Produces deterministic rule IDs so agents can reason about failures.

Examples:
  adform validate --account btd_main
  adform validate --account btd_main --rules
  adform validate --account btd_main --rules --json
`

const helpPlan = `plan: compute desired operations from config + state

Usage:
  adform plan --account <account> [--refresh=true|false] [--include-drift=true|false] [--only kinds] [--out file]

Key flags:
  --refresh       Query Meta to refresh managed resources before finalizing plan.
  --include-drift Include drift-only operations when refresh is enabled.
  --only          Comma-separated kind filter (campaign,adset,ad,creative,audience,asset,catalog).
  --out           Write plan JSON report.
  --verbose       Also print noop operations.

Behavior:
  - Default text output prints only actionable ops (create/update/replace/pause-orphan/drift-only).
  - Noops are counted in summary but hidden unless --verbose.
  - JSON output always contains full operation list.

Examples:
  adform plan --account btd_main --refresh=false
  adform plan --account btd_main --only campaign,adset
`

const helpApply = `apply: execute plan operations with safety guardrails

Usage:
  adform apply --account <account> [--planfile file] [--dry-run] [--refresh=true|false] [--max-ops N] [--activate] [--delete]

Key flags:
  --dry-run       Evaluate without changing Meta or state.
  --refresh       Refresh from Meta before apply when planfile is not provided.
  --max-ops       Guardrail on actionable operations only (noop/drift-only excluded).
  --activate      Allow ACTIVE statuses if policies permit.
  --delete        Archive orphan resources (status=ARCHIVED) instead of pausing them.

Policy guardrails:
  - ACTIVE resources blocked when account.policies.allow_activate=false.
  - --delete blocked when account.policies.no_delete=true.

Behavior:
  - Applies by dependency order.
  - Orphans (resources in state not in config) are paused by default, archived with --delete.
  - Writes apply reports to reports/.

Examples:
  adform apply --account btd_main --dry-run --refresh=false
  adform apply --account btd_main --refresh=false
  adform apply --account btd_main --delete --dry-run --refresh=false
`

const helpAssets = `assets: asset lifecycle workflow

Usage:
  adform assets <subcommand> [flags]

Subcommands:
  upload   Upload local image/video files to Meta.
  list     List tracked assets from state/config.
  verify   Verify asset references and metadata integrity.
  gc       Remove orphan asset records (supports dry-run).

Run:
  adform assets --help
  adform assets upload --help
`

const helpStats = `stats: Meta insights metrics

Usage:
  adform stats --account <account> [--level campaign|adset|ad] [--last 7d] [--event purchase] [--breakdown ...]

Key flags:
  --level           reporting level.
  --last            date window (7d, 30d, last_7d, this_month, etc.).
  --event           conversion action type selector (purchase alias supported).
  --breakdown       comma-separated insights breakdowns.
  --export          write JSON report.
  --save-snapshot   store payload in state snapshots table.
  --json            machine-readable output.

Behavior:
  - Fetches insights from act_<ad_account_id>/insights.
  - Computes summary: spend, impressions, clicks, conversions, conversion_value, ctr, cpc, cpa, roas.
`

const helpFeed = `feed: scrape account product feed (catalog source of truth)

Usage:
  adform feed --account <account> [--url <feed-url>] [--format auto|xml|csv] [--limit N]

Key flags:
  --url       Override account.meta.product_feed_url.
  --format    Parser mode. auto detects from content.
  --limit     Limit number of products in output.
  --export    Write full JSON payload to file.
  --json      Print machine-readable JSON payload.

Behavior:
  - Reads URL from account.meta.product_feed_url when --url is omitted.
  - Fetches and parses XML (RSS/Atom Merchant style) and CSV/TSV feeds.
  - Extracts product id/title/url/price/sale_price/availability and inferred in_stock.

Examples:
  adform feed --account btd_main
  adform feed --account btd_main --limit 50
  adform feed --account btd_main --format csv --json
`

const helpPhotoEdit = `photoedit: edit an image with PhotoRoom "describe any change"

Usage:
  adform photoedit --image <path> --prompt "<instruction>" [--out <path>]

Key flags:
  --image              Input image path (required)
  --prompt             Edit instruction (required)
  --out                Output path (default <image>.edited.<format>)
  --format             Export format: png|jpg|jpeg|webp (default png)
  --remove-background  Remove subject background before edit (default false)
  --seed               Optional deterministic seed for edit generation
  --timeout            HTTP timeout (default 2m)
  --endpoint           API endpoint override (default https://image-api.photoroom.com/v2/edit)
  --api-key            Direct API key override
  --account            Optional account to resolve platforms.photoroom.api_key from <account>/accounts.yml
  --json               Print machine-readable output

Auth resolution:
  1) --api-key
  2) <account>/accounts.yml -> platforms.photoroom.api_key (when --account is set)
  3) PHOTOROOM_API_KEY

Behavior:
  - Calls PhotoRoom Image Editing API with:
      describeAnyChange.mode=ai.auto
      describeAnyChange.prompt=<prompt>
      imageFile=<input image>
  - Writes edited image to --out.

Examples:
  adform photoedit --image image.png --prompt "replace plate with marble texture"
  adform photoedit --account btd_main --image btd_main/landing/assets/images/hero.jpg --prompt "make it brighter and cleaner" --format webp
  adform photoedit --image image.png --prompt "add warm studio lighting" --out output/hero-v2.png --json
`

const helpK8s = `k8s: generate landing Kubernetes manifests

Usage:
  adform k8s --account <account> [--out <account>/landing/k8s] [--namespace default] [--image <image>]

Key flags:
  --account     Account/property name (required)
  --root        Repo root (default .)
  --out         Output directory (default <account>/landing/k8s)
  --namespace   Kubernetes namespace
  --image       Container image to deploy
  --force       Overwrite existing files

Generated files:
  deployment.yaml, service.yaml, configmap.yaml, secret.example.yaml, README.md

Behavior:
  - Uses 1 replica by default for SQLite bandit single-writer safety.
  - Does not generate Ingress or PVC manifests.
  - Mounts site.yml via ConfigMap.
  - Uses /tmp for landing state DB inside the container.
  - Expects pages/theme/assets baked in image under /app/<account>/landing.

Examples:
  adform k8s --account btd_main
  adform k8s --account btd_main --namespace marketing --image ghcr.io/acme/adform:2026-02-20
`

const helpServe = `serve: run landing pages HTTP server

Usage:
  adform serve --account <account>

Flag/env precedence:
  1) CLI flags
  2) Environment variables
  3) <account>/landing/site.yml (legacy landing/site.yml supported)

Key flags:
  --account        Account name (fallback: ADFORM_SERVER_ACCOUNT)
  --bind           Bind override (fallback: ADFORM_SERVER_BIND, then site.yml runtime.bind)
  --root           Repo root override (fallback: ADFORM_SERVER_ROOT, default ".")
  --state          Landing state db path override (fallback: ADFORM_SERVER_STATE_PATH)
  --no-hot-reload  Disable dev hot reload
  --log-level      debug|info|warn|error

Environment:
  ADFORM_SERVER_ENV               dev|prod (default dev)
  ADFORM_SERVER_ACCOUNT           default account when --account is omitted
  ADFORM_SERVER_BIND              bind override
  ADFORM_SERVER_ROOT              root override
  ADFORM_SERVER_STATE_PATH        state path override
  ADFORM_SERVER_TRUST_PROXY       true|false
  ADFORM_SERVER_PUBLIC_BASE_URL   override runtime.public_base_url
  ADFORM_SERVER_MAIN_SITE_BASE_URL override runtime.main_site_base_url
  POSTHOG_API_KEY                 required when site.yml posthog.enabled=true
  META_CAPI_TOKEN                 optional (future server-side CAPI support)
  REDIS_PASSWORD                  optional if bandit.storage.type=redis and password_env points to it

Behavior:
  - Dev default: hot reload for <account>/landing/site.yml, theme.css, pages/*.yml.
  - Keeps last known-good config when reload validation fails.
  - Endpoints: /healthz, /assets/*, /theme.css, /r, /{slug}.
  - Default bandit storage: local sqlite (.adform/landing_state_<account>.db).
  - Optional Redis backend via <account>/landing/site.yml bandit.storage.type=redis.

Examples:
  adform serve --account btd_main
  ADFORM_SERVER_ENV=prod ADFORM_SERVER_ACCOUNT=btd_main adform serve
`

const helpGSC = `gsc: Google Search Console Search Analytics

Usage:
  adform gsc --account <account> [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--dimensions query]

Key flags:
  --site-url      Override platforms.google_search_console.site_url.
  --type          Search type: web|image|video|news|discover|googleNews.
  --dimensions    Comma-separated: query,page,country,platform,date,search_appearance.
  --country       Optional country filter (e.g. USA, PHL).
  --platform      Optional platform filter: desktop|mobile|tablet.
  --query         Optional query substring filter.
  --page          Optional page substring filter.
  --limit         Max output rows (0 = all).
  --row-limit     API page size per request (1-25000).
  --export        Write full JSON payload to file.
  --json          Print machine-readable JSON payload.

Behavior:
  - Auth uses Google credentials JSON from:
    platforms.google_search_console.credentials_file or credentials_json in <account>/accounts.yml.
  - credentials_json supports get_env(...), so you can keep secrets in env vars.
  - Query metrics include clicks, impressions, ctr, position.
  - platform dimension/filter maps to GSC device internally.

Examples:
  adform gsc --account btd_main --since 2026-01-01 --until 2026-02-01 --dimensions page
  adform gsc --account btd_main --dimensions query,country,platform --type web
  adform gsc --account btd_main --dimensions query --platform mobile --country PHL --json
`

const helpPostHog = `posthog: product metrics from PostHog Queries API (HogQL)

Usage:
  adform posthog sales --account <account> [--since YYYY-MM-DD] [--until YYYY-MM-DD]

Key flags:
  --project-id            Override platforms.posthog.project_id.
  --host                  Override platforms.posthog.host (default https://app.posthog.com).
  --event-order-completed Override order event name (default "Order Completed").
  --event-product-added   Override add-to-cart event name (default "Product Added").
  --event-product-viewed  Override product viewed event name (default "Product Viewed").
  --sales-sql             Inline custom sales HogQL.
  --added-sql             Inline custom add-to-cart HogQL.
  --viewed-sql            Inline custom product-viewed HogQL.
  --sales-sql-file        Path to custom sales HogQL query.
  --added-sql-file        Path to custom add-to-cart HogQL query.
  --viewed-sql-file       Path to custom product-viewed HogQL query.
  --export                Write JSON payload to file.
  --json                  Print machine-readable output.

Behavior:
  - Auth uses platforms.posthog.api_key from <account>/accounts.yml (supports get_env(...)).
  - Configurable in accounts.yml:
    platforms.posthog.host / project_id / api_key / events.* / queries.*
  - Subcommand aliases: sales (preferred), products (legacy alias).
  - Runs 3 HogQL queries (sales/add-to-cart/viewed) then merges by product fields.
  - Provides product-level totals: qty, purchases, value, add_to_cart_count, product_view_count.

Template vars available in custom queries:
  {{since}}, {{until}}, {{event_order_completed}}, {{event_product_added}}, {{event_product_viewed}}

Examples:
  adform posthog sales --account btd_main --since 2026-01-01 --until 2026-02-01
  adform posthog sales --account btd_main --event-product-added "Added to Cart"
  adform posthog sales --account btd_main --sales-sql-file queries/posthog_sales.sql --json
`

const helpLog = `log: ad account activity stream (change log)

Usage:
  adform log --account <account> [--since <date|rfc3339|unix>] [--until <date|rfc3339|unix>] [--limit 200]

Key flags:
  --since     Lower bound for activity events. Supports YYYY-MM-DD, RFC3339, or unix seconds.
  --until     Upper bound for activity events. Supports YYYY-MM-DD, RFC3339, or unix seconds.
  --limit     Requested page size for Meta API pagination.
  --export    Write full JSON payload to file.
  --json      Print machine-readable JSON payload.

Behavior:
  - Fetches from act_<ad_account_id>/activities.
  - Returns all paginated rows within bounds.
  - Text output prints a table + row count summary.

Examples:
  adform log --account btd_main --since 2026-01-01 --until 2026-02-19
  adform log --account btd_main --since 2026-02-01T00:00:00Z --until 2026-02-19T23:59:59Z --json
`

const helpDrift = `drift: remote drift reporting

Usage:
  adform drift --account <account> [--json]

Behavior:
  - Compares managed local config intent with remote snapshots.
  - Produces drift summary by severity/action class.
`

const helpImport = `import: scaffold YAML and state from existing Meta account

Usage:
  adform import --account <account> [--force] [--from-state] [--campaign filter] [--status ACTIVE|PAUSED]

Key flags:
  --from-state      skip remote sync and scaffold from local SQLite only.
  --force           overwrite existing files.
  --dry-run         plan scaffold without writing files.
  --preserve-status keep remote status values when available.

Behavior:
  - Remote mode syncs campaigns/adsets/ads/creatives/audiences/assets/catalogs into state.
  - Renders canonical YAML graph under <account>/meta (legacy meta/<account> supported).
  - Initializes state hash baseline so subsequent plans only show real config deltas.
`
