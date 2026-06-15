# Nerve Center CE (Community Edition)

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/vuejs-%2335495e.svg?style=for-the-badge&logo=vuedotjs&logoColor=%234FC08D)
![OpenWrt](https://img.shields.io/badge/OpenWrt-1B828C.svg?style=for-the-badge&logo=home-assistant&logoColor=white)

Nerve Center CE is an open-source fleet management controller designed for OpenWrt devices. It uses an Open Core model to provide advanced networking features directly from a centralized orchestrator.

## Key Features

- **Zero-Touch Provisioning**: Auto-adopt and configure remote OpenWrt devices by just connecting them to the internet.
- **Enterprise AIOps Suite**: Includes Auto-VPN Hub-and-Spoke Mesh (WireGuard), WPA-Enterprise (802.1X), Dynamic VLAN Steering, and 802.11w MFP.
- **Integrated FreeRADIUS Identity Matrix**: Multi-tenant RADIUS server running as a sidecar to authenticate WPA-Enterprise users centrally.
- **Zero Trust Micro-segmentation**: Fully automated deployment and configuration of Tailscale/Headscale across the fleet.
- **Deep Packet Inspection & Smart QoS**: L7 traffic analysis (nDPI) and Bufferbloat elimination using CAKE SQM algorithms.
- **Remote Diagnostics**: Execute UI-driven packet captures (tcpdump) and speed tests (iperf3) directly from the browser.
- **Unified Site Settings**: Global infrastructure management layer to manage fleet-wide configuration (Network, Wireless, Services, Security) from a single Vantablack-themed panel.
- **SD-WAN (mwan3)**: Robust link failover and load balancing capability for maximum uptime.
- **Advanced Telemetry (InfluxDB)**: Monitor fleet metrics, bandwidth top talkers, connection flows and L2 topologies.
- **Webhooks & Billing APIs**: Built-in outgoing notifications for Node/Incident state changes and aggregated Landlord usage metrics.
- **Geospatial Tracking**: Topographical node mapping across deployments using Leaflet.
- **Captive Portal**: Hospitality-grade guest WiFi gateway using FAS with opennds.

## Quickstart Deployment

You can quickly deploy the Nerve Center CE stack using Docker Compose:

```bash
# 1. Clone the repository
git clone https://github.com/examplecorp/nerve-center-ce.git
cd nerve-center-ce

# 2. Setup your environment
cp .env.example .env

# 3. Start the services
docker-compose up -d
```

The Unified Control Panel will now be available on your host.
