# Testing

## Required Local Gates

```bash
go test -race -count=1 ./...
go vet ./...
npm --prefix web test -- --run
npm --prefix web run build
docker compose config --quiet
git diff --check
```

Generation and operation status changes should also exercise the SQL seam with:

```bash
go test ./internal/database -run 'TestQueueDeviceOperation|TestRecordDeviceOperationStatus' -count=1
```

Agent contracts are shell checks and should run after any agent or bootstrap change:

```bash
sh -n devices/agent.sh devices/99-nerve-center-bootstrap
devices/agent_contract_test.sh
devices/agent_log_collection_test.sh
```

## Database Integration Tests

Database tests skip by default and never use the controller's configured database implicitly. To run the migration contract against a disposable PostgreSQL instance:

```bash
DATABASE_URL_TEST=postgres://user:password@localhost:5432/testdb \
OPENWRT_INTEGRATION_DB=1 \
go test ./internal/database -run TestSiteConfigMigrationContract -count=1
```

The migration contract creates a temporary tenant schema, simulates a legacy `site_configs` table, runs the real tenant migration and verifies the upgrade is idempotent.

## Test Boundaries

Prefer public seams: HTTP handlers, service functions that render or validate commands, shell contract scripts and disposable database schemas. Avoid tests that require live routers, real production credentials or destructive remote operations. Benchmark SSH/iperf paths should remain opt-in in environments with explicit test devices.

CI runs Go tests, frontend tests/builds, Compose validation and agent contract checks. See `.github/workflows/ci.yml` for the authoritative pipeline.
