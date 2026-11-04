# Діагностика prefix masks на повному локальному корпусі

## Висновок

Попередні 6/16 HTTP 503 із [restore preparation](../2026-11-04-corpus-restore/REPORT.md)
не відтворилися: усі **160/160 HTTP-запитів** успішні. Це не виправлення
і не спростування попереднього спостереження. Під час старої проби інший cluster
відновлював повний corpus; зараз concurrent restore/dump не було. Причинного
зв'язку з contention або станом кешів не доведено: старі SQL-плани/timers відсутні.

Усі **32/32 SQL-плани** використовують `translations_exact` для відбору dataset
і мовної пари; `source_key LIKE` — postfilter, а не index condition. Після нього
йде Sort → Limit (у 4/32 планах також Gather Merge). До **179 848 rows removed
by filter**; reported значення однакові для обох limits у кожному case.
Для parallel scans EXPLAIN показує округлене середнє за loops, не точну суму. Малий `limit=1` не усуває сканування пари.

## Середовище та протокол

Application revision: `fd86a9bec15f7be701fed02fca01b2b96ea94091` (PR23 merge).
Використано retained release binary SHA-256
`a79f4021fa5938369158184bcb1e648ba51bfd250fbdfab4db17911405b61f90`
з restore bundle, зібраний на `dd2039a`; `cmd/`, `internal/`, `migrations/`, `web/`
та Go dependencies між цими revisions незмінні.
PostgreSQL 17.11, original new-service corpus bind, **2 CPU / 8 GiB**;
HTTP **1 CPU / 512 MiB**, read-only filesystem/mounts, restricted LOGIN,
лише loopback. DB `temp_file_limit=20GB`. [Startup](startup.json), HTTP startup/log checks у сусідніх JSON.
Oracle й legacy checkout не використовувалися.

Dataset містить 56 455 068 translations, 992 пари, ready/active generation 1.
Взято **ті самі перші 16 cases** приватного workload попередньої діагностики,
без виключення невдалих cases. Exact source не містять `*`/`?`; prefix — той самий
source із доданим `*`, normalizer `Auto`. Це вузька positive вибірка, не всі маски.

Послідовність: HTTP `before` → два попередні EXPLAIN case 6 → HTTP `reverse`
→ matrix EXPLAIN → HTTP `html`. У `before` для кожного case limits 1, 50;
у `reverse` — 50, 1. У цих фазах exact/prefix мають `format=plain`.
Остання фаза: limit 1, exact plain / prefix `format=html`. Explicit html і відсутній
format обирають одну HTML-гілку в `internal/httpui/request.go`; попередня проба
використовувала відсутній format. HTML-проба виконана вже після прогрівання.
OS cache не скидали; рестарт PostgreSQL не означає cold disk cache.

## Фактичні результати

[Summary](summary.json) обчислено з [before](http-before.jsonl),
[reverse](http-reverse.jsonl), [html](http-html.jsonl) і `matrix-*.json`.
Тривалості HTTP — client wall time, не server-side SLO timer.

| Фаза | Запити / HTTP 200 | Max prefix limit 1 | Max prefix limit 50 |
|---|---:|---:|---:|
| before | 64 / 64 | 655.117 ms | 77.068 ms |
| reverse | 64 / 64 | 75.498 ms | 92.704 ms |
| html | 32 / 32 | 90.420 ms | — |

Різницю першої фази не відділено від впливу порядку/прогрівання; не приписувати
її самому limit. Усі 32 EXPLAIN завершилися, max execution **74.412 ms**.
[Приклад case 6 / limit 1](matrix-6-1.json).

Діагностичний SQL повторює first-page mask query з
`internal/storage/postgres/page.go`: dataset/from/to + LIKE із escape `\`,
C-order source_key/source_text/target_text/id, SQL LIMIT = UI limit + 1.
Використано `EXPLAIN (ANALYZE, BUFFERS, SETTINGS, FORMAT JSON)`, pgx та окреме
з'єднання restricted reader на план; лише в ньому statement_timeout 30 s,
client context 45 s. Application deadline **2 s незмінний**, HTTP retries не додані.
EXPLAIN instrument overhead і HTTP admission/connect/інші lookup запити
не дозволяють прямо прирівнювати ці timings.

Публічні плани містять allowlisted node names, numeric measurements та predicate
presence flags; literal Filter/Index Cond/Sort Key вилучено. Flags перевіряються
за private raw plans. Shared Read Blocks означає misses shared buffers,
не доведені фізичні disk reads. `track_io_timing=off`;
[settings/statistics](settings-statistics.json) містить estimates та старий
last_autoanalyze; ANALYZE у цій пробі не запускали.

## Межі, збереження і наступний крок

[Baseline](baseline.json) = [final state](final-state.json): application counters,
report, publication та generation незмінні. Це state snapshot, не повторна повна
row multiset звірка. [Lifecycle](lifecycle.json): тимчасовий reader і власні
containers/network прибрані; retained corpus та backup/restore copies збережені.

Application відкриває нове pgx-з'єднання на HTTP borrow (`internal/ops/search.go`);
між запитами немає спільного prepared-plan cache. Наявний B-tree exact index із колонкою key_hash
не забезпечує prefix access. Повний B-tree над усім source_key не можна
додавати без урахування admitted 64 KiB значень (коментар initial migration).

Наступний технічний зріз — bounded-prefix access-path experiment на **окремій
копії**, із full LIKE recheck, перевірками Unicode/escaping/довгих значень,
leading wildcards, pagination, index size та write/WAL cost. Це пропозиція,
схему/індекси зараз не змінено. Далі потрібні контрольовані cache/contention
проби і ширший workload. Причина старих 503, AT-16, crawler budget,
storage/server capacity та production acceptance залишаються відкритими.

## Відтворення та перевірка

Приватні входи, raw plans і metadata: ignored
`data/experiments/mask-plans-2026-11-04/`; one-off harness у `tmp/mask-plans/`.
Вони недоступні у clean checkout. Реальні терміни, HTTP bodies, secrets та raw
плани не публікуються; public JSON достатні для повторного підрахунку висновків,
але не для незалежного відтворення workload. [Validation](validation.json)
фіксує checks та sequential self-review; незалежного reviewer не було.

Пояснення PostgreSQL: [EXPLAIN](https://www.postgresql.org/docs/17/using-explain.html)
про index conditions/postfilters, BUFFERS й instrumentation;
[index types](https://www.postgresql.org/docs/17/indexes-types.html) про
anchored LIKE та hash equality. Це інтерпретація виміряних планів, не доказ
причини попередніх timeout.
