# internal/wsrelay

Parent: `internal/AGENTS.md`

## OVERVIEW
Websocket relay manager that bridges HTTP-like requests over provider sessions for the service websocket gateway.

## WHERE TO LOOK
- `manager.go`: gateway path, upgrade handling, provider naming, session replacement, disconnect hooks.
- `session.go`: per-provider session lifecycle, request multiplexing, outstanding-call cleanup.
- `message.go`: wire message types and payload envelopes.
- `http.go`: non-streaming and streaming request/response bridging.

## LOCAL CONVENTIONS
- Provider names are normalized to lowercase, and a new connection replaces the older session for the same provider.
- Message IDs are mandatory; request/response correlation lives on those IDs.
- Disconnects must clean up waiting callers and trigger the configured disconnect hook.
- Keep websocket path and upgrade behavior aligned with how `sdk/cockpit/service.go` wires the gateway.

## CHECKS
```bash
go test ./internal/wsrelay/...
```
