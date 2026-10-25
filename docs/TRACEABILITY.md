# Трасування та перевірка

Критерії AT визначені в [REQUIREMENTS](REQUIREMENTS.md); сценарії — у
[USE_CASES](USE_CASES.md); компоненти — у [HLD](HLD.md).
**Таблиця нижче задає цільове покриття. Жоден повний AT ще не закритий;
фактично пройдені часткові перевірки наведено окремо.**
FR-12 / UC-07 / AT-12 скасовані 2026-10-23 (OQ-04); їхні ID збережено для історії.
При реалізації додати точний шлях тесту та фактичний результат, не оголошувати
всю вимогу виконаною після часткової реалізації.

| Вимога | Use case | Компонент HLD | Перевірка | Етап |
|---|---|---|---|---|
| FR-01 | UC-01, UC-03 | FUN-CATALOG | AT-01 | M1, M2 |
| FR-02 | UC-01 | FUN-LOOKUP | AT-02 | M1 |
| FR-03 | UC-02 | FUN-LOOKUP | AT-03 | M1 |
| FR-04 | UC-01, UC-02 | FUN-LOOKUP | AT-04 | M1, M2 |
| FR-05 | UC-01, UC-03 | FUN-UI | AT-05 | M1, M2 |
| FR-06 | UC-04 | FUN-EXPORT | AT-06 | M2 |
| FR-07 | UC-05 | FUN-IMPORT | AT-07 | M1, M2 |
| FR-08 | UC-06 | FUN-IMPORT | AT-08 | M1, M2 |
| FR-09 | UC-05, UC-06 | FUN-IMPORT | AT-09 | M2 |
| FR-10 | UC-06 | FUN-PUBLISH | AT-10 | M1, M2 |
| FR-11 | UC-09 | FUN-IMPORT | AT-11 | M2 |
| FR-12 — скасовано | UC-07 — скасовано | — | AT-12 — скасовано | — |
| FR-13 | UC-08 | FUN-OPS | AT-13 | M3 |
| FR-14 | UC-10 | FUN-SEO, FUN-UI, FUN-CATALOG | AT-19 | M1, M2, M3 |
| NFR-01 | UC-01, UC-06 | FUN-LOOKUP, FUN-IMPORT | AT-14 | M1 |
| NFR-02 | UC-01, UC-02, UC-04 | FUN-LOOKUP, FUN-UI, FUN-EXPORT | AT-15 | M1, M2 |
| NFR-03 | UC-02, UC-06 | FUN-LOOKUP, FUN-IMPORT | AT-16 | M2, M3 |
| NFR-04 | UC-01, UC-06, UC-08 | FUN-OPS | AT-17 | M1, M3 |
| NFR-05 | UC-05, UC-06 | FUN-CATALOG, FUN-IMPORT | AT-18 | M2 |

## Як перевіряти при реалізації

- Unit tests: нормалізація, маски/escaping, cursor, fallback, CSV parsers і mappings.
- PostgreSQL integration tests: реальна collation, LIKE Unicode, довгі ключі,
  транзакційність, повтор batch, atomic publish та rollback.
- Browser/HTTP smoke: no-JS форма, всі стани результату, locale,
  text/plain та download; fixtures з HTML/SQL-метасимволами.
- Migration rehearsal: повний source manifest, reconciliation digests, перерваний
  імпорт, exports, backup/restore та контрольні запити на реальному corpus.

Команди Go-каркаса доступні в [README](../README.md); `go test ./...`
перевіряє реалізований M1-01. Tests інших задач додаються разом із поведінкою.
Не додавати модельних викликів або статистичних метрик тексту замість цих тестів.

AT-16: погоджено p95 звичайного пошуку <=1 s на сервері та DB deadline 2 s;
орієнтовний пік до 10 запитів/с. Профіль запитів і протокол навантаження ще
потрібно погодити. [Локальна SQLite-проба](_assessments/2026-10-23-sqlite/REPORT.md)
і [проба всього корпусу](_assessments/2026-10-24-sqlite-full/REPORT.md)
є дослідженнями спрощеного SQL storage, не проходженням AT-16 або HTTP SLO.
Повна БД — 12.43 GiB; 300 SQL-запитів із частотою 10/с без помилок,
сукупний p95 22.746 ms на developer machine з неконтрольованим кешем ОС.

AT-19: [SEO_PLAN](SEO_PLAN.md); обхід синтетичних сторінок, локалізовані
URL/canonical, sitemap/discovery та перевірка навантаження роботів.
Планування не означає реалізації або фактичної індексації.

Локальна [репетиція копіювання/відновлення](_assessments/2026-10-24-sqlite-full/restore-result.json):
127.065 s, SHA-256 збігаються, SQLite quick_check=ok, 56 455 068 rows.
Це лише експериментальна закрита БД на тому самому developer filesystem;
AT-13 і повне disaster recovery за 24 години ще не перевірені.

## Фактичні діагностичні перевірки — 2026-10-24

[Аудит CSV](_assessments/2026-10-24-csv-audit/REPORT.md): усі 992 файли
прочитані, 56 455 068 різних кортежів, 0 структурних відхилень;
повторні SHA-256 і strict gzip checks пройшли. Це факти про джерело для
AT-07/09/18, не перевірка production import, ledger або round-trip exports.

`python3 -m unittest discover -s docs/_assessments/2026-10-24-csv-audit -v`:
7 tests пройшли. Фактичний шлях —
[test_audit.py](_assessments/2026-10-24-csv-audit/test_audit.py).
Це tests діагностичного інструмента, а не реалізовані acceptance tests сервісу.

