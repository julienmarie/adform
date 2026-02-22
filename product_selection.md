# Product Selection (Brand-Led Push List v3)

Date context:
- Sales window analyzed: `2026-01-20` to `2026-02-19`
- Feed snapshot: `2026-02-19`
- Live website verification for Neuvic SKUs: `2026-02-21`

This file reflects latest decisions:
- Push brand/collection clusters.
- Keep Calvisius/Cru caviar on hold.
- Keep Burrata frozen on hold.
- Move Caviar de Neuvic to launch lane with landing-page-first sequencing.

## 1) Push Now: Bordier Butters
Brand signal: 13 SKUs, 10 in stock, 30d value `PHP 93,526`.

Run now (in stock):
- `339997` Bordier Wild Garlic and Kampot Pepper Butter
- `265990` Bordier Doux (Unsalted) Butter
- `33119` Bordier Sale 4% (Demi Sel) Butter
- `191524` Bordier Sel Fume (Smoked Salt) Butter
- `191527` Bordier Yuzu Butter
- `191399` Bordier Algues (Seaweed) Butter
- `266000` Bordier Espelette Pepper Butter
- `266006` Bordier Madagascar Vanilla Butter
- `340003` Bordier Roscoff Onion Butter
- `424898` Bordier Melanosporum Black Truffle Butter

Hold (OOS):
- `265994` Bordier Creme Fraiche
- `266014` Bordier Olive Oil Lemon Butter
- `294375` Bordier Sarrazin (Buckwheat) Butter

## 2) Push Now: Perrotte Jams
Brand signal: 7 SKUs, 4 in stock, 30d value `PHP 19,370`.

Run now (in stock):
- `477386` Maison Perrotte Gariguette Strawberry Jam
- `477442` Maison Perrotte William Pear Marmalade with Grand Cru Raiatea Vanilla
- `477452` Maison Perrotte Roussillon Red Apricot Jam
- `477476` Maison Perrotte Pineapple with Larusee Verte Absinthe

Hold (OOS):
- `477264` Provence Fig
- `477456` Orange Maltaise
- `477472` Tulameen Raspberry

## 3) Push Now: Aberdeen Angus IGP Steaks
Brand signal: 4 SKUs, 3 in stock.

Run now (in stock):
- `493407` Scottish Aberdeen Angus IGP Hanger Steak
- `243064` Secret Ultra Tender Cut of Aberdeen Angus IGP (Flat Iron)
- `493389` Bavette Flank Steak (Scottish Aberdeen Angus IGP)

Hold (OOS):
- `242476` 30 Days Dry Aged Aberdeen Scottish IGP Bone-in Ribeye

## 4) Push Now: Mavrommatis Greek Yogurts
Brand signal: 2 SKUs in current feed; 30d value `PHP 49,250`.

Run now:
- `493243` Mavrommatis Greek Yogurt with Honey and Pecan Nuts (in stock)

Backorder/watch:
- `493248` Mavrommatis Plain Greek Yogurt 10% Fat (OOS today; expected Monday)
- Mavrommatis 0% low fat: user indicates incoming Monday; verify feed presence before activation

## 5) Push Now: English Tea Time Collection
Collection signal: 13 selected SKUs, all in stock, 30d value `PHP 81,941`, purchases `120`.

Core set:
- `381896` Devon Cream Clotted Cream
- `443241` Madge Classic Crumpets
- `443277` Madge Wholemeal Crumpets
- `443536` Madge Classic Scones
- `443544` Madge Cranberry and White Chocolate Scones
- `443559` Madge Date Scones
- `443583` Madge Raisin Scones
- `457919` Bow Tie Duck x Madge Echire Butter Scones
- `477386` Perrotte Gariguette Strawberry Jam
- `477442` Perrotte Pear Marmalade
- `477452` Perrotte Apricot Jam
- `477476` Perrotte Pineapple Jam
- `309514` Mariage Freres Sencha Matcha Emeraude Sachets

Note:
- Tea availability is currently thin (only one obvious in-stock tea in feed sample), so page should prioritize scones/crumpets/clotted cream/jams and treat tea as optional module.

## 6) Push Now: Italian Spirit Collection (In-Stock Subset)
Collection signal (full intended set): 17 SKUs, 8 currently in stock, 30d value `PHP 144,513`.

Run now (in stock):
- `476116` Laudemio Gathering Gift Box Set
- `467880` Tirrena + Laudemio Box Set
- `393838` Ultra Rare 84 Months Parmigiano Reggiano DOP
- `27405` Parmigiano Reggiano DOP Stravecchio 36 months
- `180775` Special Selection Parma Ham
- `480645` San Daniele Parma Ham (18 months)
- `62326` Special Selection Speck IGP
- `219777` Stefania Calugi Truffle Flavored Dressing with Balsamic Vinegar

Hold (OOS today):
- Laudemio oils (`161`, `463801`)
- Giusti and other balsamic variants (`276054`, `276062`, `343463`, `385665`, `461237`, `469499`, `469504`)

## 7) Push Now: Rodel Brand Set
Brand signal: 4 SKUs, 3 in stock, 30d value `PHP 18,170`.

Run now:
- `257001` Rodel Vintage Sardines in Olive Oil (Millesime)
- `256943` Rodel Chica Pica Sardines
- `256949` Rodel Sardines in Olive Oil and Lemon

Hold (OOS):
- `256966` Rodel Boneless Vintage Sardines in Olive Oil

## 8) Launch Lane: Caviar de Neuvic (Landing Page First, Stock-Gated Paid)
Verification source: live product pages on `bowtieduck.com` with IDs present in page markup.

Landing page build scope (all live URLs):
- `501004` Caviar de Neuvic Baeri Signature
- `501009` Caviar de Neuvic Baeri Reserve
- `501015` Caviar de Neuvic Oscietre Signature
- `501037` Caviar de Neuvic Oscietre Reserve
- `501041` Caviar de Neuvic Trout Roe
- `501047` Caviar de Neuvic Organic Trout Roe
- `501079` Caviar de Neuvic Smoked Trout Roe
- `501088` Caviar de Neuvic KETA Wild Salmon Roe
- `501092` Caviar de Neuvic Organic Brook Salmon Roe
- `501098` Caviar de Neuvic Smoked Pike Roe
- `501103` Caviar de Neuvic Whitefish Roe
- `501107` Caviar de Neuvic Blinis

Paid activation state (today):
- Eligible for paid now (live): `501098`
- Backorder/notify-first merchandising: `501004`, `501009`, `501015`, `501037`, `501041`, `501047`, `501079`, `501088`, `501092`, `501103`, `501107`

Variant notes (for LP and ads deep-link QA):
- Baeri Signature `50g`: `501006`
- Baeri Reserve `50g`: `501011`
- Oscietre Signature `50g`: `501017`
- Oscietre Reserve `50g`: `501039`

## 9) Explicit Holds (Do Not Push Now)
- All Calvisius and Cru caviar SKUs (temporary hold by decision)
- `25853` Burrata di Puglia Frozen

## 10) Daily Ops Rule
Before any budget/status change:
1. Re-check in-stock state.
2. Keep only in-stock SKUs active.
3. Update ad/collection composition if restocks or backorders change.
4. For Neuvic, keep backorder SKUs in landing pages but suppress paid spend unless stock is live.
