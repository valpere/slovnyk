# Повний локальний backup/restore schema 4

## Результат

2026-11-08 корпус **56 455 068 translations / 992 pairs** відновлено зі
свіжого logical backup у новий порожній PostgreSQL cluster із новим owner.
**Усі 17 table multisets і sequences збігаються**, включно з ledger schema 4;
`translations_prefix64` valid/ready, definition та всі migration rows тотожні.
Ready/active generation 1 збережено. [Summary](summary.json),
[source index](source-index.json), [target index](target-index.json).

Від початку dump до acceptance — **3359.010 s**,
разом із cleanup — 3360.094 s.
Preflight, попередня перервана спроба й source inventory до dump сюди не входять.
Це один developer host, не production/off-host RTO acceptance.

## Диск, переривання та відновлення роботи

Перед стартом вільно близько 102.6 GiB; budget на основі попереднього observed
restore peak, нового index, dump, двох файлових копій і резерву 30 GiB —
119.7 GiB. Це оцінка, не upper bound. **Власник окремо дозволив видалити лише
стару schema 3 restored `.pgdata`**: звільнено 58.56 GiB. Старий backup bundle,
restored files та початкова schema 4 dev-БД збережені.
[Owner-authorized cleanup](previous-storage-preflight.json).

Перша спроба перервана перезавантаженням під час source inventory: завершеного
inventory/dump/target ще не було. Журнали й receipts збережено окремо;
PostgreSQL мав exit 0, без OOMKilled, із logged shutdown complete. Це не доказ
цілісності після reboot: її підтвердив **повторний повний inventory**, тотожний
постміграційному inventory PR27. Retry не перезаписав старих результатів.
[Interruption checkpoint](previous-interruption.json), [source inventory](source-inventory.json).

## Комплект і fresh restore

Writers зупинені; перед dump інших client backends у source БД не було.
Пауза збережена через dump та копіювання filesystem. Pinned PostgreSQL 17.11
`pg_dump --format=custom --no-owner` дав **6400096170 bytes**;
archive SHA-256 знято й повторно звірено до restore.
У bundle — той самий static Go binary для source/target controls, manifest,
**1731 artifacts і 992 sources**. Files копіювалися
незалежними regular files, без hardlinks/reflinks; source/backup/restored
relative paths, sizes і SHA-256 перевірені, files/directories fsync виконано.
[Inventory](file-inventory.json), [file restore](files-restored.json), [protocol](protocol.json).

Новий cluster має інший system identifier, порожню application schema та
нові NOLOGIN groups, restricted LOGINs і окремого NOLOGIN non-superuser owner.
`pg_restore --no-owner --role=<new-owner> --exit-on-error --single-transaction`
пройшов за **2314.079 s**, без `--clean`, `--no-acl` або
попередніх application migrations. Schema 4 index відновлений саме з dump.
Перед HTTP виконано ANALYZE. [Startup](target-startup.json),
[restore](restore-execution.json), [ANALYZE](analyze-execution.json).

Binary COPY oracle читає всі колонки/рядки: row SHA-256 із field count,
length/null markers та bytes; count + sum mod 2^256 + XOR порівнюються без
залежності від порядку. Це probabilistic multiset check для поточних built-in/
domain encodings, не нова source reconciliation. Послідовні reads не є спільним
snapshot; writers зупинені. Sequence values і config звірені окремо.
[Source](source-inventory.json), [target](target-inventory.json),
[sequence config](target-sequence-config.json).

## HTTP та privileges

Source HTTP controls виконані **до restore**, без concurrent restore;
потім source cluster зупинений і лишався недоступним протягом target acceptance.
**55/55 HTTP 200**: 16 exact, 16 prefix,
16 word та 7 continuations, виданих перед restore.
Sizes/SHA-256 тіл, Content-Type і Cache-Control збігаються з source.
Спільний keyring приватний rehearsal; історичних production cursors немає.
[Source controls](source-http-check.json), [target controls](target-http-check.json).

Додатково всі **736 sitemap shards, index і 992 downloads** повністю прочитані
через target HTTP та звірені за sizes/SHA-256. Порожній artifact root дає
health 200, readiness/sitemap 503. Усі schema objects мають нового owner;
Спроби UPDATE/DDL/register від HTTP-ролі та DELETE/DDL від operator-ролі
заборонені фактично (42501).
Bundled CLI повторив `dataset export`, operator повторив існуючу sitemap
registration через SET LOCAL ROLE, без зміни state/report.
[ACL](ownership-privileges.json), [missing files](missing-artifacts.json),
[export retry](operator-retry.json), [registry retry](operator-sitemap-retry.json).
Один dataset не покриває retained A/B; це перевіряє synthetic lifecycle test.

## Ресурси, cleanup та межі

Source/target: 2 CPU / 8 GiB; target maintenance_work_mem 512 MiB,
parallel maintenance workers 0, per-process temp cap 20 GB; durability on.
HTTP: 1 CPU / 512 MiB, non-root, loopback only, read-only rootfs/mounts.
Docker OOMKilled=false; окремого cgroup OOM-events/RSS обліку немає.
Min observed restore free space — **76.043 GiB**;
це samples, не точний peak accounting. Guards не спрацювали.
Restored DB — **60.011 GiB**, WAL/temp/artifacts/bundle окремо.
[Progress](restore-progress.jsonl), [storage](target-storage.json), [runtime](runtime-resources.json).

Власні containers/networks і тимчасові reader/operator LOGINs прибрано;
початкова БД, новий backup/restored cluster/file copies збережені.
Сторонні containers, Oracle й legacy не змінювалися. Private config/keyring,
terms, HTTP paths і raw diagnostics лишилися ignored; public evidence містить
лише metadata. Run: `data/experiments/schema4-restore-2026-11-08-retry/`;
one-off scripts: `tmp/schema4-restore-retry/`, відсутні у clean checkout.
[Lifecycle](lifecycle.json).

`make check test test-race test-smoke` — PASS. Послідовне self-review за
code-review та verification-before-completion перевірило scope, privacy,
інваріанти й межі evidence; незалежного reviewer немає. CI — на PR head.

FR-13/AT-13 отримали повний локальний schema 4 restore evidence.
Production/off-host RTO, secret escrow, RPO/retention, target storage/capacity,
AT-16, cache/concurrency/crawler workloads та причина історичних 503 відкриті.
Ця позитивна control-вибірка не є capacity benchmark або SLO acceptance.
