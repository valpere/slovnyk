# M1-01: локальний Go-каркас — 2026-10-24

## Результат

M1-01 done. Створено Go-модуль `github.com/valpere/slovnyk` за фактичним
Git remote, `cmd/slovnyk serve`, `internal/ops`, Make targets, окрему
Compose PostgreSQL та `.env.example`. Пакети наступних етапів створюватимуться
в міру реалізації. AGENTS.md, старий проєкт і дані не змінювалися;
сервер Oracle не використовувався. Commits не створювалися.

Go **1.26.10**, без зовнішніх dependencies: stdlib HTTP, signal handling,
JSON slog, testing. Версія закріплена в go.mod/.go-version; Go toolchain
із цією версією реально виконав build/test. Це patch із виправленнями
безпеки, див. [Go release history](https://go.dev/doc/devel/release#go1.26.10).
PostgreSQL **17.11 Alpine** — відтворювана локальна baseline під поточні
контракти, не остаточний вибір production engine;
[release notes](https://www.postgresql.org/docs/release/17.11/).
Образ закріплений за digest
`sha256:eee6c6e1b0894fa4fe0075eb054af9fd1cf39a35321c2d4110626cfe506411fb`.
Середовище перевірки: Linux amd64, Docker 29.9.2, Compose v5.6.1.

## Реалізована поведінка

- За замовчуванням HTTP слухає `127.0.0.1:8080`.
- `/healthz` → 200 JSON; `/readyz` → 503 JSON із `storage_not_initialized`.
  Обидва не кешуються; GET/HEAD, інші методи → 405.
- Storage ще не підключено. Readiness не можна увімкнути прапорцем;
  DB/schema/verified active dataset/artifact checks — M1-03/05.
- SIGINT/SIGTERM припиняє приймання запитів, очікує їх завершення до
  `SLOVNYK_SHUTDOWN_TIMEOUT` (10s default), потім примусово закриває
  з'єднання з ненульовим exit при перевищенні deadline.
- JSON lifecycle logs у stderr. Тексти запитів/access log відсутні;
  errors config та listener не відтворюють довільні вхідні значення.
- CLI приймає лише `serve`; invalid command/config → exit 2,
  execution error → 1, успішне завершення → 0.
- HTTP має read-header/read/write/idle timeouts і header limit.

Dev Compose має власні project/network/volume, PostgreSQL на loopback;
не монтує legacy або experiment data. `dev-db-down` зберігає volume.
`test-integration` створює інший project, synthetic credentials, випадковий
loopback port; trap видаляє лише власні ресурси. Тест не використовує DSN.

## Фактичні перевірки

| Команда / перевірка | Результат |
|---|---|
| `make build` | exit 0, `bin/slovnyk` |
| `make test` | exit 0; cmd/slovnyk та internal/ops |
| `make check` | exit 0; gofmt, go vet, git diff --check |
| `make test-race` | exit 0; обидва packages, без race reports |
| `make test-integration` | exit 0; server_version_num=170011, SELECT 1; cleanup виконано |
| Реальний процес, SIGTERM та SIGINT | exit 0 в обох; GET health=200, HEAD body empty, readiness=503; lifecycle logs без query text |

Unit tests перевіряють defaults/overrides, відхилення небезпечного config,
HTTP statuses, drain активного запиту, force-close при timeout і bind failure.
[Process smoke JSON](process-smoke.json), [hashes та checks](checks.json).
Команди відтворення й environment settings — у [README](../../../README.md).
В integration target перевірено **тільки середовище PostgreSQL**, не
application schema, drivers, права, транзакції або дані.

## Межі й продовження

AT-14/17 покриті частково; повна readiness діагностика залишається відкритою.
M1 загалом не завершено. Пошук, UI, parser, normalized keys, storage та
publication ще відсутні; експериментальні SQLite БД не підключені до каркаса.
Наступна задача — M1-02: нормалізація, lossless CSV4/gzip, manifest/hash
fixtures. Остаточний DB engine та deployment storage budget відкриті;
M1-02 не потребує навантаження на Oracle чи нового дослідження повного корпусу.
