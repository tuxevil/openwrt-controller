# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
as of the 1.0.0 release. Pre-1.0 versions follow `0.y.z` and may break the API
without a deprecation period.

## [Unreleased]

### Added
- `CONTRIBUTING.md` with full contribution workflow
- `CHANGELOG.md` (this file)
- `.editorconfig` for cross-editor consistency
- `.gitattributes` for line-ending normalisation
- Comprehensive `.dockerignore` (docs, tests, .env excluded from image)
- Pinned `govulncheck v1.1.4` in CI (was `@latest`)
- `CONTROLLER_SSH_ALLOW_GROUP_READ`, `WS_ALLOWED_ORIGINS`, `REQUIRE_TLS`,
  `TLS_CERT`, `TLS_KEY`, `ALLOW_LEGACY_PROVISION`, `TELEGRAM_ENCRYPTION_KEY`,
  `PG_BIND_ADDR`, `INFLUX_BIND_ADDR`, `PG_MAX_OPEN_CONNS`,
  `PG_MAX_IDLE_CONNS`, `LOG_FORMAT` in `.env.example`
- Release workflow (`.github/workflows/release.yml`): multi-arch
  (linux/amd64 + linux/arm64), Docker Hub + GHCR push, SPDX SBOM,
  cosign keyless signing, GitHub Release with the SBOM attached
- Prometheus `/metrics` endpoint with custom registry and HTTP request
  duration / status-class counters
- Structured logging via `log/slog` (text or JSON, `--log-format`)
- `/healthz` (liveness) and `/readyz` (readiness) probes — anonymous,
  no auth, safe for orchestrators
- Frontend testing setup with `vitest` + `@vue/test-utils` + `happy-dom`
  (initial test: `usePolling` composable, 4 cases)
- OpenAPI 3.1 spec at `openapi.yaml` documenting 33 endpoints
- Graceful shutdown on SIGTERM/SIGINT (15s drain) in main.go
- HTTP server timeouts (`ReadHeaderTimeout`, `ReadTimeout`,
  `WriteTimeout`, `IdleTimeout`)

### Changed
- Container now runs as non-root user (`app`, uid 10001) with
  `read_only: true` filesystem compatibility
- `Dockerfile` base images pinned to specific patch versions
  (Go 1.25.4, Node 22.11.0, Alpine 3.21.3)
- `docker-compose.yml`: `freeradius` pinned to 3.2.5-alpine, `postgres`
  to 15.13-alpine, `influxdb` to 2.7.10-alpine; healthchecks on all 4
  services; `cap_drop: ALL` + `cap_add: NET_BIND_SERVICE` on the
  controller; `depends_on` with `condition: service_healthy`
- Hard-coded DSNs in `cmd/scratch` and `cmd/test_topo` removed (now
  read `DATABASE_URL` with a fail-fast check)
- Container healthcheck now uses the anonymous `/healthz` endpoint
- `govulncheck` no longer pulled as `@latest` on every CI run

### Fixed
- Container no longer requires JWT for the orchestrator's healthcheck
- `oapi-codegen/runtime` indirect dependency retained (it is required
  transitively by `influxdb-client-go/v2`; `go mod tidy` is a no-op)
- Stale `dependabot/npm_and_yarn/web/*` remote branches pruned
- Local `.env` (already gitignored) deleted from the working tree

## [0.1.0] - 2026-06-16

Initial public release. See README and SECURITY.md for the security
baseline. Earlier history lives in the git log (`git log --oneline`).
