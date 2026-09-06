# Sentinel identity and trusted-client specification

## Objective

Make Global Pulse and Sentinel reports readable by resolving node/client MACs to stable labels, and let an administrator mark a client as a trusted operator endpoint for its site. Trusted activity is annotated as expected context, not silently ignored.

## Acceptance criteria

- Logs use a node label fallback chain: configured name, telemetry hostname, model, then a stable short identifier; `UNKNOWN` is not emitted when telemetry identifies the node.
- MAC/IP references in context and Sentinel log evidence include a label, site, uplink when known, and trust state.
- An administrator can label, trust, expire, and untrust a client from the site client view.
- Trusted operator activity lowers lateral-movement suspicion, while failed authentication and unrelated indicators remain visible.
- Existing client-hostname labels continue to work.

## Data model

Trust is stored alongside the existing site-scoped client identity record: MAC, label, reason, actor, created/updated timestamps, and optional expiration. The MAC is a lookup key, not an authentication factor.

## Verification

- Go unit tests for identity normalization, fallback labels, MAC/IP annotation, and trust expiry.
- Handler/database tests for trust validation and site scoping.
- Frontend tests for rendering labels/trust state and issuing trust/untrust requests.
- `go test ./...`, `go vet ./...`, `npm test -- --run`, and `npm run build`.
