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
AGENT_VERSION_NUMBER_FILE="/etc/nerve/agent-version-number"
EOF
```

`REQUIRE_TLS=true` rejects an `http://` controller URL. `CONTROLLER_CA_FILE` is optional when the controller certificate chains to the device trust store; `CONTROLLER_PINNED_PUBKEY` can use curl's `sha256//...` public-key pin format. Never use `-k` to bypass certificate verification.

The bootstrap script installs the agent and configures the service. It does not invent a device token. Its template is HTTPS-first and fails if the root-password placeholder is left unchanged.

The bootstrap and running agent verify the Ed25519 signature and SHA-256 hash
of updates. Each site artifact also has a monotonic `version_number`; the
agent persists the installed number outside the signed script and rejects
missing, malformed, or non-increasing update metadata. The running agent does
not install or remove packages; optional capabilities must be provisioned
explicitly by the image or a typed controller operation.

## Preserve the agent through a firmware upgrade

Preserving only the agent script leaves the node offline in Nerve Center if
OpenWrt drops the procd service, device identity, token or runtime configuration.
The durable state under `/etc/nerve` also prevents previously accepted wireless
or WireGuard policy from being reapplied on the first pull after reboot.

1. On the device, install [the agent retention template](../devices/nerve-agent.keep)
   as `/lib/upgrade/keep.d/nerve-agent`. If the device uses non-default paths for
   the agent, TLS CA, credentials or transaction state, add those paths. For
   locally installed services and guards, include their scripts, configuration
   and existing startup links in the backup.
2. Check `sysupgrade -l` for the agent script, init script, startup link, identity,
   token, runtime configuration and existing transaction files. Create a
   configuration backup with `sysupgrade -b`, copy it off-device and inspect
   the archive's file list. The archive contains credentials and must stay
   private. If supplying a custom archive with `sysupgrade -f`, make sure it
   also contains the retention rule for the next upgrade.
3. Use an Attended Sysupgrade image for the exact device profile to retain
   installed packages. With `owut`, run `check` for the target version, then
   `download` to build and verify without flashing. Check the generated package
   manifest for the active Wi-Fi stack, steering daemon, wired management and
   agent dependencies. Run `sysupgrade -T` on the resulting image before applying.
4. Verify the new firmware version and SSH access after reboot. Confirm fresh
   telemetry and configuration pulls in Nerve Center, check that the agent is
   enabled and running, and compare network and wireless configuration against
   the pre-upgrade baseline. Verify that every intended radio is broadcasting
   and that steering peers are visible. A DFS radio may need to finish its
   channel availability check before it starts broadcasting.
5. Compare both the running state and boot enablement of services against the
   baseline. A package baked into the image can enable a service that was
   previously disabled. During the UniFi AC Mesh test from OpenWrt 25.12.5 to
   25.12.6, DAWN started automatically. Returning DAWN to its prior disabled
   state preserved the device's deployment with usteer active.

If a device responds over SSH but Nerve Center shows it offline after an
upgrade, check `/etc/init.d/agent`, `/etc/nerve` and the device identity and token.
Restore the files for that same device from a trusted backup. Before starting
the agent with missing transaction state, compare the controller's wireless
and WireGuard configuration with the live settings and check for pending
operations. The first configuration pull can modify live settings when local
state is missing.

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
`plan_hash`, a positive device generation, ordered operations for the selected
safe namespaces with per-namespace observed-state hashes, health checks, and
`confirmation_policy: "local_auto"`.
The agent reports it in `change_set_transaction` and keeps standalone
operation status in the separate `transaction` envelope.
Its telemetry capabilities include the versioned contract
`device_change_set: {version: 2, namespaces: ["system", "dhcp", "firewall",
"dropbear", "sqm"], max_operations: 8, confirmation_policies: ["local_auto"]}`.
The controller requires this exact v2 contract before queuing safe changesets;
a legacy boolean or an unsupported contract is rejected without mutation.

Fleet safe rollouts are asynchronous. The controller returns `202` after the
immutable draft and first phase are durable. Later phases are delivered only
after the previous phase reports terminal success; a failed canary leaves
remaining devices in `WAITING`.

## Local Responsibilities

The agent runs under procd and performs bounded log collection, configuration pulls, telemetry heartbeats, optional surveys and local Threat Shield handling. It must not block its heartbeat on `logread` or an unavailable optional utility.

## Contract Checks

```bash
sh -n devices/agent.sh devices/99-nerve-center-bootstrap
devices/agent_contract_test.sh
devices/agent_log_collection_test.sh
```

For device-side troubleshooting, collect `logread`, the agent version, controller URL without secrets and the last HTTP status received.
