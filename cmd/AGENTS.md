# cmd

Parent: `./AGENTS.md`

## OVERVIEW
Checked-in binaries only. This tree currently ships the `cockpit` main binary.

## WHERE TO LOOK
- `cockpit/main.go`: flags, `.env` load, Nacos bootstrap, auth-dir resolution, and service dispatch.

## LOCAL CONVENTIONS
- `cockpit/main.go` may orchestrate startup, but reusable behavior should move into `internal/` or `sdk/` quickly.
- Keep the split between flag parsing, Nacos bootstrap, auth-dir resolution, access-provider registration, and service handoff readable.
- New top-level modes should route through `internal/cmd/` instead of growing large inline branches.
- Child rules in `cockpit/AGENTS.md` override this file for the binary.

## CHECKS
```bash
go build -o test-output ./cmd/cockpit
go test ./cmd/...
```
