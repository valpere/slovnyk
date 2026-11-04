# Повний локальний backup/restore

## Результат

2026-11-04 виконано logical backup і restore retained корпусу в **інший порожній
PostgreSQL cluster**: 56 455 068 translations, 992 pairs, ready/active generation 1.
Усі 17 таблиць, identity sequence, artifact/source bytes та HTTP controls
збіглися. Вихідна БД під час target HTTP acceptance була **зупинена**.
[Assertions](assertions.json), [lifecycle](lifecycle.json).

Від початку backup до acceptance — **3333.552 s**;
разом із cleanup — 3391.861 s.
Це локальна developer-host репетиція; production/off-host RTO <=24 h не закрито.

## Комплект та відновлення

Operator writers не працювали; перед dump не було інших client backends.
Пауза writers збережена через dump і файлове копіювання. [Protocol](protocol.json).
Pinned PostgreSQL 17.11 `pg_dump --format=custom --no-owner`: exit 0,
**6,400,095,751 bytes**, 533.956 s включно з hash.
[Archive SHA-256](dump.json). Повний dump містить schema, migration ledger,
raw/provenance/report/publication rows та sequence state; cluster globals не копіювалися.

Окремі regular-file копії **1731 artifacts і 992 CSV.gz**,
manifest та перевіреного release binary створені з fsync; розміри/SHA-256
перечитані з джерела та backup. Потім artifacts/sources/manifest скопійовані ще
раз у нові restore paths і звірені. Hardlinks/reflinks не використовувалися.
[Повний metadata inventory](file-inventory.json), [copy receipt](files-restored.json).
Application binary з попередньої перевірки (`dd2039a`), SHA той самий;
application code не змінювався, нові зміни — test/doc/evidence.

