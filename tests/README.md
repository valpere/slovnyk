# Перевірки проєкту

Ці перевірки призначені для розробників і CI. Вони не залежать від особистих
посіпак агента й не є операторськими утилітами сервісу.

| Команда | Перевірка |
|---|---|
| `make check` | gofmt/vet, staged/unstaged whitespace, repository checks |
| `go test ./tests/repository -count=1 -v` | Активні Markdown links/whitespace, ignore даних і sources, regression fixtures checker |
| `make test-smoke` | Build сервісу та Go process tests із реальними SIGTERM/SIGINT |
| `make test-integration` | Власна disposable PostgreSQL 17.11; schema/import/publication/lookup/readiness/CLI |
| `./tests/check-patch.sh [base-commit]` | Staged/unstaged і base→HEAD whitespace; historical assessments виключені |

Repository check охоплює README, data/minions/utilities/tests/testdata README
та `docs/*.md`. Historical assessments не вважаються актуальними інструкціями;
їхні bytes/hashes не змінюються. Links перевіряються локально, без мережі чи
anchors. Ignore перевіряється NUL-delimited paths і `--no-index`, також для
tracked sources. Go compiler/vet замінює попередню Python syntax check.

HTTP process suite має build tag `smoke`; після `make build` можна виконати
`go test -tags smoke ./tests/processsmoke -count=1 -v`. Вона видаляє успадковані
`SLOVNYK_*`, запускає власний loopback process без storage configuration,
перевіряє health, readiness=503/storage_not_initialized, lifecycle та відсутність
query text у logs. Proxies/redirects вимкнені, розміри й очікування обмежені.
Помилковий external address відхиляється до HTTP; child process прибирається.
Search UI/capacity цим suite не перевіряються.

Reports: `tmp/tests/repository-check.json`, `http-smoke.json`. Fixtures тестів —
`tmp/utilities-tests/`. PostgreSQL runner використовує лише власний generated
DSN, sequential suites й cleanup власних container/network/volume.
Oracle, legacy та наявні application databases не задіяні. Python не потрібний.