## Повна локальна модель та ресурсна проба — 2026-10-24

[Звіт](_assessments/2026-10-24-provenance/REPORT.md): по 56 455 068
translations/import_records/origins; кожен raw translation і origin звірений
із CSV ordinal, усі 992 source/DB/export canonical digests збігаються.
Повний foreign_key_check=ok. Окремий [validator](_assessments/2026-10-24-provenance/validate.py)
перевіряє ledger mapping і фактичний publication state; результат —
[validation.json](_assessments/2026-10-24-provenance/validation.json).
Dataset validating, active=NULL; це evidence для AT-06/07/09/18,
не production import/retry/CAS або завершена міграція.

`python3 -m unittest discover -s docs/_assessments/2026-10-24-provenance -v`:
4 tests пройшли; [test_probe.py](_assessments/2026-10-24-provenance/test_probe.py).
Покрито raw Unicode/CRLF round trip, FK rejection, hash collision failure,
відсутність publication і length-prefix encoding.

БД — 25.54 GiB без covering index. До ANALYZE — 100/100 SQL timeouts
через dataset scan; після ANALYZE — 0/100 errors, SQL p95 430.974 ms.
Локально з cgroup 768 MiB / 12.5% одного CPU — 20/100 mask timeouts,
сукупний p95 2 194.194 ms включає помилки й queue. Це невдала constrained
проба цього layout, не перевірка Oracle/HTTP/AT-16; потрібні оптимізація
та реалістичний профіль. Синтетичні тести не замінюють Go acceptance tests.

## Перевірка SQLite covering index — 2026-10-24

[Звіт](_assessments/2026-10-24-sqlite-indexed/REPORT.md): SHA-256 копії
повної моделі збігся з базовою БД до додавання index. Після DDL —
56 455 068 translations, dataset validating, active=NULL/generation=0.
EXPLAIN: prefix covering range, infix covering pair scan, без temp sort.
Повний source/DB/export reconciliation не повторювався; база копії —
попередній звірений прототип. Зміна індексу не реалізує production mask compiler.

Незмінений load.py попередньої проби: 100 unrestricted queries, 0 errors;
300 constrained queries при 10/s, 768 MiB/12.5% одного CPU — 0 errors,
p95 exact 0.395 ms, prefix 0.302 ms, infix 202.129 ms, сукупний 105.955 ms.
Попередні 20/100 mask errors усунені для цього профілю; різна кількість
спостережень зафіксована у звіті. Це SQL із теплим кешем на Ryzen,
не проходження Oracle/HTTP/AT-16. БД — 29.66 GiB; storage питання лишаються.
Нові Go acceptance tests не створені й не заявляються як пройдені.

## Компактні SQLite keys — 2026-10-24

[Звіт](_assessments/2026-10-24-sqlite-compact/REPORT.md): integer dataset/artifact
FK зі збереженням UUID та SHA-256 у catalog. Повне streaming порівняння
по 56 455 068 translations/ledger/origins, catalogs, checkpoint/export metadata
пройшло; foreign_key_check=ok. Це рівність звіреній базі, не повторний
source CSV audit або production importer. Dataset validating, active=NULL.

`python3 -m unittest discover -s docs/_assessments/2026-10-24-sqlite-compact -v`:
3 synthetic tests пройшли — lossless conversion, detection raw-origin corruption,
cross-dataset reference rejection. Фактичні шляхи й hashes — у звіті.

Повна БД з тими самими indexes — 20.85 GiB (−29.70%).
Незмінений load.py: 100 unrestricted та 300 constrained SQL queries,
0 errors в обох запусках; constrained p95 103.494 ms при 10/s,
768 MiB/12.5% одного CPU. Це не HTTP/Oracle/AT-16 acceptance.
Цей експеримент не реалізує acceptance tests сервісу; M1 загалом залишається open.

## Детальні контракти для тестів

- AT-01/09/11/18: [DATA_MODEL](DATA_MODEL.md).
- AT-06–11/18: [IMPORT_CONTRACT](IMPORT_CONTRACT.md), включно з CRLF і stale CAS.
- AT-02–05/15/16: [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md), включно з byte-limited pages.
- AT-12 скасовано; [LEGACY_COMPATIBILITY](LEGACY_COMPATIBILITY.md) — історична довідка.

## Go-каркас M1-01 — виконано 2026-10-24

Implementation: [cmd/slovnyk/main.go](../cmd/slovnyk/main.go),
[internal/ops](../internal/ops/), [compose.yaml](../compose.yaml),
[Makefile](../Makefile), [PostgreSQL smoke](../tests/postgres.sh).

