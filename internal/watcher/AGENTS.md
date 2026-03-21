# internal/watcher

Parent: `internal/AGENTS.md`

## OVERVIEW
Config and auth hot-reload pipeline. Combines file watching, config reload, auth synthesis, redacted diffs, and batched update dispatch.

## WHERE TO LOOK
- `watcher.go`: watcher lifecycle and snapshot state.
- `config_reload.go`: debounce and reload flow.
- `dispatcher.go`: queued auth update delivery.
- `synthesizer/`: config-backed and file-backed auth synthesis.
- `diff/`: redacted change reporting and hashing.

## LOCAL CONVENTIONS
- Config changes and file-backed auth changes flow through the same update queue.
- Keep diffs redacted. Structural summaries are okay, secrets are not.
- Config shape changes usually require paired updates in `synthesizer/config.go` and `diff/` helpers.
- Debounce behavior exists for stability. Do not remove it casually.
- Extend `watcher_test.go` instead of starting parallel watcher suites; the end-to-end reload matrix is intentionally centralized there.
- Child rules in `diff/AGENTS.md` and `synthesizer/AGENTS.md` override this file inside those subtrees.

## CHECKS
```bash
go test ./internal/watcher/...
```
