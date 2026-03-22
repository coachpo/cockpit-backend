# internal/api/handlers/management

Parent: `internal/api/AGENTS.md`

## OVERVIEW
Persistent management API. Owns config edits, auth-file lifecycle, model definitions, OAuth callbacks/session cleanup, and quota endpoints.

## WHERE TO LOOK
- `handler.go`: middleware, key handling, persistence helpers, and config saver injection.
- `config_basic.go`, `config_lists.go`, `config_lists_oauth.go`: config editing endpoints.
- `auth_files.go`, `auth_files_write.go`, `auth_files_helpers.go`, `auth_files_oauth.go`: auth listing/CRUD, shared auth helpers, and Codex OAuth callback flow.
- `model_definitions.go`, `oauth_sessions.go`, `oauth_callback.go`, `quota.go`: catalog and remaining operational endpoints.

## LOCAL CONVENTIONS
- Persist config through the injected `ConfigSource.SaveConfig`, not plain marshal/write code.
- Management authentication is Bearer-only.
- Redact or avoid secrets in responses and logs.
- Reuse `h.persist()` and the helper setters in `handler.go` for scalar and list config endpoints. Full YAML replacement is the only path that calls the saver directly.
- Auth-file writes are dual-update operations: persist through `authStore`, then mirror runtime state through `authManager` with `coreauth.WithSkipPersist` to avoid double writes.
- OAuth sessions are in-memory only; do not assume restart persistence.

## RECENT CLEANUPS
- `logs.go` and `usage.go` were removed. Do not send contributors looking for legacy management endpoints that no longer exist.

## TESTS
- Local package tests under this folder.
- Broader route wiring and management behavior still live under `go test ./internal/api/...`.

## CHECKS
```bash
go test ./internal/api/handlers/management/...
go test ./internal/api/...
```
