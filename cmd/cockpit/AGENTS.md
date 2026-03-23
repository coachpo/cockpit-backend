# cmd/cockpit

Parent: `cmd/AGENTS.md`

## OVERVIEW
Primary binary entrypoint. Owns flag parsing, env bootstrap, static-vs-Nacos config selection, and handoff into `cmd.StartService`.

## WHERE TO LOOK
- `main.go`: flag definitions, `.env` load, `NACOS_ADDR` bootstrap, auth-dir resolution, access-provider registration, and runtime startup.

## LOCAL CONVENTIONS
- Keep new top-level flags and branch points readable, but move real behavior into `internal/cmd`, `internal/api`, `internal/nacos`, or `sdk/cliproxy` quickly.
- Preserve the current startup flow: `.env` load, config/auth store selection, logging config, auth-dir resolution, access-provider registration, then service dispatch.
- When `NACOS_ADDR` is set, bootstrap both config and auth stores through `internal/nacos`; keep the static-file fallback aligned.
- The blank import of `internal/translator` is intentional and required for translator registration side effects.

## CHECKS
```bash
go build -o test-output ./cmd/cockpit
```
