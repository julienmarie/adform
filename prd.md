adform — Complete Specification (Meta Ads as Code, Terraform-style, GitOps, Go)

One-liner

adform is a Go CLI that manages a Meta Ads account declaratively from a Git repo using a Terraform-style plan/apply workflow, with gitignored SQLite state, asset library management, Insights stats, and a repo-native LLM agent workflow (Codex/Claude) that proposes PRs.

⸻

1) Goals

Must-have
	•	Meta-only (Marketing API + Insights).
	•	Declarative configs in YAML, stored in Git.
	•	Terraform-like workflow:
	•	adform validate
	•	adform plan
	•	adform apply
	•	State stored in gitignored SQLite (.adform/state.db).
	•	Safe-by-default:
	•	No deletes in v1
	•	Budget change caps
	•	Replace semantics for “unsafe/immutable-ish” changes
	•	Orphan handling = pause
	•	Asset library management:
	•	upload images/videos
	•	deduplicate by sha256 (local) where possible
	•	map to Meta image_hash / video_id
	•	Performance stats via Insights (adform stats).
	•	Import/bootstrap:
	•	adform import creates the repo tree + YAML from the running account (best-effort).

Explicit non-goals (v1)
	•	Google Ads
	•	Continuous reconciliation daemon (no Argo-style controller)
	•	Full coverage of every Meta feature (v1 focuses on a practical subset)
	•	Auto-applying by LLM (agent proposes PRs; CI/human applies)

⸻

2) Key Concepts & Model

2.1 Source of truth
	•	Git YAML = desired state
	•	Meta account = actual state
	•	SQLite = mapping + memory (logical keys ↔ Meta IDs + last applied hash + drift)

2.2 Identifiers (keys)

Every resource must have a stable key: in YAML.
All references use *_key.

Keys must:
	•	be stable across file moves
	•	not rely solely on paths
	•	be deterministic on import (slug + short hash of Meta ID)

2.3 Tree layout vs graph

Disk layout is a human-friendly tree reflecting Meta containment where it’s real:
	•	Campaign → Ad Sets → Ads
But graph-shared objects live at account-global scope:
	•	Creatives
	•	Assets
	•	Audiences

⸻

3) Repo Layout (Canonical)

meta/
  <account_name>/
    account.yml

    assets/
      images/
      videos/
    assets.yml                # optional manifest (recommended)

    audiences/
      <audience_key>.yml      # v1: references to meta_id

    creatives/
      <creative_key>.yml      # can be declarative or reference-only after import

    campaigns/
      <campaign_key>/
        campaign.yml
        adsets/
          <adset_key>/
            adset.yml
            ads/
              <ad_key>.yml
              <ad_key>.yml
reports/                      # gitignored (stats/import/apply reports)
.adform/                       # gitignored
  state.db
  cache/
AGENTS.md
.gitignore

.gitignore must include:

.adform/
reports/


⸻

4) Configuration Spec (YAML)

4.1 meta/<account_name>/account.yml

account_name: btd_main

meta:
  ad_account_id: "act_1234567890"
  currency: "PHP"            # ISO 4217
  timezone: "Asia/Manila"    # IANA
  page_id: "123..."
  instagram_actor_id: "456..."
  pixel_key_default: "btd-pixel"

budgets:
  unit: major                # major | minor
  # major => 2500 means PHP 2,500.00
  # minor => 250000 means centavos

policies:
  no_delete: true
  allow_activate: false      # apply refuses ACTIVE unless overridden
  orphan:
    on_missing_in_config: pause  # pause | ignore (v1 supports pause)
  budget:
    max_increase_ratio: 0.20
    max_decrease_ratio: 0.50
    max_daily_budget_major: 20000

naming:
  # used by import and optional validation
  campaign_prefix: "BTD |"
  adset_prefix: "BTD |"

Token handling

Token is never in YAML. Read from env:
	•	META_ACCESS_TOKEN

