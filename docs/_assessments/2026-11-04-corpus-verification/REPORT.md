# Повний локальний verify та CSV exports

## Результат

2026-11-04 actual `dataset verify --loss-approval` завершився exit 0:
**992 source files, 56 455 068 parsed = unique**, 0 duplicates/rejected.
Повторено source/normalization reconciliation, ledger/origins та inventory checks;
per-file counters тотожні попередньому повному reconciliation.
Створено **992 окремі CSV4.gz exports**, 532,085,555 compressed bytes;
row counts кожної пари збігаються з manifest/source audit.
[Report](report.json), [invocation](invocation.json), [assertions](assertions.json).

Dataset `96541c0b-ac71-4e38-b57c-b593e82ac9d5`, manifest fingerprint
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Run revision `a8577c02f49ab646e319e3d812ac7322cb94ff75`;
binary з application revision `5b4011f9e6187839330e6c8f96a50ec645c9e64e`,
SHA-256 `d874b3a906021a320c36d3b0ef1a7233e03625fb617f4f77638b01779266e629`.
Application code не змінювався, повторного import не було.

[Baseline](baseline.json) → [final state](final-state.json): `validating → ready`,
retained report path/hash заповнені, exports 0 → 992. Active null/generation 0,
publication events/sitemaps 0, source counters та import runs не змінені.
Власник [явно погодив](../../DECISIONS.md#owner-approval-2026-11-04)
відсутні historical count/status/queries та невідому історичну повноту.
[Receipt](owner-approval.json) і known gaps збережені дослівно; exceptions порожні.
Жодні словникові записи не вилучалися.

## Повторне читання exports

Окремий actual `bin/slovnyk dataset export ID --artifacts ROOT` завершився exit 0
за **33.018 s**. Він прочитав retained report із БД,
перевірив його hash, receipt binding, inventory усіх пар і кожний compressed
artifact hash/bytes, gzip CSV до EOF, locales, строгий порядок raw tuples,
row counts і tuple digests. Output bytes точно збігаються з verify stdout.
[Execution](export-check-execution.json). Це read-only revalidation, не regeneration.

## Середовище та виміри

Локальна збережена bind DB, schema 3, PostgreSQL 17.11, durability on.
Власний Compose project `slovnyk-corpus-20261025`, loopback port,
**1 CPU / 8 GiB RAM**, `temp_file_limit=20GB` на PostgreSQL process.
CLI поза container cgroup; профіль не є Oracle capacity acceptance.

Успішний verify зайняв **2462.304 s (0.684 h)**,
включно з завершальним polling до 10 s. Це не end-to-end migration або RTO.
Моніторинг: 245 samples, nominal 10 s; minimum sampled free
175.025 GiB, maximum sampled DB
64.316 GiB, WAL
1.000 GiB. [Summary](progress-summary.json).
Disk floor 30 GiB, DB ceiling 85 GiB та 24 h limit не спрацювали.
Cumulative PostgreSQL temp bytes не є peak live temp usage; source FD/artifact
observations не є verified checkpoints. Зміни free space не приписуємо лише цьому run.

## Переривання, lifecycle та межі доказу

Перша спроба не завершилася: PostgreSQL отримав fast shutdown request,
container exit 0, OOM false; CLI повернув `import_storage_failure`, terminal
monitor receipt відсутній. Ініціатора зупинки не встановлено.
[Observation та останній sample](interrupted-attempt.json).
Після restart baseline точно збігся з попереднім reconciliation.
Новий run використав окрему `data/experiments/corpus-verification-2026-11-04-retry/`;
старі результати не перезаписувалися. Успіх належить лише повторній спробі.

Після revalidation власні container/network прибрані, bind DB і referenced
artifacts/snapshot/інші retained inputs збережені. [Lifecycle](lifecycle.json).
Oracle і legacy не задіяні; у Git лише metadata, без credentials/CSV/DB contents.
Повний корпус не публікували; HTTP/crawl, semantic quality, production storage/SLO,
multi-dataset operation та off-host backup/restore залишаються окремими gates.

`make check test` — PASS. Послідовний self-review перевірив counts/hashes,
approval scope, state transition, privacy та посилання; незалежного review не було.
Application code незмінний, synthetic integration локально не повторювали;
CI перевіряється окремо для фінального PR head, merge виконує користувач.
[Validation](validation.json), `reviewed-inputs.sha256` — snapshot перевірених входів
без самопосилання; історичні hash inventories не переписувалися.
