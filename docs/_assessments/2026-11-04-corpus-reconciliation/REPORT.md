# Повна локальна source/DB звірка корпусу

## Результат

2026-11-04 actual `bin/slovnyk dataset reconcile` завершився exit 0:
992 complete/checksum-verified CSV.gz, **56 455 068 parsed = unique**,
0 duplicates/rejected. Raw tuples, normalized keys/hashes, ledger/origins,
checkpoints, source metadata та inventory узгоджені. Per-file counts збігаються
з аудитом 2026-10-24 і завершеним import. [Report](report.json),
[assertions](assertions.json), [invocation](invocation.json).

Dataset `96541c0b-ac71-4e38-b57c-b593e82ac9d5`, fingerprint
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Runtime revision `efca45f54ab4ccc97699275dc204c392dac66d39`;
application code з merge PR #16 (`63a7ce2`), без зміни реалізації.
Binary SHA-256 — у invocation. Повторного import не було.

[Baseline](baseline.json) та [final metadata](final-state.json) рівні:
dataset лишився `validating`, active null/generation 0, retained report відсутній,
exports/sitemap inventory/publication events — 0. Прочитано всі доступні джерела;
це не доказ історичної повноти або мовної якості. Known gap про відсутні
legacy count/status/query metadata збережений дослівно, exceptions порожні.
Погодження конкретного звіту втрат та approval-loss adapter ще потрібні;
strict verify, exports, ready/publication, HTTP/crawl/SLO та full backup/restore
цим запуском не закриті.

## Середовище та ресурсні межі

Та сама локальна bind DB/schema 3/PostgreSQL 17.11; durability on.
Окремий Compose project `slovnyk-corpus-20261025`, loopback port;
1 CPU, **8 GiB RAM**, `temp_file_limit=20GB` на PostgreSQL process.
CLI працює поза container cgroup. Це developer run, не Oracle capacity gate.

Повторний повний run — **2180.458 s (0.606 h)**,
включно з інтервалом завершального polling до 10 s; це не end-to-end migration.
Моніторинг: 217 samples, nominal 10 s, minimum sampled free
175.163 GiB, maximum sampled DB
64.315 GiB, WAL
1.000 GiB. [Summary](progress-summary.json).
30 GiB disk floor, 85 GiB DB ceiling і 24 h time limit не спрацювали.
Зміни free space поза погодженим прибиранням не приписуються цьому процесу.
Cumulative `pg_stat_database.temp_bytes` не є live peak temp usage.
Source FD positions — спостереження, не verified checkpoints.

## Погоджене прибирання та попередня спроба

Користувач погодив рівно два файли з [preflight](../2026-11-03-reconciliation-storage/REPORT.md):
`sqlite-full/restored.sqlite3` та `provenance-indexed/model.sqlite3` у
`data/experiments/`. Видалено лише їх; free space безпосередньо після операції
64.211 GiB. [Cleanup receipt](cleanup.json). Seed/base/compact SQLite,
CSV snapshot, private config, PostgreSQL bind та JSON results збережені.
Відтворення похідних SQLite файлів описано у preflight; це не off-host backups.

Спроба 2026-11-03 не залишила terminal receipt або фінального report;
останній sample — 572.692 s. Під час наступного огляду CLI/monitor відсутні,
container exited 0, OOM false. [Observation](interruption-observation.json).
Причину не встановлено; ці дані не дають success або точного часу до interruption.
Після restart persistent baseline збігся. Новий run виконав replay від початку,
його monitor запущений окремим процесом. Старі результати не перезаписувалися.

## Перевірка, review та lifecycle

Application CLI/SQL перевірено actual full run. `make check test` пройшли;
Go-посіпаку `corpus-progress` перевірено на synthetic partial final line і
відсутньому source field наступного sample: timestamp спостереження зберігається,
а поточний elapsed оновлюється. Посіпака лише читає локальні metadata.
Послідовний self-review перевірив відповідність counts/hashes/report попередньому
audit/import, незмінність persistent metadata та приватність evidence;
незалежного review не було. Full synthetic integration локально не повторювали:
application code незмінний; CI виконує його окремо.

Власні container/network прибрані після перевірок, bind DB та її directory
metadata збережені. [Lifecycle](lifecycle.json). Oracle і legacy не задіяні.
Local progress/stderr лишаються в ignored `data/experiments/`; у Git лише metadata.
Список перевірених входів — `reviewed-inputs.sha256` (без самопосилання).
