# sdk/access

Parent: `sdk/AGENTS.md`

## OVERVIEW
Inbound request-auth provider registry and manager. This package gates API requests before they reach runtime auth execution.

## WHERE TO LOOK
- `registry.go`: provider interface, global registry, registration order.
- `manager.go`: ordered authentication walk and error folding.
- `types.go`: config-facing provider definitions.
- `errors.go`: public auth error codes.

## LOCAL CONVENTIONS
- Registration order matters; `Manager.Authenticate` walks providers until one succeeds or returns a terminal auth error.
- Distinguish `NotHandled`, `NoCredentials`, and `InvalidCredential` correctly so fallback behavior stays predictable.
- This package gates inbound requests; login flows and token persistence live in `sdk/auth`.
- Built-in config-backed providers must stay compatible with `internal/access/reconcile.go`.

## CHECKS
```bash
go test ./sdk/access/...
```
