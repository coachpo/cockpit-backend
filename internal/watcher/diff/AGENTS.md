# internal/watcher/diff

Parent: `internal/watcher/AGENTS.md`

## OVERVIEW
Redacted config and auth change reporting. This folder owns human-readable watcher diffs for retained config and auth surfaces.

## WHERE TO LOOK
- `config_diff.go`, `auth_diff.go`: redacted summary builders.
- Keep this folder focused on retained auth/config diff behavior; removed compat or alias config sections should not reappear here.

## LOCAL CONVENTIONS
- Never emit raw secrets. Summaries should use counts, hashes, or created and updated wording.
- Keep message strings stable when possible; package tests assert exact human-readable output.
- Config-shape additions usually need new diff coverage to match synthesizer changes.
- Proxy URLs and auth metadata must stay masked before they reach logs or responses.

## CHECKS
```bash
go test ./internal/watcher/diff/...
```
