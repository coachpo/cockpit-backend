# internal/translator

Parent: `internal/AGENTS.md`

## OVERVIEW
Internal translation bridge layered on top of `sdk/translator`. The checked-in tree covers Codex and OpenAI-style request and response paths.

## WHERE TO LOOK
- `init.go`: blank-import registration hub.
- `translator/translator.go`: wrapper around the SDK registry.
- `codex/openai/chat-completions/`, `codex/openai/responses/`: Codex to OpenAI-shaped translations.
- `openai/openai/chat-completions/`, `openai/openai/responses/`: OpenAI request and response wrappers.

## LOCAL CONVENTIONS
- Register new translators in both the leaf package `init.go` and the root `internal/translator/init.go` import list.
- Preserve the current leaf naming pattern for request/response files and tests.
- Keep OpenAI `chat-completions` and `responses` behavior separate; they are not interchangeable here.
- Extend the existing translation matrices like `codex_openai_request_test.go` and `codex_openai_response_test.go` instead of creating parallel suites.
- This tree is dense but pattern-driven. Extend the existing folder layout instead of inventing parallel shapes.

## GOTCHAS
- Blank-import registration is side-effect driven. Missing either the leaf `init.go` or the root import silently drops a translator.
- `sdk/translator` owns the generic contracts; this tree should stay focused on internal registrations and wrappers.

## CHECKS
```bash
go test ./internal/translator/...
```
