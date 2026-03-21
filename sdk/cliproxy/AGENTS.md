# sdk/cliproxy

Parent: `sdk/AGENTS.md`

## OVERVIEW
Primary embed surface for running Cockpit as a library. `Builder` assembles config/auth sources and default managers, `Service` owns lifecycle, and subpackages cover auth plus thin runtime contracts.

## WHERE TO LOOK
- `builder.go`: fluent setup, config/auth source defaults, and access-provider registration.
- `service.go`, `service_runtime.go`, `service_models.go`: lifecycle, runtime auth/websocket wiring, and model registration.
- `providers.go`, `model_registry.go`, `watcher.go`: service wiring and auth/provider rebinding.
- `auth/`: dense runtime auth conductor.
- `executor/`: thin public runtime contracts.

## LOCAL CONVENTIONS
- `Build` requires both config and config path.
- `WithConfigSource` and `WithAuthStore` default to static Nacos adapters; keep those defaults aligned with `cmd/cockpit` bootstrap.
- Register access providers before service startup; builder already does the default config-access wiring.
- `executor/` is a thin public runtime contract; keep runtime behavior out of that leaf package.
- Public-service behavior changes should update external docs/help-site guidance and SDK-facing tests when those docs exist.
- Child rules in `sdk/cliproxy/auth/AGENTS.md` override this file for auth conductor details.

## RECENT CLEANUPS
- `usage/` and `pprof_server.go` were removed. Do not point new contributors at those old extension points.

## CHECKS
```bash
go test ./sdk/cliproxy/...
```
