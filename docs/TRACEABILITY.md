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
[Makefile](../Makefile), [PostgreSQL smoke](../minions/test-postgres.sh).

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
[ізольований runner](../minions/test-postgres.sh).

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
