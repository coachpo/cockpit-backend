# internal/api

Parent: `internal/AGENTS.md`

## OVERVIEW
Gin-based HTTP layer. Owns server construction, middleware ordering, `/v1*` routes, websocket attachment, and lazy management enablement.

## WHERE TO LOOK
- `server.go`: `Server`, `NewServer`, route setup, hot-reload handling.
- `handlers/management/`: config/auth/quota/oauth management APIs and upstream API tools.

## LOCAL CONVENTIONS
- Management routes stay behind lazy registration and the management middleware path.
- Request logging must keep skipping management paths and must allow `/api/provider/...`.
- Preserve the current middleware layering in `NewServer`; ordering matters for logging and auth.
- Keep websocket route attachment and `/v1` route shapes stable when refactoring server setup.

## CHECKS
```bash
go test ./internal/api/...
```
