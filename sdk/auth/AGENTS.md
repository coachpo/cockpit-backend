# sdk/auth

Parent: `sdk/AGENTS.md`

## OVERVIEW
SDK-side auth contracts, Codex login helpers, and token-store plumbing.

## WHERE TO LOOK
- `interfaces.go`: `Authenticator`, `LoginOptions`, refresh contract.
- `manager.go`: auth orchestration.
- `codex.go`, `codex_device.go`: provider entrypoints.
- `refresh_registry.go`: refresh integration.

## LOCAL CONVENTIONS
- Keep provider login helpers aligned with the `Authenticator` contract: `Provider`, `Login`, `RefreshLead`.
- Keep persistence store injection explicit at call sites instead of hiding it behind package-global registries.
- Public auth API changes often require follow-up work in `sdk/cliproxy/auth` and provider-specific `internal/auth` packages.

## CHECKS
```bash
go test ./sdk/auth/...
```
