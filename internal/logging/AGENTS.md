# internal/logging

Parent: `internal/AGENTS.md`

## OVERVIEW
Shared logging layer. This package owns base logrus setup, Gin request logging, panic recovery, and request ID propagation.

## WHERE TO LOOK
- `global_logger.go`: `SetupBaseLogger`, custom formatter, Gin writer wiring, and log-directory resolution.
- `gin_logger.go`: request logging middleware, panic recovery, skip markers, and AI-route request ID injection.
- `requestid.go`: context and Gin helpers for request ID creation and lookup.

## LOCAL CONVENTIONS
- Call `SetupBaseLogger` before assuming Gin writers or the shared formatter exist; it is intentionally guarded by `sync.Once`.
- `GinLogrusLogger` only assigns request IDs to AI-facing routes. Preserve the current prefix list and skip-hook behavior unless routing semantics change.
- Keep management-log skipping and `/api/provider/...` visibility aligned with `internal/api` expectations; logging behavior is part of the HTTP surface.
- Request IDs live in both the request context and Gin context. Preserve both paths when threading logs through async helpers.
- Mask secrets before they hit log lines. Reuse `internal/util` masking helpers instead of open-coding redaction.

## CHECKS
```bash
go test ./internal/logging/...
```
