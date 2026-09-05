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

The controller publishes `signature` and `signature_algorithm: Ed25519` in `GET /api/agent/latest` when signing is configured.

## Configure Devices

Set `AGENT_UPDATE_PUBLIC_KEY` in `devices/99-nerve-center-bootstrap` to the Base64-encoded raw 32-byte public key before building or provisioning an image. The bootstrap injects it into `agent.sh`.

Devices verify the SHA-256 hash and Ed25519 signature before replacing the agent. Invalid signatures, invalid hashes, and unsupported verification tools leave the current agent unchanged.

## Rotation

1. Generate a new key pair outside the repository.
2. Deploy the new private key to the controller.
3. Rebuild or reprovision agents with the matching public key.
4. Confirm a signed update succeeds on one canary device before fleet distribution.
5. Remove the old private key only after all devices have received the new public key.

Existing unsigned versions remain supported for the transition. New production deployments should configure signing and the public key together; otherwise updates are hash-verified only.

## Recovery

If a signed update fails, the agent keeps the previous script and logs the failure. Restore the matching public key or deploy a manually reviewed agent through the existing bootstrap/break-glass procedure.
