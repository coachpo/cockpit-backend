# internal/api

Parent: `internal/AGENTS.md`

## OVERVIEW
Gin-based HTTP layer. Owns server construction, middleware ordering, `/v1*` routes, and websocket attachment.

## WHERE TO LOOK
- `server.go`: `Server`, `NewServer`, route setup, hot-reload handling.
- `handlers/management/`: config/auth/quota/oauth management APIs.

## LOCAL CONVENTIONS
- Management routes are always route-mounted and use Bearer-only authentication.
- Request logging must allow `/api/provider/...`.
- Preserve the current middleware layering in `NewServer`; ordering matters for auth.
- Keep websocket route attachment and `/v1` route shapes stable when refactoring server setup.

## CHECKS
```bash
go test ./internal/api/...
```
