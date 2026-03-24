# internal/watcher/synthesizer

Parent: `internal/watcher/AGENTS.md`

## OVERVIEW
Runtime auth synthesis layer. This folder turns config-backed sources into `sdk/cliproxy/auth.Auth` entries for watcher dispatch.

## WHERE TO LOOK
- `config.go`: config-backed auth generation.
- `context.go`: synthesis inputs and clocks.
- `helpers.go`, `interface.go`: shared helpers and contracts.

## LOCAL CONVENTIONS
- Synthesized auth output must be deterministic and idempotent for the same input snapshot.
- `Attributes` fields like `source`, `auth_kind`, base URLs, and proxy URLs are downstream contract, not incidental data.
- New config-backed auth sources usually need both synthesis logic here and matching diff or update coverage.
- Keep generated auth compatible with watcher equality and service-side state preservation.

## CHECKS
```bash
go test ./internal/watcher/synthesizer/...
```
