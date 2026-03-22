# internal/constant

Parent: `../AGENTS.md`

## OVERVIEW
Provider and protocol constants shared across internal packages.

## WHERE TO LOOK
- `constant.go`: shared constant values used by internal runtime and auth code.

## LOCAL CONVENTIONS
- Keep constants stable and centralized here instead of scattering duplicate literals through handlers or executors.
- If a value becomes part of the public SDK surface, move or mirror it under `sdk/` deliberately.
