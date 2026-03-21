# sdk/api/handlers/openai

Parent: `sdk/api/handlers/AGENTS.md`

## OVERVIEW
OpenAI-compatible HTTP, Responses, and websocket handlers. This package owns payload coercion and the public wire shape embedders depend on.

## WHERE TO LOOK
- `openai_handlers.go`: `/v1/models`, `/v1/chat/completions`, `/v1/completions`, and Responses-to-ChatCompletions coercion.
- `openai_responses_handlers.go`: `/v1/responses` and `/v1/responses/compact` HTTP flows.
- `openai_responses_websocket.go`: `/v1/responses/ws`, request normalization, incremental input, local prewarm, and websocket event forwarding.
- `*_test.go`: compact-response, stream-error, and websocket compatibility coverage.

## LOCAL CONVENTIONS
- Keep OpenAI-compatible error bodies, SSE framing, and websocket event shapes stable; downstream tools treat them as public behavior.
- `/v1/chat/completions` intentionally accepts Responses-format payloads and rewrites them before execution; preserve tool metadata through that conversion path.
- `/v1/responses/compact` is non-streaming only and must execute with alt `responses/compact`.
- Responses websocket flow preserves `previous_response_id` only for websocket-capable upstreams, synthesizes local prewarm responses for `generate=false`, and closes execution sessions on disconnect.
- Add focused tests when changing request normalization, stream chunk rewriting, or websocket completion and error handling. Extend `openai_responses_websocket_test.go` instead of creating parallel websocket suites.

## CHECKS
```bash
go test ./sdk/api/handlers/openai/...
go test ./sdk/api/handlers/...
```
