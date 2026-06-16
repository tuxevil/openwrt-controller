# Security Policy

## Supported Versions

Only the latest commit on `main` receives security updates.

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Use one of these private channels instead:

- **GitHub Security Advisory:** https://github.com/tuxevil/openwrt-controller/security/advisories/new (preferred)
- **Email:** tuxevil@gmail.com with subject `[SECURITY] openwrt-controller`

Please include:

- The component or file affected (e.g. `internal/api/handlers/auth.go`)
- A description of the issue and how to reproduce it
- Any proof-of-concept code or commands
- The impact you anticipate (auth bypass, RCE, data leak, DoS, etc.)
- Your suggested fix, if you have one

You should receive an acknowledgement within **72 hours**. We aim to issue a patch
within **14 days** for critical issues. Once the fix is published, you will be
credited in the release notes unless you ask to remain anonymous.

## Scope

In scope:

- The Go backend under `cmd/` and `internal/`
- The Vue frontend under `web/src/`
- The OpenWrt agent under `devices/`
- The official `Dockerfile` and `docker-compose.yml`

Out of scope:

- Issues in third-party dependencies (please report upstream)
- DoS via missing rate limiting on endpoints not protected by auth (known design choice for self-hosted dev installs)
- Vulnerabilities that require already having SUPERADMIN credentials

## Hardening Checklist for Operators

Before exposing this controller to the public internet:

- [ ] Generate a strong `JWT_SECRET` (≥ 32 chars, `openssl rand -base64 48`)
- [ ] Change every default password in `.env` (Postgres, InfluxDB, Telegram)
- [ ] Encrypt the Telegram bot token: set `TELEGRAM_ENCRYPTION_KEY` (any passphrase; used to derive AES-256-GCM key for at-rest encryption of the token in `platform_settings`)
- [ ] Bind PostgreSQL and InfluxDB to `127.0.0.1` instead of `0.0.0.0` in
      `docker-compose.yml` if you do not need remote access
- [ ] Set `REQUIRE_TLS=true` and provide `--tls-cert`/`--tls-key` so the
      controller refuses to start on plain HTTP
- [ ] Put the controller behind TLS (Traefik / Caddy / nginx reverse proxy)
- [ ] Set `WS_ALLOWED_ORIGINS=<your-domain>` to restrict WebSocket origin
      (default: reject all). WebSockets now use single-use ticket auth
      (`POST /api/ws-ticket` → `?ticket=<id>`) — JWTs no longer appear
      in URLs or access logs.
- [ ] Set `ALLOW_LEGACY_PROVISION=false` once all OpenWrt agents have been
      updated to send the `X-Device-Token` header (prevents config leaks)
- [ ] Rotate `api_key` of every site after first boot (UI → Site Settings →
      Rotate Key, or `POST /api/sites/{id}/rotate-key`)
- [ ] Verify the controller SSH private key (`certs/id_controller`) has
      mode `0600` — the new KeyStore refuses to load wider permissions
      unless `CONTROLLER_SSH_ALLOW_GROUP_READ=1` is set
- [ ] Regenerate `certs/id_controller` (Ed25519) and push the new public key
      to every adopted OpenWrt device
- [ ] Enable a firewall on the host (`ufw`, `nftables`) and only expose
      port 443 publicly
- [ ] Subscribe to this repository to receive security advisories
