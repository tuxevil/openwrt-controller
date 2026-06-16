# OMEGA Central Controller (Nerve Center CE)

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/vuejs-%2335495e.svg?style=for-the-badge&logo=vuedotjs&logoColor=%234FC08D)
![OpenWrt](https://img.shields.io/badge/OpenWrt-1B828C.svg?style=for-the-badge&logo=openwrt&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-316192.svg?style=for-the-badge&logo=postgresql&logoColor=white)
![InfluxDB](https://img.shields.io/badge/InfluxDB-22ADF6.svg?style=for-the-badge&logo=influxdb&logoColor=white)

**OMEGA Central Controller (Nerve Center CE)** is a next-generation, open-source fleet management controller designed exclusively for OpenWrt devices. Built on an Open Core model, it provides advanced enterprise networking features directly from a centralized, highly concurrent orchestrator.

Whether you are managing a single site or a globally distributed fleet, OMEGA Central Controller enables unified configuration, advanced telemetry, deep security, and zero-touch provisioning—all through a modern, responsive single-pane-of-glass interface.

## 🌟 Key Features

### 📡 Network Orchestration & Management
- **Zero-Touch Provisioning (ZTP):** Auto-adopt and configure remote OpenWrt devices simply by bringing them online.
- **Unified Site Management:** Group devices by physical or logical sites. Manage global and site-specific configurations (WLAN, DHCP, DNS, Network interfaces) from a single interface.
- **VPN Mesh Orchestration:** Fully automated deployment and configuration of WireGuard-based Hub-and-Spoke and Full-Mesh topologies (`vpn_mesh`). Also supports Zero Trust micro-segmentation with Tailscale/Headscale integration.
- **SD-WAN & Multi-WAN:** Robust link failover, load balancing (mwan3), and dynamic path selection to maximize uptime.
- **Wireless & RF Intelligence:** Centralized SSID management, WPA-Enterprise (802.1X), Dynamic VLAN Steering, and 802.11w MFP.

### 🛡️ Security & Threat Intelligence
- **Integrated Identity Matrix (FreeRADIUS):** A multi-tenant RADIUS server running as a sidecar to authenticate WPA-Enterprise users seamlessly.
- **Threat Shield & Sniper Reaper:** Active monitoring and automated mitigation of network anomalies and unauthorized intrusion attempts.
- **Captive Portal:** Hospitality-grade guest WiFi gateway using FAS with openNDS.
- **Bandwidth Sentry & Smart QoS:** Eliminate bufferbloat using CAKE SQM algorithms, with deep packet inspection capabilities.

### 📊 Advanced Telemetry & Visibility
- **Real-Time Panopticon:** Monitor CPU, RAM, uptime, DHCP clients, interface traffic, and wireless signal strength across the entire fleet in real time.
- **Time-Series Metrics:** Native InfluxDB integration ensures high-resolution data retention for historical analysis and trend prediction.
- **Top Talkers & Flow Radar:** Pinpoint bandwidth hogs instantly with localized traffic analysis (`nDPI` / Flow Sense).
- **Echolocation & Geospatial Tracking:** Topographical node mapping across deployments using interactive maps (Leaflet/Vue).
- **Dynamic Topology Mapping:** Visual graph-based representations of L2/L3 topologies.

### 🛠️ Diagnostics & Troubleshooting
- **Remote Terminal:** Web-based CLI (xterm.js) directly into any managed OpenWrt device via secure tunnels.
- **On-Demand Diagnostics:** Execute UI-driven packet captures (tcpdump) and speed tests (iperf3) directly from the browser.
- **UCI Ops:** Direct interaction with OpenWrt's Unified Configuration Interface (UCI) from the centralized dashboard.

### 🏢 Administration & Integration
- **Multi-Tenancy & RBAC:** Granular Role-Based Access Control allowing multiple tenants (Landlords) to safely manage their own sites.
- **Webhooks & ChatOps:** Built-in external notifications for node status changes, alert thresholds, and incident reporting.
- **Billing APIs:** Aggregated usage metrics designed to integrate with external billing or ISP management platforms.
- **Omada Migrator:** Tooling to seamlessly transition devices from TP-Link Omada to OMEGA.

---

## 🏗️ Architecture & Tech Stack

### Backend
- **Language:** Go 1.25 (High concurrency, extremely low footprint)
- **Database (Relational):** PostgreSQL 15 (Sites, Devices, Users, Settings via `pgx`)
- **Database (Time-Series):** InfluxDB 2.7 (High-frequency telemetry, device metrics)
- **Authentication:** JWT & PBKDF2 cryptography
- **Frameworks/Libs:** Gorilla WebSockets, Google UUID, Crypto

### Frontend
- **Framework:** Vue.js 3 (Composition API)
- **Tooling:** Vite, TailwindCSS (for the Vantablack-themed UI)
- **Visualizations:** Chart.js, v-network-graph, D3.js
- **Emulation:** xterm.js (Web-based SSH/CLI emulation)

---

## 🚀 Quickstart Deployment

You can deploy the complete Nerve Center CE stack in minutes using Docker Compose.

### 1. Clone the repository
```bash
git clone https://github.com/examplecorp/nerve-center-ce.git
cd nerve-center-ce
```

### 2. Configure the environment
```bash
cp .env.example .env
# Edit .env and set secure passwords and secrets (JWT_SECRET, Postgres credentials, InfluxDB tokens)
```

### 3. Start the services
```bash
docker-compose up -d
```
The stack spins up the following containers:
- `openwrt_controller`: The central Go API and web server (Port: `3000`)
- `openwrt_postgres`: PostgreSQL database (Port: `5432`)
- `openwrt_influx`: InfluxDB telemetry datastore (Port: `8086`)
- `openwrt_radius`: FreeRADIUS server with PostgreSQL integration (Ports: `1812/udp`, `1813/udp`)

Once running, access the Unified Control Panel at `http://localhost:3000`.

---

## 💻 Development Guide

If you wish to contribute or run the controller in development mode:

### Backend (Go)
```bash
# Install Go dependencies
go mod download

# Run the server
go run cmd/openwrt-controller/main.go
```
The backend will expect Postgres and InfluxDB to be reachable as defined in your `.env` file.

### Frontend (Vue.js)
```bash
cd web

# Install Node dependencies
npm install

# Start the Vite dev server with HMR
npm run dev
```

---

## 📁 Repository Structure

```text
.
├─── cmd/
│   └── openwrt-controller/  # Main application entrypoint
├─── internal/
│   ├��─ api/                 # HTTP Handlers, Routers, Middleware
│   ├─── database/            # Postgres & InfluxDB initialization/queries
│   ├─── models/              # Core domain data structures
│   └── services/            # Background workers (Alerts, Sniper Reaper, Threat Intel)
├─── web/                     # Vue.js Frontend Application
│   ├─── src/
│   │   ├─── components/      # Reusable UI components
│   │   └── views/           # Full page layouts (e.g., Topology, Telemetry, Orchestrator)
│   └── package.json
├─── docker-compose.yml       # Production/Staging deployment definition
├─── Dockerfile               # Multi-stage build for Go and Vue
└── go.mod                   # Go dependencies
```

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

*Powered by Go, Vue, and a passion for open networking.*
