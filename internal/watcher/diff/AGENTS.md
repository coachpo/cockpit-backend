# internal/watcher/diff

Parent: `internal/watcher/AGENTS.md`

## OVERVIEW
Redacted config and auth change reporting. This folder owns human-readable watcher diffs plus hash helpers used for safe comparisons.

## WHERE TO LOOK
- `config_diff.go`, `auth_diff.go`: redacted summary builders.
- `model_hash.go`, `models_summary.go`: model and entry hashing helpers.
- `oauth_excluded.go`, `oauth_model_alias.go`, `openai_compat.go`: focused diff helpers for complex config sections.

## LOCAL CONVENTIONS
- Never emit raw secrets. Summaries should use counts, hashes, or created and updated wording.
- Keep message strings stable when possible; package tests assert exact human-readable output.
- Config-shape additions usually need new diff coverage to match synthesizer changes.
- Proxy URLs and auth metadata must stay masked before they reach logs or responses.

## CHECKS
```bash
go test ./internal/watcher/diff/...
```
