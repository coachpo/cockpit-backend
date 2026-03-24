# sdk/cockpit/auth

Parent: `sdk/cockpit/AGENTS.md`

## OVERVIEW
Runtime auth conductor. Owns executor registration, auth selection, cooldowns, refresh scheduling, execution-model candidate prep, and persistence policy hooks.

## WHERE TO LOOK
- `conductor.go`: manager core, registration, store wiring.
- `conductor_alias.go`, `conductor_execute.go`, `conductor_selection.go`, `conductor_result.go`, `conductor_refresh.go`, `conductor_http.go`: split conductor responsibilities.
- `scheduler.go`, `selector.go`: selection and rotation strategy.
- `types.go`: auth model and runtime metadata used by selection, execution, and persistence.
- `conductor_alias.go`: execution-model candidate helpers.
- `persist_policy.go`: write suppression hooks.

## LOCAL CONVENTIONS
- Preserve auth state and model cooldown state across updates when possible.
- Selector behavior matters: round-robin and fill-first are both supported and tested.
- Execution metadata keys used by handlers and executors are part of this subsystem contract.
- Model registration and scheduler refresh happen after auth updates; do not break that sequencing casually.
- Direct model names flow through execution unchanged; do not reintroduce config-driven alias tables removed during backend trim.
- Extend the existing heavy test suite when changing retry, cooldown, or scheduler behavior. Files like `conductor_execute.go`, `conductor_refresh.go`, and `conductor_selection.go` are intentional split points, not invitations for parallel styles.

## RECENT CLEANUPS
- Standalone config-driven alias files are gone. Keep direct route-model execution in the current conductor split files; do not resurrect parallel alias modules.

## CHECKS
```bash
go test ./sdk/cockpit/auth/...
```
