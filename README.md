# 📡 openwrt-controller: The Nerve Center \[V2.5 - Omega Sniper\]

```
  _   _                       _____           _            
 | \ | |                     / ____|         | |           
 |  \| | ___ _ ____   _____ | |     ___ _ __ | |_ ___ _ __ 
 | . ` |/ _ \ '__\ \ / / _ \| |    / _ \ '_ \| __/ _ \ '__|
 | |\  |  __/ |   \ V /  __/| |___|  __/ | | | ||  __/ |   
 |_| \_|\___|_|    \_/ \___| \_____\___|_| |_|\__\___|_|   
 [ SYSTEM: OMEGA_ACTIVE ] [ AI: SENTINEL_ONLINE ] [ SHAPING: LETHAL ]
```

**openwrt-controller** is an industrial-grade, AI-driven Command & Control (C&C) system designed for managing distributed OpenWrt fleets. Engineered for geographically challenging environments and off-grid deployments, it bridges the gap between passive monitoring and autonomous, surgical network defense.

* * *

## 🏗️ Ecosystem Architecture

The system operates on a **Four-Tier Tactical Model**:

### 1\. The Core (Backend - Go)

-   **High Concurrency:** Leverages Go `sync.WaitGroups` for parallel, non-blocking SSH execution across the fleet.
-   **Cryptography:** Native `X25519` curves for WireGuard overlay and `Ed25519` for terminal identities.
-   **Idempotent Execution:** Atomic orchestrator capable of evaluating target kernel capabilities (e.g., `nftables` support) before command injection.

### 2\. The Intelligence Layer (Sentinel AI)

-   **Local LLM Integration:** Hooks directly into a local Ollama instance (llama3/mistral).
-   **Fleet-Wide Correlation:** Analyzes logs across all nodes within a rolling ±2 minute window to detect coordinated lateral movements.
-   **Autonomous Defense:** AI is authorized to execute preemptive strikes (e.g., bandwidth throttling) when specific threat vectors are identified.

### 3\. The Grid (Data Lake - Influx & Postgres)

-   **Time-Series Engine:** InfluxDB 2.x processes high-frequency telemetry (signal, noise floor, top-talker throughput).
-   **Relational Layer:** PostgreSQL 16 handles persistence for device inventory, configuration Vaults, and active shaping rules.

### 4\. The Shadow Agent (Node - `agent.sh`)

-   **Ultra-Lightweight:** Written in pure `ash` (BusyBox) with `awk` sliding-window calculations to minimize CPU/RAM footprint on legacy hardware.
-   **Resilience:** Native `procd` process management with a built-in watchdog and A/B partition rollback for self-healing.

* * *

## 🛰️ Tactical Modules

### 💻 \[MATRIX\_SHELL\] - Embedded Web Terminal

Zero-trust, clientless SSH access directly from the browser to any node in the fleet.

-   **Engine:** WebSockets bridged to Go's `crypto/ssh` package, rendered via `xterm.js`.
-   **Security:** No plain-text passwords. Authentication relies exclusively on the controller's injected `Ed25519` identity keys.
-   **Functionality:** Full TTY support, allowing real-time interaction with native OpenWrt CLI tools (`logread -f`, `htop`, `wifi`).

### ⚡ \[COMMAND\_CENTER\] - Mass Orchestrator

The primary engine for fleet-wide logic execution and state synchronization.

-   **Parallel Execution:** Dispatches UCI configurations, shell scripts, or firmware upgrades to multiple routers simultaneously via Goroutines.
-   **Global Profiles:** Syncs universal settings (e.g., NTP servers, logging endpoints, firewall zones) across the entire corporate network in a single operation.
-   **Atomic Feedback:** Returns detailed execution logs per node, instantly highlighting successes or syntax failures during mass deployments.

### 🎯 \[SNIPER\_SHAPING\] - Surgical Traffic Control

Dynamic, MAC-based bandwidth throttling injected directly into the Linux kernel's data plane.

-   **Engine:** `nftables` hooked into OpenWrt's `fw4` framework.
-   **Zero-Impact:** Dedicated `sentinel_shaping` table ensures rules don't overlap with standard firewall operations.
-   **SniperReaper:** A background garbage-collection process that automatically revokes expired shaping rules.
-   **AI Preemptive Strike:** Automatically restricts bandwidth to 512Kbps for MACs associated with brute-force attempts.

### 🧠 \[SENTINEL\_AI: FLEET\_SENSE\] - Autonomous SOC

The "brain" of the network, correlating multi-device telemetry.

-   **Reactive Pipeline:** Parses logs instantly. A "Kernel Panic" or "OOM Kill" triggers an immediate AI diagnostic loop.
-   **Alerting:** Generates high-level SOC markdown reports dispatched directly via Telegram.

### 🔒 \[SECURE\_TUNNEL\] - Cryptographic SD-WAN

Creates an encrypted Overlay Network between the controller and the fleet.

-   **Protocol:** WireGuard (`10.8.0.0/24`).
-   **Auto-Provisioning:** Agents self-install `wireguard-tools` and raise the `wg0` interface upon receiving their cryptographic profile.

### 📈 \[LOG\_HARVESTER\] - Forensic Intelligence

Centralizes the `logread` stream for remote analysis.

-   **Search Engine:** GIN (Generalized Inverted Index) on PostgreSQL enables sub-millisecond full-text searches.

### 🔄 \[AGENT\_UPDATE\_SERVICE\] - Fleet Evolution

-   **Mechanism:** Raw-API script pulling with SHA256 checksums.
-   **Self-Healing Rollback:** Restores the `.old` version if an update breaks telemetry for 3 consecutive cycles.

### 🛡️ \[THE\_VAULT\] - Configuration Integrity

-   **Snapshots:** Daily backups compressed in `.tar.gz` and stored with SHA256 hashes.
-   **Visual Diff:** A Vantablack monospaced code viewer highlighting changes in configurations between dates.

* * *

## 📑 Emergency Runbook

1.  **AI Throttling Event:** If you receive a Telegram alert stating an IP has been penalized, check the **Bandwidth Sentry** dashboard. The offending MAC will be marked with a Crimson Red crosshair `[ 🎯 ]`. You can manually lift or extend the ban from the UI.
2.  **Node Isolation:** If a node enters `OUT_OF_SYNC`, use the **Log Explorer** filtering by `ERROR`. If LuCI is still responsive, use the `[OPEN_LUCI]` button in the VPN Matrix to access the internal `10.8.0.x` IP directly.
3.  **Radio Saturation:** If clients capacity peaks and the SNR drops, trigger `RF_FIX` via the Intelligence module to heuristically reassign 2.4GHz channels.
4.  **CLI Intervention:** For advanced debugging, open the **Matrix Shell** from the node's dropdown menu to gain direct root access without leaving the dashboard.

* * *

## 🌲 Operational Context

Designed specifically for high-latency, multi-tenant rural infrastructures. The system guarantees that core corporate connectivity remains stable regardless of heavy client-side consumption, isolating faults autonomously before they impact the broader network.

**Status:** `PRODUCTION_SOAKING` **Version:** `v2.5.0-sniper` **Lead Architect:** Sebastián Real