| Частина критерію | Реальна перевірка | Результат |
|---|---|---|
| AT-14, Go build | `make build`, `go.mod`: Go 1.26.10, stdlib only | pass; сервер збирається без legacy runtime |
| AT-17, health/readiness | `internal/ops/http_test.go`: `TestHealthAndReadiness` | pass; 200 live, 503 storage_not_initialized |
| AT-17, safe config/logs | `internal/ops/config_test.go`: `TestConfigErrorsNeverContainValues`; `cmd/slovnyk/main_test.go`: `TestInvalidConfigDoesNotLogDSN`, `TestInvalidCommandDoesNotLogArguments` | pass; invalid input → exit 2 без synthetic secret |
| Process lifecycle | `internal/ops/http_test.go`: `TestShutdownDrainsInFlightRequest`, `TestShutdownDeadlineClosesConnections`, `TestListenerFailure` | pass; bounded drain/force close, safe bind failure |
| Config | `internal/ops/config_test.go`: `TestLoadConfig` | pass; defaults та environment overrides |
| Process smoke | Зібраний binary, GET/HEAD та справжні SIGINT/SIGTERM | pass, exit 0; query text не потрапляє в lifecycle logs |
| Dev/test DB environment | `make test-integration`, PostgreSQL 17.11, окремий project/random loopback port/volume, version + SELECT 1 | pass; власні ресурси видалені |
| Quality | `make test`, `make check`, `make test-race` | pass у двох Go packages; gofmt/vet/whitespace/race без помилок |

[Звіт](_assessments/2026-10-24-go-scaffold/REPORT.md),
[process smoke](_assessments/2026-10-24-go-scaffold/process-smoke.json) та
[підсумкові перевірки](_assessments/2026-10-24-go-scaffold/checks.json).
Повні AT-14/17 **не закриті**: storage driver/schema/active dataset/artifact
checks та різні причини DB unavailable/no active dataset ще не реалізовані.
PG smoke не перевіряє application transactions чи publication. M1-02–08 open;
Oracle й старий проєкт не задіяні.

## Регулярні посіпаки — 2026-10-24

Inline перевірки оформлено в [minions/](../minions/README.md), PostgreSQL
smoke перенесено зі `scripts/`; історичні hashes вище описують первинний запуск.
`make check`, `make test-smoke`, `make test-integration` після перенесення
пройшли. Окремо підтверджено rejection відсутнього Markdown target, кінцевих
пробілів та відсутнього HTTP binary; запуск Python-посіпак із `tmp/` працює.
Поточні результати — `tmp/minions/`, поза Git. Нового AT-покриття сервісу
це не додає; розробка M1 на паузі за проханням користувача.

## Формати й нормалізація M1-02 — виконано 2026-10-24

Після `continue` користувача локальну реалізацію відновлено.

| Частина критерію | Реальні test paths / scenarios | Результат |
|---|---|---|
| AT-02/03, Unicode/query/mask primitives | `internal/normalization/normalization_test.go`: `TestKeys`, `TestCompile`, `TestLongCombiningSequenceDoesNotInsertCGJ`, `TestIndependentUnicodeGolden` | pass; fold expansions, query-only trim, literal SQL metacharacters, explicit exact, controls/limits, 40 незалежних long-sequence goldens |
| AT-07/18, raw CSV і admission | `internal/importdata/csv_test.go`: `TestCSVFixtures`, `TestCSVAdmissionAndSyntax`, `TestCSVBoundaryAndRejectedOrdinals`, `TestArtifactLimit`, `TestCSVWriteRoundTrip` | pass; 1/2/3/7/4096-byte chunks, logical ordinals, BOM/CRLF/CR/LF, no final LF, field limits/reasons, writer failure |
| AT-07, gzip file completion | `internal/importdata/csv_test.go`: `TestGzipEOF` | pass; clean/multi-member EOF, truncated/corrupt trailer, trailing garbage |
| AT-07/18, manifest/identity | `internal/importdata/manifest_test.go`: `TestManifestGolden`, `TestTupleAndKeyGolden`, `TestManifestRejects`, `TestArtifactAndSyntheticReconciliation`, `TestInvalidFixtures` | pass; strict JSON/versions/paths, artifact hash, 11 occurrences / 10 unique / 1 duplicate; symlinks/size mismatch rejected |
| Quality / regression | `make build test check test-race test-smoke`; JSON Schema validation; `FuzzCSV` 15s, `FuzzManifest` 10s | pass; локально, без Oracle/legacy DB |

[Докази й обмеження](_assessments/2026-10-24-formats/REPORT.md).
AT-02/03 ще потребують SQL/HTTP lookup; AT-07/18 — staged import, ledger,
retry і publication. M1-03–08 open; жоден повний acceptance не оголошено закритим.

## PostgreSQL schema/storage M1-03 — виконано 2026-10-24

Implementation: [SQL](../migrations/000001_initial.sql),
[storage](../internal/storage/postgres/), `cmd/slovnyk` migration command,
[ізольований runner](../tests/postgres.sh).

| Частина критерію | Реальна перевірка | Результат |
|---|---|---|
| AT-01/18, schema і raw integrity | `postgres_integration_test.go`: `TestMigrations`, `TestConstraintsAndPrivileges` | pass; fresh/repeated migrations, checksum tampering refused; composite file/translation/run FK, hash/domain/field limits, 64 KiB raw fields, nullable/0/negative metadata |
| AT-02/15, exact storage primitive | `TestConstraintsAndPrivileges` під `slovnyk_http` | pass; bounded hash + residual equality, C-sorted raw variants, long keys і dataset isolation; HTTP/cursor ще немає |
| AT-09, transaction/ready backstops | `TestBatchRollbackAndPublicationConstraints`, `TestReadyTransitionSerializesWithWrites` | pass; rollback не лишає translation; ready waits for writer; terminal mutations, non-ready pointer, missing/fake event і stale generation refused |
| AT-01/18, privileges | `TestConstraintsAndPrivileges` | pass; HTTP не пише/не читає provenance, operator не змінює DDL/triggers/ledger; accepted origins match raw tuple; one-running-attempt backstop |
| Safe config/diagnostics | `migrate_test.go`: `TestConnectRejectsConfigWithoutLeakingValues`; `cmd/slovnyk/main_test.go`: `TestMigrateConfigErrorsDoNotLogSecrets` | pass; invalid config → safe error/exit 2, без DSN/password |
| Regression | `make build test check test-race test-smoke test-integration` | pass; окрема PostgreSQL 17.11, власні container/network/volume видалені |

