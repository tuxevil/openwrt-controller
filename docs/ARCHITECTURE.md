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
4. Mutating operations validate identifiers, write an audit event and use the SSH/UCI boundary.
5. Device operations resolve the device inside the authorized tenant, verify its host key and execute a constrained script.

The shipped agent reads a complete `CONTROLLER_URL` from root-owned runtime configuration. `REQUIRE_TLS=true` rejects plain HTTP before any controller request; configure a CA file or curl public-key pin when the controller uses a private PKI.

## Desired State

`site_configs` stores a site template. `RenderSiteConfig` turns that template into role-aware UCI commands for Gateway, AP and other supported roles. The controller can preview those commands before applying them.

The rollout path is deliberately explicit:

```text
desired config -> preview -> operator confirmation -> backup -> UCI batch
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
