# test

Parent: `./AGENTS.md`

## OVERVIEW
Integration-style and cross-subsystem tests. Use this folder for flows that touch several internal packages at once.

## WHERE TO LOOK
- `thinking_conversion_test.go`: large response/request conversion matrix.
- `builtin_tools_translation_test.go`: builtin tool translation behavior.
- `nacos_integration_test.go`: live Nacos smoke test gated by `COCKPIT_NACOS_SMOKE=1` plus `NACOS_ADDR`, `NACOS_USERNAME`, and `NACOS_PASSWORD`.

## LOCAL CONVENTIONS
- Use this folder only when a test spans package boundaries.
- Extend existing large matrices instead of starting many small overlapping suites.
- Keep data inline and helpers reusable; there is no fixture tree here.
- Keep long-running integration coverage env-gated and skipped by default.
- Spell out required env vars next to any live-smoke gate so contributors can opt in without reading the whole test body.

## CHECKS
```bash
go test ./test/...
```