⸻

4.2 Assets

meta/<account_name>/assets.yml (recommended)

assets:
  - key: foiegras_hero_1
    type: image
    file: "meta/btd_main/assets/images/foiegras_hero_1.jpg"
    sha256: null
    meta:
      image_hash: null
      origin: local   # local | imported

  - key: foiegras_video_1
    type: video
    file: "meta/btd_main/assets/videos/foiegras_video_1.mp4"
    sha256: null
    meta:
      video_id: null
      origin: local

Rules:
	•	For local assets, adform assets upload computes sha256 and fills sha256 + Meta ID in state/cache.
	•	For imported assets, file may be null; sha256 null; Meta hash/id known.

⸻

4.3 Audiences (v1: reference-only)

meta/<account_name>/audiences/<audience_key>.yml

key: purchasers_30d
type: custom
meta_id: "2385..."


⸻

4.4 Campaign

meta/<account_name>/campaigns/<campaign_key>/campaign.yml

key: btd-rt-purchasers-ph__9012
name: "BTD | RT | Purchasers | PH"
objective: SALES
status: PAUSED
special_ad_categories: []


⸻

4.5 Ad Set

meta/<account_name>/campaigns/<campaign_key>/adsets/<adset_key>/adset.yml

key: btd-rt-30d-purchasers__a1b2
campaign_key: btd-rt-purchasers-ph__9012

name: "BTD | RT | 30D Purchasers"
status: PAUSED

daily_budget: 2500            # interpreted per budgets.unit + currency
bid_strategy: LOWEST_COST_WITHOUT_CAP
optimization_goal: OFFSITE_CONVERSIONS
billing_event: IMPRESSIONS

promoted_object:
  pixel_key: btd-pixel
  event_type: PURCHASE

targeting:
  geo:
    countries: ["PH"]
  custom_audiences: ["purchasers_30d"]
  placements: advantage_plus

schedule:
  start_time: "2026-02-20T00:00:00+08:00"


⸻

4.6 Creative (v1: link ad, image-based)

meta/<account_name>/creatives/<creative_key>.yml

key: btd-cre-foiegras-1__9f3a
name: "Foie Gras - Static 1"
type: link_ad

page_id_ref: default
instagram_actor_id_ref: default

link:
  url: "https://thebowtieduck.com/collections/foie-gras"
  message: "Foie gras that doesn’t taste like regret."
  headline: "French Foie Gras"
  description: "Delivered cold, on time."
  image_asset_key: foiegras_hero_1

Creative reference-only form (used by import when full reconstruction is hard)

key: imported_creative__ab12
type: reference
meta_id: "2385..."
name: "Imported Creative"


⸻

4.7 Ad

meta/<account_name>/campaigns/<campaign_key>/adsets/<adset_key>/ads/<ad_key>.yml

key: btd-ad-foiegras-rt-1__c3d4
adset_key: btd-rt-30d-purchasers__a1b2
creative_key: btd-cre-foiegras-1__9f3a

name: "Foie Gras RT 1"
status: PAUSED

tracking:
  utm:
    source: facebook
    medium: paid_social
    campaign: "{{campaign.key}}"
    content: "{{creative.key}}"


⸻

5) State (SQLite, gitignored)

Path: .adform/state.db

5.1 Tables

resources
	•	account_name TEXT
	•	kind TEXT
	•	campaign|adset|ad|creative|asset_image|asset_video|audience
	•	logical_key TEXT
	•	meta_id TEXT
	•	last_applied_hash TEXT
	•	last_seen_remote_hash TEXT (optional)
	•	created_at TEXT (RFC3339)
	•	updated_at TEXT (RFC3339)

Primary key: (account_name, kind, logical_key)

locks
	•	account_name TEXT PRIMARY KEY
	•	locked_at TEXT
	•	locked_by TEXT

