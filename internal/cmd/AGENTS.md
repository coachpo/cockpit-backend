# internal/cmd

Parent: `internal/AGENTS.md`

## OVERVIEW
CLI-side startup helpers and Codex login flows used by `cmd/cockpit`, including service construction around injected config/auth sources.

## WHERE TO LOOK
- `run.go`: `StartService`, `StartServiceBackground`, builder wiring, signal handling, keep-alive setup, cloud-deploy standby.
- `openai_login.go`, `openai_device_login.go`: browser and device login entrypoints.
- `auth_manager.go`: shared SDK auth-manager construction.
- `prompt.go`: stdin prompt fallback used by login flows.

## LOCAL CONVENTIONS
- Keep flag parsing in `cmd/cockpit/main.go`; move reusable startup or login behavior here.
- `LoginOptions` is the shared contract for no-browser mode, callback-port overrides, and prompt injection.
- Login flows should reuse the `coreauth.Store` selected during bootstrap; do not create alternate Nacos/static store selection inside these helpers.
- Service startup should build through `sdk/cliproxy.NewBuilder`, passing through the `nacos.ConfigSource` and `nacos.WatchableAuthStore` chosen during bootstrap.
- Device and browser login flows should keep saved-path reporting and success messaging aligned unless provider requirements force a real divergence.

## CHECKS
```bash
go test ./internal/cmd/...
```
