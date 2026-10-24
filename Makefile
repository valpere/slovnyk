GO ?= go
PYTHON ?= python3

.PHONY: build run test check test-race test-integration test-smoke dev-db-up dev-db-down

build:
	$(GO) build -trimpath -o bin/slovnyk ./cmd/slovnyk

run:
	$(GO) run ./cmd/slovnyk serve

test:
	$(GO) test ./...

check:
	@files="$$(gofmt -l cmd internal migrations)" || exit 1; test -z "$$files" || { printf '%s\n' "$$files"; echo 'Run gofmt -w on the listed Go files' >&2; exit 1; }
	$(GO) vet ./...
	./minions/check-patch.sh
	$(PYTHON) minions/check-repository.py

test-race:
	$(GO) test -race ./...

# Isolated PostgreSQL migrations, schema, privileges and transaction tests.
test-integration:
	./minions/test-postgres.sh

test-smoke: build
	$(PYTHON) minions/smoke-http.py

dev-db-up:
	docker compose up -d --wait db

dev-db-down:
	docker compose down
