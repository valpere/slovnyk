# Стан сесії — 2026-11-10

Після merge [PR #30](https://github.com/valpere/slovnyk/pull/30)
(`78fe284097a4e5f278a37a19d8f8bf1185ca9bfe`) і команди власника `go`
додано server-side timings і завершено короткий локальний run на restored schema 4.
Це поточна пам’ятка продовження; вимоги й контракти визначає [INDEX](INDEX.md).

## Пауза над сервісом — посіпаки 2026-11-10

Власник змержив PR #36 (`99f4f95`); локальний main синхронізовано.
Сервер із новим знаком продовжує працювати на localhost:8787 для перегляду.
На прохання власника роботу над сервісом призупинено; повторювані агентські
дії винесено в [minions](../minions/README.md): точний head CI status і
CDP UI matrix/captures. Це особисті помічники агента, не утиліти сервісу.
Вони не змінюють deployment або вимоги приймання.

## Оновлення 2026-11-10

PR #35 змержено власником (`ee876c2`). На його прохання локальний сервер
знову працює на http://localhost:8787/ зі збереженим restored corpus.
Власник позитивно оцінив UI; це не приймання всіх локалей. Наступна зміна —
знак із `Original Logo2.svg` у шапці; [походження й перевірки](_assessments/2026-11-10-brand-logo/REPORT.md).
Потребує перегляду власником. Старі записи нижче є історичними.

## Підтверджений стан

- [PR #34](https://github.com/valpere/slovnyk/pull/34) змержено власником;
  merge `9c145572addad133f577854cc9215665d7a6c859`. Чотири CI jobs SUCCESS
  на head `fd7114caa46c1ca9b0dc3f006e39ea1c0807c977`.
- Власник оновив `slovnyk.sv-se.po`; [нове зіставлення](_assessments/2026-11-09-swedish-locale/REPORT.md)
  переносить 7 сумісних фраз у `web/locales/sv.json` із новим source hash.
  22 нові фрази залишаються `ai`; статус шведської локалі — `ai-draft`.

- [PR #33](https://github.com/valpere/slovnyk/pull/33) змержено власником;
  merge `5472976f0157298d101f6ce08e3e121e39ae9e2d`, усі чотири CI jobs SUCCESS
  на head `7b389d5618805ca9bd5d893ada2aeaa158a24e8c`.
- Наступний локальний зріз — [дизайн](UI_DESIGN.md) за наданими власником
  Town brief, брендбуком і кремовим зразком. [Screenshots/review](_assessments/2026-11-09-brand-design/REPORT.md)
  містять synthetic дані; остаточне дизайн/мовне приймання ще відкрите.
  Локальний `localhost:8787` переведено на справжній restored corpus для
  ручного перегляду; службові імена/cleanup збережено лише в ignored checkpoint.
  Стан старіших runtime cleanup нижче — історичний. Oracle не чіпали.

- [PR #32](https://github.com/valpere/slovnyk/pull/32) змержено власником
  2026-11-09 13:52:32 UTC, merge `c26a2cace0abc2954ee5a2d61bdd829498365cfa`.
  Чотири CI jobs SUCCESS на head `970523816146f85189c22caf11f660519e4a7f1e`.
- Новий локальний зріз — [UI review](UI_REVIEW.md): synthetic Go-посіпака
  `minions/ui-preview`, desktop/mobile screenshots і ручний checklist.
  Чотири українські labels уточнено, origins змінено на `ai`; всі 31 catalogs
  залишаються `ai-draft`. Мовне/дизайн-приймання власником ще відкрите.

- [PR #31](https://github.com/valpere/slovnyk/pull/31) змержено власником
  2026-11-08 15:44:44 UTC, merge `68655c445527851dc5aa630731014eadb124362b`.
  Чотири CI jobs SUCCESS на head `4655d79bd0ef8d773df039914f93ec2d501ed3f6`.
  Локальна main синхронізована після merge; feature branch видалено.

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

## Останній зріз і наступна задача

[План першого запуску](LAUNCH_PLAN.md) підготовлено локально 2026-11-09:
serving payload ~71.575 GiB, сценарій restore із локальним bundle і плановими
WAL/temp/reserve ~134.384 GiB **без ОС**. Це припущення для вибору цілі,
не upper bound чи погоджений бюджет. Нових DB/server probes не було.
Власник погодив весь traffic/crawler/download profile (див. DECISIONS)
і підтвердив: Oracle ще зайнятий, зараз готуємо вимоги; SSH не виконували.
Resource/storage budget, сервер і deployment залишаються відкритими.

[Search timings](SEARCH_TIMING.md) opt-in, default off, INFO; ніяких query/URL/pair/error
strings. [Run](_assessments/2026-11-08-server-timings/REPORT.md): 1010/1010 HTTP 200,
978/978 events, 110/110 prior response oracles; ordinary/mixed handler p95
18.998/20.365 ms. Власний runtime/restricted LOGIN прибрано, state незмінний;
full inventory не повторювали. Inputs і raw logs — ignored
`data/experiments/server-timings-2026-11-08/`, scratch `tmp/server-timings/`.
Окремого незалежного reviewer не було; послідовне self-review й локальні checks
зафіксовано у звіті. CI/merge перевіряти через GitHub, не припускати завершення.

1. Погоджено власником 2026-11-09 mix: 50% exact positive, 25% exact negative,
   20% prefix, 5% інших masks, до 10 searches/s; crawler до 1 page/s,
   до 2 concurrent requests, один download stream. Resource budget відкрито.
   AT-16 відкрито до перевірки на обраному сервері.
2. Вибрати сервер/storage і виконати одну адресну target acceptance з цим
   профілем, readiness й off-host restore; UI/translation приймає власник.
   Cold-host/low-memory/startup, backup escrow/RPO/retention ще відкриті.
3. Не повторювати широкі локальні import/reconciliation/upgrade/restore/workloads
   без нового конкретного ризику. Historical 503 не пояснено.
4. Review → commit → PR → чотири green CI на exact head. Merge — власник;
   branch deletion після merge увімкнено. Failed CI виправити й повторити.
   Oracle зайнятий, legacy/сторонні containers не чіпати; нових видалень не дозволено.

## Відновлення після переривання

Стара спроба `schema4-restore-2026-11-08/` зберігає interruption/storage receipts;
її `run.py` не є resumable, повторно не запускати. Успішний retry —
`schema4-restore-2026-11-08-retry/`: усі metadata, `lifecycle.json`, backup,
restored cluster/files збережені. Після reboot source повністю переінвентаризовано,
збіг із PR27 підтверджено. Дозвіл видалення стосувався лише старого
`corpus-restore-2026-11-04/.pgdata` і вже виконаний; інші копії не видаляти.

## Що залишається відкритим

AT-16, target/cold-host/low-memory/startup приймання, реалізація/перевірка crawler budget,
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
