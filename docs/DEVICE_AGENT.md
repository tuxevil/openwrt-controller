# OpenWrt Device Agent

## Installation Modes

For a single device, download `devices/agent.sh` through the authenticated agent endpoint, configure the controller URL and site key, make it executable and register the procd service. For repeatable deployments, include `devices/99-nerve-center-bootstrap` in an Image Builder profile.

The bootstrap script installs the agent and configures the service. It does not invent a device token.

## Enrollment Contract

1. The device identifies itself by its bridge MAC address.
2. The first configuration request sends `X-Site-Key` and may omit `X-Device-Token`.
3. The controller returns `config.device_token`.
4. The agent stores it in `/etc/nerve-device-token` with mode `0600`.
5. Later configuration and telemetry requests send `X-Device-Token`.

Existing devices must complete this transition before legacy provisioning is disabled. A token mismatch is rejected; a site key alone is not a permanent device credential.

## Local Responsibilities

The agent runs under procd and performs bounded log collection, configuration pulls, telemetry heartbeats, optional surveys and local Threat Shield handling. It must not block its heartbeat on `logread` or an unavailable optional utility.

## Contract Checks

```bash
sh -n devices/agent.sh devices/99-nerve-center-bootstrap
devices/agent_contract_test.sh
devices/agent_log_collection_test.sh
```

For device-side troubleshooting, collect `logread`, the agent version, controller URL without secrets and the last HTTP status received.
