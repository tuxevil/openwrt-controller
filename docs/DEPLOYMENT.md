# Deployment

## Local Docker Compose

Use Compose for development, evaluation and small all-in-one installations:

```bash
cp .env.example .env
# Set all required secrets in .env
docker compose --env-file .env up -d
docker compose ps
curl http://127.0.0.1:3000/healthz
```

PostgreSQL and InfluxDB bind to loopback by default. Change `PG_BIND_ADDR` or `INFLUX_BIND_ADDR` only when a private network requires direct access.

## Native systemd

The production backend in the reference environment runs as a native systemd service, not as the controller container. Build and install the binary, then point the unit at a protected environment file:

```bash
go build -o /opt/omega/openwrt-controller ./cmd/openwrt-controller
systemctl restart openwrt-controller
systemctl status openwrt-controller
journalctl -u openwrt-controller -f
```

The unit should use `EnvironmentFile=`, a dedicated service account, restrictive permissions for `.env` and `certs/id_controller`, and `Restart=always`. Keep PostgreSQL and InfluxDB on private interfaces.

## Readiness Checks

- `/healthz` is a cheap anonymous liveness check.
- `/readyz` reports whether required dependencies are ready.
- `docker compose config --quiet` validates interpolation before startup.
- PostgreSQL migrations run at startup for landlord and active tenant schemas.

## Production Preparation

1. For direct controller TLS, set `REQUIRE_TLS=true`, `HTTPS_PORT=8443`, and provide readable certificate and key files. For TLS termination at a reverse proxy, keep the internal hop private and configure every agent's `CONTROLLER_URL` with the proxy's `https://` URL; do not set controller `REQUIRE_TLS=true` unless the controller itself also has certificates.
2. Restrict `WS_ALLOWED_ORIGINS` to the dashboard origin.
3. Keep database ports private and expose only the controller entry point.
4. Mount the controller CA or certificate chain on devices when it is not in their default trust store; never use curl's insecure mode.
5. Set `ALLOW_LEGACY_PROVISION=false` after deploying an agent that supports device tokens.
6. Verify controller SSH key permissions and host-key storage.
7. Back up `.env`, PostgreSQL and InfluxDB according to the operator's recovery policy.

See [SECURITY.md](../SECURITY.md) for the complete checklist.
