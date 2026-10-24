GO ?= go

.PHONY: build run test check test-race test-integration test-smoke dev-db-up dev-db-down utilities

build:
	$(GO) build -trimpath -o bin/slovnyk ./cmd/slovnyk

run:
	$(GO) run ./cmd/slovnyk serve

test:
	$(GO) test ./...

check:
	@files="$$(gofmt -l cmd internal migrations utilities tests minions)" || exit 1; test -z "$$files" || { printf '%s\n' "$$files"; echo 'Run gofmt -w on the listed Go files' >&2; exit 1; }
	$(GO) vet ./...
	./tests/check-patch.sh
	$(GO) test ./tests/repository -count=1

test-race:
	$(GO) test -race ./...

# Isolated PostgreSQL migrations, schema, privileges and transaction tests.
test-integration:
	./tests/postgres.sh

test-smoke: build
	$(GO) test -tags smoke ./tests/processsmoke -count=1 -v

dev-db-up:
	docker compose up -d --wait db

dev-db-down:
	docker compose down

utilities:
	$(GO) build -trimpath -o bin/prepare-artifacts ./utilities/prepare-artifacts
