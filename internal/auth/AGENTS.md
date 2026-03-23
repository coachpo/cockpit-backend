# internal/auth

Parent: `internal/AGENTS.md`

## OVERVIEW
Private auth implementations. The checked-in provider flow is Codex only; placeholder and secondary provider trees were removed in recent cleanup commits.

## WHERE TO LOOK
- `codex/`: auth entrypoints, token parsing, OAuth server, PKCE helpers, and provider errors.
- `models.go`: shared `TokenStorage` contract.

## LOCAL CONVENTIONS
- Keep provider-specific flows inside the provider folder: auth entrypoints, token parsing, OAuth server, PKCE helpers, and provider errors.
- Avoid cross-provider imports. Shared contracts belong at the package root, not copied between provider dirs.
- Adding a provider is never auth-only. Expect follow-up edits in `internal/config/`, `internal/watcher/`, and often runtime/auth orchestration layers.
- Preserve file-backed token behavior expected by watcher synthesis and SDK token stores.
- Child rules in `codex/AGENTS.md` override this file inside the provider implementation.

## RECENT CLEANUPS
- `empty/` and other legacy provider folders were pruned. Treat stale references to them as dead guidance.

## CHECKS
```bash
go test ./internal/auth/...
```
