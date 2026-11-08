# Стан сесії — 2026-11-08

Після merge [PR #29](https://github.com/valpere/slovnyk/pull/29)
(`f752e36ae984729ab63fea6db70a484d6327ab51`) і команди власника `go`
завершено ширші локальні HTTP/cache/contention probes на restored schema 4.
Це поточна пам’ятка продовження; вимоги й контракти визначає [INDEX](INDEX.md).

## Підтверджений стан

- [PR #27](https://github.com/valpere/slovnyk/pull/27) змержено власником
  2026-11-04; merge commit `991b72173dc68edc4e691df35b106ba00d79145b`.
  Чотири CI jobs на head `d191aa5d79282afafc1d0705342bf23ae0522314` успішні.
- Початкова локальна dev-БД має **schema 4**, 56 455 068 translations,
  992 мовні пари; dataset ready/active, generation 1. Dataset ID:
  `96541c0b-ac71-4e38-b57c-b593e82ac9d5`.
- Префіксний індекс — 2.428 GiB, команда міграції — приблизно 177 s,
  БД — 66.744 GiB без WAL/artifacts. Усі 16 application table inventories
  і sequences незмінні; ledger отримав лише migration 4.
- 32/32 плани фактичного `ReadPage` мають prefix Index Cond та full LIKE
  recheck; результати й lookahead збігаються з початковим запитом.
  Для 135 HTTP responses до/після збігаються розміри/SHA-256 тіл,
  Content-Type і Cache-Control, включно із 7 pre-upgrade cursors.
- `make check test test-race test-smoke` пройшов перед PR27.
  [Повний звіт](_assessments/2026-11-04-full-prefix/REPORT.md) містить методику,
  metadata, межі доказів та послідовне self-review; окремого reviewer не було.

- [Новий повний backup/restore schema 4](_assessments/2026-11-08-schema4-restore/REPORT.md)
  відновив 17 table multisets/sequences/config, ledger 1–4 та valid prefix index
  у fresh cluster із новим non-superuser owner. Source/target inventories тотожні.
  Restored DB — 60.011 GiB без WAL/artifacts.
- 55/55 HTTP 200 тотожні за sizes/body hashes/Content-Type/Cache-Control:
  16 exact, 16 prefix, 16 word і 7 pre-restore continuations. Усі 736 shards,
  index і 992 downloads повністю звірені через target HTTP з вимкненим source.
  Missing artifacts, restricted ACL та operator retry перевірені фактично.
- Від початку dump до acceptance — 3359.010 s,
  мінімальний observed restore free — 76.043 GiB.
  Це один host, не off-host RTO/capacity acceptance. Власний runtime прибрано.
  `make check test test-race test-smoke` — PASS; code не змінювався.

- [Нові HTTP/cache/contention probes](_assessments/2026-11-08-query-workloads/REPORT.md):
  **1858/1858 HTTP 200**, 330 prior response oracles і 508 negative body checks
  збігаються. Client errors/SQLSTATE 57014/OOM — 0 у виміряних фазах.
  32 spaced і 8 largest pairs; serial/warm, ordinary/mask/mixed 10/s,
  16-worker one-pair bursts і recovery. Largest burst HTTP p95 1513.180 ms,
  client scheduled-to-complete p95 4370.988 ms; це не server timing.
- Після **обох** запусків повні 17 table inventories/sequences і state збігаються
  з restore. DB 2 CPU/8 GiB, HTTP 1 CPU/512 MiB; source лишався вимкненим.
  OS cache не скидали. Largest HTTP lifetime peak ~512 MiB включає startup/file cache;
  це не heap. Cgroup memory.max events були до controls, OOM/kill — 0.
- Новий Go-посіпака `minions/http-probe` обліковує arrivals/queue/response hashes
  без URL/тіл у результатах. Oversize private config regression виправлено red→green.
  `make check test test-race test-smoke` — PASS на фінальному code diff.
  Послідовне self-review; незалежного reviewer не було. Власний runtime прибрано.

## Що збережено локально

Усі шляхи нижче відносні до кореня checkout; `data/experiments/` ignored.
Credentials, реальні терміни, raw plans, keyring і HTTP paths не публікувати.

| Об’єкт | Шлях / стан |
|---|---|
| Початкова dev-БД | `data/experiments/postgres-corpus-preflight-2026-10-25/.pgdata`, schema 4 |
| Immutable exports/report/sitemaps | `data/experiments/corpus-verification-2026-11-04-retry/artifacts/` |
| Попередній backup bundle | `data/experiments/corpus-restore-2026-11-04/backup/`, schema 3 |
| Попередній restored cluster | Видалений 2026-11-08 з окремого дозволу власника; звільнено 58.56 GiB |
| Попередні restored files | `data/experiments/corpus-restore-2026-11-04/restored/` |
| Останні inventories/diagnostics | `data/experiments/full-prefix-2026-11-04/` |
| Одноразові harness scripts | `tmp/full-prefix/`, `tmp/corpus-restore/`; відсутні в clean checkout |
| Перервана schema 4 спроба | `data/experiments/schema4-restore-2026-11-08/`, scratch `tmp/schema4-restore/` |
| Успішний schema 4 backup/restored cluster/files | `data/experiments/schema4-restore-2026-11-08-retry/`, scratch `tmp/schema4-restore-retry/` |
| Нові workload runs | `data/experiments/query-workloads-2026-11-08-retry/`, `data/experiments/query-workloads-largest-2026-11-08/`; scratch `tmp/query-workloads/` |
| Перша workload preflight спроба | `data/experiments/query-workloads-2026-11-08/`, зупинена до runtime через host permissions PGDATA |
| Повторюваний агентський inventory | `minions/corpus-inventory/`, Go, 17 таблиць + sequences |

Зафіксований cleanup прибрав власні containers/networks і тимчасовий reader LOGIN.
Початкові bind data, backup bundle, restored files та inputs збережено;
видалений лише старий restored DB bind за окремим рішенням власника.
На початку нової сесії працювали сторонні `sla-testdb`, `docker-mongodb-1`
та `open-webui`; їх не чіпати. Після цієї репетиції доступно близько 84.6 GiB.
Старі ephemeral ports/container IDs і видалені LOGINs повторно не використовувати.
[Карта даних](../data/README.md) пояснює історію й приватні inputs.

## Наступна задача

Після merge PR із цим evidence — **безпечні server-side measurements** перед
подальшими capacity висновками. Спершу перевірити Git/PR/CI, прочитати INDEX і
[останній звіт](_assessments/2026-11-08-query-workloads/REPORT.md).
Не повторювати import, reconciliation, schema upgrade, full restore або щойно
завершені workloads без нової причини.

1. Описати й реалізувати мінімальні server-side search timings/класи результатів
   без raw query/URL/DSN. Відділити admission/connect/SQL/handler та client queue;
   нові метрики не повинні непомітно змінювати deadline чи відповіді.
2. Погоджений traffic/crawler/resource profile потрібен для AT-16. Нинішні
   fixed samples і client p95 не є server SLO або deployment acceptance.
   Historical 503 у нових runs не відтворилися; їхню причину не оголошувати доведеною.
3. Перед новими DB probes rediscover потрібний bind/private config, free disk,
   створити власні capped runtime/restricted LOGIN. Старі ports/IDs/roles вже невалідні.
   Startup/readiness cache charging, cold-host та довгі маски окремо відкриті.
4. Прибрати власний runtime, зберегти retained data, review → commit → PR → green CI.
   Merge виконує власник. Oracle зайнятий; додаткові видалення не дозволені.

## Відновлення після переривання

Стара спроба `schema4-restore-2026-11-08/` зберігає interruption/storage receipts;
її `run.py` не є resumable, повторно не запускати. Успішний retry —
`schema4-restore-2026-11-08-retry/`: усі metadata, `lifecycle.json`, backup,
restored cluster/files збережені. Після reboot source повністю переінвентаризовано,
збіг із PR27 підтверджено. Дозвіл видалення стосувався лише старого
`corpus-restore-2026-11-04/.pgdata` і вже виконаний; інші копії не видаляти.

## Що залишається відкритим

AT-16, server-side timings, інші cold-host/low-memory/довгі mask workloads, crawler budget,
причина історичних HTTP 503, target server/storage capacity, off-host RTO,
secret escrow/RPO/retention та мовне/дизайн-приймання. Нові bounded локальні
вибірки включають negative/infix/concurrency, але не закривають ці gates.
Oracle root близько 45 GiB за старим заміром
не вміщує поточну БД; нового server probe не було.

## Правила продовження

- Розробка локально; зайнятий Oracle `psysoft` не навантажувати.
  `../slovnyk1.0/` лишити незмінним. Не змінювати наявний `AGENTS.md`.
- Коміти обов’язкові; review перед PR, усі CI мають пройти. Failed checks:
  з’ясувати й усунути причину, перезапустити. Merge виконує власник.
  Автовидалення merged branches увімкнене в репозиторії.
- `utilities/` — інструменти сервісу/операторів; `minions/` — лише особисті
  помічники агента; `tmp/` — тимчасові речі. Корисні повторювані harness
  переносити з tmp у minions; Python service utilities переписувати на Go.
- Playwright не використовувати; за потреби браузера — наявний Chromium CDP 9222.
  Не запускати інших агентів без чинної авторизації.
- Поточний стан визначати за файлами й Git: старий AGENTS містить історичний
  опис «specifications only», а застосунок, Go module і тести вже існують.
