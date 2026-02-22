---
name: map-creator
description: Create custom map visuals and map-generation code from user requirements, reference images, and place-level geography inputs. Use when Codex must design or generate static city/region maps, reproduce a visual map style, assemble OpenStreetMap-based layers, troubleshoot map rendering errors, or deliver runnable Python mapping workflows with geospatial libraries.
---

# Map Creator

Build reproducible map outputs instead of one-off screenshots. Convert a visual target and place brief into runnable code and iteratively refine until the map matches the requested look and labeling.

## Workflow

### Step 1: Capture the map brief

Collect:

- place scope: city, metro area, region, country
- output type: static image, notebook output, script, or markdown + code
- visual direction: minimal, monochrome, vintage, transit-like, terrain-like
- required layers: roads, water, parks, buildings, admin boundaries, labels
- emphasis rules: what should be most visible and what should be de-emphasized
- delivery constraints: dimensions, DPI, file type, deadline, licensing constraints

If the user gives a reference image, ask for explicit fidelity target:

- "near replica"
- "style inspired"
- "same hierarchy, different palette"

### Step 2: Choose generation path

Use this decision rule:

- `OSM + GeoPandas + Matplotlib (+ Contextily)` for styled static maps and publication outputs.
- `Folium/Leaflet` for interactive web maps.

Default to the article pattern for static style replication:

- open data from OpenStreetMap
- fetch geometry layers by tags
- style layers with explicit z-order and line widths
- add labels selectively for readability

### Step 3: Produce a first runnable draft

When generating code, include:

- environment setup (`pip install ...`)
- data pull section
- layer normalization section (CRS, geometry cleanup)
- style config dictionary
- plot section with explicit axis limits and layer order
- export section (`.png` or `.svg`)

Prefer parameters over hardcoded constants:

- `place_name`
- `bbox_padding`
- `figsize`
- palette tokens
- line width scales

### Step 4: Run an iterative refinement loop

Use this loop:

1. Execute map generation code.
2. Capture exact error or visual miss.
3. Apply the smallest targeted fix.
4. Re-run and compare against requested hierarchy and style.
5. Repeat until acceptance criteria are met.

Do not replace large sections blindly after an error. Preserve working sections.

### Step 5: Enforce map quality checks

Verify before final output:

- critical roads are visible at target export size
- water and green spaces are distinguishable
- labels do not collide on priority features
- map bounds are correct and not blank
- attribution and data-source notes are present where required

## Troubleshooting defaults

For article-derived failure patterns and fixes, read:

- `references/map-making-playbook.md`

Apply those fixes first when the same symptoms appear.

## Output defaults

Unless the user requests otherwise:

1. Provide a brief map plan.
2. Provide complete runnable code.
3. Provide a compact "what changed and why" section for each iteration.
4. Provide final execution command and expected output file path.
