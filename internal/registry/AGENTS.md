# internal/registry

Parent: `internal/AGENTS.md`

## OVERVIEW
Global model catalog and availability registry. Static `models.json` feeds dynamic per-client registration and background refresh.

## WHERE TO LOOK
- `model_registry.go`: live registry, provider-aware `ModelInfo`, hook callbacks, availability cache.
- `model_updater.go`: embedded fallback plus remote refresh loop and change callback.
- `model_definitions.go`: static catalog lookup helpers.
- `models/models.json`: bundled model metadata snapshot.

## LOCAL CONVENTIONS
- Dynamic client registrations win over static catalog lookups when both exist.
- Keep `models/models.json` and updater parsing in sync; CI refreshes the bundled file before builds.
- Grouped provider behavior matters: changed Codex tiers collapse into one logical provider in refresh notifications.
- Hook callbacks must stay non-blocking and panic-safe.

## CHECKS
```bash
go test ./internal/registry/...
```
