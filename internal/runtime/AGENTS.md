# internal/runtime

Parent: `internal/AGENTS.md`

## OVERVIEW
Runtime execution layer. The checked-in runtime is currently the Codex execution bridge, and nearly all behavior lives in `executor/`.

## WHERE TO LOOK
- `executor/`: Codex HTTP/websocket execution bridge plus shared proxy, logging, request-shaping, and usage helpers.

## LOCAL CONVENTIONS
- Runtime behavior changes should preserve the contract expected by `sdk/cockpit/auth` executors.
- Prefer local runtime helpers over leaking execution details back into API handlers.
- Keep Codex transport, payload, proxy, and usage behavior inside executor helpers instead of scattering it across callers.
- Switch to `executor/AGENTS.md` for provider-execution specifics.

## CHECKS
```bash
go test ./internal/runtime/...
```
