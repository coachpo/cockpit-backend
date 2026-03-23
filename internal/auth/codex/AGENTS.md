# internal/auth/codex

Parent: `../AGENTS.md`

## OVERVIEW
Codex-only auth implementation. This folder owns OAuth URL and token exchange, the local callback server, PKCE generation, JWT claim parsing, credential filenames, and file-backed token storage.

## WHERE TO LOOK
- `openai_auth.go`: OAuth constants, auth URL generation, code exchange, refresh flow, and token-storage updates.
- `oauth_server.go`: localhost callback server, `/auth/callback` and `/success`, timeout handling, and shutdown behavior.
- `openai.go`, `jwt_parser.go`: PKCE/token data types plus ID-token claim parsing.
- `pkce.go`: verifier/challenge generation.
- `token.go`, `filename.go`: persisted JSON shape, metadata merge, and auth filename conventions.
- `errors.go`: user-facing OAuth and authentication error types.

## LOCAL CONVENTIONS
- Keep the OAuth redirect flow aligned end to end: auth URL params, local callback server, and redirect URI must change together.
- PKCE is required for code exchange; do not bypass `PKCECodes` or inline verifier/challenge generation in callers.
- Persist auth files through the existing JSON shape and `misc.MergeMetadata`; watcher and runtime code expect those file-backed token fields to stay stable.
- Credential filenames encode email, plan, and account distinctions; keep `filename.go` aligned with any plan-type or provider-prefix changes.
- Parse ID-token claims through `ParseJWTToken` before deriving account or email metadata; do not duplicate claim extraction in callers.

## CHECKS
```bash
go test ./internal/auth/...
```