[Звіт](_assessments/2026-10-24-postgres-schema/REPORT.md).
SQL guards не замінюють import verify або durable artifact/CAS service.
Повні AT-01/02/09/15/18 ще відкриті. M1-04–08 open; Oracle/legacy не задіяні.

## Staged import/retry M1-04 — виконано 2026-10-24

Реалізовано `internal/importer/`, `slovnyk import inspect/run`, session advisory
lock на виділеному connection, COPY batch, transactional ledger/origins/issues/
counters/checkpoint, deterministic IDs/ownership і перевірку committed prefix.
JSON містить completion/issue/accounting status; `inspect` не обіцяє distinct counts.
Успіх — `validating`, active pointer незмінний. [Звіт](_assessments/2026-10-24-staged-import/REPORT.md).

| Реалізована частина | Точні tests | Фактичний результат |
|---|---|---|
| Inventory/CLI, admission/hash, безпечні помилки | `internal/importer/importer_test.go: TestInspect`; `cmd/slovnyk/import_test.go` | PASS |
| Clean/retry після кожного batch, validating replay | `internal/importer/importer_integration_test.go: TestRetryEveryBatchAndValidating` | PASS; 6 місць interruption, різні batch sizes, тотожні IDs/raw/ledger/origins/pair counts |
| Batch rollback | `TestRollbackBeforeCheckpoint` у тому самому файлі | PASS; до commit не збережено жодної частини batch |
| Rejections, gzip/syntax/identity failure | `TestRejectionsFatalStreamsAndIdentity` | PASS; prefix accounting, incomplete/failed, без terminal resume |
| Session lock, orphan attempt, реальна втрата session | `TestSessionLockAndAbandonedAttempt`, `TestLostSessionStopsWriterAndCanResume` | PASS; bounded contention та retry після `pg_terminate_backend` |
| Raw collision/replay safety | `TestCollisionGuardBeforeMerge`, `TestReplayDetectsDamagedLedger` | PASS; failure до merge/додавання нових даних |
| Ownership між artifacts, порожній gzip | `TestCrossFileOwnershipAndEmptyArtifact` | PASS; порядок manifest/batch size не змінює результат |
| CLI під operator LOGIN | `cmd/slovnyk/import_integration_test.go: TestImportRunCLI` | PASS; 11 occurrences, 10 unique, 1 duplicate; повтор validating |

`make check`, `make test`, `make test-race`, `make test-smoke` і
`make test-integration` — PASS. Останній запускає schema/importer/CLI suites
послідовно на власній disposable PostgreSQL 17.11 та прибирає її ресурси.
Runtime logs — `tmp/m1-04/`; synthetic runtime fixtures також там.
AT-07/08/09/11/18 покриті частково; повний corpus, verify/exports/ready/CAS
і production capacity ще не перевірені. M1-01–04 done; M1-05–08 open.

## M1-05 — synthetic verification/publication, 2026-10-24

Реалізація: `internal/importer/reconcile.go`, `internal/artifacts/`,
`internal/publication/`, `cmd/slovnyk/dataset.go`, `internal/ops/readiness.go`.

| Перевірка | Фактичний test path / сценарій |
|---|---|
| AT-06 subset | `internal/publication/publication_integration_test.go`: source/ledger/origins/counts/metadata reconciliation, 4 per-pair exports, raw tuple digest + CSV4.gz round-trip; repeat verify/export; empty pair; rejected/gap/failed/corrupt fixtures |
| AT-10 subset | `internal/artifacts/store_test.go`: real child-process crashes на file installation boundaries, no-clobber inode reuse; verifier child exit before ready commit, transaction rollback; CAS concurrency/stale/ABA/retained rollback |
| AT-09/18 subset | `internal/ops/readiness_integration_test.go`: HTTP read-only role, 503→200 recovery, corrupt-file restart remains 503; `cmd/slovnyk/import_integration_test.go`: operator CLI positive verify/export/publish, stale generation |
| Regular-file guard | `internal/artifacts/store_linux_test.go`: FIFO відхиляється до blocking open |
| Configuration/privacy | `internal/ops/config_test.go`, `cmd/slovnyk/dataset_test.go`: paired HTTP settings, required generation/ref, safe bounded errors |

Фінальні `make check`, `test`, `test-race`, `test-smoke`, `test-integration` —
**PASS**, локально на disposable PostgreSQL 17.11; точний review scope — у
[звіті](_assessments/2026-10-24-publication/REPORT.md).
Повний corpus та power-loss/production durability gate не запускалися.
Loss approval adapter ще попереду; strict M1 path не обходить ready gate.
M1-01–05 done; M1-06–08 open. Наступна задача — M1-06.

## M1-06 — application search/cursor, 2026-10-24

Реалізація: `internal/lookup/`, `internal/storage/postgres/page.go`;
HTTP parsing/rendering залишаються M1-07.

