# internal/cmd

Parent: `internal/AGENTS.md`

## OVERVIEW
CLI-side startup helpers used by `cmd/cockpit`. This package is intentionally small today and owns foreground/background service startup around injected config and auth sources.

## WHERE TO LOOK
- `run.go`: `StartService`, `StartServiceBackground`, `cockpit.NewBuilder` wiring, and signal handling.

## LOCAL CONVENTIONS
- Keep flag parsing in `cmd/cockpit/main.go`; move reusable startup or login behavior here.
- Service startup should build through `sdk/cockpit.NewBuilder`, passing through the `nacos.ConfigSource` and `nacos.WatchableAuthStore` chosen during bootstrap.
- Reuse the already-selected `config.Config`, `nacos.ConfigSource`, and `nacos.WatchableAuthStore`; do not reload config or auth state inside this package.
- Keep foreground and background startup behavior consistent when service lifecycle rules change.
- Login-specific flows no longer live in this directory; do not point contributors at removed `openai_*`, prompt, or auth-manager helper files.

## CHECKS
```bash
go test ./internal/cmd/...
```
