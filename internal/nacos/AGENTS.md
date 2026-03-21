# internal/nacos

Parent: `internal/AGENTS.md`

## OVERVIEW
Config and auth source abstraction layer. This package supplies both remote Nacos-backed stores and static file-backed fallbacks used by bootstrap, builder defaults, and watcher wiring.

## WHERE TO LOOK
- `interfaces.go`: `ConfigSource` and `WatchableAuthStore` contracts shared across bootstrap paths.
- `client.go`: env-driven Nacos client construction, namespace/group/cache/log settings.
- `config_store.go`: remote config load/save/watch, sanitization, and remote-management secret hashing.
- `auth_store.go`: remote auth list/save/delete/watch backed by JSON entries in Nacos.
- `static_store.go`: local file-backed fallbacks that satisfy the same interfaces but return `ErrStaticMode` on mutation.
- `errors.go`: static-mode sentinel errors.

## LOCAL CONVENTIONS
- Keep the remote and static implementations behaviorally aligned at the interface boundary: same mode strings, same watch lifecycle, same auth/config semantics where possible.
- Hash plaintext remote-management secrets before publishing remote config.
- Avoid duplicate watch notifications by comparing checksums and normalized entry state before invoking callbacks.
- Nacos data IDs are stable API: `proxy-config` for config and `auth-credentials` for auth metadata.
- Bootstrap chooses stores in `cmd/cockpit/main.go`; service defaults live in `sdk/cliproxy/builder.go`. Keep both sides in sync.

## CHECKS
```bash
go test ./internal/nacos/...
```