| Покрита частина | Фактичні test paths / сценарії |
|---|---|
| AT-02/03/04/15 subset | `internal/lookup/lookup_integration_test.go: TestSearchModesAndStablePages`: exact/mask auto, explicit exact wildcard, Unicode/folding, literal `%`/`_`/backslash, same-locale, C ordering, raw grouping і paging без втрат/повторів |
| Pinned dataset/version, AT-04 subset | `TestCursorPinAuthorizationAndChanges`: retained старий dataset після publish, changed params/limit, signed missing/unpublished dataset, wrong pair/filter anchor, unavailable normalizer |
| Bounded pages/cursor, AT-15 subset | `TestByteBudgetAndLongAnchor`: 64 KiB fields, expanded keys, whole entries, last-included cursor, terminal page |
| DB deadline, AT-16 subset | `TestWholeLookupDeadlineAndRecovery`: pin delay + actual PostgreSQL lock, загальні 2 s → 503, recovery на новому connection; не production p95 |
| Cursor integrity | `internal/lookup/cursor_test.go`: `TestCursorGolden`, `TestCursorRejectsAlterations`, `TestKeyRotationAndCopies`, `FuzzCursor`; independent Python golden в `testdata/lookup/cursor-v1.json` |
| Validation/errors/budget | `internal/lookup/lookup_test.go`: `TestValidationBeforeStorage`, `TestFingerprintEffectiveMode`, `TestLargestEscapedEntryFits`, `TestErrorStatuses` |

`make check`, `make test`, `make test-race`, `make test-smoke` та
`make test-integration` — **PASS**, exit 0. Інтеграційний runner використовував
власну disposable PostgreSQL 17.11, після завершення видалив її ресурси.
Bounded `FuzzCursor` з `-fuzztime=10s -parallel=2` — **PASS**, 69 183 executions.
Runtime logs: `tmp/m1-06/` (ignored). [Звіт і review scope](_assessments/2026-10-24-lookup/REPORT.md).
Final HTTP response size/escaping, acquisition deadline та повні AT ще відкриті.
M1-01–06 done; наступний M1-07. Oracle і повний corpus не задіяні.

## Go tooling та review → PR → CI — 2026-10-24

За прямою вказівкою користувача tools розділено за призначенням:
`utilities/prepare-artifacts` — загальнодоступна service utility;
`tests/` — стандартні checks для людей/CI; `minions/` — особисті помічники агента.
Три Python-команди переписано на Go, shell test runners перенесено в `tests/`.
Regression paths: `tests/repository/check_test.go`,
`utilities/prepare-artifacts/main_test.go`, `tests/processsmoke/*_test.go`.
Деталі, review scope та результати — у [звіті](_assessments/2026-10-24-go-utilities/REPORT.md).
Це обслуговування tooling, не нове AT-покриття пошукового UI.

## M1-07 — перший HTTP зріз, 2026-10-24

M1-07 **in progress**; це не повне AT-05/19 або готовий локалізований UI.

| Покрита частина | Фактичні перевірки |
|---|---|
| AT-02/03/04/15 HTTP subset | `internal/httpui/handler_test.go`: validation precedence до backend, empty form, duplicates, invalid encoding/NUL/controls/size, single URL decode, escaping, HTML/plain continuation, 400/404/410/503, HEAD |
| Остаточний body limit | `TestFinalBodyBudget`: HTML expansion/plain overflow → коротка 503 до headers; boundary 1 MiB; без часткового успішного body |
| HTTP role + SQL | `internal/ops/readiness_integration_test.go: TestReadinessRecoveryWithHTTPRole`: synthetic publish, lookup і наступна сторінка під `slovnyk_http`, зміна формату між сторінками, locale/pair/no-match statuses |
| Connection deadline | `internal/ops/search_test.go`: canceled context і TCP peer, що не завершує PostgreSQL handshake; connect вкладається у спільні 2 s, не storage default 5 s |
| Config/process | `TestCursorKeyringConfiguration`: rotation, missing/invalid/duplicate keys і safe errors; `tests/processsmoke/smoke_test.go`: actual binary routes, health/readiness, SIGINT/SIGTERM, query не потрапляє в logs |

`make check`, `make test`, `make test-race`, `make test-smoke`,
`make test-integration` — **PASS**, exit 0. Лише disposable local PostgreSQL 17.11
та synthetic data. [Self-review і screenshots](_assessments/2026-10-24-http-lookup/REPORT.md).
Chromium CDP 9222, page scripts disabled: desktop/mobile форма; не повний browser
acceptance або перевірка всіх accessibility станів. Локалізація, catalog/cookies,
SEO, downloads, production p95 і повний corpus залишаються відкритими.

Follow-up transport review: [звіт](_assessments/2026-10-24-http-target/REPORT.md).
`TestTargetLimitBeforeRouting` та actual process smoke перевірили target guard
до canonical redirects: query 24 KiB → 200, понад 32 KiB → 414, включно з
`//search`. `make check test test-race test-smoke` повторно PASS після зміни.
Go parser має окремий hard ceiling 64 KiB для request line + headers.

## M1-07 — каталог, локалізація й налаштування, 2026-10-24

Наступний зріз покриває частину FR-05 / AT-05, FR-01 language inventory;
M1-07 загалом залишається in progress.