apply_log (recommended)
	•	id INTEGER PK
	•	account_name TEXT
	•	applied_at TEXT
	•	actor TEXT
	•	plan_json TEXT
	•	result_json TEXT

snapshots (optional but useful for agent)
	•	id INTEGER PK
	•	account_name TEXT
	•	created_at TEXT
	•	stats_json TEXT

⸻

6) CLI Spec

All commands accept:
	•	--account <account_name> (maps to meta/<account_name>)
	•	--root <path> default .
	•	--state <path> default .adform/state.db
	•	--json for machine output
	•	--verbose

6.1 adform init

Bootstrap repo / account tree + AGENTS.md.

Usage:
	•	adform init
	•	adform init --account btd_main
	•	adform init --account btd_main --sample
	•	adform init --account btd_main --ci
	•	adform init --account btd_main --force

Creates (idempotent):
	•	.gitignore entries for .adform/ and reports/
	•	AGENTS.md if missing
	•	meta/<account>/... tree
	•	sample YAMLs if --sample
	•	GitHub workflows + PR template if --ci

⸻

6.2 adform validate

Validates:
	•	schema
	•	referential integrity (keys exist)
	•	currency/budget rules
	•	policy constraints (e.g. ACTIVE not allowed)

Exit codes:
	•	0 ok
	•	1 validation errors

⸻

6.3 adform plan

Computes deterministic plan by:
	•	loading desired tree
	•	rendering references and templates
	•	refreshing remote objects for managed resources
	•	diffing desired vs remote
	•	producing ordered operations

Operations:
	•	create
	•	update
	•	replace (create new + pause old)
	•	pause-orphan
	•	noop
	•	drift-only (remote changed but desired unchanged)

Flags:
	•	--only kind=campaign|adset|ad|creative|asset|audience
	•	--refresh default true
	•	--include-drift default true
	•	--out plan.json optional plan artifact

⸻

6.4 adform apply

Applies a plan with lock + guardrails.

Flags:
	•	--planfile plan.json (optional; otherwise recompute plan)
	•	--max-ops <n>
	•	--activate (allow setting ACTIVE if policies allow)
	•	--budget-cap-delta 0.2 override
	•	--dry-run

Behavior:
	•	acquire lock in SQLite
	•	execute operations in dependency order:
	1.	assets (uploads if needed)
	2.	campaigns
	3.	creatives
	4.	adsets
	5.	ads
	6.	orphan pauses
	•	update state per resource after successful operation
	•	write report to reports/apply-YYYYMMDD-HHMM.md (+ json)

Exit codes:
	•	0 success
	•	2 blocked by policy
	•	1 error

⸻

6.5 adform stats

Fetch Insights and output table/JSON.

Flags:
	•	--level campaign|adset|ad (default campaign)
	•	--last 7d|30d|... (default 7d)
	•	--compare last_7d prev_7d
	•	--breakdown age,gender,platform_position
	•	--export reports/<name>.json
	•	--save-snapshot writes to SQLite snapshots

Metrics (v1):
	•	spend, impressions, clicks, ctr, cpc
	•	conversions (configurable event type)
	•	conversion_value if available
	•	derived: CPA, ROAS

⸻

6.6 adform drift

Compares:
	•	last applied hash
	•	remote current fields

Outputs drift list and severity.

⸻

6.7 adform assets

Subcommands:
	•	adform assets upload --type image|video --path <glob>
	•	adform assets list
	•	adform assets verify
	•	adform assets gc --dry-run

Rules:
	•	local dedup by sha256 when file exists
	•	imported assets tracked by meta_id only

⸻

6.8 adform import

Creates/updates the tree from the running account and fills state.

Usage:
	•	adform import --account btd_main
	•	adform import --account btd_main --since 30d
	•	adform import --account btd_main --campaign "BTD | RT |"
	•	adform import --account btd_main --status ACTIVE
	•	adform import --account btd_main --dry-run
	•	adform import --account btd_main --preserve-status
	•	adform import --account btd_main --force
	•	adform import --account btd_main --out meta/btd_main_imported

