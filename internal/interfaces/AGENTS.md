# internal/interfaces

Parent: `../AGENTS.md`

## OVERVIEW
Shared internal contracts for handlers, model metadata, and error payload shapes.

## WHERE TO LOOK
- `api_handler.go`: `APIHandler` contract used by handler wiring and tests.
- `client_models.go`, `types.go`: shared internal data shapes.
- `error_message.go`: reusable error payload helpers.

## LOCAL CONVENTIONS
- Keep this package contract-oriented; runtime behavior belongs in callers.
- Prefer widening these interfaces here over duplicating lookalike structs in handlers or tests.