| Покрита частина | Фактичні перевірки |
|---|---|
| Browser/manual fallback, ресурси | `internal/i18n/i18n_test.go`: 31 catalog × 22 keys, 84 PO texts і 598 AI texts, provenance, mask hints, English fallback, manual priority, regional/script matching, nb-NO regression |
| Cookies, початкова/збережена пара | `internal/httpui/preferences_test.go`: незалежність UI/pair, річні host-only cookies, Secure/HttpOnly/SameSite, invalid/unsupported preferences, explicit/partial pair, unavailable saved pair без заміни, ambiguity і raw regional codes |
| Каталог і помилки | `TestCatalogRouteAndPinnedSearch`, `TestUnconfiguredCatalogNotice`, `TestCatalogFailurePreservesSearch`: HTML/plain inventory/counts, спільний pin/deadline, localized notices, 503 catalog vs 200 existing results/form |
| Реальний PostgreSQL + HTTP role | `internal/ops/readiness_integration_test.go`: imported/verified/published synthetic dataset, catalog read під slovnyk_http, browser proposal і manual cookie, continuation зі зміною format/UI, pinned catalog під AccessExclusive lock active pointer; initial read timeout і unknown dataset → unavailable |
| Actual process | `tests/processsmoke/smoke_test.go`: `/languages` GET/HEAD без storage → 503, `/?ui=uk` → localized HTML/cookie; попередні lifecycle/privacy/target checks збережено |

`make check test test-race test-smoke` і `make test-integration` — PASS, exit 0.
Лише disposable PostgreSQL 17.11 і synthetic fixtures; не capacity test.
[Self-review та screenshots](_assessments/2026-10-24-catalog-localization/REPORT.md):
наявний Chromium CDP 9222, page scripts disabled, synthetic handler fixture,
uk desktop/mobile, language catalog, German 320px layout. Це visual check,
не повний browser/accessibility acceptance. OQ-06 (мовна якість), фінальний
дизайн, SEO/downloads, production p95 і corpus migration лишаються відкритими.

## M1-07 — завантаження, 2026-10-25

FR-06 / AT-06 synthetic subset; M1-07 загалом лишається in progress.

| Покрита частина | Фактичні перевірки |
|---|---|
| Manifest і raw codes | `internal/httpui/downloads_test.go`: `TestDownloadManifest`, `TestPlainManifestPreservesRawLocales` — HTML/plain metadata, локалізація, final body guard, lossless TAB/CR/LF/quotes |
| File HTTP | `TestExportHTTPRepresentation`: GET/HEAD/ranges/ETag, attachment, immutable cache, error headers, закриття file descriptor |
| Validation/admission | `TestExportFailuresAndAdmission`: invalid IDs/query до backend, 404/503, максимум 8 передач |
| Великий stream | `TestLargeExportOutlivesSearchWriteTimeout`: actual HTTP server, valid gzip понад 1 MiB, передача після звичайного total write timeout без truncation |
| SQL/publication/files | `internal/ops/readiness_integration_test.go`: restricted HTTP role, gzip CSV parsing до EOF, rows/size/SHA, HEAD/range, unpublished rejection, retained download після publication іншого dataset, readiness gate, missing/symlink rejection |
| Process/resources | `tests/processsmoke/smoke_test.go`: downloads GET/HEAD без storage → 503; `internal/i18n/i18n_test.go`: 31 catalogs × 28 keys, 84 legacy / 784 AI texts |

`make check test test-race test-smoke` та `make test-integration` — **PASS**,
exit 0. Лише disposable local PostgreSQL 17.11 і synthetic fixtures; runner
видалив власні container/volume. Logs: `tmp/downloads/final-checks.log` і
`tmp/downloads/final-integration.log` (ignored). Перша перевірка links виявила
відсутній report; після його додавання повторний `make check` пройшов.
[Self-review і screenshots](_assessments/2026-10-24-downloads/REPORT.md).
Лише local synthetic data; не production acceptance або capacity proof.
SEO, мовне приймання та production migration залишаються відкритими.

## M1-07 — постійні словникові сторінки, 2026-10-25

FR-14 / AT-19 synthetic subset; повний sitemap/crawl acceptance ще відкритий.

| Покрита частина | Фактичні перевірки |
|---|---|
| Стабільна raw identity | `internal/entries/entries_test.go`: independent golden, різний case/NFC/пробіли/напрямок, length-prefixed encoding, fixed URL для 64 KiB |
| SQL, publication і continuation | `internal/entries/entries_integration_test.go: TestPublishedDictionaryPages`: restricted HTTP role, SQL/Go parity, index-compatible plan, 50+5 translations, cursor іншого написання, unpublished rejection, active replacement/removal, retained continuation під lock active pointer, deadline |
| Upgrade physical schema | `internal/storage/postgres/postgres_integration_test.go: TestEntryIndexUpgradePreservesReadyData`: schema 1 відхилено, migration 2 працює на ready dataset без зміни 64 KiB raw text |
| HTTP indexing і discovery | `internal/httpui/entries_test.go`: anonymous cookie-independent HTML, escaping, canonical fixed origin, content languages, HEAD, continuation noindex, final body guard, strict query, search links до окремих raw variants |
| Robots/domain/process | `TestRobotsAndPrimaryDomain`: GET/HEAD, fixed-origin 308 із query, forwarded headers ignored; `tests/processsmoke/smoke_test.go`: actual binary robots та storage-unavailable word routes |

`make check test test-race test-smoke` і `make test-integration` — **PASS**,
exit 0, local disposable PostgreSQL 17.11, synthetic data. Logs:
`tmp/seo/checks.log`, `tmp/seo/final-integration.log` (ignored).
Початковий integration fixture помилково повторно використовував connection,
який publication закриває, та не створював artifact root; fixture виправлено,
повний повторний suite пройшов. Це не зміна production ownership contract.
[Self-review і screenshots](_assessments/2026-10-25-dictionary-pages/REPORT.md):
наявний Chromium CDP 9222, page scripts disabled, desktop/mobile visual check.
Повний корпус, index build cost, crawler capacity, deployment TLS/redirects,
мовне/дизайн/accessibility приймання не перевірялися; M1-07 in progress.

