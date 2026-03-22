# sdk/proxyutil

Parent: `../AGENTS.md`

## OVERVIEW
Public proxy parsing and transport-construction helpers shared by SDK consumers.

## WHERE TO LOOK
- `proxy.go`: proxy `Setting` parsing plus direct/HTTP/SOCKS transport and dialer builders.
- `proxy_test.go`: supported modes, validation, and transport behavior checks.

## LOCAL CONVENTIONS
- Keep this package transport-level only: parse settings, build transports/dialers, return explicit modes.
- Preserve `inherit`, `direct`/`none`, and explicit proxy URL semantics across both HTTP and connection-layer helpers.

## ANTI-PATTERNS
- Do not move runtime auth, retry, or executor behavior into this package.
- Do not add provider-specific proxy rules here; callers should decide how to use the returned transport or dialer.
