# Map Making Playbook

This playbook captures practical patterns derived from the Medium article on Claude 3.5 Sonnet map making and adapts them for repeatable Codex execution.

## Recommended static stack

- `osmnx` for street and OSM feature retrieval
- `geopandas` for geometry operations and plotting
- `matplotlib` for styling and output control
- `contextily` for optional tile basemaps

Install baseline dependencies:

```bash
pip install osmnx geopandas matplotlib contextily
```

## Prompt pattern for first draft

When the user wants code generation, build the task prompt with:

1. Place and extent definition
2. Visual style target (or reference image notes)
3. Required map layers and label priorities
4. Technical constraints (libraries, output size, file type)
5. Request for full runnable script

Use a structure such as:

```text
Create a complete Python script that generates a styled map for <place>.
Use OpenStreetMap data and <libraries>. Include layers for <layers>.
Match this style direction: <style notes>.
Return runnable code with dependency list and output to <filename>.
```

## Iteration protocol

After each run:

1. Capture exact runtime error or visual mismatch.
2. Patch only the failing section.
3. Keep style constants and working data-loading code stable.
4. Re-run and compare to the acceptance criteria.

This "small-fix loop" is the core article pattern and avoids regressions.

## Common failure patterns and fixes

### Symptom: `list index out of range` while extracting features

Likely cause:
- assuming a layer exists when the query returned empty data

Fix:
- guard access with emptiness checks
- branch gracefully when a feature collection is missing

### Symptom: Map renders with little or no visible data

Likely cause:
- incorrect axis extent or CRS mismatch

Fix:
- project all layers to a consistent CRS before plotting
- set axis limits from combined geometry bounds
- apply basemap only after confirming geometry extents

### Symptom: Key errors for style lookup tables

Likely cause:
- unseen road or feature class missing from style dictionary

Fix:
- add a default fallback style for unknown classes
- normalize class names before lookup

### Symptom: Roads or labels look too faint

Likely cause:
- low linewidth/z-order contrast at export scale

Fix:
- increase linewidth for major classes
- raise z-order for priority layers
- tune text halo/stroke for labels

## Visual hierarchy defaults

Start with:

- background and water first
- park/landuse second
- minor roads next
- major roads above
- labels last (priority-filtered)

Preserve this order unless the user asks for a different hierarchy.

## Acceptance checklist

- Place boundary and extent are correct
- Layer visibility matches requested emphasis
- Label density is readable at final size
- Output file is generated successfully
- Data source attribution is included when needed
