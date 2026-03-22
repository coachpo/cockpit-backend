# internal/access

Parent: `internal/AGENTS.md`

## OVERVIEW
Built-in request-auth wiring. This folder bridges config-backed API keys into `sdk/access` without leaking request-auth logic into handlers or executors.

## WHERE TO LOOK
- `reconcile.go`: provider diffing, reuse, change summaries, and manager updates after config changes.
- `config_access/provider.go`: config-backed API key provider, header/query credential extraction, and key normalization.

## LOCAL CONVENTIONS
- Reconcile through `ApplyAccessProviders`; do not register or unregister providers ad hoc from API handlers, executors, or watcher callers.
- `sdk/access` owns the `Manager`, `Provider`, `Result`, and auth-error contracts. This folder only maps config state into that public surface.
- Preserve provider ordering and identifier stability; `sdk/access.Manager.Authenticate` walks providers in order.
- Config API keys are Bearer-only.
- Normalize and dedupe configured keys before registration. Empty config should unregister the config-backed provider cleanly.

## CHECKS
```bash
go test ./internal/access/...
go test ./sdk/access/...
```