Modes:
	•	default scaffold-only (minimal stable subset + references)
	•	--full best-effort deeper spec extraction

Key generation:
	•	slug(name) + "__" + short(meta_id)
	•	collisions resolved by extending hash.

Creative handling:
	•	default to type: reference with meta_id
	•	best-effort extraction for simple link ads if feasible

Assets:
	•	imported assets produce assets.yml entries with origin: imported, no file/sha256.

Safety:
	•	import never mutates Meta
	•	default YAML statuses set to PAUSED unless --preserve-status

Outputs:
	•	writes reports/import-YYYYMMDD-HHMM.md summary

⸻

7) Replace vs Update Policy (v1 defaults)

These are adform policies (can be configurable later).

Campaign
	•	update: name, status
	•	replace: objective

Ad Set
	•	update: name, status, daily_budget (within caps), schedule.start_time
	•	replace: targeting, optimization_goal, billing_event, bid_strategy, promoted_object

Replace means:
	•	create new adset with new key (or auto-generated)
	•	pause old adset
	•	optionally clone ads if configured (v2)

Creative
	•	treat as immutable: any change ⇒ create new creative

Ad
	•	update: name, status
	•	replace: creative_key change ⇒ create new ad + pause old

Orphans
	•	if removed from config and previously managed: pause (never delete)

⸻

8) Guardrails
	•	policies.no_delete=true enforced (v1 hardcoded anyway).
	•	allow_activate=false by default:
	•	apply refuses to set ACTIVE unless --activate and policy allows.
	•	Budget caps:
	•	per-apply max increase/decrease ratios
	•	optional absolute daily cap in major units
	•	Max ops per apply (default 200)
	•	All operations logged to apply_log and report files.

⸻

9) LLM Agent Workflow (Repo-native)

Agent is Codex/Claude in the repo. It should:
	•	run adform stats to collect evidence
	•	propose YAML changes as a PR
	•	run adform validate + adform plan
	•	include plan summary and rationale in PR description
	•	never run apply unless explicitly told

⸻

10) AGENTS.md (Required)

adform init must create AGENTS.md at repo root with:
	•	core principles (Git truth, state not in Git, PR-only)
	•	command recipes: validate/plan/stats
	•	“what not to do”
	•	how to handle replace semantics (create new keys, pause old)
	•	credential handling (META_ACCESS_TOKEN env)
	•	expected output format (include plan summary, risks, rollback)

(Use the AGENTS.md content from the previous message, updated to --account and meta/<account> paths.)

⸻

11) CI/CD (Optional via adform init --ci)

PR workflow
	•	checkout
	•	adform validate --account <account>
	•	adform plan --account <account>
	•	upload plan artifact / comment summary

main workflow
	•	checkout
	•	adform apply --account <account>
	•	upload apply report

⸻

12) Implementation Guidance (Go)

Suggested package structure:

cmd/adform/
internal/config/      # tree loader, YAML parsing, schema validation
internal/render/      # refs/templates, canonical JSON, hashing
internal/state/       # sqlite store, locking, apply logs
internal/meta/        # Graph API client, retries, rate-limit handling
internal/plan/        # diff + policy matrix + ordering
internal/apply/       # executor (idempotent), reports
internal/assets/      # sha256, upload, mapping
internal/stats/       # Insights queries + normalization
internal/importer/    # read existing account, write tree + state
internal/report/      # markdown + json output formatting

SQLite driver:
	•	prefer modernc.org/sqlite (pure Go) to avoid CGO friction.

⸻

13) Default Initialization Output (adform init --account X --sample)

Creates a minimal PAUSED example:
	•	1 campaign
	•	1 adset referencing a stub audience
	•	1 creative referencing a sample local image (not uploaded until assets/upload or apply)
	•	1 ad referencing the creative
	•	assets folder with .keep
