# cmd/cockpit

Parent: `../AGENTS.md`

## OVERVIEW
Primary binary entrypoint. Owns flag parsing, strict Nacos-only config/auth bootstrap, and handoff into `internal/cmd.StartService`.

## WHERE TO LOOK
- `main.go`: flag definitions, `NACOS_ADDR` bootstrap, access-provider registration, and runtime startup.

## LOCAL CONVENTIONS
- Keep new top-level flags and branch points readable, but move real behavior into `internal/cmd`, `internal/api`, `internal/nacos`, or `sdk/cockpit` quickly.
- Preserve the current startup flow: Nacos-only config/auth bootstrap, logging config, access-provider registration, then service dispatch.
- `NACOS_ADDR` is required and startup exits on Nacos bootstrap failure; if startup semantics change, update this file and `backend/AGENTS.md` together.
- The blank import of `internal/translator` is intentional and required for translator registration side effects.

## CHECKS
```bash
go build -o /tmp/cockpit ./cmd/cockpit
```
