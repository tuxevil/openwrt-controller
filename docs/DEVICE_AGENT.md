# OpenWrt Device Agent

## Installation Modes

For a single device, download `devices/agent.sh` through the authenticated agent endpoint, configure a complete controller URL, make it executable and register the procd service. Use HTTPS for enrollment, updates, configuration and telemetry on any untrusted path. For repeatable deployments, include `devices/99-nerve-center-bootstrap` in an Image Builder profile.

Example download and configuration:

```sh
wget -O /usr/sbin/nerve-agent.sh \
  "https://controller.example.com:8443/api/agent/latest/raw" \
  --header="X-Site-Enrollment-Token: SITE_ENROLLMENT_TOKEN"
chmod 755 /usr/sbin/nerve-agent.sh

umask 077
cat >/etc/nerve/agent.conf <<'EOF'
CONTROLLER_URL="https://controller.example.com:8443/api"
REQUIRE_TLS="true"
CONTROLLER_CA_FILE="/etc/ssl/certs/controller-ca.pem"
CONTROLLER_PINNED_PUBKEY=""
DEVICE_ID_FILE="/etc/nerve-device-id"
DEVICE_TOKEN_FILE="/etc/nerve-device-token"
ENROLLMENT_TOKEN_FILE="/etc/nerve/enrollment-token"
ENROLLMENT_NONCE_FILE="/etc/nerve/enrollment-nonce"
AGENT_UPDATE_PUBLIC_KEY_FILE="/etc/nerve/agent-update-public-key"
EOF
```

`REQUIRE_TLS=true` rejects an `http://` controller URL. `CONTROLLER_CA_FILE` is optional when the controller certificate chains to the device trust store; `CONTROLLER_PINNED_PUBKEY` can use curl's `sha256//...` public-key pin format. Never use `-k` to bypass certificate verification.

The bootstrap script installs the agent and configures the service. It does not invent a device token. Its template is HTTPS-first and fails if the root-password placeholder is left unchanged.

## Enrollment Contract

1. The device identifies itself by its bridge MAC address.
2. The first enrollment request sends the short-lived `X-Site-Enrollment-Token` and a nonce.
3. The controller returns a device token and binds the nonce to that device.
4. The agent stores it in `/etc/nerve-device-token` with mode `0600`.
5. The agent deletes the enrollment token and nonce after successful enrollment.
6. Later configuration, telemetry and update requests send `X-Device-Token`.

Existing devices must complete this transition before legacy provisioning is disabled. A token mismatch is rejected; an enrollment token is not a permanent device credential.

## Operation Status Contract

Typed operations carry `operation_id`, `plan_hash` and, for generation-bound plans, a positive `generation`. The agent persists the generation beside its durable transaction journal and echoes it in telemetry status. Older agents may omit the generation; the controller still requires matching operation and plan identities and leaves generation columns unchanged for that compatibility path.

The safe rollout slice carries one `DeviceChangeSet` through the same config
pull and telemetry requests. It contains `change_set_id`, `device_id`,
`plan_hash`, a positive device generation, one `system` operation with its
observed-state hash, health checks, and `confirmation_policy: "local_auto"`.
The agent reports it in `change_set_transaction` and keeps standalone
operation status in the separate `transaction` envelope.
Its telemetry capabilities include `device_change_set: true`; the controller
requires that capability before queuing the safe rollout slice.

## Local Responsibilities

The agent runs under procd and performs bounded log collection, configuration pulls, telemetry heartbeats, optional surveys and local Threat Shield handling. It must not block its heartbeat on `logread` or an unavailable optional utility.

## Contract Checks

```bash
sh -n devices/agent.sh devices/99-nerve-center-bootstrap
devices/agent_contract_test.sh
devices/agent_log_collection_test.sh
```

For device-side troubleshooting, collect `logread`, the agent version, controller URL without secrets and the last HTTP status received.
