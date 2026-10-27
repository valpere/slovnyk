# Повний staged PostgreSQL import корпусу

## Результат і scope

Запуск 2026-10-25, фінальна SQL-звірка 2026-10-27. Actual CLI з main
`2ffb9ab33d41d734bf04b6f62632c6033bfa8d1a` після PR #14, без змін
application code, schema або dependencies. `make build` створив binary із
SHA-256 у [invocation](invocation.json). Це локальний M2 import subset;
production, Oracle, legacy directory і browser не задіяні.

| Перевірка | Результат |
|---|---:|
| Файлів / complete / checksum-verified | 992 / 992 / 992 |
| Parsed / accepted unique | 56 455 068 / 56 455 068 |
| Duplicate occurrences / rejected / import issues | 0 / 0 / 0 |
| Actual ledger / origins / translations | по 56 455 068 |
| Artifact counts проти audit 2026-10-24 | збіг усіх 992 |
| Actual pair counts проти audit та catalog | збіг усіх 992 |
| Dataset state / CLI exit | validating / 0 |
| Active dataset / generation | null / 0 |

Input — та сама окрема frozen copy 992 CSV.gz, 535 766 648 compressed bytes,
32 locales; [підготовка](../2026-10-25-csv-manifest/REPORT.md),
[inspect](../2026-10-25-go-corpus-inspect/REPORT.md).
Manifest SHA-256 до/після та fingerprint:
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Dataset ID лишився `96541c0b-ac71-4e38-b57c-b593e82ac9d5`.

## Відновлення та звірки

Відновлено bind DB попереднього [preflight](../2026-10-25-postgres-corpus-preflight/REPORT.md)
із 114 000 committed records. Без ручних UPDATE виконано ту саму команду:

```sh
bin/slovnyk import run \
  --manifest data/experiments/csv-manifest-2026-10-25/manifest.json \
  --source data/experiments/csv-manifest-2026-10-25/snapshot \
  --batch-size 1000
```

`SLOVNYK_OPERATOR_DSN` передано лише через environment з private credentials;
loopback port отримано від власного Compose project. Restricted operator без
superuser/createdb/createrole/replication/bypassrls. PostgreSQL 17.11/schema 3,
той самий pinned image; fsync, synchronous_commit і full_page_writes увімкнені.

Attempt 3 автоматично позначив покинутий attempt 2 як
`interrupted / writer_disconnected`, перевірив committed prefix і завершив
усі файли. Попередні attempts не переписані як успішні.
[Точний CLI stdout](run.json), [execution](execution.json),
[final state](final-state.json), [comparison](report-comparison.json).

Після паузи контейнер спостережено зупиненим: exit 0, OOMKilled=false;
причину зупинки не встановлено. Перша спроба SQL-звірки не почалася через
недоступний контейнер; CLI comparison уже пройшов. Відновлено лише власний
існуючий container, без повторного import або ініціалізації DB.
[Restart observation](verification-restart.json).

[Accounting SQL](accounting.sql) агрегує кожну велику таблицю окремо після
завершення writer. Для кожного artifact ledger має рівно ordinals `1..parsed`,
лише unique outcomes; origins і checkpoint дорівнюють parsed. Actual translation
counts кожної пари збігаються з catalog та попереднім audit.
[Ledger](ledger-counts.json), [origins](origin-counts.json),
[checkpoints](checkpoints.json), [translations](translation-pair-counts.json),
[catalog](stored-pair-counts.json).

Counts і SHA-256 рядків попереднього prefix 55 000 у всіх трьох таблицях
побайтово збігаються за тим самим digest algorithm із preflight:
[after](prefix-after.json), [before](../2026-10-25-postgres-corpus-preflight/prefix-before.json).
[Assertions](assertions.json) — PASS. [Verification state](verification-state.json)
підтверджує schema 3, той самий validating dataset, active null, 0 running imports
та checkpoint mismatches.

## Час і ресурси

Третій attempt тривав **16 277.102 s — 4 год 31 хв 17 s**, включно з replay
114 000 records і monitoring. Це не час повної міграції від порожньої DB:
перші два preflight attempts, наступні verify/exports і restore сюди не входять.

