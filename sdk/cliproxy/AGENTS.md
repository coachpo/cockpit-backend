# sdk/cliproxy

Parent: `sdk/AGENTS.md`

## OVERVIEW
Primary embed surface for running Cockpit as a library. `Builder` assembles config/auth sources and default managers, `Service` owns lifecycle, `auth/` owns the dense runtime conductor, and `executor/` stays a small public runtime-contract leaf.

## WHERE TO LOOK
- `builder.go`: fluent setup, config/auth source inputs, strict selector/config validation, and access-provider registration.
- `service.go`, `service_runtime.go`, `service_models.go`: lifecycle, runtime auth/websocket wiring, and model registration.
- `providers.go`, `model_registry.go`, `watcher.go`: service wiring and auth/provider rebinding.
- `auth/`: dense runtime auth conductor; use its child `AGENTS.md` for scheduler and auth-state details.
- `executor/`: thin public runtime contracts and context helpers; this file remains the local guide for that small leaf.

## LOCAL CONVENTIONS
- `Build` requires config plus config/auth sources; it rejects invalid routing/config state instead of normalizing legacy compatibility aliases.
- `WithConfigSource` and `WithAuthStore` are required builder inputs; `Build()` fails if either is nil.
- Register access providers before service startup; builder already does the default config-access wiring.
- `executor/` is a thin public runtime contract; keep runtime behavior out of that leaf package.
- Public-service behavior changes should update external docs/help-site guidance and SDK-facing tests when those docs exist.
- Only `auth/` has a child `AGENTS.md` today; `executor/` is intentionally small and stays documented here.

## RECENT CLEANUPS
- `usage/` and `pprof_server.go` were removed. Do not point new contributors at those old extension points.

## CHECKS
```bash
go test ./sdk/cliproxy/...
```
