# internal/runtime/executor

Parent: `internal/runtime/AGENTS.md`

## OVERVIEW
Runtime bridge from the core auth manager to active upstream providers. The checked-in executors now center on Codex and OpenAI-compatible upstreams, with shared translation, logging, payload, proxy, and usage helpers.

## WHERE TO LOOK
- `codex_auto_executor.go`, `codex_executor.go`: Codex auto-routing facade plus stateless HTTP executor.
- `codex_websockets_executor.go`, `codex_websockets_execute.go`, `codex_websockets_stream.go`, `codex_websocket_*.go`, `codex_websockets_executor_sessions.go`: websocket execution, transport primitives, and session lifecycle.
- `openai_compat_executor.go`, `openai_compat_executor_helpers.go`: OpenAI-compatible upstream bridge and shared `statusErr` helpers.
- `cache_helpers.go`, `proxy_helpers.go`, `logging_helpers.go`, `payload_helpers.go`, `token_helpers.go`, `usage_helpers.go`, `codex_executor_helpers.go`: shared infrastructure.
- `thinking_providers.go`: blank-import side-effect registrations.

## LOCAL CONVENTIONS
- Executors implement `Identifier`, `PrepareRequest`, `HttpRequest`, `Execute`, `ExecuteStream`, `CountTokens`, and `Refresh`.
- Codex has three layers: `codex_auto_executor.go` picks WS vs HTTP, `codex_executor.go` owns stateless HTTP work, and the websocket files own long-lived session flow.
- Preserve the shared execution pipeline: translate request, apply thinking/payload config, inject provider headers, call upstream, parse/report usage, and translate response.
- Use proxy-aware HTTP client helpers instead of ad hoc transports.
- Raw JSON mutation with `gjson` and `sjson` is normal here. Prefer local helper files over new cross-package helpers when executor-specific rules grow.
- Extend the existing websocket and executor tests instead of starting parallel suites; this package intentionally keeps dense protocol coverage in place.
- Usage reporting is intentionally a no-op stub today; wire new accounting carefully instead of assuming the old usage manager still exists.

## RECENT CLEANUPS
- Claude/Qwen/Antigravity executor branches, `cloak_obfuscate.go`, `cloak_utils.go`, and `user_id_cache.go` are gone. Do not resurrect their guidance from old docs.

## GOTCHAS
- Codex: websocket auto-routing, `responses/compact` fallback, and session state live outside the plain HTTP executor.
- OpenAI-compatible upstreams: preserve upstream error headers and proxy precedence.

## CHECKS
```bash
go test ./internal/runtime/executor/...
```
