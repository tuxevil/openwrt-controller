# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
as of the 1.0.0 release. Pre-1.0 versions follow `0.y.z` and may break the API
without a deprecation period.

## [Unreleased]

### Added
- `CONTRIBUTING.md` with full contribution workflow
- `.editorconfig` for cross-editor consistency
- `.gitattributes` for line-ending normalisation
- Comprehensive `.dockerignore` (docs, tests, .env excluded from image)
- Pinned `govulncheck` version in CI (was `@latest`)
- `CONTROLLER_SSH_ALLOW_GROUP_READ`, `WS_ALLOWED_ORIGINS`, `REQUIRE_TLS`,
  `TLS_CERT`, `TLS_KEY`, `ALLOW_LEGACY_PROVISION`, `TELEGRAM_ENCRYPTION_KEY`,
  `PG_BIND_ADDR`, `INFLUX_BIND_ADDR` in `.env.example`
- Release workflow with `goreleaser`
- Prometheus `/metrics` endpoint
- Structured logging via `log/slog`
- `/healthz` (liveness) and `/readyz` (readiness) probes

### Changed
- Container now runs as non-root user (`app`, uid 10001)
- `Dockerfile` base images pinned to specific patch versions
- `freeradius` image pinned to a specific version in `docker-compose.yml`
- Hard-coded DSNs in `cmd/scratch` and `cmd/test_topo` removed (use `DATABASE_URL`)
- Module path updated to `github.com/tuxevil/openwrt-controller`
- Frontend dependencies corrected to real published versions

### Fixed
- `.gitignore` contradictions (docs/, AGENTS.md, etc. that were ignored
  but tracked)
- `oapi-codegen/runtime` indirect dependency removed via `go mod tidy`
- Container healthcheck no longer requires JWT auth
- `go mod tidy` is now run in CI to keep `go.sum` clean

## [0.1.0] - 2026-06-16

Initial public release. See README and SECURITY.md for the security
baseline. Earlier history lives in the git log (`git log --oneline`).