## M1-08 — наскрізний CLI/HTTP lifecycle, 2026-10-25

`tests/serviceflow/lifecycle_test.go: TestServiceLifecycle` включено до
`make test-integration`, `-race` для test harness; child service — звичайний build.
Сценарій не викликає application services напряму: CLI subprocesses та loopback
HTTP працюють із новою порожньою БД й окремими operator/HTTP LOGINs.

| Покрита частина | Фактичні перевірки |
|---|---|
| AT-07/08/10 synthetic path | CLI migrate/import (batch 7)/verify/exports/publish A; stale generation → conflict; publish B; rollback A з перевіркою receipts/generation |
| AT-02/03/04/15 HTTP subset | Exact/mask, 1-row continuation A після publish B і restart, stable article A→B→A, retained article continuation A, invalid cursor → 400 |
| AT-06 synthetic download | Manifest link; compressed size/SHA відповідають verify report; gzip CSV до EOF містить 53 рядки; A export незмінний після B і restart |
| AT-17 readiness subset | Порожній active → 503; publish → ready; лише власна БД ALLOW_CONNECTIONS=false + terminate sessions → health 200/search 503; відновлення connections → автоматично ready й попередні results; process restart |
| Ролі/приватність/cleanup | HTTP LOGIN не superuser, UPDATE active pointer заборонено; child env очищено від успадкованих SLOVNYK_*; query/password/DSN відсутні в logs; graceful shutdown; власні БД/LOGINs/processes/scratch видалено |

`make build check test test-race test-smoke`, `go vet -tags integration ./tests/serviceflow`
та `make test-integration` — **PASS**, exit 0. Повний integration suite також
повторно перевірив `TestRetryEveryBatchAndValidating`,
`TestLostSessionStopsWriterAndCanResume`, `TestRejectionsFatalStreamsAndIdentity`,
`TestVerificationFaultIsolation`, `TestVerificationCrashProcess` та довгі
raw fields/body limits у lookup/httpui suites. Це різні перевірки, не всі
fault scenarios відтворено в одному process-level lifecycle.

Лише disposable local PostgreSQL 17.11 та synthetic corpus; container/volume
прибрано runner. Logs: `tmp/serviceflow/checks.log`, `final-integration.log`
(ignored). [Self-review](_assessments/2026-10-25-service-lifecycle/REPORT.md).
Немає browser/capacity/full corpus/off-host restore/production acceptance;
M1-07 мовне/дизайн-приймання та full SEO discovery відкриті. Цей зріз не
закриває всі AT і не змінює вимоги.

## M1-07 — офлайн sitemap, 2026-10-25

FR-14 / AT-19 offline subset; HTTP discovery та full-corpus acceptance відкриті.

| Покрита частина | Фактичні перевірки |
|---|---|
| XML, limits, identity | `internal/sitemaps/sitemaps_test.go: TestShardBoundariesAndDeterminism`: 1/50 000/50 001 URL, namespace, fixed canonical URLs/root-level shard links, counts, bytes/SHA, durable manifest, deterministic retry зі збереженням inode |
| Failure/retry | `TestIncompleteStreamsNeverCommitReport`: empty/invalid/duplicate/decreasing IDs, iterator failure, cancellation, shard limit → без success/report/temp files; `TestCorruptExistingShardFailsClosed`: corrupt existing shard відхиляється без overwrite |
| SQL та state | `internal/sitemaps/sitemaps_integration_test.go: TestReadyDatasetDiscovery`: operator role, validating/unknown/invalid rejection, ready pre-publication, raw case/NFC/spaces/pair identities, кілька translations → один URL, unchanged active/generation, незмінний результат після publish |
| CLI та CI | `cmd/slovnyk/import_integration_test.go: TestDatasetCLI`: verify/export/sitemap із JSON dataset; `TestDatasetArgumentsAndSecrets`: invalid flags/DSN без витоку; `tests/postgres.sh` запускає sitemap suite у disposable DB |

`make build check test test-race test-smoke`,
`go vet -tags integration ./internal/sitemaps ./cmd/slovnyk` та
`make test-integration` — **PASS**, exit 0. Фінальна integration перевірка
включає buffered writes/root-level links та весь попередній lifecycle suite.
Перша fixture не мала різних translations одного source; її замінено спеціальними
synthetic records, не послабленням dedup assertion. Logs:
`tmp/offline-sitemaps-checks.log`, `tmp/offline-sitemaps-final-integration.log`
(ignored). Локальний PostgreSQL 17.11; runner прибрав container/network/volume.
[Self-review](_assessments/2026-10-25-offline-sitemaps/REPORT.md).
Повний SQL sort/temp-disk cost, crawler load, HTTP serving, publication inventory
і production acceptance не перевірені; M1-07 in progress.

## M1-07 — HTTP sitemap discovery, 2026-10-25

FR-14 / AT-19 synthetic discovery subset. Schema 3, без змін raw identity.

