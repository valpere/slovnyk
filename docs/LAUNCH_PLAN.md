# План першого запуску Slovnyk

Стан: **підготовка, production не розгорнуто**. PR #38 змержено;
локальна schema 4, пошук, exports, sitemaps і restore перевірені.
Цей документ визначає наступні операторські кроки, не підтверджує готовність
сервера й не дає дозволу змінювати зайнятий Oracle, DNS або сторонні сервіси.
Traffic/crawler profile погоджено; власник ще обирає сервер і приймає
UI/переклади та відкриття доменів.

[Початковий профіль сервера](SERVER_PROFILE.md) і [deployment runbook](../deploy/README.md)
підготовлено локально: systemd, PostgreSQL, TLS proxy, readiness/disk і Telegram.
Налаштування цілі, credentials, зовнішній watchdog і off-host restore ще попереду.

## 1. Вибрати ціль і перевірити місткість

[Кандидати](SERVER_CANDIDATES.md) перевірені за публічними джерелами
2026-11-10: netcup Lite 3 — запропонована перша ціль для quote/acceptance,
не замовлений сервер. Актуальний Oracle Always Free A1 не покриває профіль;
можливість платного варіанта залежить від account quote.

[Початковий замір Oracle](_assessments/2026-10-23-server/REPORT.md) історичний:
1 GB RAM, root близько 45 GiB. Поточний повний PostgreSQL layout на такому
root не поміщається навіть без резерву. Звільнення іншого проєкту не збільшує диск.
Для конкретної нової цілі спочатку зафіксувати архітектуру, виділену CPU quota,
RAM/swap, файлові системи, usable free bytes та інші workloads read-only.

[Розрахунок](_assessments/2026-11-09-launch-plan/storage-budget.json)
спирається на schema 4 backup/restore 2026-11-08; GiB = 2^30 bytes.

| Складова | Обсяг / статус |
|---|---:|
| БД після upgrade / restore | 66.744 / 60.011 GiB, виміряно |
| Immutable artifacts, 1731 файл | 4.331 GiB, inventory |
| Вихідні CSV, 992 файли | 0.499 GiB, inventory |
| Один backup payload: dump + files + manifest + binary | 10.809 GiB, без малих службових inventories |
| WAL / temporary files / вільний резерв | 2 / 20 / 30 GiB, **планові припущення** |

Для планування беремо більшу виміряну БД: serving payload близько **71.575 GiB**.
Разом із локальним backup payload та наведеними allowances — **134.384 GiB**,
**без ОС, інших сервісів, filesystem overhead та додаткових backups**.
Це арифметичний сценарій, не гарантований peak або погоджений мінімальний диск.
WAL/temp можуть перевищити allowances; per-session `temp_file_limit` не є
загальним лімітом. До операції потрібні actual free-space preflight і stop threshold.
Другий dataset із такими самими розмірами додає орієнтовно **71.076 GiB**
DB/artifacts, до WAL/temp; початковий бюджет не покриває retained/staging growth.

Локальні CPU/RAM caps (DB 2 CPU/8 GiB, HTTP 1 CPU/512 MiB) — умови експерименту,
не виміряний мінімум хоста. Для ОС, proxy й filesystem cache потрібен запас;
512 MiB HTTP peak включає startup/artifact cache. Capacity конкретного сервера
підтверджує лише його приймання. Початковий орієнтир 80–100 GiB із старого
звіту не є актуальним повним restore budget для нинішнього layout.

## 2. Підготувати release та відновити перевірений dataset

1. Зафіксувати release commit, Go build environment, CPU architecture і SHA-256
   binary. Для іншої архітектури потрібна відповідна збірка й її перевірка.
2. Перенести перевірений schema 4 bundle за [BACKUP_RESTORE](BACKUP_RESTORE.md)
   у захищене місце; звірити inventory/hashes. Окремо підготувати cursor keyring,
   конфігурацію й credentials. Rehearsal passwords/ports не переносити.
