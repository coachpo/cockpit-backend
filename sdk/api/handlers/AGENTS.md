# sdk/api/handlers

Parent: `sdk/api/AGENTS.md`

## OVERVIEW
Reusable handler layer sitting above runtime auth execution. Normalizes request metadata, error bodies, streaming behavior, and protocol-specific handlers.

## WHERE TO LOOK
- `handlers.go`: base handler type, request context wiring, response-error logging.
- `context.go`, `keepalive.go`, `error_response.go`, `execution.go`: split execution helpers and request behavior utilities.
- `stream_forwarder.go`, `header_filter.go`: shared response mechanics.
- `openai/`: protocol-specific route handlers.

## LOCAL CONVENTIONS
- Keep OpenAI-compatible error formatting stable.
- Preserve context metadata keys for pinned auth, selected-auth callbacks, and execution sessions.
- Streaming and non-streaming keepalive behavior is part of the public behavior surface.
- Public handler packages should stay split by protocol instead of growing `handlers.go` indefinitely.
- Child rules in `sdk/api/handlers/openai/AGENTS.md` override this file for OpenAI route, SSE, and websocket specifics.
- Add focused tests when changing bootstrap retry or header passthrough logic.

## CHECKS
```bash
go test ./sdk/api/handlers/...
```
