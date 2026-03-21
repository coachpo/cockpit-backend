# sdk/cliproxy/auth

Parent: `sdk/cliproxy/AGENTS.md`

## OVERVIEW
Runtime auth conductor. Owns executor registration, auth selection, cooldowns, refresh scheduling, alias resolution, and persistence policy hooks.

## WHERE TO LOOK
- `conductor.go`: manager core, registration, store wiring.
- `conductor_alias.go`, `conductor_execute.go`, `conductor_selection.go`, `conductor_result.go`, `conductor_refresh.go`, `conductor_http.go`: split conductor responsibilities.
- `scheduler.go`, `selector.go`: selection and rotation strategy.
- `types.go`: auth model, metadata, and config-model pool helpers.
- `oauth_model_alias.go`: OAuth alias resolution and suffix preservation.
- `persist_policy.go`: write suppression hooks.

## LOCAL CONVENTIONS
- Preserve auth state and model cooldown state across updates when possible.
- Selector behavior matters: round-robin and fill-first are both supported and tested.
- Execution metadata keys used by handlers and executors are part of this subsystem contract.
- Model registration and scheduler refresh happen after auth updates; do not break that sequencing casually.
- OAuth model alias and config-model pools are the active alias paths; keep them aligned with `internal/config` sanitization and `sdk/cliproxy/service.go` model registration.
- Extend the existing heavy test suite when changing retry, cooldown, alias-pool, or scheduler behavior. Files like `conductor_execute.go`, `conductor_refresh.go`, and `conductor_selection.go` are intentional split points, not invitations for parallel styles.

## RECENT CLEANUPS
- Standalone `api_key_model_alias.go` is gone. API-key alias logic now lives in `conductor_alias.go` alongside helper tables; do not resurrect a parallel alias file.

## CHECKS
```bash
go test ./sdk/cliproxy/auth/...
```
