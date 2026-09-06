# Security Model

## Trust Boundaries

- Browser to controller: JWT authentication, role checks and restricted WebSocket ticket authentication.
- Controller to database: server-side credentials from the environment; tenant schema identifiers are validated before SQL interpolation.
- Controller to device: SSH public-key authentication with persisted TOFU host keys.
- Device to controller: the agent uses a complete controller URL; production runtime configuration requires HTTPS, with optional CA or public-key pinning. First enrollment uses a short-lived site enrollment token and subsequent pulls, updates and telemetry use a per-device token.

## Privileged Actions

Configuration writes, fleet sync, safe rollout, key operations and other privileged actions are audited. Safe rollout defaults to preview and requires explicit confirmation. UCI namespaces and command identifiers are allow-listed before shell generation.

## Secrets

- Keep `.env`, TLS keys, controller SSH keys and device token files out of version control.
- Use mode `0600` for private keys and device token files.
- Rotate site keys and controller/device SSH keys according to the deployment policy.
- Set `TELEGRAM_ENCRYPTION_KEY` when using Telegram so the bot token is encrypted at rest.

## Threat Shield

Threat Shield is optional and disabled by default. The controller and agent reject malformed, private, reserved, multicast and non-routable CIDRs before a list reaches an edge firewall. Do not activate it in production based only on a successful unit test.

## Reporting

Do not disclose vulnerabilities in public issues. Use the private channels and response expectations in [SECURITY.md](../SECURITY.md).
