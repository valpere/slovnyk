# Стан сесії — 2026-11-08

Після merge PR #28 (`7dddd6750ea7ab4240523c1294794e27b3756662`) і команди
власника `go` завершено повний локальний backup/restore schema 4.
Перша спроба перервана reboot до dump; retry виконано в окремому каталозі.
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
| Повторюваний агентський inventory | `minions/corpus-inventory/`, Go, 17 таблиць + sequences |

Зафіксований cleanup прибрав власні containers/networks і тимчасовий reader LOGIN.
Початкові bind data, backup bundle, restored files та inputs збережено;
видалений лише старий restored DB bind за окремим рішенням власника.
На початку нової сесії працювали сторонні `sla-testdb`, `docker-mongodb-1`
та `open-webui`; їх не чіпати. Після цієї репетиції доступно близько 84.7 GiB.
Старі ephemeral ports/container IDs і видалені LOGINs повторно не використовувати.
[Карта даних](../data/README.md) пояснює історію й приватні inputs.

## Наступна задача

Після merge PR з цим evidence — **ширші локальні HTTP/cache/contention probes**.
Спершу перевірити Git/PR/CI та прочитати INDEX, останній
[restore звіт](_assessments/2026-11-08-schema4-restore/REPORT.md) і
[prefix evidence](_assessments/2026-11-04-full-prefix/REPORT.md).
Не повторювати import, reconciliation, schema upgrade або успішний full restore.

1. Визначити bounded workload із negative, prefix, leading-wildcard/infix,
   continuations, skew/різними парами та concurrency; поточні 16 positive cases
   не є репрезентативним traffic model. Зберігати реальні terms/paths приватно.
2. Розрізняти application/DB/OS cache й contention; перезапуск containers
   не означає cold OS cache. Не скидати host caches і не створювати глобальний
   pressure на машині зі сторонніми containers. Спершу обрати ізольовану методику.
3. Rediscover bind/image/private config і вільний диск. Source та restored DB
   тепер schema 4; запускати лише явно обрану ціль із фіксованими caps і restricted
   LOGIN. Зберегти state/inventories; не змінювати schema/deadline без доказів.
4. Відокремлювати діагностику latency/503 від доказів causality й AT-16 acceptance;
   звітувати workload, caps, errors, sample size/cache limits. Oracle не задіювати.
5. Прибрати власний runtime, зберегти дані, review → commit → PR → green CI.
   Merge виконує власник. Жодних додаткових видалень не погоджено.

## Відновлення після переривання

Стара спроба `schema4-restore-2026-11-08/` зберігає interruption/storage receipts;
її `run.py` не є resumable, повторно не запускати. Успішний retry —
`schema4-restore-2026-11-08-retry/`: усі metadata, `lifecycle.json`, backup,
restored cluster/files збережені. Після reboot source повністю переінвентаризовано,
збіг із PR27 підтверджено. Дозвіл видалення стосувався лише старого
`corpus-restore-2026-11-04/.pgdata` і вже виконаний; інші копії не видаляти.

## Що залишається відкритим

AT-16, cold-cache/negative/infix/concurrency workloads, crawler budget,
причина історичних HTTP 503, target server/storage capacity, off-host RTO,
secret escrow/RPO/retention та мовне/дизайн-приймання. Поточна positive локальна
вибірка не закриває ці gates. Oracle root близько 45 GiB за старим заміром
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
