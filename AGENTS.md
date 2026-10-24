# Repository Guidelines

## Project Structure & Module Organization

Slovnyk is a multilingual dictionary rewrite targeting Go and PostgreSQL. This checkout currently contains specifications only: no application, `go.mod`, tests, or web assets exist yet.

- `README.md`: project status and entry points.
- `docs/INDEX.md`: document hierarchy, milestones, and open decisions; read this first.
- `docs/REQUIREMENTS.md`, `*_CONTRACT.md`, and `DATA_MODEL.md`: behavior and detailed contracts.
- `docs/IMPLEMENTATION_PLAN.md`: ordered M1 implementation tasks.
- `docs/_assessments/`: historical review reports, snapshots, and supporting artifacts.
- `tmp/`: ignored local instructions and scratch work; unavailable in a clean checkout.

The planned layout in `docs/HLD.md` uses `cmd/slovnyk/`, domain packages under `internal/`, SQL in `migrations/`, templates/static/locales under `web/`, and synthetic fixtures in `testdata/`. Create these as implementation requires.

## Build, Test, and Development Commands

No build, development server, Make targets, or application test commands are available yet. Use `git diff --check` to check patch whitespace and `git diff --stat` to review scope.

M1-01 must introduce reproducible setup and document working commands in README. Proposed Make targets are `build`, `test`, `check`, `test-integration`, and `test-race`; they are not implemented. Once a Go module exists, establish `go build ./...`, `go test ./...`, `go vet ./...`, and `go test -race ./...`.

## Coding Style & Naming Conventions

Follow existing Markdown headings, relative links, and uppercase specification filenames. Preserve requirement and acceptance identifiers such as `FR-01` and `AT-02`. Existing specifications are primarily Ukrainian.

For new Go code, use `gofmt` indentation, lowercase package names, and standard Go identifier conventions. No formatter or linter configuration currently exists.

## Testing Guidelines

Use Go's standard testing framework with `*_test.go` files and `TestXxx` functions when implementation begins. Cover normalization, masks, cursors, and import parsing; use real isolated PostgreSQL for transaction and publication tests. Keep fixtures synthetic. No numerical coverage threshold is defined. Record actual test paths and results in `docs/TRACEABILITY.md`; planned checks are not passing tests.

## Commit & Pull Request Guidelines

History contains only short imperative subjects such as `Add gitignore`; no formal convention or PR template exists. Use focused commits. PRs should describe behavior, related requirements/issues, validation commands and results, and remaining limitations. Include screenshots for UI changes.

## Contract & Data Safety

Resolve conflicting specifications together; implementation must not silently change requirements. Keep `../slovnyk1.0/` unchanged and use a separate dev/test database. Never commit secrets or publish real dictionary data before provenance and licensing questions are resolved.
