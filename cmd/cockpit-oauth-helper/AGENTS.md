# cmd/cockpit-oauth-helper

Parent: `../AGENTS.md`

## OVERVIEW
OAuth helper binary. Owns helper-specific flag parsing and logger setup, then hands off into `internal/cmd.RunOAuthHelper` for the interactive callback-forwarding flow.

## WHERE TO LOOK
- `main.go`: `-target` and `-no-browser` flags, base logger setup, and the `internal/cmd.RunOAuthHelper` call.

## LOCAL CONVENTIONS
- Keep `main.go` focused on CLI parsing and process exit codes. Reusable helper behavior belongs in `internal/cmd/oauth_helper.go`.
- Preserve the current handoff shape from `cmd/cockpit-oauth-helper/main.go` into `internal/cmd.RunOAuthHelper` when extending the helper.
- Keep helper UX text and flag semantics aligned with the backend callback flow instead of duplicating logic in this directory.

## CHECKS
```bash
go build -o /tmp/cockpit-oauth-helper ./cmd/cockpit-oauth-helper
```
