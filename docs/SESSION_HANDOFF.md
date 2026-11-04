# Передача сесії — 2026-11-04

Роботу призупинено на прохання власника до завтра, 2026-11-05.
Продовжувати технічні роботи лише після його наступної команди.
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

## Що збережено локально

Усі шляхи нижче відносні до кореня checkout; `data/experiments/` ignored.
Credentials, реальні терміни, raw plans, keyring і HTTP paths не публікувати.

| Об’єкт | Шлях / стан |
|---|---|
| Початкова dev-БД | `data/experiments/postgres-corpus-preflight-2026-10-25/.pgdata`, schema 4 |
| Immutable exports/report/sitemaps | `data/experiments/corpus-verification-2026-11-04-retry/artifacts/` |
| Попередній backup bundle | `data/experiments/corpus-restore-2026-11-04/backup/`, schema 3 |
| Попередній restored cluster | `data/experiments/corpus-restore-2026-11-04/.pgdata`, schema 3 |
| Попередні restored files | `data/experiments/corpus-restore-2026-11-04/restored/` |
| Останні inventories/diagnostics | `data/experiments/full-prefix-2026-11-04/` |
| Одноразові harness scripts | `tmp/full-prefix/`, `tmp/corpus-restore/`; відсутні в clean checkout |
| Повторюваний агентський inventory | `minions/corpus-inventory/`, Go, 17 таблиць + sequences |

Зафіксований cleanup прибрав власні containers/networks і тимчасовий reader LOGIN.
На момент паузи працюють лише сторонні `docker-mongodb-1` і `open-webui`;
їх не чіпати. Bind data, backup, restored copies та inputs збережено.
Вільно приблизно 101 GiB — датований замір, перед новою роботою перевірити знову.
Старі ephemeral ports/container IDs і видалені LOGINs повторно не використовувати.
[Карта даних](../data/README.md) пояснює історію й приватні inputs.

## Перша задача після відновлення

**Повний локальний backup/restore schema 4** за [процедурою](BACKUP_RESTORE.md).
Попередній успішний full restore перевіряв schema 3; новий індекс у ньому відсутній.

1. Перевірити Git/стан PR цієї пам’ятки, прочитати INDEX, останній full-prefix
   звіт і попередній [full restore](_assessments/2026-11-04-corpus-restore/REPORT.md).
   Не повторювати import, reconciliation чи upgrade вже оновленої dev-БД.
2. До запуску оцінити storage для нового dump, копій artifacts/sources, fresh
   cluster, index build, WAL/temp і запасу щонайменше 30 GiB. Не припускати,
   що поточних 101 GiB вистачить. За дефіциту запропонувати конкретні варіанти;
   не видаляти збережені backup/restored/source copies без окремого рішення.
3. Створити нову run directory та окремий fresh cluster; попередній schema 3
   bundle і restored cluster не перезаписувати. Rediscover source bind/image/
   private config, зафіксувати ресурсні ліміти й тримати всіх writers зупиненими
   під час dump та копіювання файлів. Keyring зберігати приватно.
4. Перевірити dump/hash, усі 17 таблиць/sequences, index definition/validity,
   artifact/source inventories, новий owner/ACL і operator retry. Перед HTTP —
   ANALYZE; source вимкнути для target acceptance. Перевірити readiness,
   exact/prefix/cursors, downloads/sitemaps та відмови за відсутніх artifacts.
5. Зафіксувати elapsed/storage/WAL/resource/privacy evidence, прибрати лише
   власний runtime, зберегти всі результати; review → commit → PR → green CI.

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
