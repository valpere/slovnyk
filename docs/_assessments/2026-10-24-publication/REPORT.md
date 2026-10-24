# M1-05 — verification, immutable artifacts та publication

Дата: 2026-10-24. Scope: локальний synthetic CSV adapter, PostgreSQL 17.11.
Baseline commit: `47739920abe34309854132424c75a42ec2a3c186`.

## Реалізація

- `internal/importer/reconcile.go`: replay всіх source bytes/EOF/checksums,
  counts/checkpoints/ledger, raw translations/normalized keys та origins.
  COPY/replay helpers спільні з importer; попередні retry tests збережені.
- `internal/publication/`: exact catalog inventory, deterministic unique ownership,
  NULL legacy metadata/identity provenance, per-pair C-sort tuple digest,
  lossless CSV4.gz round-trip, exact/literal-mask SQL controls на першому tuple.
- `internal/artifacts/`: complete temp inode, chmod/fsync file, SHA-256,
  atomic hard-link installation без overwrite, fsync directories; final paths
  версійні та content-addressed. Весь artifact inventory/report durable до ready.
- `dataset verify/export/publish/rollback`: окремий operator DSN, dedicated
  connection; shared import/verify writer lock і dataset row lock. Publish
  перевіряє report/files під active row lock, CAS generation і append-only history
  commit разом. Rollback лише до раніше опублікованої ready version.
- HTTP: read-only role/mount, schema/report/artifact checks до readiness;
  background pointer polling, повна перевірка на startup/reconnect/new generation.
  Readiness без storage config лишається 503. Operator HTTP routes відсутні.

## Рев’ю

Послідовне рев’ю за `code-review`: contracts, state/concurrency, failures,
callers/tests та документація. Це self-review, не незалежний консиліум.
Scope зафіксовано у `reviewed-inputs.json` (SHA-256 файлів фінального diff;
цей report та сам inventory не включені, щоб уникнути self-reference).

Підтверджений P2, виправлений до commit: artifact reader перевіряв regular-file
лише після open; FIFO міг заблокувати startup/publish. Тепер regular-file guard
виконується до open, Linux regression `TestRefuseFIFOWithoutOpening` проходить.
Додатково HTTP-role test явно перевіряє `current_user`, а не покладається на
мутований pgx config при повторному використанні original connection string.
Невиправлених підтверджених findings у фінальному scope не залишилося.

## Перевірки

`make check`, `make test`, `make test-race`, `make test-smoke`,
`make test-integration` — **PASS** після фінальних виправлень.
Логи: `tmp/m1-05/check.log`, `go-checks.log`, `integration.log` (ignored).
Provisioning minion також виконано на окремому root у `tmp/m1-05/`.
GitHub CI запускається після commit/push; його результат наведено у відповіді.

Нові перевірки:

- Store: повторний install зберігає inode; corrupted existing target не
  перезаписується; path/symlink/FIFO guards, partial write і fault propagation.
- Child-process exit на written/file-synced/linked/directory-synced boundaries:
  final file видимий лише повністю; retry без clobber. Реальний verifier child exit
  після reports/files до ready commit: DB лишається validating, exports rollback.
- PostgreSQL: 11 occurrences = 10 unique + 1 duplicate, 4 pairs; повторний
  verify/export тотожний; empty-pair export; stale CAS, concurrent writers,
  A→B→A rollback/ABA; never-published rollback rejected.
- Negative fixtures: missing source/file, corrupted report/export, failed state,
  wrong pair count/origin locator/provenance/metadata, rejected records і known gap;
  pointer/history не змінюються, окрема clean version працює після failures.
- HTTP role: CheckActive без private ledger grants; `/readyz` 503→200 після
  recovery, corrupted artifact після restart не стає ready. CLI positive flow
  verify/export/publish та stale publish; аргументи/DSNs не потрапляють у errors.

## Межі та продовження

Це synthetic AT-06/10 subset, не повна production durability/capacity перевірка.
Process termination не моделює power loss, controller cache чи втрату диска.
Потрібен локальний filesystem з atomic hard links/fsync; root provisioned через
`minions/prepare-artifacts.py`. HTTP кешує перевірені immutable bytes до зміни
покоління/recovery; final files заборонено змінювати чи прибирати. GC вимкнений.

Нині будь-які issues/rejections/gaps/exceptions блокують ready: окремий
owner-approved loss adapter ще не реалізовано. Права користувача та раніше
погоджений процес схвалення втрат цим не переглядаються. Повний реальний Go
import, production storage budget/backup/restore та цільовий сервер не перевірені.
Oracle, legacy code/data, експериментальні БД та AGENTS.md не змінювалися.
M1-01–05 done; далі M1-06 — search/cursor.
