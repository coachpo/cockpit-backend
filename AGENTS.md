# PROJECT KNOWLEDGE BASE

**Generated:** 2026-03-21T23:19:20+02:00
**Commit:** 4deb29c
**Branch:** main

## OVERVIEW
Cockpit v6 is a Go 1.26 proxy plus embeddable SDK centered on Codex OAuth, OpenAI-compatible upstream routing, hot-reloadable config/auth state, and websocket relay. `cmd/` stays thin around the `cockpit` binary, `internal/` owns runtime and support details, and `sdk/` exposes the reusable service/auth/handler surface.

## HIERARCHY RULE
Read the nearest `AGENTS.md` first. Child files are deltas for their folder, not restatements of the root file.

## STRUCTURE
```text
./
|- cmd/                 # checked-in `cockpit` binary entrypoint
|- internal/            # private runtime, management, logging, utility, watcher, and relay code
|- sdk/                 # embeddable public surface
|- test/                # cross-subsystem matrices
|- temp/                # tracked runtime stats output under `temp/stats/`
|- config.example.yaml  # config-key inventory
|- .env.example         # env var starter file
|- Dockerfile           # container build with ldflags metadata injection
|- docker-compose.yml   # local Nacos + Cockpit stack
|- docker-build.sh      # interactive docker workflow
|- .sisyphus/plans/     # local planning notes used during deep work
`- docs/                # gitignored scratch tree, not checked-in user docs
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Start the binary | `cmd/cockpit/main.go` | flags, `.env` load, cloud deploy detection, Nacos/static bootstrap, service handoff |
| Service startup helpers | `internal/cmd/` | `StartService`, `StartServiceBackground`, and cloud standby |
| Built-in request access wiring | `internal/access/` | reconciles config API keys into the `sdk/access` manager |
| Config/auth backends | `internal/nacos/` | remote Nacos stores plus static file-backed fallbacks |
| HTTP routing + management | `internal/api/server.go` | `/v1*` routes, websocket attachment, lazy `/v0/management` enablement |
| Management persistence APIs | `internal/api/handlers/management/` | config edits, auth files, quota toggles, OAuth callbacks, API tools |
| Request logging | `internal/logging/` | base logger, Gin middleware, request IDs |
| Shared internal contracts | `internal/interfaces/` | handler and client-model interfaces reused across handlers and tests |
| Proxy/auth utility helpers | `internal/util/` | auth-dir resolution, masking, proxy helpers, model/tool-name helpers |
| Config lifecycle | `internal/config/` | split schema, load, and sanitization flow |
| Model catalog | `internal/registry/` | dynamic registry plus embedded catalog lookup |
| Hot reload | `internal/watcher/` | reload, synthesis, diff, dispatch |
| Thinking normalization | `internal/thinking/` | parse, validate, and apply reasoning config |
| Provider execution | `internal/runtime/executor/` | Codex + OpenAI-compatible bridges and proxy-aware helpers |
| Translator system | `internal/translator/`, `sdk/translator/` | side-effect registrations over public format contracts |
| Websocket relay gateway | `internal/wsrelay/` | provider sessions, request multiplexing, stream relay |
| Public embed API | `sdk/cliproxy/` | `Builder`, `Service`, watcher/auth integration |
| Public HTTP handlers | `sdk/api/handlers/` | reusable request execution and error surface |
| Integration coverage | `test/` | large matrix, builtin-tool translation, and env-gated Nacos smoke tests |

## CODE MAP
| Symbol | Location | Role |
|--------|----------|------|
| `main` | `cmd/cockpit/main.go` | CLI entrypoint, env/config bootstrap |
| `StartService` | `internal/cmd/run.go` | service assembly, signal handling, keep-alive shutdown hook |
| `access.ApplyAccessProviders` | `internal/access/reconcile.go` | reconciles config-backed request auth providers into `sdk/access` |
| `NewServer` | `internal/api/server.go` | Gin engine, middleware, route setup, management enablement |
| `logging.SetupBaseLogger` | `internal/logging/global_logger.go` | shared logrus + Gin writer bootstrap |
| `util.ResolveAuthDir` | `internal/util/util.go` | auth-dir normalization shared by bootstrap and logging |
| `(*NacosConfigStore).SaveConfig` | `internal/nacos/config_store.go` | Nacos-backed config persistence path |
| `nacos.NewClientFromEnv` | `internal/nacos/client.go` | env-driven remote config/auth bootstrap |
| `GetGlobalRegistry` | `internal/registry/model_registry.go` | global model availability registry |
| `(*Watcher).Start` | `internal/watcher/watcher.go` | config/auth watch loop and update dispatch |
| `(*Service).Run` | `sdk/cliproxy/service.go` | service lifecycle, watcher/auth integration |
| `(*Builder).Build` | `sdk/cliproxy/builder.go` | dependency defaults, access wiring, service assembly |
| `(*Manager).Handler` | `internal/wsrelay/manager.go` | websocket relay upgrade endpoint |

