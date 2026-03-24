# internal/nacos

Parent: `internal/AGENTS.md`

## OVERVIEW
Config and auth source abstraction layer. This package supplies the Nacos-backed stores used by bootstrap, management, builder wiring, and watcher reloads.

## WHERE TO LOOK
- `interfaces.go`: `ConfigSource` and `WatchableAuthStore` contracts shared across bootstrap paths.
- `client.go`: env-driven Nacos client construction, namespace/group/cache/log settings.
- `config_store.go`: remote config load/save/watch, sanitization, and remote-management secret hashing.
- `auth_store.go`: remote auth list/save/delete/watch backed by JSON entries in Nacos.

## LOCAL CONVENTIONS
- Hash plaintext remote-management secrets before publishing remote config.
- Avoid duplicate watch notifications by comparing checksums and normalized entry state before invoking callbacks.
- Nacos data IDs are stable API: `proxy-config` for config and `auth-credentials` for auth metadata.
- Bootstrap chooses stores in `cmd/cockpit/main.go`; service defaults live in `sdk/cockpit/builder.go`. Keep both sides aligned on the Nacos-only contract.

## CHECKS
```bash
go test ./internal/nacos/...
```
