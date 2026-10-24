# M1-04: staged CSV import/retry — 2026-10-24

## Реалізація

`internal/importer/` з'єднує strict manifest V1, immutable compressed artifacts,
lossless CSV4.gz parser, identity mapping і PostgreSQL. CLI:

```sh
./bin/slovnyk import inspect --manifest testdata/import/manifest.json --source testdata/import
./bin/slovnyk import run --manifest testdata/import/manifest.json --source testdata/import --batch-size 100
```

`inspect` не потребує БД і звітує occurrences/rejections; distinct counts невідомі
(`distinct_counts_available=false`). `run` вимагає `SLOVNYK_OPERATOR_DSN` до
окремої нової БД. JSON report — stdout; exit 0 — виконана команда, 1 — failure,
2 — некоректні аргументи/config/manifest. Відхилені записи видимі у звіті;
успішне staging із ними **не означає** дозволу на публікацію.

Fingerprint визначає dataset. Artifacts обробляються за SHA-256, записи — за
ordinal; перше raw входження володіє `accepted_unique`, решта — duplicates.
Tuple hash match звіряє всі чотири raw поля до merge. Translation IDs
детерміновані відносно цього порядку; batching не змінює ідентичність.
Обидва legacy-коди `sr-sp`/`sr-rs` лишаються окремими. CSV4 metadata — SQL NULL;
0, count/status чи історія запитів не вигадуються. Exceptions не застосовуються
як автоматичний дозвіл вилучити дані.

Виділене pgx connection тримає session advisory lock увесь attempt і завжди
закривається після нього. Mapping lock key V1: перші 8 bytes
SHA-256(`slovnyk-import-lock-v1\x00` + fingerprint), signed big-endian int64.
Типовий lock wait — 5 s. Попередній orphan `running` стає `interrupted`
тільки після захоплення lock; новий attempt отримує окремий UUID.

Пакети — до 1000 records (типово 100), до 4 MiB decoded fields; приватна temp
таблиця наповнюється COPY, далі SQL обробляє пакет множинами. Translations,
ledger, origins, issues, pair/file counters і checkpoint комітяться разом.
Операторському login потрібні права групи `slovnyk_operator` і database TEMP;
DDL application schema він не виконує. Застосовані migrations не змінено.

Перед parsing перевіряються compressed size/hash; той самий descriptor хешується
повторно під час parsing. EOF включає gzip checksum/trailer. Snapshot має
залишатися immutable протягом команди. Retry перечитує файл, звіряє committed
prefix із ledger/raw origins, не додає origins повторно. Partial batch при
помилці відкочується. Fatal syntax/hash/collision/replay failure → `failed`;
DB/process interruption лишає retryable staging. Failed dataset не resume-иться.

Успішний `run` завершується **`validating`**, без ready/report/export/publish.
Active pointer жодна імпортна операція не змінює. JSON є оперативним звітом,
а не durable owner-approved loss/verify report M1-05.

## Фактичні перевірки

На disposable PostgreSQL 17.11 через `make test-integration`:

- Clean fixture: 11 occurrences, 10 distinct tuples, 1 duplicate, 11 origins,
  4 language pairs; raw CR/LF/CRLF, Unicode, пробіли й локалі збережені.
- `TestRetryEveryBatchAndValidating`: interruption після кожного із 6 пакетів,
  resume з іншим batch size і повтор у validating; повна рівність IDs, raw tuples,
  keys/hashes, ownership, origins і pair counts з clean run.
- `TestRollbackBeforeCheckpoint`: fault перед commit не залишає частини пакета.
- `TestRejectionsFatalStreamsAndIdentity`: blank/NUL/invalid UTF-8 окремо враховані;
  syntax/trailer/hash failures блокують завершення, failed незворотний.
- `TestSessionLockAndAbandonedAttempt`, `TestLostSessionStopsWriterAndCanResume`:
  bounded contention, завершення writer після реального `pg_terminate_backend`,
  відновлення committed prefix і orphan attempt.
- `TestReplayDetectsDamagedLedger`, `TestCollisionGuardBeforeMerge`:
  зіпсований locator і штучний hash match з іншими raw полями відхиляються.
- `TestCrossFileOwnershipAndEmptyArtifact`: duplicate між файлами, порожній gzip,
  незалежність ownership від manifest order і batch size.
- `cmd/slovnyk/import_integration_test.go`: inspect/run CLI, повторне staging
  під окремим LOGIN, який успадковує operator privileges; без schema-owner прав.
- Unit/CLI tests перевіряють inspect, hash failure, cancellation, відсутність
  аргументів/DSN/secrets у JSON помилках. Duplicate artifact hash уже покритий
  strict manifest tests M1-02.

`make check`, `make test`, `make test-race`, `make test-smoke` також пройшли.
HTTP досі має readiness 503; smoke не потребує браузера.
Виявлено race у Compose healthcheck: Unix socket міг прийняти тимчасовий
init-сервер за готовий. Healthcheck і runner тепер перевіряють TCP loopback;
ініціалізаційний сервер цього порту не слухає.

Runtime evidence: `tmp/m1-04/integration.log`, `checks.log`; тимчасові synthetic
fixtures також у `tmp/m1-04/` і видаляються після тестів. PostgreSQL minion
видаляє власні container/network/volume; suites запускаються послідовно,
оскільки schema suite навмисно перебудовує схему своєї disposable БД.

## Межі результату

M1-04 done; AT-07/08/09/11/18 покриті лише в synthetic CSV staging частині.
Повний corpus через цей Go importer ще не завантажувався; throughput/storage
і production capacity не виміряні. Verify, loss approval, durable exports,
ready, publication/rollback CAS та HTTP storage readiness — M1-05.
Oracle, legacy code/data, експериментальні БД і `AGENTS.md` не змінювалися.
Playwright, браузер і зовнішні AI API не використовувалися.