## COMMANDS
```bash
go build -o test-output ./cmd/cockpit
go test ./...
go test ./internal/...
go test ./sdk/...
go test ./test/...
docker compose up -d --remove-orphans --no-build
docker compose build --build-arg VERSION="..." --build-arg COMMIT="..." --build-arg BUILD_DATE="..."
```

## REPO-WIDE CONVENTIONS
- `internal/` owns runtime details. `sdk/` owns public contracts and reusable entrypoints.
- Keep `cmd/` thin. Push behavior into `internal/` or `sdk/` quickly.
- `config.example.yaml` is the fastest inventory of supported config keys.
- Built-in request access providers reconcile through `internal/access/` and `sdk/access/`; do not register them ad hoc from handlers or executors.
- Config-source changes now span `cmd/cockpit`, `internal/cmd`, `internal/nacos`, `internal/watcher`, and `sdk/cliproxy`.
- Config-shape changes still span `internal/config/`, `internal/watcher/synthesizer/`, `internal/watcher/diff/`, and often `sdk/cliproxy/auth/`.
- Request logging skips management endpoints on purpose and must keep allowing `/api/provider/...`.
- `sdk/access` handles inbound request auth; `sdk/auth` covers login and token stores; `sdk/cliproxy/auth` is the runtime auth conductor.
- Extend existing large matrices like `test/thinking_conversion_test.go`, `internal/watcher/watcher_test.go`, and `sdk/api/handlers/openai/openai_responses_websocket_test.go` instead of creating parallel suites.
- This checkout has no tracked README or help-site docs; do not assume them locally.

## ANTI-PATTERNS (THIS PROJECT)
- Do not write config through ad hoc YAML marshaling when management code expects `ConfigSource.SaveConfig`.
- Do not bypass lazy management registration or the management middleware path.
- Do not bypass proxy-aware HTTP helpers inside executors.
- Do not log secrets, raw management keys, or unredacted auth payloads.
- Do not treat `internal/translator/` as a casual edit zone; registration is side-effect driven.
- Do not copy stale `cmd/server`, `README`, `examples/`, or removed provider paths from old docs or scripts.

## UNIQUE STYLES
- Translators register through blank imports and leaf `init.go` files.
- Executors mutate raw JSON with `gjson` and `sjson` instead of deep struct graphs.
- Watcher diffs prefer redacted summaries and hashes over secret-bearing values.
- Nacos backends and static file stores share the same config/auth interfaces.
- Websocket relay traffic moves through typed message envelopes keyed by request ID.

## STALE OR PRUNED AREAS
- This checkout has no tracked `README.md`, `README_CN.md`, or `.goreleaser.yml`; do not point contributors at them without re-adding them.
- Backend CI does exist under `.github/workflows/ci.yml`; keep service-local checks there rather than moving them to the meta-repo root.
- `docs/` is ignored by `.gitignore`; treat it as scratch output, not canonical documentation.
- `examples/` is gone.
- Recent cleanup commits removed placeholder auth providers, `sdk/cliproxy/usage`, `sdk/translator/builtin`, and legacy executor helpers like `cloak_*` and `user_id_cache.go`.
- `docker-build.sh` no longer supports `--with-usage`; do not reintroduce usage preservation without restoring a matching management API surface.

## NOTES
- `cmd/cockpit/main.go` loads `.env`, resolves Nacos vs static config/auth stores, configures logging, resolves auth dir, registers access providers, then waits for cloud config or starts the proxy service.
- `Dockerfile` injects `Version`, `Commit`, and `BuildDate` into `cmd/cockpit/main.go`; keep container build path aligned with the binary directory.
- `test/thinking_conversion_test.go` is intentionally large. Extend the existing matrix instead of starting parallel styles.
- `test/nacos_integration_test.go` is a live smoke test gated by `COCKPIT_NACOS_SMOKE=1` plus Nacos credentials.
- `internal/wsrelay/` is wired from `sdk/cliproxy/service.go`; keep relay work scoped there instead of reviving removed API scaffolding.
- `temp/stats/` is tracked runtime output, not source.
