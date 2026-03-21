# internal/config

Parent: `internal/AGENTS.md`

## OVERVIEW
Config schema and lifecycle. The config package is now split by concern across schema/types, load flow, and sanitization helpers.

## WHERE TO LOOK
- `config.go`: `Config`, provider `*Key` structs, and shared schema types.
- `config_load.go`: load/hash flow and top-level config normalization.
- `config_sanitize.go`: sanitize/normalize helpers for config sections.
- `sdk_config.go`: SDK-facing config embedding.

## LOCAL CONVENTIONS
- New provider config needs a `*Key` struct, sanitize path, and wiring into load/save flow.
- Keep `config.example.yaml` aligned with behavior here.
- Global OAuth alias and exclusion maps live here; document whether a feature is config-wide or credential-specific.
- Persist through `ConfigSource.SaveConfig`; static mode must stay read-only and Nacos mode owns mutations.

## CROSS-SUBSYSTEM IMPACT
Changing config shape usually requires watcher synthesizer and diff changes, plus any affected auth or runtime wiring.

## CHECKS
```bash
go test ./internal/config/...
```
