# sdk

Parent: `../AGENTS.md`

## OVERVIEW
Public, embeddable surface. This tree is what downstream tools should depend on instead of `internal/`.

## WHERE TO LOOK
- `cockpit/`: service lifecycle, builder, watcher hookup, model registration.
- `access/`: inbound request-auth provider registry and manager.
- `api/handlers/`: reusable HTTP handler layer.
- `auth/`: authenticator contracts and Codex login helpers.
- `translator/`: public translation registry and pipeline.
- `proxyutil/`: thin proxy parsing helpers shared by public callers; has its own child `AGENTS.md`.

## LOCAL CONVENTIONS
- Public API changes should ripple into external docs/help-site guidance and package tests when those docs exist; this checkout has no tracked README tree.
- Keep reusable contracts here even if internals have richer implementations.
- Keep request gating in `access/`; keep login/token-store concerns in `auth/`; keep runtime auth selection and cooldown logic in `cockpit/auth/`.
- Keep thin helper packages thin; do not move runtime-only behavior into `proxyutil/`.
- If a subfolder has its own child `AGENTS.md`, switch to it for local rules.

## RECENT CLEANUPS
- `docs/` is gitignored scratch output and `examples/` is gone. Treat old docs/examples references as stale until user docs are reintroduced.

## CHECKS
```bash
go test ./sdk/...
```