| Покрита частина | Фактичні перевірки |
|---|---|
| Registry/state/privileges | `internal/sitemaps/sitemaps_integration_test.go: TestReadyDatasetDiscovery`: generated inventory registration/retry, HTTP read, unpublished rejection, active index, same-size corruption → Check failure, direct writes/HTTP register denied, incomplete SQL function call rolls back inventory |
| Physical upgrade | `internal/storage/postgres/postgres_integration_test.go: TestEntryIndexUpgradePreservesReadyData`: schema 1 → current 3, старий inventory відхилено, ready raw data незмінні |
| HTTP semantics | `internal/httpui/sitemaps_test.go: TestSitemapHTTP`: GET/HEAD, XML/ETag/cache policies, fixed IDs, cookies/language/forwarded headers не впливають, strict query/path, conditional 304, Range 206/416, safe no-store/noindex errors |
| Robots/process | `TestRobotsAndPrimaryDomain`: fixed canonical Sitemap directive; `tests/processsmoke/smoke_test.go`: real binary GET/HEAD index без storage → 503 |
| Publication/recovery | `tests/serviceflow/lifecycle_test.go: TestServiceLifecycle`: actual CLI sitemap before publish, unpublished shard denial, active index A→B→A, retained A, index/shard hashes/cache headers, DB outage/recovery, restart; same-size corrupt registered shard blocks readiness on another restart |

`make build check test test-race test-smoke`,
`go vet -tags integration ./internal/sitemaps ./internal/ops ./internal/storage/postgres ./tests/serviceflow`
та `make test-integration` — **PASS**, exit 0. Останній integration run включає
вузький SQL write API та додатковий restart із same-size corrupt shard.
Synthetic local PostgreSQL 17.11, окремі LOGINs у lifecycle; runner прибрав
container/network/volume. Logs: `tmp/http-sitemaps-final-checks.log`,
`tmp/http-sitemaps-final-integration.log` (ignored).
Повний corpus/crawler capacity, readiness hashing cost і production acceptance
не доведені. Inventory optional для publication, HTTP mount immutable/read-only.
[Self-review](_assessments/2026-10-25-http-sitemaps/REPORT.md).

## M1-07 — Багатомовна доступність, 2026-10-25

FR-05 / AT-05, technical synthetic subset; M1-07 лишається in progress.

| Покрита частина | Фактична перевірка |
|---|---|
| Keyboard/no-JS search | Chromium/CDP 9222: перший Tab фокусує локалізований skip link, Enter → main, Tab → query; synthetic Arabic query через keyboard submit створює GET `/search` з точною парою й текстом |
| Language/direction | DOM/computed styles: Ukrainian UI, `ar-SA` source з RTL, `en-US` target; German UI з Arabic/Latin target rows у різних напрямках; нерозбірні tags → explicit unknown language |
| Landmarks/reflow | Browser accessibility tree містить main/navigation/search; layout metrics без horizontal overflow при 320px для search, article з long/multiline текстом, languages, downloads, no-match і failure; desktop 1280px |
| Translation integrity/regressions | `internal/i18n/i18n_test.go: TestCatalogCompletenessAndProvenance` — 31 draft catalogs × 29 keys; наявні HTTP escaping, plain/HTML, errors, continuation і final body budget tests |

`make build check test test-race test-smoke` — **PASS**, exit 0. Browser checks —
PASS, exit 0. [Report і screenshots](_assessments/2026-10-25-multilingual-accessibility/REPORT.md).
Preview використовував actual HTTP handler із synthetic callbacks без БД.
PostgreSQL код не змінено; локальний integration suite для цього зрізу не
повторювався. Screen reader speech, інші browser engines, повний accessibility
аудит, фінальне мовне/дизайн приймання та M2/M3 не підтверджені.

## FR-13 — локальна backup/restore репетиція, 2026-10-25

AT-13 synthetic subset; окрема БД/root у тому самому disposable cluster.

| Покрита частина | Фактична перевірка |
|---|---|
| Повнота БД | `tests/serviceflow/restore_test.go: tableInventory/restoreFixture`: custom dump/restore, counts і length-prefixed SHA-256 sorted JSON rows кожної таблиці, включно з manifests/provenance/ledger/reports/active generation/history |
| Файли | `restoreFixture`: всі artifact/source relative paths, sizes і SHA-256 збігаються після копіювання у нові roots; 10 artifacts і 4 CSV/manifest source files |
| Відновлений сервіс | `tests/serviceflow/lifecycle_test.go: TestServiceLifecycle`: actual binary читає нову БД, поки вихідна недоступна; byte-identical exact/mask search, search/article continuations, active download; retained B download hash і shard, active sitemap hashes/cache |
| Неповний комплект і права | Той самий lifecycle: empty artifact root → health 200, readiness/sitemap/download 503; повний root → ready, HTTP UPDATE denied, actual operator CLI повторює sitemap registration зі сталим report |

`make build check test test-race test-smoke`,
`go vet -tags integration ./tests/serviceflow` і `make test-integration` —
**PASS**, exit 0. Final integration включає restored mask comparison та
race-enabled harness. Локальний PostgreSQL 17.11; runner прибрав власні
container/network/volume. Logs: `tmp/backup-restore/checks.log`,
`tmp/backup-restore/final-integration.log` (ignored).
[Self-review і межі](_assessments/2026-10-25-backup-restore/REPORT.md),
[процедура](BACKUP_RESTORE.md). Fresh-cluster roles, off-host transfer, secure
keyring/config recovery, full-corpus capacity і RTO <=24 h не перевірено;
FR-13/AT-13 повністю не закриті. Production runtime/schema не змінено.