3. На обраній цілі provision окремі group roles/LOGINs і новий non-superuser
   owner; restore в порожню БД, `ANALYZE`, відновлення immutable files й ACL.
   HTTP використовує restricted LOGIN і read-only artifact mount, без owner DSN.
   Existing dump містить migration ledger: не запускати `migrate` для створення
   таблиць перед restore. Повний повторний import незмінених CSV не потрібний.
4. Спершу запустити на loopback. Перевірити `/healthz` і `/readyz`, schema 4,
   active dataset/generation, inventories/sequences, artifact hashes та controls
   із джерелом, недоступним цілі. Health 200 сам по собі не означає readiness.
   Current binary й старий bundled binary мають різні release hashes;
   їхню сумісність перевіряти явно, зберігати обидва release records.

## 3. Адресне приймання на цільовому сервері

Власник **погодив 2026-11-09** пошуковий mix: 50% exact positive, 25% exact negative,
20% short prefix, 5% інших masks, до 10 searches/s. Для target run планується 5 хвилин.
Також погоджено crawler/download budget: 1 word page/s, до 2 concurrent crawler
requests і один artifact download stream. Actual bandwidth/resource budgets
і місткість retention ще не визначені.
Backup: до відкриття й після оновлень, off-host на локальний комп’ютер;
[політика](BACKUP_RESTORE.md#політика-першого-read-only-запуску-2026-11-10).
Secret escrow ще не перевірено. Traffic/crawler числа не є реалізованими rate limits; `/robots.txt` не забезпечує їх дотримання.

На обраній цілі перевірити ordinary subset handler p95 <=1 s,
DB budget 2 s, HTTP errors/timeouts, memory/OOM, CPU throttling, WAL/temp/disk
і readiness. Використати фіксовані positive/negative/prefix та leading/infix
cases, включно з найбільшими парами, cursors і controls до/після; не відбирати
лише швидкі запити. Відділяти [server timings](SEARCH_TIMING.md) від client queue.
Cold startup виміряти окремо; не скидати host cache глобально на shared host.
Off-host recovery має повернути придатний сервіс за <=24 h, включно з transfer,
restore, файлами, secrets і control checks; локальні ~56 хвилин цього не доводять.

## 4. Прийняти UI та підготувати домени

Матеріали й порядок ручного приймання — у [UI_REVIEW](UI_REVIEW.md).
Власник перевіряє desktop/mobile, keyboard flow, browser-language negotiation,
ручну UI-мову й пару, no-match підказку про маски, downloads та word pages.
Зафіксувати прийняті мови й виправлення перекладів; screenshots не означають
автоматичного мовного/дизайн-приймання.

На target підготувати TLS для обох доменів і proxy до loopback HTTP.
Зберігати оригінальний `Host`: alias redirect у застосунку використовує його,
не `X-Forwarded-Host`. Перевірити `slovnyk.org.ua` → fixed
`https://slovnyk.org` зі збереженням escaped path/query, canonical/robots/sitemap,
secure cookies й Cache-Control. Proxy access/error logs також не повинні
публікувати пошукові URL або credentials. До зміни DNS перевірити virtual hosts
адресним резолвінгом; readiness має бути доступна оператору.

## 5. Відкрити сервіс і зберегти можливість відновлення

Після проходження gates — окрема операторська зміна DNS/proxy для обох доменів,
зовнішні smoke checks і контроль errors/readiness/disk. Погодити stop/rollback
критерії до відкриття. Перший реліз read-only; повернення до загиблого старого
сервісу неможливе. Якщо попереднього нового release/dataset немає, зупинити
проблемний запуск і відновити перевірений комплект за [планом](MIGRATION.md#5-відкат).
Не видаляти failed/retained datasets або backups для звільнення місця автоматично.

Підсумковий receipt: release/hash, target/resources/storage, approved profile,
restore elapsed, inventories/files/controls, timings/errors, UI-рішення,
DNS/TLS перевірки, backup location/RPO/retention та rollback owner.
Secrets і raw user queries не входять у публічний receipt.
