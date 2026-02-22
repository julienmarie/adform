# btd_main Meta Strategy (Brand-Led Rebuild)

## 1) Locked Constraints
- Account: `btd_main` (`bowtieduck.com`)
- Spend cap: `PHP 100,000/month` (underspend allowed)
- Attribution/payback: Meta `7-day click`, `7-day` operating horizon
- Stage target now: ROAS `>= 6` (then 8 -> 10 -> 12)
- Margin baseline: `~40%` gross margin (Echire is intentional loss leader)
- Mix target: new/repeat `60/40`
- Geo split: Metro Manila vs Province (Cebu + Davao priority)

## 2) Current Situation (Last 30d: 2026-01-20 to 2026-02-19)
Data source: `adform stats`, `adform posthog sales`, `adform feed`.

- Meta spend: `PHP 156,726.37`
- Meta purchase value: `PHP 341,270.20`
- Meta ROAS: `2.18`
- Feed stock: `529` in stock / `1,077` out of stock
- OOS drag from recent sales-linked SKUs: `PHP 524,674`

Conclusion: we must run strict in-stock + brand/collection control, not broad all-products delivery.

## 3) User-Approved Direction Updates
- Reorganize around specific brand instances and collections.
- Push these clusters:
  - Perrotte jams
  - Aberdeen Angus IGP steaks
  - Mavrommatis Greek yogurts
  - Bordier butters
  - "English Tea Time" collection (scones, crumpets, clotted cream, jams, teas)
  - "Italian Spirit" collection (Laudemio + balsamic + 84m parmigiano + parma + speck)
  - Rodel as dedicated brand page
- Do not push now:
  - Calvisius and Cru caviar
  - Burrata frozen
- Neuvic: backorder today; launch only when feed is live.

## 4) Budget Architecture (PHP 100k/month)

### 4.1 Allocation
- Campaign A: Retargeting + Repeat -> `PHP 35,000`
- Campaign B: Metro Acquisition (Brand/Collection-led) -> `PHP 30,000`
- Campaign C: Province Acquisition (Trust-first + same collections) -> `PHP 20,000`
- Campaign D: Launch/Restock Bursts (Neuvic, Mavrommatis restock, Mons support) -> `PHP 10,000`
- Campaign E: Exploration lane -> `PHP 5,000`

### 4.2 Pacing Rules
- 3-day ROAS < 4.5: reduce next-day spend by 20%
- 3-day ROAS < 3.5: reduce next-day spend by 40%
- 7-day ROAS >= stage target + stable volume: scale +15% max / 48h

## 5) Campaign Structure

### Campaign A: Retargeting + Repeat (35%)
- ATC 1-7d, VC 1-7d, VC 8-14d, Purchasers 30-180d
- Dynamic catalog with strict whitelist only

### Campaign B: Metro Acquisition (30%)
Separate ad sets by cluster:
- Bordier Butters
- Perrotte Jams
- Aberdeen Angus IGP
- English Tea Time Collection
- Italian Spirit Collection
- Mavrommatis Yogurts

### Campaign C: Province Acquisition (20%)
- Cebu trust-led ad set
- Davao trust-led ad set
- Light rest-of-province test
- Each ad set runs trust creatives + top 2 collections only

### Campaign D: Launch/Restock Bursts (10%)
- Mavrommatis 10% + 0% restock (expected Monday)
- Neuvic burst only when feed confirms SKU availability

### Campaign E: Exploration (5%)
- One hypothesis at a time (page, copy, SKU angle)

## 6) What We Push Now (In Ads)
Master SKU list: `product_selection.md`.

Priority order:
1. Bordier butters
2. English Tea Time collection
3. Perrotte jams
4. Aberdeen Angus IGP cuts
5. Mavrommatis (Honey + Pecan now; 10%/0% once live)
6. Italian Spirit collection (only in-stock products from that set)
7. Rodel brand set

Hard exclusions for now:
- Calvisius + Cru caviar
- Burrata frozen

## 7) Creative System (Static-first)

### 7.1 Province trust creatives (always on)
Reuse now:
- `product-name-2025-02-26-6062d24cf8d887831f1249f286ac2fb1__079927`
- `product-name-2025-03-13-1ee8bd0023a0ce789ef50ba12f7ff1f3__927437`
- `product-name-2025-03-13-36008a8231405a241faecc55814bfc15__158512`
- `make-your-life-taste-better-2025-03-12-83e830b6a5f58638b742f8d5cb642abe__857982`

### 7.2 Brand/collection creative cadence
Weekly minimum:
- 2 statics: Bordier
- 2 statics: English Tea Time
- 2 statics: Aberdeen / Perrotte rotation
- 2 statics: Trust/Delivery proof for Province

## 8) Guardrails
- Kill ad: spend > 1.2x target CPA and 0 purchases, or ROAS < 1.0 after meaningful spend
- Kill ad set: ROAS < 1.5 over 7 days
- Echire cap: max 12% of account spend
- OOS auto-block daily

## 9) Product Page Roadmap
Detailed product-team recommendations: `product_team_pages.md`.

## 10) Operational Notes
- `allow_activate: false` in `btd_main/meta/account.yml`; keep this until final review.
- Neuvic currently not detected in feed snapshot; do not force launch until feed confirms SKUs.
