# internal/runtime

Parent: `internal/AGENTS.md`

## OVERVIEW
Runtime execution layer. Most work lives in `executor/`, which bridges auth-selected clients to upstream provider behavior.

## WHERE TO LOOK
- `executor/`: provider runtime bridge and shared execution helpers.

## LOCAL CONVENTIONS
- Runtime behavior changes should preserve the contract expected by `sdk/cliproxy/auth` executors.
- Prefer local runtime helpers over leaking execution details back into API handlers.
- Keep provider transport, payload, proxy, and usage behavior inside executor helpers instead of scattering it across callers.
- Switch to `executor/AGENTS.md` for provider-execution specifics.

## CHECKS
```bash
go test ./internal/runtime/...
```
