# internal/util

Parent: `internal/AGENTS.md`

## OVERVIEW
Shared utility layer for cross-cutting helpers that are reused across bootstrap, logging, executors, and translators. This folder already mixes auth-dir resolution, masking, model/provider lookup, and JSON/tool-name helpers.

## WHERE TO LOOK
- `util.go`: log-level changes, auth-dir resolution, writable-path lookup, and generic auth counting.
- `provider.go`: model/provider lookup and sensitive-value masking.
- `header_helpers.go`: custom-header extraction from auth attributes.
- `translator.go`: gjson/sjson JSON walking, key renaming, best-effort JSON fixing, and tool-name canonicalization.
- `image.go`, `ssh_helper.go`: leaf helpers with narrower call sites; keep them leaf-level instead of turning this package into a second runtime layer.

## LOCAL CONVENTIONS
- Put helpers here only when they are genuinely cross-cutting. If the behavior is specific to logging, executors, watchers, or management, keep it in that subsystem instead.
- Reuse the existing masking helpers (`HideAPIKey`, `MaskSensitiveHeaderValue`, `MaskSensitiveQuery`) before inventing new redaction code.
- Translator JSON rewriting here is low-level helper logic. Protocol-specific translation behavior still belongs in `internal/translator` or `sdk/translator`.
- Preserve auth-dir normalization and writable-path behavior because bootstrap, logging, and file-backed stores rely on the same path semantics.

## CHECKS
```bash
go test ./internal/util/...
```
