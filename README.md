# 📡 openwrt-controller: The Nerve Center \[V2.0 - Extended\]

Plaintext

```
  _   _                       _____           _            
 | \ | |                     / ____|         | |           
 |  \| | ___ _ ____   _____ | |     ___ _ __ | |_ ___ _ __ 
 | . ` |/ _ \ '__\ \ / / _ \| |    / _ \ '_ \| __/ _ \ '__|
 | |\  |  __/ |   \ V /  __/| |___|  __/ | | | ||  __/ |   
 |_| \_|\___|_|    \_/ \___| \_____\___|_| |_|\__\___|_|   
 [ SYSTEM_STATUS: OMEGA_ACTIVE ] [ OVERLAY: WIREGUARD_ENCRYPTED ]
```

**openwrt-controller** is an industrial-grade Command & Control (C&C) system designed for managing OpenWrt fleets operating in geographically dispersed and connectivity-challenged environments. It bridges the gap between simple monitoring and full-scale SD-WAN orchestration.

* * *

## 🏗️ Ecosystem Architecture

The system operates on a **Three-Tier Tactical Model**:

### 1\. The Core (Backend - Go)

-   **High Concurrency:** Leverages Go's `sync.WaitGroups` and channels for parallel SSH execution across hundreds of nodes without blocking the main event loop.
-   **Cryptography:** Implements native `X25519` curves for the WireGuard overlay and `Ed25519` for terminal identities.
-   **Middleware:** Robust JWT (JSON Web Token) authentication system with automatic key rotation and role-based access control.

### 2\. The Grid (Data Lake - Influx & Postgres)

-   **Time-Series Engine:** InfluxDB 2.x processes high-frequency telemetry (signal, noise floor, throughput).
-   **Relational Layer:** PostgreSQL 16 handles persistence for device inventory, site profiles, and incident auditing.
-   **Forensic Search:** Implements a GIN (Generalized Inverted Index) on the logs table for sub-millisecond full-text searches.

### 3\. The Shadow Agent (Node - agent.sh)

-   **Efficiency:** Written in pure `ash` (BusyBox), optimized for low-resource hardware (e.g., WNDR3700 with 32MB RAM).
-   **Resilience:** Native `procd` process management with a built-in watchdog for self-healing.

* * *

## 🛰️ Detailed Module Breakdown

### 🔒 \[SECURE\_TUNNEL\] - Cryptographic SD-WAN

Creates an encrypted Overlay Network between the controller and the fleet.

-   **Protocol:** WireGuard.
-   **Addressing:** Static IP pool on the `10.8.0.0/24` subnet.
-   **Direct Access:** Point-to-point tunneling allowing direct access to local LuCI web interfaces without port forwarding.
-   **Auto-Provisioning:** The agent detects missing `wireguard-tools` and self-installs them via `apk/opkg` upon receiving the VPN profile.

### 📈 \[LOG\_HARVESTER\] - Forensic Intelligence

Centralizes the `logread` stream from the entire fleet for remote analysis.

-   **Pattern Parser:** Heuristic algorithms to classify severity levels (Critical, Error, Warning, Info).
-   **String Sanitization:** Advanced character escaping for injecting complex system logs into JSON without buffer corruption.
-   **Alert Integration:** Directly linked to the incident engine; a "Kernel Panic" in a remote node triggers an instant notification.

### 🔄 \[AGENT\_UPDATE\_SERVICE\] - Code Evolution

Enables fleet-wide telemetry logic updates without physical or manual intervention.

-   **Mechanism:** Raw-API binary/script pulling with SHA256 checksum verification.
-   **A/B Partitioning:** Maintains a functional `.old` copy on the router.
-   **Self-Healing Rollback:** If the new script fails to report successful telemetry three consecutive times, the router restores the previous version and restarts the service.

### ☢️ \[RF\_INTELLIGENCE\] - Spectrum Optimization

Heuristic analysis of the wireless physical layer.

-   **SNR Analysis:** Dynamic calculation of the Signal-to-Noise Ratio for every connected client.
-   **Remediation:** `RF_FIX` logic that suggests or executes channel reassignments (1, 6, 11) based on the lowest reported noise floor.

### 🛡️ \[THE\_VAULT\] - Configuration Integrity

A secure repository for `/etc/config` files with guaranteed integrity.

-   **Snapshots:** Daily backups compressed in `.tar.gz` and stored with SHA256 hashes.
-   **Visual Diff Engine:** A monospaced code viewer highlighting changes in Firewall, Network, or WiFi settings between specific dates.

* * *

## 📑 Tactical Operations Guide

### Provisioning a New Node

1.  **Handshake:** The router contacts the controller via the Provisioning API.
2.  **Identity:** The controller generates WireGuard keys and assigns a `10.8.0.x` IP.
3.  **Injection:** The agent downloads the site profile, installs dependencies, and raises the `wg0` interface.

### Emergency Recovery Procedure

1.  **Observation:** Check the **Global Health Score** on the Dashboard sidebar.
2.  **Isolation:** Use the **Log Explorer** to search for synchronization errors or auth failures.
3.  **Access:** Click **\[OPEN\_LUCI\]** to enter the router via its internal Tunnel IP.
4.  **Recovery:** If configurations are corrupted, apply a `Restore` from **The Vault**.

* * *

## 🌲 Infrastructure Context (Pallatanga)

The system is optimized for the user's specific environmental constraints:

-   **Energy Efficiency:** Low-impact notifications to avoid unnecessary radio wake-ups in secondary nodes.
-   **Connectivity:** Extreme tolerance for high latency and micro-outages common in mountain radio links.
-   **Security:** Total isolation of the corporate sites via centralized firewall orchestration.

* * *

**Status:** `READY_FOR_PRODUCTION` **Binary:** `v2.0.4-omega` **Maintainer:** Sebastián Real

