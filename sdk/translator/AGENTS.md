# sdk/translator

Parent: `sdk/AGENTS.md`

## OVERVIEW
Public translation registry and middleware pipeline. This is the reusable surface wrapped by `internal/translator`.

## WHERE TO LOOK
- `registry.go`, `format.go`, `formats.go`, `types.go`: registry and format contracts.
- `pipeline.go`: request/response middleware pipeline.
- `helpers.go`: convenience translation entrypoints around the default registry.

## LOCAL CONVENTIONS
- Public translation behavior should stay format-driven and registry-backed.
- Builtin registrations happen through blank imports in `internal/translator`, not under this directory.
- Keep middleware additions generic enough for embedders; internal-only provider quirks belong in `internal/translator`.
- If translation contracts change here, review both SDK callers and internal wrapper usage.

## RECENT CLEANUPS
- `builtin/` was removed. Any guidance still pointing here is stale.

## CHECKS
```bash
go test ./sdk/translator/...
```
