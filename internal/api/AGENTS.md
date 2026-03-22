# internal/api

Parent: `../AGENTS.md`

## OVERVIEW
Gin-based HTTP layer. Owns server construction, middleware ordering, `/v1*` routes, `/v0/management`, OpenAPI surface tests, and websocket attachment.

## WHERE TO LOOK
- `server.go`: `Server`, `NewServer`, shared route setup, hot-reload handling.
- `server_management.go`: lazy management surface wiring and auth middleware attachment.
- `server_keepalive.go`, `server_update.go`: keepalive endpoints and update metadata plumbing.
- `server_options.go`, `server_options_cleanup_test.go`: route options cleanup and shared preflight behavior.
- `openapi_surface_test.go`: asserts `api/openapi.yaml` stays aligned with the trimmed live surface.
- `handlers/management/`: config/auth/quota/oauth management APIs and persistence helpers.

## LOCAL CONVENTIONS
- Management routes are always route-mounted and use Bearer-only authentication.
- Request logging must allow `/api/provider/...`.
- Preserve the current middleware layering in `NewServer`; ordering matters for auth.
- Keep websocket route attachment and `/v1` route shapes stable when refactoring server setup.
- Keep `api/openapi.yaml` and `openapi_surface_test.go` aligned when changing public management schemas or removing routes.

## CHECKS
```bash
go test ./internal/api/...
```
