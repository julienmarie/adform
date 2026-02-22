# AGENTS.md

## Core Principles
- Git is the source of truth for desired state under `meta/<account>/`.
- SQLite state is local only and must stay out of Git (`.adform/state.db`).
- All production changes should flow through pull requests.

## Common Commands
- `adform validate --account <account>`
- `adform plan --account <account> --out reports/plan.json`
- `adform stats --account <account> --level campaign --last 7d`

## What Not To Do
- Do not commit '.adform/' or 'reports/' artifacts.
- Do not manually edit SQLite state unless recovering from a known issue.
- Do not run `adform apply` without reviewing plan output.

## Replace Semantics
- For immutable/unsafe changes, create new keys in YAML and pause old resources.
- Avoid in-place mutation for creatives.

## Credentials
- Export `META_ACCESS_TOKEN` in the environment.
- Never store tokens in YAML or commit them.

## PR Output Format
- Include validation result.
- Include plan summary (create/update/replace/pause/noop).
- Include risks and rollback notes.
