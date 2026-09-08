# Architecture

## Runtime Components

| Component | Responsibility | Primary interface |
|---|---|---|
| Go controller | API, authentication, orchestration and workers | HTTP(S), WebSocket, SSH |
| Vue SPA | Dashboard and operator workflows | REST API and WebSocket tickets |
| PostgreSQL | Users, tenants, sites, desired state, audit and vault metadata | Tenant schemas plus `public` |
| InfluxDB | Device telemetry, signal history and benchmark measurements | Influx line protocol and Flux |
| OpenWrt agent | Heartbeat, configuration pull, telemetry and local safeguards | HTTPS with a configured CA or public-key pin; explicit HTTP compatibility only |
| FreeRADIUS | Optional WPA-Enterprise and VLAN identity service | RADIUS, PostgreSQL |

## Request Flow

1. The browser authenticates with JWT and selects a site or tenant context.
2. Middleware validates the token, role and tenant schema before the handler runs.
3. Read operations query the tenant schema or InfluxDB.
4. Mutating operations validate identifiers, write an audit event and queue typed device operations; fleet orchestration persists an immutable rollout draft before execution. The safe single-device `system` slice derives a `DeviceChangeSet` and delivers it through config pull instead of SSH.
5. Device operations resolve the device inside the authorized tenant, verify its host key and execute a constrained script.

The shipped agent reads a complete `CONTROLLER_URL` from root-owned runtime configuration. `REQUIRE_TLS=true` rejects plain HTTP before any controller request; configure a CA file or curl public-key pin when the controller uses a private PKI.

## Device Operation Identity

Each typed device operation has three independent identities:

- `generation` identifies the desired-state revision for that device.
- `plan_hash` identifies the immutable command content.
- `operation_id` identifies one application attempt.

`DeviceChangeSet` adds a stable logical `change_set_id` around its ordered
namespace entries. Its content `plan_hash` excludes the attempt and device
generation, while the device-local generation is reserved atomically when the
changeset is queued. Changeset progress is reported in the separate
`change_set_transaction` telemetry envelope so it cannot hide standalone
operation status.

`QueueDeviceOperation` reserves an unbound plan's next device generation in the same database update that writes `pending_operation`. Generation-bound retries must name the current revision; stale or future revisions are rejected. Agent status echoes the generation when present, and the controller advances observed generations only for a matching `COMMITTED` status. Legacy agents may omit the generation during rollout, but their operation and plan identities must still match; such a status does not advance generation columns.

Fleet rollouts have a separate site-scoped sequence in `rollout_runs`. The
legacy SSH/UCI path uses that sequence only for ordering, audit and draft
fencing; it does not write any device generation column. Device generations
are reserved and advanced only by typed device operations.

## Desired State

`site_configs` stores a site template. `RenderSiteConfig` turns that template into role-aware UCI commands for Gateway, AP and other supported roles. The controller can preview those commands before applying them.

The rollout path is deliberately explicit. Preview is the point at which the
rendered commands and read-only UCI observations are persisted; apply cannot
re-render the site template or silently target a different device set.

```text
 desired config -> preview/draft -> operator confirmation -> stale check -> backup -> UCI batch
                                                      -> health check
                                                      -> success or rollback
```

## Tenant Boundary

Landlord data lives in `public`. Tenant-owned operational data lives in a validated PostgreSQL schema derived from the tenant alias. Handlers must obtain the schema from authenticated context or a validated site key; request headers are not trusted by themselves.

## Source References

- Routes: `internal/api/routes.go`
- Tenant middleware: `internal/api/middleware/`
- Database DDL and migrations: `internal/database/postgres.go`
- Desired state: `internal/services/orchestrator_engine.go`
- SSH host keys: `internal/orchestrator/hostkey.go`
- UCI safety boundary: `internal/services/uci_bridge.go`
