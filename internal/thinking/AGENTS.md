# internal/thinking

Parent: `internal/AGENTS.md`

## OVERVIEW
Unified thinking-config parser, validator, and applier. This package normalizes reasoning controls before provider-specific request mutation.

## WHERE TO LOOK
- `types.go`: `ThinkingConfig`, modes, levels, provider applier contract.
- `validate.go`: capability-aware normalization and clamping.
- `apply.go`, `convert.go`, `suffix.go`, `strip.go`: request parsing and transformation helpers.
- `provider/codex/`, `provider/openai/`: provider-specific apply logic.

## LOCAL CONVENTIONS
- Normalize suffix and body inputs into `ThinkingConfig` before provider-specific apply logic runs.
- Validate against registry model info before mutating request bodies.
- Keep provider-specific behavior inside `provider/`; shared conversions and clamping stay at the package root.
- When behavior changes, extend `test/thinking_conversion_test.go` alongside local package tests; keep adding cases to the existing suffix/body matrices instead of starting parallel suites.

## CHECKS
```bash
go test ./internal/thinking/...
go test ./test/...
```
