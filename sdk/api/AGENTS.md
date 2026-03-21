# sdk/api

Parent: `sdk/AGENTS.md`

## OVERVIEW
Public HTTP-facing SDK surface. Root files handle shared options and management helpers, while `handlers/` owns request execution mechanics.

## WHERE TO LOOK
- `management.go`, `options.go`: package-level API support.
- `handlers/`: reusable protocol handlers and streaming helpers.

## LOCAL CONVENTIONS
- Keep public request/response behavior stable for embedders.
- Route protocol-specific behavior into `handlers/` rather than bloating root package files.
- Child rules in `handlers/AGENTS.md` override this file for detailed handler behavior.

## CHECKS
```bash
go test ./sdk/api/...
```
