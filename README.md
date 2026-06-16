# OMEGA Central Controller (Nerve Center CE)

[![CI](https://github.com/tuxevil/openwrt-controller/actions/workflows/ci.yml/badge.svg)](https://github.com/tuxevil/openwrt-controller/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8.svg?logo=go&logoColor=white)](go.mod)
[![Vue 3](https://img.shields.io/badge/vue-3.5-35495e.svg?logo=vuedotjs&logoColor=%234FC08D)](web/package.json)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-25.x-1B828C.svg?logo=openwrt&logoColor=white)](https://openwrt.org/)

An industrial-grade, open-source fleet controller for OpenWrt devices — a libre alternative to TP-Link Omada, Ubiquiti UniFi, and Cisco Meraki.

A single Go binary + Vue dashboard orchestrates dozens or hundreds of OpenWrt routers/APs over SSH, WireGuard, and signed HTTPS, with multi-tenant isolation, zero-touch provisioning, automated VPN mesh, FreeRADIUS sidecar, and AI-driven anomaly detection.

> **Status:** Beta. Used in production by the maintainer; API and DB schema may change. See [SECURITY.md](SECURITY.md) before exposing to the public internet.

---

## Why this exists

Vendor controllers lock you into their hardware. OpenWrt runs on hundreds of router models from dozens of vendors, but managing more than ~5 devices manually via LuCI/SSH does not scale. This project bridges that gap: keep your hardware freedom, get an enterprise-grade control plane.

| You have... | You get... |
|---|---|
| 50 OpenWrt APs across 10 client sites | One dashboard, declarative site templates, per-tenant isolation |
| A new device to deploy in the field | Flash with Image Builder + `99-nerve-center-bootstrap` → auto-adopts on first boot |
| Two sites that need a site-to-site VPN | One click → WireGuard keys generated, peers configured, firewall opened |
| Suspicious traffic on a remote AP | Real-time `nf_conntrack` flows + threat-intel firewall + local LLM analysis |

---

## Key Features

### Network orchestration

- **Zero-Touch Provisioning (ZTP)** — Devices auto-adopt on first boot using a site API key embedded in `/etc/uci-defaults/`
- **Unified Site Settings** — One pane for WLANs, DHCP, DNS, firewall, port-forwarding, 802.11r/k/v, SD-WAN failover; rendered per device role (Gateway / AP / IoT)
- **VPN Mesh** — Automated WireGuard Hub-and-Spoke or Full-Mesh; ed25519 key rotation; UCI injection via SSH
- **Zero Trust overlay** — Native Tailscale/Headscale support with pre-auth keys
- **Central UCI** — LuCI-grade configuration editor for *any* UCI namespace, with atomic `uci batch` snapshots and automatic rollback on syntax error

### Security & identity

- **FreeRADIUS sidecar** — PostgreSQL-backed `radcheck`/`radreply`, WPA2/3-Enterprise, dynamic VLAN assignment via `Tunnel-Private-Group-Id`, MAB for IoT
- **Threat Shield (IPS)** — Cron-merged blocklists (Firehol L1, Spamhaus DROP, Emerging Threats) injected as `nftables` sets; per-site toggle
- **Sentinel AI** — Local Ollama integration analyzes fleet logs in rolling 2-min windows for coordinated lateral movement
- **Multi-tenant RBAC** — SUPERADMIN / ADMIN / OPERATOR / VIEWER; PostgreSQL schema-level isolation per tenant
- **Audit log** — Every API call, mass command, and TTY transcript persisted (El Panóptico)

### Telemetry & visibility

- **Real-time dashboards** — CPU, RAM, uptime, DHCP leases, signal/noise, per-interface bps from a lightweight `ash` agent
- **InfluxDB time-series** — 1s telemetry resolution; signal heatmaps; top talkers
- **Flow Radar** — Zero-overhead `awk` parsing of `/proc/net/nf_conntrack`; backend enriches with threat-intel
- **Echolocation** — L2 topology auto-discovery via bridge/ARP/LLDP; D3 force-directed graph
- **Geo mapping** — Leaflet-based site map with lat/lon
- **WIDS/WIPS** — 802.11w MFP, deauth-attack mitigation, RF channel optimization

### Operations

- **Matrix Shell** — Browser SSH over WebSockets to any node (xterm.js); full session recording
- **Diagnostics** — Remote `tcpdump` (OOM-protected, RAM-rotated) and `iperf3` from the UI; `.pcap` download
- **The Vault** — Periodic `sysupgrade --create-backup`, integrity diffs, compliance auditor via LLM
- **Bandwidth Sentry** — Surgical per-MAC `nftables` rate-limiting with auto-GC and AI-triggered penalties
- **DPI & SQM** — CAKE bufferbloat elimination + `iptables-mod-ndpi` for BitTorrent/P2P blocking
- **Omada Migrator** — Drag-and-drop import of `omada_export.json` (DHCP reservations, port forwards) into site templates

### Integrations

- **Webhooks** — Signed (HMAC-SHA256) HTTP callbacks on incidents/node-down
- **ChatOps RAG** — Natural-language `~` terminal: *"Which device used the most bandwidth last night at site Alpha?"* → SQL/Influx query → ASCII table
- **Billing API** — Per-tenant aggregate (sites × devices) for MSP/MSSP invoicing

---

## Architecture

```text
                    ┌──────────────────┐
                    │   Vue Dashboard  │  (single-page, served from Go on :3000)
                    └────────┬─────────┘
                             │ REST + WebSocket + JWT
                    ┌────────▼─────────┐
                    │  Go Backend      │
                    │  (cmd/openwrt-   │      ┌──────────────┐
                    │   controller)    ├─────►│  PostgreSQL  │  ← Sites, devices, configs,
                    │                  │      │   (per-tenant │     RBAC, vouchers, vault
                    │  - HTTP API      │      │    schema)   │
                    │  - SSH executor  │      └──────────────┘
                    │  - WG keygen     │      ┌──────────────┐
                    │  - LLM bridge    ├─────►│  InfluxDB    │  ← Time-series telemetry
                    │  - Threat-intel  │      └──────────────┘
                    │    cron          │      ┌──────────────┐
                    │                  ├─────►│  FreeRADIUS  │  ← WPA-Enterprise, VLAN
                    └────┬─────┬───────┘      └──────────────┘     steering, MAB
                         │     │
                  SSH/UCI│     │HTTPS+X-Site-Key
                         │     │
                   ┌─────▼─────▼──────────────────────────┐
                   │      OpenWrt fleet (any model)       │
                   │                                       │
                   │   /usr/sbin/nerve-agent.sh (pure ash) │
                   │      ↑ self-updates from controller   │
                   │                                       │
                   │  procd init  •  WireGuard overlay     │
                   │  nftables    •  uhttpd + rpcd         │
                   └───────────────────────────────────────┘
```

---

## Quick Start (5 minutes)

### Prerequisites
- Linux host with Docker + Docker Compose v2
- A public or VPN-reachable IP for the controller (devices need to dial home)

### 1. Clone & configure

```bash
git clone https://github.com/tuxevil/openwrt-controller.git
cd openwrt-controller

# Generate strong secrets — JWT_SECRET MUST be ≥ 32 chars
cat > .env <<EOF
JWT_SECRET=$(openssl rand -base64 48)
POSTGRES_PASSWORD=$(openssl rand -base64 24)
INFLUXDB_PASSWORD=$(openssl rand -base64 24)
INFLUX_TOKEN=$(openssl rand -hex 32)
EOF
chmod 600 .env
```

### 2. Launch the stack

```bash
docker compose --env-file .env up -d
docker compose logs -f openwrt-controller   # watch for "Bootstrap SUPERADMIN user created!"
```

The bootstrap password is printed once. Copy it.

### 3. Open the dashboard

```text
http://localhost:3000
# Login: admin / <bootstrap password from logs>
```

### 4. Adopt your first device

1. **Dashboard → Sites → New Site** — note the generated `SITE_KEY`
2. On the OpenWrt device, install the agent (one command):
   ```sh
   wget -O /usr/sbin/nerve-agent.sh \
     "http://YOUR_CONTROLLER_IP:3000/api/agent/latest/raw" \
     --header="X-Site-Key: YOUR_SITE_KEY"
   chmod +x /usr/sbin/nerve-agent.sh
   sed -i 's|TU_API_KEY_AQUI|YOUR_SITE_KEY|;s|REPLACE_WITH_CONTROLLER_IP|YOUR_CONTROLLER_IP|' \
     /usr/sbin/nerve-agent.sh
   ```
3. Register the procd service (see [`devices/agent`](devices/agent)) and `start`.
4. Within 30 s the device appears in the dashboard as `Pending` → click **Adopt**.

For mass deployments, bake [`devices/99-nerve-center-bootstrap`](devices/99-nerve-center-bootstrap) into your OpenWrt Image Builder profile and devices will adopt themselves on first boot.

---

## Hardening before going public

The default `docker-compose.yml` exposes Postgres (`5432`) and InfluxDB (`8086`) on `0.0.0.0` for ease of development. Before exposing the controller to the internet:

1. Bind databases to `127.0.0.1` in `docker-compose.yml`
2. Put the controller behind TLS (Traefik / Caddy / nginx)
3. Run through the operator checklist in [SECURITY.md](SECURITY.md)
4. Rotate `api_key` on every site after first boot
5. Subscribe to repository security advisories

---

## Development

### Backend

```bash
go mod download
go vet ./cmd/openwrt-controller/... ./internal/...
go build -o /tmp/oc ./cmd/openwrt-controller
JWT_SECRET=$(openssl rand -base64 48) /tmp/oc
```

The backend serves the prebuilt Vue SPA from `web/dist/` if present, otherwise the API only.

### Frontend

```bash
cd web
npm ci
npm run dev          # Vite dev server with HMR on :5173 (proxies API to :3000)
npm run build        # production build to web/dist/
```

### Database migrations

PostgreSQL migrations run automatically on startup. New tables go in `internal/database/postgres.go::createLandlordTables()` (global) or `RunTenantMigrations()` (per-tenant schema). InfluxDB buckets and tasks are declared in `internal/database/influx.go`.

### Project layout

```text
.
├── cmd/
│   └── openwrt-controller/    Entry point of the Go binary
├── internal/
│   ├── api/
│   │   ├── handlers/          ~50 HTTP handlers, one file per domain
│   │   ├── middleware/        JWT auth, tenant context, RBAC
│   │   └── routes.go          net/http ServeMux v1.22+ routing
│   ├── database/              Postgres init, queries, InfluxDB client
│   ├── models/                Shared structs (Device, Site, User, WLAN…)
│   ├── orchestrator/          SSH executor, VPN mesh engine, UCI bridge
│   └── services/              Background workers (Sentinel, Sniper, Threat Intel…)
├── web/                       Vue 3 + Vite + Tailwind frontend
│   └── src/views/             One Vue SFC per dashboard route
├── devices/
│   ├── agent.sh               OpenWrt agent (pure ash, ~600 lines)
│   ├── agent                  procd init script
│   └── 99-nerve-center-bootstrap   uci-defaults first-boot script
├── docker-compose.yml         All services for local/Coolify deployment
├── Dockerfile                 Multi-stage build (Vue + Go → alpine, ~50 MB)
└── .github/workflows/ci.yml   Build/test on every push & PR
```

---

## Deployment

| Target | How |
|---|---|
| **Local Docker** | `docker compose --env-file .env up -d` |
| **Coolify** | See [COOLIFY.md](COOLIFY.md) — both all-in-one and split-services flows |
| **Bare-metal systemd** | `go build` + place binary, `.env`, and an `EnvironmentFile=` systemd unit. Sample unit:<br>`[Service]`<br>`ExecStart=/opt/nerve-center/openwrt-controller`<br>`EnvironmentFile=/opt/nerve-center/.env`<br>`Restart=always` |
| **Kubernetes** | Not officially supported yet — PRs welcome |

---

## Roadmap

- [ ] OpenWrt 24.10 LTS package signing verification
- [ ] Per-tenant data export / GDPR delete
- [ ] Native ARM64 binaries for Apple Silicon dev hosts
- [ ] Helm chart
- [ ] Tailscale OAuth (replacing pre-auth keys)
- [ ] Built-in Grafana dashboards as JSON
- [ ] Mobile-friendly dashboard layout

---

## Contributing

PRs welcome. Before opening one:

1. Run the CI locally: `go vet ./cmd/openwrt-controller/... ./internal/...` and `cd web && npm run build`
2. Add or update tests for any business logic change
3. If you change the DB schema, add a migration in `internal/database/postgres.go` — never break existing tenant schemas
4. Sign your commits if you can (`git commit -S`)
5. For anything security-sensitive, follow [SECURITY.md](SECURITY.md) instead of opening a public issue

### Reporting bugs

Use [GitHub Issues](https://github.com/tuxevil/openwrt-controller/issues) for non-security bugs. Please include:
- Controller version (`git rev-parse --short HEAD`)
- OpenWrt version of the affected device(s)
- Relevant logs from `journalctl -u openwrt-controller` and `logread` on the device

### Reporting vulnerabilities

See [SECURITY.md](SECURITY.md). **Do not** open a public issue.

---

## Acknowledgments

Built on the shoulders of giants:

- [OpenWrt](https://openwrt.org/) — the firmware that makes any of this possible
- [pgx](https://github.com/jackc/pgx), [gorilla/websocket](https://github.com/gorilla/websocket), [golang-jwt](https://github.com/golang-jwt/jwt), [crypto/ssh](https://pkg.go.dev/golang.org/x/crypto/ssh)
- [Vue.js](https://vuejs.org/), [Vite](https://vitejs.dev/), [TailwindCSS](https://tailwindcss.com/), [xterm.js](https://xtermjs.org/), [v-network-graph](https://github.com/dash14/v-network-graph), [Chart.js](https://www.chartjs.org/)
- [FreeRADIUS](https://freeradius.org/), [InfluxDB](https://www.influxdata.com/), [PostgreSQL](https://www.postgresql.org/)
- Threat intelligence feeds: [Firehol](https://iplists.firehol.org/), [Spamhaus](https://www.spamhaus.org/drop/), [Emerging Threats](https://rules.emergingthreats.net/)

---

## License

MIT — see [LICENSE](LICENSE).

Built and maintained by [Sebastian Real](https://github.com/tuxevil).
