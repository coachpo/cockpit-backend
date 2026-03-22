# internal

Parent: `../AGENTS.md`

## OVERVIEW
Private implementation tree. API routing, config/auth backends, model registry, runtime execution, translators, hot reload, logging, and relay details live here.

## WHERE TO LOOK
- `access/`: built-in request-access provider wiring and reconcile helpers.
- `api/`: Gin server, route registration, websocket attachment, management routes.
- `auth/`: private provider auth flows; current checked-in provider is Codex.
- `browser/`, `constant/`, `interfaces/`, `misc/`: small support leaves for browser launch, provider constants, shared contracts, and focused helpers. Each has its own child `AGENTS.md` now.
- `cmd/`: service startup helpers and cloud-deploy standby used by `cmd/cockpit`.
- `config/`: split config schema, load flow, and sanitization.
- `logging/`: base logrus setup, Gin request logging, and request IDs.
- `nacos/`: remote/static config and auth backends shared by bootstrap and watcher code.
- `registry/`: model catalog and live availability registry.
- `runtime/executor/`: upstream execution bridge.
- `thinking/`: unified reasoning config parsing and validation.
- `translator/`: request/response translation matrix.
- `util/`: auth-dir resolution, header masking, proxy helpers, and model/tool-name utilities.
- `watcher/`: reload, synthesis, diff, dispatch.
- `wsrelay/`: provider websocket relay manager and session lifecycle.

## LOCAL CONVENTIONS
- A provider-facing change often requires coordinated edits across `access/`, `config/`, `watcher/`, `runtime/executor/`, and `sdk/cliproxy/auth/`.
- Request-access updates belong in `access/` plus `sdk/access/`; do not register providers from API handlers or executors.
- Nacos-backed config/auth wiring belongs in `nacos/`, `cmd/`, `watcher/`, and `sdk/cliproxy/`; do not hide it in ad hoc helpers.
- CLI startup behavior belongs in `cmd/`; HTTP routing and management belong in `api/`.
- Favor subsystem-local helpers over new global utility dumping grounds. If a helper only serves logging or shared utility code, check `logging/AGENTS.md` or `util/AGENTS.md` first.
- Keep `browser/`, `constant/`, `interfaces/`, and `misc/` as narrow support leaves instead of turning them into new subsystem dumping grounds.
- If a folder has its own child `AGENTS.md`, switch to that file for detailed rules.

## CHECKS
```bash
go test ./internal/...
```
