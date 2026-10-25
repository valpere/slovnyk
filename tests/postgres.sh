#!/bin/sh
# Disposable schema/transaction tests. Never uses an application/legacy DSN.
set -eu
cd "$(dirname "$0")/.."
project="slovnyk-test-$$"
restore_project="$project-restore"
export POSTGRES_USER=slovnyk_test POSTGRES_PASSWORD=synthetic-test-only POSTGRES_DB=slovnyk_test
cleanup() {
    cleanup_status=0
    docker compose -p "$restore_project" down -v >/dev/null || cleanup_status=$?
    docker compose -p "$project" down -v >/dev/null || cleanup_status=$?
    return "$cleanup_status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Empty host port lets Docker allocate an unused loopback port for this test.
SLOVNYK_DEV_DB_PORT= docker compose -p "$project" up -d --wait --wait-timeout 60 db
docker compose -p "$project" exec -T db sh -ec '
    export PGPASSWORD="$POSTGRES_PASSWORD"
    test "$(psql -X -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SHOW server_version_num")" = 170011
    test "$(psql -X -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "SELECT 1")" = 1
'
address=$(docker compose -p "$project" port db 5432)
dsn="postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD@$address/$POSTGRES_DB?sslmode=disable"
SLOVNYK_MIGRATION_DSN="$dsn" go run ./cmd/slovnyk migrate
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/storage/postgres -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/importer -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/publication -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/lookup -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/entries -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/sitemaps -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./internal/ops -count=1 -v
SLOVNYK_TEST_DSN="$dsn" go test -tags integration ./cmd/slovnyk -count=1 -v
container=$(docker compose -p "$project" ps -q db)
POSTGRES_USER=slovnyk_restore_admin POSTGRES_DB=slovnyk_restore_test SLOVNYK_DEV_DB_PORT= \
    docker compose -p "$restore_project" up -d --wait --wait-timeout 60 db
restore_address=$(docker compose -p "$restore_project" port db 5432)
restore_dsn="postgresql://slovnyk_restore_admin:$POSTGRES_PASSWORD@$restore_address/slovnyk_restore_test?sslmode=disable"
restore_container=$(docker compose -p "$restore_project" ps -q db)
SLOVNYK_TEST_DSN="$dsn" SLOVNYK_TEST_DB_CONTAINER="$container" \
    SLOVNYK_RESTORE_TEST_DSN="$restore_dsn" SLOVNYK_RESTORE_TEST_CONTAINER="$restore_container" \
    go test -race -tags integration ./tests/serviceflow -count=1 -v
printf '%s\n' 'PostgreSQL migrations, privileges, transactions, import/retry, publication, lookup, readiness, CLI, service lifecycle and local backup/restore checks passed.'
