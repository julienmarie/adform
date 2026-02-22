# Decision Journal

## Purpose
Daily operating log for `btd_main` so spend, stock, and creative decisions are traceable, reversible, and ROAS-linked.

## Locked Operating Context
- Monthly spend cap: `PHP 100,000`
- KPI: Meta `7-day click` ROAS
- Payback window: `7 days`
- Stage target (current): ROAS `>= 6`
- Margin baseline: `~40%` (Echire is allowed loss leader)
- Mix target: new/repeat `60/40`
- Risk posture: balanced
- Review cadence: daily

## Strategy Anchors
- Use hybrid structure: Advantage+/catalog mechanics + manual controls for geo and SKU governance.
- Keep Metro and Province split.
- Keep old campaign structures paused while rebuild runs.
- Never run paid spend on out-of-stock SKUs.
- Launch Neuvic with landing-page-first sequencing; gate paid scaling by live stock state.

## Non-Negotiable Rules
- Do not exceed monthly cap.
- If ROAS weakens, underspend is the default response.
- No major budget change without log entry.
- For immutable/unsafe changes, create new YAML keys and pause old resources.
- Do not run Neuvic scale spend on products showing restock-only status.

## SKU State Definitions
- `WHITELIST`: in-stock SKU approved for active spend.
- `HOLD`: in-stock SKU with weak recent efficiency (can be tested at low spend).
- `BLOCK`: out-of-stock SKU or repeated poor performer.

## Budget Governance
- Monthly cap: `PHP 100,000`
- Weekly soft cap: `PHP 23,000`
- Daily working average: `PHP 3,333`

Pacing rules:
- If trailing 3-day ROAS < `4.5`: reduce next-day spend by `20%`
- If trailing 3-day ROAS < `3.5`: reduce next-day spend by `40%`
- If trailing 7-day ROAS >= stage target with stable volume: scale winners by `+15%` max every 48h

## Kill and Scale Rules
- Kill ad if:
  - spend > `1.2 x target CPA` and `0` purchases
  - or ROAS < `1.0` after meaningful spend
- Kill ad set if:
  - ROAS < `1.5` over 7-day window
- Scale ad set if:
  - ROAS >= `6` and >= `5` purchases in 7 days

## Loss-Leader Control (Echire)
- Keep active as traffic/repeat catalyst.
- Cap Echire-attributed spend at `12%` max of monthly total.
- Require cross-sell adjacency in retargeting (cheese, charcuterie, pantry).

## Rollback Triggers
Rollback if any of the following occur after a change set:
- Account 3-day ROAS drops > `25%`
- Purchase volume drops > `20%` without external explanation
- OOS SKU spend is detected

Rollback action:
- Revert latest change set
- Restore previous budget distribution
- Log root cause and prevention control

## Daily Decision Protocol
1. Check feed stock status and refresh SKU `WHITELIST/HOLD/BLOCK`.
2. Review 1d/3d/7d by campaign, ad set, ad.
3. Check pacing vs cap and stage target.
4. Apply kill/scale rules.
5. Review nationwide trust creative performance (Province lane).
6. Log all changes and expected impact.

## Known Risks to Track
- OOS concentration on high-demand products.
- Province expansion diluting ROAS without trust conversion lift.
- Launch bursts cannibalizing core profitable SKU delivery.
- Loss-leader over-allocation.

## Open Tracking Items
- Category-level margin map beyond global 40%.
- Province-level fulfillment SLA impact on CVR.
- SKU-level contribution margin for top 30 products.
- Neuvic stock and restock ETA reliability between site pages and feed.
- Neuvic LP performance benchmark before ad scale (CVR, ATC rate, notify rate).

## Initial Decisions Logged
Date: `2026-02-20`

- Confirmed monthly cap and underspend policy.
- Confirmed staged ROAS model starting at 6.
- Confirmed 60/40 new/repeat objective.
- Approved full campaign rebuild from scratch.
- Approved Metro vs Province split with Cebu and Davao focus.
- Approved daily operating checks.
- Approved hybrid structure (Advantage+ + manual controls).
- Approved trust-creative-first approach for Province scaling.
- Approved brand-led structure: Perrotte, Aberdeen IGP, Mavrommatis, Bordier.
- Approved collection-led structure: English Tea Time and Italian Spirit.
- Approved hold list: Calvisius/Cru caviar and Burrata frozen.
- Approved Neuvic as launch queue only after feed availability confirmation.

Date: `2026-02-21`

- Confirmed Caviar de Neuvic product pages are live on site (12 URLs discovered in sitemap and page markup).
- Approved policy change: Neuvic is no longer "queue only"; move to active launch lane with landing pages first.
- Approved stock-gated paid rule: run paid spend only on live Neuvic SKUs; keep restock SKUs in notify/backorder modules.
- Approved new Neuvic landing page scope under product team page spec (`/caviar-de-neuvic`).
- Deferred Neuvic ad-set expansion until LP QA + stock-state automation checks are complete.

## Daily Log Template
Use this block every day:

```md
### YYYY-MM-DD
- Stage target:
- Spend today:
- Spend MTD:
- Trailing ROAS (1d / 3d / 7d):
- Top winners (campaign/adset):
- Top losers (campaign/adset):
- SKU whitelist changes (add/remove):
- OOS blocks enforced:
- Budget changes:
- Status changes:
- Creative changes:
- Province trust ad notes:
- Launch lane notes (Mons/Neuvic):
- Risks observed:
- Decision summary:
- Expected impact next 48h:
- Rollback trigger watch:
```

## Experiment Log Template
Use this for controlled tests:

```md
### Test ID:
- Hypothesis:
- Scope (campaign/ad set/SKU set):
- Start date:
- End date:
- Success metric:
- Guardrail metric:
- Result:
- Decision (scale/iterate/stop):
```
