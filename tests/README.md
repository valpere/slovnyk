# Перевірки проєкту

Ці перевірки призначені для розробників і CI. Вони не залежать від особистих
посіпак агента й не є операторськими утилітами сервісу.

| Команда | Перевірка |
|---|---|
| `make check` | gofmt/vet, staged/unstaged whitespace, repository checks |
| `go test ./tests/repository -count=1 -v` | Активні Markdown links/whitespace, ignore даних і sources, regression fixtures checker |
| `make test-smoke` | Build сервісу та Go process tests із реальними SIGTERM/SIGINT |
| `make test-integration` | Власна disposable PostgreSQL 17.11; schema/import/publication/lookup/readiness/CLI та повний service lifecycle |
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

## Наскрізний service lifecycle

`serviceflow/lifecycle_test.go: TestServiceLifecycle` (tag `integration`)
виконується останнім через `make test-integration`. Потрібен C compiler:
цей test harness запускається з `-race`; child binary збирається звичайно.
Runner задає лише DSN власного контейнера; тест додатково вимагає loopback
`postgresql://…/slovnyk_test`, створює власну унікальну порожню БД й два LOGINs,
які успадковують лише operator/HTTP group privileges. HTTP write denial
перевіряється перед запуском. Наявна БД runner не очищається тестом.

Фактичний CLI виконує migrations, import batches по 7 rows, verify/exports,
publish A/B, stale generation rejection та rollback. Loopback HTTP перевіряє
exact/mask, stable article, manifest link, 53-row gzip CSV та compressed hash,
retained continuations/files після publication, readiness recovery після
ізольованого connection outage та cursor після process restart із тим самим
keyring. Health залишається 200 при DB outage, search повертає 503. Logs не
мають містити synthetic private query, DSN чи пароль. Cookies/UI й body limits
детальніше покриті у `internal/httpui/`, interruption/retry/corrupt streams —
у `internal/importer/`, no-clobber/crash publication — у `internal/publication/`.

Child processes мають deadlines; cleanup прибирає процеси, потім власну БД,
LOGINs і `tmp/serviceflow/run-*`. Runner видаляє container/volume навіть при
помилці suite. Немає browser, реального корпусу або production acceptance.

### Backup/restore у service lifecycle

Після restart сценарій знімає custom `pg_dump`. Runner запускає другий
порожній PostgreSQL 17.11 cluster з окремим volume/loopback port лише перед
serviceflow. Обидва контейнери мають Compose limits 512 MiB/1 CPU кожен.
`restore_test.go` звіряє Compose labels/різні container IDs і PostgreSQL system
identifiers; на цілі немає source owner чи application groups.

Новий NOLOGIN non-superuser schema owner володіє новою UTF-8 БД. Перша спроба
`pg_restore --single-transaction --exit-on-error --no-owner --role=<owner>`
без groups відхиляється без часткової schema. Після bootstrap group roles і
restricted LOGINs повторний restore працює; ownership schema/relations/functions/
types, counts/digests усіх таблиць і paths/bytes/hashes artifact/source files
перевіряються. Жоден source cluster role не копіюється через globals dump.

Runner передає `SLOVNYK_TEST_DSN`, `SLOVNYK_TEST_DB_CONTAINER`,
`SLOVNYK_RESTORE_TEST_DSN`, `SLOVNYK_RESTORE_TEST_CONTAINER`. Використовуйте
`make test-integration`, який створює й прибирає обидві цілі, включно з частковими
setup failures. PostgreSQL tools працюють у контейнерах; host packages не потрібні.

Оригінальна БД недоступна під час приймання restored HTTP. DB-only restore
не проходить readiness; повна копія відтворює active exact/mask/article, старі
continuations, retained A/B downloads/sitemaps. HTTP UPDATE/DDL/register і
operator direct inventory DELETE/DDL відхиляються з SQLSTATE 42501; actual
operator CLI повторює sitemap registration через SECURITY DEFINER нового owner.
Той самий synthetic cursor keyring заданий тестом. Off-host transfer, secret
escrow і full-corpus RTO це не перевіряє. [Процедура](../docs/BACKUP_RESTORE.md).
