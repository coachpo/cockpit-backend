# internal/browser

Parent: `../AGENTS.md`

## OVERVIEW
Browser-launch support leaf used by OAuth and local callback flows.

## WHERE TO LOOK
- `browser.go`: best-effort browser launch helper.

## LOCAL CONVENTIONS
- Keep this package narrow and side-effect free apart from the launch attempt.
- Push OAuth semantics up into management handlers or auth flows; this leaf should stay launch-only.
