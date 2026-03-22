# internal/misc

Parent: `../AGENTS.md`

## OVERVIEW
Focused helper leaf for callback parsing, credential/header utilities, and example-config bootstrap support.

## WHERE TO LOOK
- `oauth.go`: secure OAuth state generation and callback URL parsing.
- `credentials.go`, `header_utils.go`: small auth/header helpers.
- `copy-example-config.go`: bootstrap helper for example config material.

## LOCAL CONVENTIONS
- Keep helpers here focused and reusable across subsystems; do not turn this into a general dumping ground.
- OAuth parsing here should stay transport-neutral; provider-specific flow control belongs in auth or management packages.
