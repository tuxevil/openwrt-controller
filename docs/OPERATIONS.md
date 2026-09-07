# Operations

## Safe Configuration Changes

For a single device, use `POST /api/devices/{device_id}/safe-rollout`:

1. Send the UCI command list without `confirm`.
2. Review the returned preview and selected namespace.
3. Repeat with `confirm=true` only after verifying the device and change.
4. Confirm the response and the device rollout status in the dashboard.

For a queued operation, the response includes `operation_id`, `plan_hash` and
the device `generation` reserved for that attempt. Use all three values when
correlating agent telemetry with the controller audit trail.

The controller creates a Vault backup before applying a confirmed change. The remote batch traps failures, checks connectivity and rolls back when its health target fails. The single-device endpoint uses `1.1.1.1` by default; configured site targets are applied by fleet sync.

## Fleet Sync

Site Settings saves desired state separately from applying it. Preview renders
commands per device role, performs read-only UCI preflight, and creates an
immutable rollout draft with a generation and plan hash. Apply must send that
draft's `rollout_id`; the controller rejects missing, already-claimed, or
stale drafts and never re-renders the site template during apply. Configure
health targets under `ROLLOUT HEALTH CHECKS`; valid IP addresses and DNS-style
hostnames are deduplicated before execution.

Do not use fleet sync for an unreviewed firewall, WAN, DNS or wireless change.
Start with preview and a small device subset.

## Drift And Recovery

- `GET /api/devices/{device_id}/drift` performs a read-only UCI comparison.
- `GET /api/sites/{site_id}/drift-summary` reports fleet-level desired/observed hashes.
- Vault diff compares the contents of real `.tar.gz` backups.
- Firmware sysupgrade is intentionally unavailable until a staged compatibility workflow exists.

If a rollout fails, keep the device online for inspection, review the audit event and Vault backup, and do not immediately retry the same command. Use the device terminal only with an approved recovery procedure.

## Monitoring

Check these surfaces during an incident:

```bash
systemctl status openwrt-controller
journalctl -u openwrt-controller --since "15 minutes ago"
docker compose ps
docker compose logs --tail=100 postgres influxdb
```

On an OpenWrt node:

```sh
logread | tail -n 100
/etc/init.d/nerve-agent status
```

Threat Shield remains disabled unless its blocklist has been validated in an isolated test and the operator has an explicit activation plan.