CPU cap PostgreSQL залишався 1. Memory cap змінювався **512 MiB → 8 GiB →
16 GiB** без restart: [перша зміна](resource-change.json),
[друга зміна](resource-change-2.json). На 512 MiB вибірка активності показувала
переважно index I/O; на 8 GiB cgroup був заповнений переважно file cache,
спостерігалися refaults. Локальний host мав запас пам’яті. Application CLI
працював поза DB cgroup. Це змішаний профіль експерименту, не benchmark Micro,
не порівняння швидкості фаз і не доказ необхідності 16 GiB RAM для сервісу.

Guard limits: minimum free disk 30 GiB, maximum DB 85 GiB, maximum attempt
24 h; жоден guard не спрацював. 1 612 nominal 10-second polls читали лише
малі counters та size metadata, без повного count великих таблиць.
[Progress summary](progress-summary.json) зберігає SHA-256 повного локального log:
minimum sampled free disk **55.08 GiB**, maximum sampled WAL **1.203 GiB**.
Це дискретні спостереження, не continuous peak metrics.

Final `pg_database_size`: **69 058 074 291 bytes — 64.315 GiB**; WAL directory
окремо **1 GiB**. Relations з indexes: translations 28 713 631 744 bytes,
ledger 19 765 239 808, origins 20 569 432 064. Це один staging dataset, без
exports, другого dataset, full backup/restore copies або production temp space.

За [профілем Oracle від 2026-10-23](../2026-10-23-server/REPORT.md) root filesystem
близько 45 GiB, весь block device 46.6 GiB. **Цей PostgreSQL layout одного dataset
не поміщається на зафіксованому диску.** Нової перевірки Oracle не виконано;
це порівняння з історичним виміром, не оцінка поточного provisioned storage.
Два datasets такого розміру потребували б понад 128 GiB лише для DB; WAL,
exports, backup та запас потрібні додатково. Остаточний storage/server gate відкритий.

## Збереження та незакриті перевірки

Original results: `data/experiments/postgres-corpus-import-2026-10-25/`.
DB лишилася у `.pgdata/` попереднього preflight root; нової копії DB не створено.
Після звірок власні container/network прибрані, directory device/inode/mode/owner
збережені: [lifecycle](lifecycle.json). Private credentials, input, DB та raw tuples
не комітяться. Цей assessment містить точні metadata-only копії JSON та SQL
перевірок; [procedure](procedure.json), [environment](environment.json).
Для нового запуску відновіть власний Compose project із тим самим bind/image
та credentials, оновіть dynamic loopback port; не reinitialize DB.

Counts не доводять повної рівності raw tuples/normalization. Повний source replay
через verifier, digests/exports round-trip, ready/publication, HTTP/crawl/SLO,
full-corpus backup/restore та historical/semantic completeness ще не перевірено.
Clean-vs-retry import від порожньої DB окремо не повторювався.

Нуль rejected не означає відсутності історичних втрат: CSV4 не містить старих
count/status/query metadata. [Known gaps](known-gaps.json) збережені, exceptions
порожні; цей запуск не схвалює втрати. Поточний strict verifier блокує ready за
наявності known_gaps; approval-loss adapter і погодження конкретного звіту
власником залишаються потрібними. Не прибирайте gap із manifest для обходу guard.

## Review та validation

Послідовний self-review за code-review skill, не незалежне рев’ю: isolation,
retry/accounting, digest scope, metadata privacy, mixed resource profile,
known-gap gate і claims активних docs. [Reviewed inputs](reviewed-inputs.sha256)
фіксують перевірені bytes; сам report та inventory виключені.
Підтверджених невиправлених findings немає. Exact copies metadata, accounting
assertions і byte-identical prefix evidence перевірено; `make check test` та
`git diff --check` — PASS, exit 0. Local disposable integration не повторено:
application/schema/SQL не змінено; actual corpus SQL verification вище — окремий
доказ. Незмінний CI включає Go/race/HTTP smoke та PostgreSQL integration;
результат exact-head pull_request run фіксується у PR.
AGENTS.md та історичні assessments не змінено.
