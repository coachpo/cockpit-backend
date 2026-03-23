# internal/api/handlers/management

Parent: `../../AGENTS.md`

## OVERVIEW
Persistent management API. Owns config edits, auth-file lifecycle, upstream API-call proxying for managed auths, OAuth callbacks/session cleanup, and quota endpoints for the trimmed Codex-focused management surface.

## WHERE TO LOOK
- `handler.go`: config saver injection, auth manager/store wiring, persistence helpers, and log-directory hook-up.
- `config_basic.go`, `config_lists.go`: scalar runtime toggles plus API-key and Codex-key list endpoints.
- `auth_files.go`, `auth_files_write.go`, `auth_files_helpers.go`, `auth_files_oauth.go`: auth listing/CRUD, shared auth helpers, and Codex OAuth bootstrap flow.
- `oauth_sessions.go`, `oauth_callback.go`: in-memory OAuth session tracking and callback completion.
- `api_call.go`: upstream API-call proxying through managed auth files.
- `quota.go`: switch-project quota toggle.
- `api_call_test.go`, `auth_files_delete_test.go`, `auth_files_oauth_test.go`, `config_lists_codex_test.go`, `mutation_static_mode_test.go`, `test_store_test.go`: package-local behavior tests.

## LOCAL CONVENTIONS
- Persist config through the injected `ConfigSource.SaveConfig`, not plain marshal/write code.
- The current trimmed `/v0/management` surface is mounted without request-access middleware; if auth enforcement returns, update this file alongside the route tests.
- Redact or avoid secrets in responses and logs.
- Reuse `h.persist()` and the helper setters in `handler.go` for scalar and list config endpoints. Full YAML replacement is the only path that calls the saver directly.
- Auth-file writes are dual-update operations: persist through `authStore`, then mirror runtime state through `authManager` with `coreauth.WithSkipPersist` to avoid double writes.
- OAuth sessions are in-memory only; do not assume restart persistence.
- Keep Codex-focused list/config endpoints and the managed-auth API-call helper here; do not resurrect removed multi-provider or legacy log/usage handlers from stale docs or old branches.

## RECENT CLEANUPS
- `logs.go`, `usage.go`, and older provider-specific list handlers are gone. Do not send contributors looking for legacy management endpoints that no longer exist.

## TESTS
- Local package tests under this folder.
- Broader route wiring and management behavior still live under `go test ./internal/api/...`.

## CHECKS
```bash
go test ./internal/api/handlers/management/...
go test ./internal/api/...
```
