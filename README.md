# OMEGA OpenWrt Controller

[![CI](https://github.com/tuxevil/openwrt-controller/actions/workflows/ci.yml/badge.svg)](https://github.com/tuxevil/openwrt-controller/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8.svg?logo=go&logoColor=white)](go.mod)
[![Vue](https://img.shields.io/badge/Vue-3-42b883.svg?logo=vuedotjs&logoColor=white)](web/package.json)

OMEGA is a self-hosted control plane for OpenWrt fleets. It combines declarative site configuration, tenant isolation, device telemetry, secure SSH operations, topology visibility and staged rollout safeguards in one Go backend and Vue dashboard.

> **Status: Beta.** The project is suitable for controlled deployments. API and database contracts may change before a stable release. Read the [security checklist](SECURITY.md) before exposing a controller publicly.

## What It Provides

- **Fleet control:** site templates rendered by device role, UCI import, WLAN roaming settings, DHCP, DNS, firewall, SQM and SD-WAN.
- **Safe operations:** read-only previews, synchronous Vault backups, explicit rollout confirmation, health checks and rollback-protected UCI batches.
- **Device lifecycle:** first enrollment with a site key, per-device token handoff, telemetry heartbeat and procd-based agent operation.
- **Visibility:** CPU, memory, link and client telemetry, Starlink health, configuration drift, mesh benchmarks and hierarchical topology.
- **Security boundaries:** PostgreSQL schema isolation per tenant, JWT/RBAC, audited privileged actions, SSH host-key TOFU and constrained UCI command namespaces.
- **Optional services:** WireGuard mesh, Tailscale/Headscale, FreeRADIUS, Threat Shield, packet capture, iperf3, webhooks and OpenAI-compatible AI analysis.

## Architecture At A Glance

```text
Vue SPA  <---- REST / WebSocket / JWT ---->  Go controller
                                               |-- PostgreSQL: tenants, sites, desired state
                                               |-- InfluxDB: telemetry and benchmarks
                                               |-- FreeRADIUS (optional)
                                               |-- SSH / WireGuard / HTTPS
                                                        |
                                             OpenWrt devices + nerve-agent
```

The production backend can run as a native systemd service. Docker Compose is the supported local and all-in-one deployment path. See [architecture](docs/ARCHITECTURE.md) for boundaries and data flows.

## Quick Start

### Requirements

- Linux host
- Docker Engine and Docker Compose v2
- Network reachability from OpenWrt devices to the controller
- OpenSSL for generating local secrets

### Start The Local Stack

```bash
git clone https://github.com/tuxevil/openwrt-controller.git
cd openwrt-controller
cp .env.example .env
```

Set strong values for `POSTGRES_PASSWORD`, `INFLUXDB_PASSWORD`, `INFLUX_TOKEN` and `JWT_SECRET` in `.env`, then:

```bash
chmod 600 .env
docker compose --env-file .env up -d
docker compose logs -f openwrt-controller
```

Open `http://localhost:3000`. The initial administrator password is generated and logged on first boot unless `SUPERADMIN_DEFAULT_PASSWORD` is set.

### Adopt A Device

1. Create a site in the dashboard and copy its site key.
2. Install the agent and configure its controller URL and site key, or bake `devices/99-nerve-center-bootstrap` into an OpenWrt Image Builder profile.
3. Start the procd agent on the device.
4. Adopt the pending device in the dashboard.

The first configuration pull transfers a per-device token. Later configuration and telemetry requests use that token. Follow the [device agent guide](docs/DEVICE_AGENT.md) for exact field and endpoint behavior.

## Documentation

The README is intentionally an entry point. Detailed operational material lives in `docs/`:

| Guide | Purpose |
|---|---|
| [Documentation index](docs/README.md) | Map of project documentation and source references |
| [Architecture](docs/ARCHITECTURE.md) | Components, tenant boundaries and data flows |
| [Deployment](docs/DEPLOYMENT.md) | Docker, native systemd and production preparation |
| [Operations](docs/OPERATIONS.md) | Preview, backup, rollout, rollback and health procedures |
| [Security model](docs/SECURITY_MODEL.md) | Authentication, authorization, secrets and threat boundaries |
| [Device agent](docs/DEVICE_AGENT.md) | Enrollment, tokens, telemetry and OpenWrt installation |
| [Testing](docs/TESTING.md) | Quality gates, integration tests and safe local verification |
| [API reference](openapi.yaml) | OpenAPI 3.1 endpoint contract |
| [Contributing](CONTRIBUTING.md) | Development workflow and repository conventions |
| [Security policy](SECURITY.md) | Private vulnerability reporting and operator checklist |

## Development

```bash
go mod download
go test -race -count=1 ./...
go vet ./...
go build -o /tmp/openwrt-controller ./cmd/openwrt-controller

npm --prefix web ci
npm --prefix web test -- --run
npm --prefix web run build
```

Run the full contract checks before opening a change:

```bash
docker compose config --quiet
sh -n devices/agent.sh devices/99-nerve-center-bootstrap
devices/agent_contract_test.sh
devices/agent_log_collection_test.sh
git diff --check
```

Database migration tests are opt-in and use a disposable database URL:

```bash
DATABASE_URL_TEST=postgres://... OPENWRT_INTEGRATION_DB=1 \
  go test ./internal/database -run Migration -count=1
```

## Project Layout

```text
cmd/                    Executables; openwrt-controller is the production binary
internal/api/           HTTP routes, handlers, middleware and SPA serving
internal/database/      PostgreSQL migrations/queries and InfluxDB client
internal/orchestrator/  SSH, host keys, UCI execution and VPN operations
internal/services/      Desired state, telemetry, benchmarks and workers
internal/models/        Shared domain structures
web/                    Vue 3 + Vite dashboard
devices/                OpenWrt agent and first-boot bootstrap
docs/                   Detailed project documentation
```

## Support And Contributions

Use [GitHub Issues](https://github.com/tuxevil/openwrt-controller/issues) for reproducible non-security bugs. Include the controller commit, OpenWrt versions, relevant controller logs and device `logread` output.

Do not publish vulnerability details in an issue. Use the private process in [SECURITY.md](SECURITY.md).

Contributions should use focused commits, add tests for business logic and add forward-compatible migrations for schema changes. The project is released under the [MIT License](LICENSE).
