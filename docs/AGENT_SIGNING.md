# Agent Update Signing

Agent updates can be authenticated with Ed25519 signatures. The controller signs the exact `script_content` returned by the agent version endpoints; the device verifies the downloaded bytes before replacing its current script.

## Generate Keys

Generate keys outside the repository. Keep the private key in a secret manager or deployment environment:

```bash
umask 077
install -d -m 700 "$HOME/.config/openwrt-controller/agent-signing"
openssl genpkey -algorithm Ed25519 -out "$HOME/.config/openwrt-controller/agent-signing/private.pem"
openssl pkey -in "$HOME/.config/openwrt-controller/agent-signing/private.pem" -pubout -out "$HOME/.config/openwrt-controller/agent-signing/public.pem"
```

The private key must never be committed, pasted into tickets, or logged.

## Configure The Controller

`AGENT_UPDATE_SIGNING_KEY` is the Base64-encoded raw Ed25519 private key. Convert it from the PEM key with an approved deployment secret-management procedure, then inject it into the controller environment. Do not put it in `.env` files that might be copied into source archives.

The controller publishes `signature` and `signature_algorithm: Ed25519` in `GET /api/agent/latest` only when signing is configured. If the signing key is missing, the endpoint fails closed instead of serving unsigned metadata.

## Configure Devices

Set `AGENT_UPDATE_PUBLIC_KEY` in `devices/99-nerve-center-bootstrap` to the Base64-encoded raw 32-byte public key before building or provisioning an image. The bootstrap uses the short-lived site enrollment token to fetch metadata and raw bytes, verifies both before installation, then stores the key in `/etc/nerve/agent-update-public-key`.

Devices verify the SHA-256 hash and Ed25519 signature before replacing the agent. Runtime settings and credentials are stored in `/etc/nerve/agent.conf` and separate root-owned files, so the signed `agent.sh` remains byte-for-byte identical to the published artifact. Invalid, missing, or unsigned metadata leaves the current agent unchanged.

Legacy agents that authenticate only with `X-Site-Key` are not offered this
artifact, because it requires external runtime configuration. Enroll those
devices first, then enable signed updates; legacy telemetry/config access can
remain temporarily enabled with `ALLOW_LEGACY_PROVISION=true`.

## Rotation

1. Generate a new key pair outside the repository.
2. Provision the new public key to a canary image while the old signer remains active.
3. Deploy the new private key and publish a signed artifact.
4. Confirm a signed update succeeds on one canary device before fleet distribution.
5. Remove the old private key only after all devices have received the new public key.

There is no unsigned fallback after a device has a pinned public key. During a key migration, overlap must be implemented explicitly; an absent signature is never an automatic downgrade.

## Recovery

If a signed update fails, the agent keeps the previous script and logs the failure. Restore the matching public key or deploy a manually reviewed agent through the existing bootstrap/break-glass procedure.
