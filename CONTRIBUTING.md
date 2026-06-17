# Contributing

Thanks for your interest in contributing to **openwrt-controller**!

This document covers everything you need to open a useful pull request.
For security issues, see [SECURITY.md](SECURITY.md) — do **not** open a public issue.

## Code of conduct

Be kind, be technical. We follow the [Go community code of conduct](https://go.dev/conduct)
in spirit. Disagreement is welcome; personal attacks are not.

## Project layout

```
cmd/        # One binary per subcommand (openwrt-controller is the production one)
internal/   # All non-exported Go packages (api, database, services, orchestrator, …)
web/        # Vue 3 + Vite SPA
devices/    # OpenWrt-side agent (shell script + procd init)
docs/       # Specs, references, design notes
.github/    # CI workflows
```

## Workflow

1. **Open an issue first** for any non-trivial change. Align on the design
   before you write code.
2. Fork the repo, create a branch from `main` (`feat/<short-name>`,
   `fix/<short-name>`, `refactor/<short-name>`).
3. Make your changes in small, focused commits.
4. Open a pull request against `main` and reference the issue.

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject, imperative, ≤50 chars>

<body — explain WHY, not WHAT. Wrap at 72 chars.>
```

Allowed types: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`,
`ci`, `chore`, `security`.

Examples:

```
feat(api): add /api/v1/sites/{id}/rotate-key endpoint
fix(orchestrator): close SSH session on early return
ci: pin govulncheck to v1.1.4
```

**Logs and code comments are English only.** Spanish is fine in PR
discussions, but anything committed to the repo (commit messages, inline
comments, log strings) is English. See `docs/CONTRIBUTING_LOGS.md`.

**Sign your commits** when possible (`git commit -S`). Unsigned commits are
accepted but signed ones are preferred.

## Local development

### Prerequisites

- Go ≥ 1.25 (`go version`)
- Node.js ≥ 20 (`node --version`)
- Docker + Docker Compose (for the full stack)
- PostgreSQL 15 + InfluxDB 2.x (or use `docker compose up postgres influxdb`)

### Backend

```bash
go vet ./...
go test -race -count=1 ./...
go build -o /tmp/openwrt-controller ./cmd/openwrt-controller
```

Run the server:

```bash
cp .env.example .env  # fill in real values
docker compose up -d postgres influxdb
DATABASE_URL=postgres://postgres:postgres@localhost:5432/openwrthub \
JWT_SECRET=$(openssl rand -base64 48) \
INFLUX_URL=http://localhost:8086 \
INFLUX_TOKEN=dev-token \
go run ./cmd/openwrt-controller
```

### Frontend

```bash
cd web
npm ci
npm run dev      # dev server with HMR
npm run build    # production bundle to web/dist
```

## Testing

- **All new business logic MUST come with tests.** No exceptions.
- Use table-driven tests where it makes sense.
- Integration tests that need a DB should use the `DATABASE_URL_TEST`
  env var and skip cleanly if it's not set.
- Run `go test -race -count=1 ./...` before pushing.
- The CI test command is the source of truth; if it works locally, it'll
  work in CI.

## Database changes

- Schema migrations live in `internal/database/postgres.go` (see
  `applyMigrations`).
- **Never** break an existing tenant schema. Add a new migration step.
- Test the migration both forward and backward.

## API changes

- This project is pre-1.0. We may break the API without a deprecation
  period, but please call it out in the PR.
- When we ship 1.0 we will introduce `/api/v1/` URL versioning (see
  the roadmap in the README).

## Pull request checklist

- [ ] CI is green (`backend`, `frontend`, `docker`, `gitleaks`)
- [ ] `go vet ./...` is clean
- [ ] Tests added or updated for any business logic change
- [ ] Public APIs documented (godoc on exported Go symbols, JSDoc-style
      comments on exported Vue composables)
- [ ] No secrets, no debug `fmt.Println` left over
- [ ] `go mod tidy` was run (no diff in `go.mod`/`go.sum` after)
- [ ] Conventional Commits format

## Reporting bugs

Open a [GitHub issue](https://github.com/tuxevil/openwrt-controller/issues)
and include:

- Controller version (`git rev-parse --short HEAD`)
- OpenWrt version of the affected device(s)
- Relevant logs from `journalctl -u openwrt-controller` and `logread` on
  the device
- Steps to reproduce

## Reporting vulnerabilities

See [SECURITY.md](SECURITY.md). **Do not** open a public issue.

## License

By contributing, you agree that your contributions will be licensed
under the [MIT License](LICENSE).
