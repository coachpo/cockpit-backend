# api

Parent: `../AGENTS.md`

## OVERVIEW
Checked-in API contract snapshot. This folder currently exists to hold `openapi.yaml` for the trimmed management surface.

## WHERE TO LOOK
- `openapi.yaml`: current OpenAPI document; keep it aligned with `internal/api/openapi_surface_test.go`.

## LOCAL CONVENTIONS
- Treat `openapi.yaml` as a contract snapshot, not a speculative design doc.
- When removing or trimming management endpoints, update the spec and the matching surface tests together.

## ANTI-PATTERNS
- Do not reintroduce removed multi-provider paths by copying stale schemas from old branches.
