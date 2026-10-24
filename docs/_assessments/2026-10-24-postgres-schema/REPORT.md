# M1-03: PostgreSQL schema та storage — 2026-10-24

## Реалізація

`migrations/000001_initial.sql` створює лише нову схему `slovnyk`:
datasets/catalog/translations, files/ledger/origins/runs/checkpoints/issues,
legacy snapshot metadata, exports, active generation та publication history.
Composite FK зберігають dataset identity. SHA-256 domains мають рівно 32 bytes;
SQL перевіряє manifest/tuple/key hashes. Raw поля — до 64 KiB без B-tree індексу
на довгих texts; exact index містить лише bounded locale/hash keys.

Ready/failed content immutable; row-level `FOR SHARE` серіалізує його запис
із ready transition. Raw translation identity незмінна після insert; origins
пов'язуються з accepted ledger occurrence та звіряють чотири raw поля.
Active pointer має generation+1/ready backstops; deferred constraints вимагають
matching append-only event у тій самій транзакції.

`internal/storage/postgres/` використовує pinned `pgx/v5 v5.11.0`;
embedded migrations виконуються атомарно під advisory lock і checksum ledger.
`slovnyk migrate` вимагає явний `SLOVNYK_MIGRATION_DSN`, має bounded timeout
і безпечні логи без driver/DSN details. HTTP startup не виконує DDL.
Schema check перевіряє версії/хеші; exact primitive використовує SQL parameters,
indexed hash + residual equality та C sort. Групи HTTP/operator не мають LOGIN;
HTTP читає runtime tables, operator не володіє схемою й не може змінювати DDL,
triggers, migration ledger або виконувати TRUNCATE. Elevated/inheriting наявні
групи відхиляються; login credentials provision-яться окремо.

## Фактичні перевірки

`make test-integration` піднімає власну PostgreSQL 17.11 із pinned image,
random loopback port і synthetic credentials; CLI migrate застосовує schema.
Go integration tests підтвердили:

- fresh/repeated migrations, rebuild з нуля та checksum mismatch refusal;
- 64 KiB source/target/key, normalized raw variants, exact/C order і dataset isolation;
- cross-dataset translation/run checkpoint FK, malformed hash/oversize/duplicate rejection;
- accepted/duplicate origins і raw identity, NULL/0/negative metadata;
- HTTP read-only та operator DDL/trigger/ledger restrictions;
- rollback batch, terminal immutability, non-ready/missing-event/stale/fake publication rejection;
- concurrent content writer блокує ready transition; після commit ready нові записи refused.

`make build test check test-race test-smoke` — exit 0; lifecycle SIGINT/SIGTERM
і readiness 503 збережені. `TestMigrateConfigErrorsDoNotLogSecrets` підтверджує
exit 2 без synthetic password у логах. Integration runner — exit 0, власні
container/network/volume видалені. Логи: `tmp/m1-03/checks.log`, `integration.log`.

## Межі й наступний крок

Це schema/storage primitives, а не staged import або publication service.
Немає replay/checkpoints application logic, session writer lock, artifact verify,
fsync/no-clobber, expected-generation CLI чи HTTP lookup/cursor. Ручний SQL
owner/operator не є способом обійти затверджений verify contract; SQL readiness
checks не доводять наявність durable files. HTTP storage readiness підключається
у M1-05; наразі 503 `storage_not_initialized`. Повні acceptance ще не закриті.

PostgreSQL — локальна baseline; вибір production engine/server не змінено.
Oracle, legacy checkout і реальний корпус не змінювалися; браузери не запускалися.
`AGENTS.md` не змінювався. Наступне — M1-04, inventory та staged import/retry.

Перевірені primary references: [pgx](https://pkg.go.dev/github.com/jackc/pgx/v5),
[PostgreSQL row/advisory locks](https://www.postgresql.org/docs/17/explicit-locking.html),
[GRANT](https://www.postgresql.org/docs/17/sql-grant.html).
