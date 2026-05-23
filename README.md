# Nerve Center CE (Community Edition)

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white)
![Vue.js](https://img.shields.io/badge/vuejs-%2335495e.svg?style=for-the-badge&logo=vuedotjs&logoColor=%234FC08D)
![OpenWrt](https://img.shields.io/badge/OpenWrt-1B828C.svg?style=for-the-badge&logo=home-assistant&logoColor=white)

Nerve Center CE is an open-source fleet management controller designed for OpenWrt devices. It uses an Open Core model to provide advanced networking features directly from a centralized orchestrator.

## Key Features

- **Zero-Touch Provisioning**: Auto-adopt and configure remote OpenWrt devices by just connecting them to the internet.
- **Unified Site Settings**: Global infrastructure management layer to manage fleet-wide configuration (Network, Wireless, Services, Security) from a single Vantablack-themed panel.
- **SD-WAN (mwan3)**: Robust link failover and load balancing capability for maximum uptime.
- **Advanced Telemetry (InfluxDB)**: Monitor fleet metrics, bandwidth top talkers, connection flows and L2 topologies.
- **802.11r Roaming**: Fast BSS Transition for seamless handoffs across APs.
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