Новий target має інший system identifier, новий NOLOGIN non-superuser owner,
окремі restricted LOGINs і canonical NOLOGIN groups; source owner спочатку
відсутній. [Startup](target-startup.json).
`pg_restore --no-owner --role=<new-owner> --exit-on-error --single-transaction`
завершився exit 0 за **2258.679 s**; без `--clean`/`--no-acl`
і без попереднього створення application tables. До HTTP acceptance виконано
[ANALYZE](analyze-execution.json) для оновлення planner statistics, як рекомендує
[PostgreSQL 17](https://www.postgresql.org/docs/17/app-pgrestore.html#APP-PGRESTORE-NOTES).
[Execution](restore-execution.json), [disk/time observations](restore-progress.jsonl).
Ця повна репетиція не повторює дорогий failed restore без groups; atomic fail-closed
case покритий synthetic `TestServiceLifecycle`.

## Суцільна перевірка даних та HTTP

[Source](source-inventory.json) і [target](target-inventory.json) тотожні.
Потоково прочитано всі колонки кожного рядка всіх 17 таблиць через binary COPY;
для кожного row hash враховано кількість fields, length/null markers та bytes.
Oracle — row count, sum SHA-256 modulo 2^256 та XOR SHA-256, незалежно від
фізичного порядку рядків. Це probabilistic multiset check, не canonical migration
digest або sorted-JSON oracle synthetic test; підтримано поточні built-in/domain
binary encodings. Сирі row bytes не зберігалися. Окремо зіставлено
`last_value/is_called` і [sequence config](target-sequence-config.json).
У поточному корпусі sequence не використана; synthetic test перевіряє non-default
`37 / true`. [Baseline](baseline.json) і [final state](target-final-state.json) тотожні.

[HTTP acceptance](target-http-check.json): 16 exact, 4 prefix,
16 word response bodies та 1 search continuation body
побайтово тотожні source, HTTP status — 200. Інші headers цим oracle не порівнювалися.
Cursor виданий у цій репетиції зі спільним приватним keyring; історичних production
keys/cursors немає. Усі **736 sitemap shards і 992 exports**, а також index
повністю перечитані через target HTTP і звірені з початковими hashes/bytes.
Другого retained dataset у цьому корпусі немає; A/B retained case покрито synthetic test.

Empty artifact root: health 200, readiness/sitemap 503. Повний root: readiness 200.
[Missing-files check](missing-artifacts.json), [readiness](target-readiness.json).
Новий owner володіє schema/relations/functions/types; HTTP UPDATE/DDL/register
та operator direct DELETE/DDL реально відхилені з SQLSTATE 42501.
[ACL checks](ownership-privileges.json). Bundled CLI повторив `dataset export`
під новим operator LOGIN; existing sitemap report registration повторена через
`SET LOCAL ROLE` operator без регенерації, перевіривши SECURITY DEFINER з новим owner.
[Export retry](operator-retry.json), [registry retry](operator-sitemap-retry.json).

## Відхилення та відкриті перевірки

Початковий capture exact/prefix/word responses зупинився на 503. Через відсутність
часткового checkpoint точний запит того запуску не збережений. Окрема
[діагностика всіх 32 запитів](source-http-diagnostic.json)
показала 16/16 exact 200 і **6/16 prefix 503** приблизно за 2 s — загальний
lookup budget. Окремий SQL timer/план не досліджувався; не називаємо всі 503
доведеними DB timeouts. Цей результат не прихований і не виправлений зміною deadline.
Для restore controls після діагностики залишено exact/word для cases 0..15,
prefix лише cases 0..3 і отриманий cursor. Це **відбір після спостереження успіхів**,
придатний для порівняння відновлення, не для оцінки capacity або частки успішних масок.
Поширити попередній mixed-load висновок на цей `limit=1` workload не можна.
Query-plan/deadline дослідження залишається наступною задачею; AT-16 відкрито.

Два harness issues усунено до restore: readiness через socket бачила тимчасовий
init server (перехід на TCP check), а відкрито названий target-pgdata заважав
`go vet ./...` через приватні PG permissions (bind перенесено в `.pgdata`, target
перезапущено, ephemeral port визначено заново). Це не дефекти app restore.

## Ресурси, зберігання та межі

Source DB: 8 GiB, CPU cap під час dump змінено 1 → 2;
[transition](source-resource-transition.json). Target: 2 CPU/8 GiB,
maintenance_work_mem 512 MiB, parallel maintenance workers 0, per-process temp cap
20 GB; fsync/full_page_writes/synchronous_commit on. HTTP: 1 CPU/512 MiB,
non-root, loopback only, read-only rootfs/mounts. Це не fixed-profile benchmark.
Docker `OOMKilled=false` для обох DB containers; disk floor 30 GiB і time guard
не спрацювали. Це не окремий cgroup-level облік OOM events.
[Runtime](runtime-resources.json), [source size](source-storage.json), [target size](target-storage.json).
Source database 64.316 GiB → restored database
**57.584 GiB**; WAL/temp/artifacts/backup окремо.
Це не підтвердження придатності Oracle root або memory capacity.

Власні containers/networks і тимчасові HTTP/operator LOGINs прибрано.
Backup bundle, restored `.pgdata` та обидві файлові копії збережені в ignored
`data/experiments/corpus-restore-2026-11-04/`. Оригінальні DB/snapshot/artifacts
залишилися на [попередніх шляхах](../../../data/README.md).
Приватні env/DSN/keyring та terms/cursor paths лишаються поза Git. Створені для
репетиції HTTP/target-operator LOGINs видалені; їхні credentials непридатні.
Початкові source admin/operator credentials залишаються приватними й не відкликані.
Новий owner/admin збережені локально для
наступних перевірок. App logs перевірено приватно; у Git тільки lifecycle metadata.
Oracle й legacy не задіяні.

Off-host transfer/durability, secret escrow, schedule/RPO/retention і реальний
server/storage gate залишаються відкритими. Одноразові scripts — ignored
`tmp/corpus-restore/`, clean checkout їх не містить. [Validation](validation.json)
фіксує фактичні tests, metadata/privacy checks та sequential self-review;
незалежного reviewer не було. CI перевіряється для фінального PR head.
