# cmd

Parent: `../AGENTS.md`

## OVERVIEW
Checked-in binaries only. This tree currently ships the `cockpit` and `cockpit-oauth-helper` binaries.

## WHERE TO LOOK
- `cockpit/main.go`: flags, Nacos bootstrap, and service dispatch.
- `cockpit-oauth-helper/main.go`: helper flags, logger setup, and handoff into `internal/cmd.RunOAuthHelper`.

## LOCAL CONVENTIONS
- `cockpit/main.go` may orchestrate startup, but reusable behavior should move into `internal/` or `sdk/` quickly.
- `cockpit-oauth-helper/main.go` should stay thin too. Keep reusable helper flow in `internal/cmd/oauth_helper.go`.
- Keep the split between flag parsing, Nacos bootstrap, access-provider registration, and service handoff readable.
- New top-level modes should route through `internal/cmd/` instead of growing large inline branches.
- Child rules in `cockpit/AGENTS.md` and `cockpit-oauth-helper/AGENTS.md` override this file for each binary.

## CHECKS
```bash
go build -o /tmp/cockpit ./cmd/cockpit
go build -o /tmp/cockpit-oauth-helper ./cmd/cockpit-oauth-helper
go test ./cmd/...
```
