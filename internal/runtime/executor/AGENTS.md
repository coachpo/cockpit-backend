# internal/runtime/executor

Parent: `internal/runtime/AGENTS.md`

## OVERVIEW
 Runtime bridge from the core auth manager to the active Codex upstream, with shared translation, logging, auth-proxy, and usage helpers.

## WHERE TO LOOK
- `codex_auto_executor.go`, `codex_executor.go`: Codex auto-routing facade plus stateless HTTP executor.
- `codex_websockets_executor.go`, `codex_websockets_execute.go`, `codex_websockets_stream.go`, `codex_websocket_*.go`, `codex_websockets_executor_sessions.go`: websocket execution, transport primitives, and session lifecycle.
- `cache_helpers.go`, `proxy_helpers.go`, `logging_helpers.go`, `token_helpers.go`, `usage_helpers.go`, `codex_executor_helpers.go`: shared infrastructure for transport, request shaping, and usage parsing.
- `http_status_error.go`: shared HTTP status and retry-after helpers used by Codex executors.
- `thinking_providers.go`: blank-import side-effect registrations.

## LOCAL CONVENTIONS
- Executors implement `Identifier`, `PrepareRequest`, `HttpRequest`, `Execute`, `ExecuteStream`, `CountTokens`, and `Refresh`.
- Codex has three layers: `codex_auto_executor.go` picks WS vs HTTP, `codex_executor.go` owns stateless HTTP work, and the websocket files own long-lived session flow.
- Preserve the shared execution pipeline: translate request, apply thinking, inject provider headers, call upstream, parse/report usage, and translate response.
- Use proxy-aware HTTP client helpers instead of ad hoc transports.
- Raw JSON mutation with `gjson` and `sjson` is normal here. Prefer local helper files over new cross-package helpers when executor-specific rules grow.
- Extend the existing websocket and executor tests instead of starting parallel suites; this package intentionally keeps dense protocol coverage in place.
- Usage reporting is intentionally a no-op stub today; wire new accounting carefully instead of assuming the old usage manager still exists.

## RECENT CLEANUPS
- Claude/Qwen/Antigravity executor branches, `cloak_obfuscate.go`, `cloak_utils.go`, and `user_id_cache.go` are gone. Do not resurrect their guidance from old docs.

## GOTCHAS
- Codex: websocket auto-routing, `responses/compact` fallback, and session state live outside the plain HTTP executor.

## CHECKS
```bash
go test ./internal/runtime/executor/...
```
