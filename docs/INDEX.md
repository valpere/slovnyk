# Slovnyk: джерело істини

Дата базового дослідження: 2026-10-21. Новий проєкт: `~/wrk/projects/slovnyk/slovnyk/`;
старий: `~/wrk/projects/slovnyk/slovnyk1.0/`.

## Статус і правила

**Мета, уточнена користувачем 2026-10-23:** перенести сервіс на нову реалізацію.
Старий код і дані доступні як джерело інформації про сервіс. Перенесення
коду чи архівів саме по собі не є результатом проєкту; технічні роботи
з даними підпорядковані відтворенню потрібних функцій сервісу.

**Підтверджено користувачем:** новий проєкт з нуля, мова Go, окрема нова БД,
наповнена даними старого сервісу; структура даних може відрізнятися зі збереженням
їхнього змісту (уточнено 2026-10-23). Основа першого релізу — пошук, маски, вибір мов,
локалізація й завантаження словників; нова статистика запитів — окремий етап.
Перший реліз безкоштовний і без реклами (підтверджено 2026-10-23).
Пошук автоматично розпізнає маски за `*` і `?` (підтверджено 2026-10-23).
Результати групуються під оригінальним вихідним словом або фразою.
Завантаження — окремий CSV.gz для кожної наявної мовної пари, без загального архіву.
Індексація словникових сторінок пошуковиками входить до першого релізу (FR-14).
Кожна така сторінка показує переклади однією обраною мовою; пара визначена URL.
Остання обрана мовна пара зберігається між відвідуваннями й підставляється у форму.
На першому візиті пропонується мова браузера → англійська; для англомовного
браузера — англійська → українська. Пропонуються лише наявні напрямки.
Сумісність старих пошукових і download-URL не потрібна (OQ-04 закрито).
Домени нового сервісу — `slovnyk.org` (основний) і `slovnyk.org.ua`
(додатковий), підтверджено 2026-10-23. Додатковий домен автоматично
перенаправляє на основний зі збереженням шляху й параметрів нових URL.
Обраний напрям UI — лаконічний сучасний пошуковий інструмент: виразний пошук,
багато вільного простору, мінімум зайвих елементів (підтверджено 2026-10-23).
Конкретні макети й критерії приймання дизайну ще уточнюються.
**Середовище, підтверджене 2026-10-23:** розробка локально, подальше розгортання
на сервері Oracle Cloud після його звільнення. Наявний `psysoft` досліджено
read-only; придатність для повного корпусу не підтверджена, альтернативи допустимі.
Характеристики й рекомендація — у [звіті](_assessments/2026-10-23-server/REPORT.md).
**Уточнення 2026-10-24:** Micro досі зайнятий іншим проєктом; його роботу
не порушувати. Через `ssh psysoft` дозволені лише легкі адресні read-only
перевірки. Імпорт, інсталяції, зміни конфігурації, перезапуски та benchmarks
на сервері не виконувати. Подальші експерименти — локально; read-only
не означає дозволу створювати навантаження.

Решта цільових рішень нижче — запропонований базовий
контракт за результатами дослідження, а не твердження про вже погоджені побажання.
M1-01 реалізовано 2026-10-24: локальний Go HTTP-каркас і dev/test середовище,
[команди запуску](../README.md#локальний-запуск),
[фактичні перевірки](_assessments/2026-10-24-go-scaffold/REPORT.md).
M1-02 реалізовано 2026-10-24: Unicode keys/masks, lossless CSV4.gz,
strict manifest і synthetic/golden fixtures;
[звіт](_assessments/2026-10-24-formats/REPORT.md).
M1-03 реалізовано: нова PostgreSQL schema, migration CLI, privileges і storage
primitives; [звіт](_assessments/2026-10-24-postgres-schema/REPORT.md).
M1-04 реалізовано: inspect/run CLI, staged CSV import, ledger/provenance,
session lock і retry; [звіт](_assessments/2026-10-24-staged-import/REPORT.md).
M1-05 реалізовано: reconciliation, durable exports/report, publish/rollback CAS
і HTTP readiness; [звіт](_assessments/2026-10-24-publication/REPORT.md).
M1-06 реалізовано: exact/mask application service, dataset-pinned signed cursor,
C seek і спільний DB deadline; [звіт](_assessments/2026-10-24-lookup/REPORT.md).
M1-07 in progress: HTTP-пошук і bounded HTML/plain UI реалізовано;
каталог, локалізація й cookies реалізовані в наступному зрізі,
[звіт](_assessments/2026-10-24-catalog-localization/REPORT.md).
Version-pinned synthetic downloads також реалізовано,
[звіт](_assessments/2026-10-24-downloads/REPORT.md). Synthetic SEO pages,
canonical і search discovery реалізовані за [SEO_PLAN](SEO_PLAN.md).
Офлайн-генерація sitemap/index для ready dataset реалізована,
[контракт](SEO_PLAN.md#офлайн-генерація-sitemap),
[self-review](_assessments/2026-10-25-offline-sitemaps/REPORT.md).
HTTP sitemap discovery та schema 3 inventory реалізовано локально,
[self-review](_assessments/2026-10-25-http-sitemaps/REPORT.md).
Клавіатурний перехід до вмісту й окремі language/direction annotations
перевірено через наявний Chromium без page JS,
[звіт](_assessments/2026-10-25-multilingual-accessibility/REPORT.md).
Повний локальний sitemap/HTTP rehearsal описано нижче; crawler capacity
та production-міграція ще попереду.
M1-08 synthetic наскрізне CLI/HTTP відтворення реалізовано:
[звіт](_assessments/2026-10-25-service-lifecycle/REPORT.md),
[команди](../README.md#наскрізне-відтворення-на-порожній-бд).
FR-13 local synthetic restore перевіряє нову БД/root при недоступній вихідній БД;
[процедура](BACKUP_RESTORE.md), [звіт](_assessments/2026-10-25-backup-restore/REPORT.md).
Наступний [fresh-cluster зріз](_assessments/2026-10-25-fresh-cluster-restore/REPORT.md)
перевіряє restore у другий порожній cluster, bootstrap ролей, новий owner та ACL.
[Повний локальний backup/restore](_assessments/2026-11-04-corpus-restore/REPORT.md)
також пройшов: 17 таблиць, sequences, файли й HTTP при зупиненому source.
Off-host і target-server RTO acceptance не закрито.
M1 загалом ще відкритий: мовне/дизайн-приймання та повний SEO discovery не закрито.
M2 preparation: Go utility [prepare-csv-manifest](../utilities/README.md#підготовка-csv-manifest)
створила inventory окремої копії 992 CSV.gz; hashes/bytes збігаються з попереднім
аудитом, [evidence](_assessments/2026-10-25-csv-manifest/REPORT.md).
Повний Go `import inspect` цієї копії пройшов: 992 complete/checksum-verified files,
56 455 068 parsed occurrences, 0 rejected; per-file counts збігаються з аудитом.
[Evidence](_assessments/2026-10-25-go-corpus-inspect/REPORT.md).
Distinct tuples в inspect не рахуються; PostgreSQL import описано нижче,
production acceptance ще не виконано.
Наступна [bounded PostgreSQL проба](_assessments/2026-10-25-postgres-corpus-preflight/REPORT.md)
закомітила 114 000 записів того самого manifest: interrupt/retry зберіг counts і
digests перших 55 000; active dataset порожній. Це частина першого файла,
не повний import/reconciliation або capacity acceptance.
Повний [staged PostgreSQL import](_assessments/2026-10-27-postgres-corpus-import/REPORT.md)
завершив цей dataset: 992 complete/checksum-verified files, 56 455 068 unique,
0 duplicates/rejected/issues; actual ledger/origins/translations, checkpoints
і per-file/pair counts узгоджені з audit. Prefix digests 55 000 збережені.
[Source/normalization reconciliation](_assessments/2026-11-04-corpus-reconciliation/REPORT.md)
та [повний verify/exports](_assessments/2026-11-04-corpus-verification/REPORT.md) пройшли 2026-11-04:
992 CSV4.gz exports, 56 455 068 записів; retained export revalidation пройшла.
Dataset перейшов у `ready`. [Погодження власника](DECISIONS.md#owner-approval-2026-11-04)
збережено в report; known gaps лишаються видимими.
Наступний [локальний sitemap/HTTP rehearsal](_assessments/2026-11-04-corpus-sitemaps/REPORT.md)
створив 36 763 066 URL у 736 shards; retry побайтово тотожний. Усі XML
перевірені; усі shards і 992 exports звірені через HTTP, sample
word/search/redirect та restart пройшли. Локальна publication: generation 1.
Production, crawler/search SLO та full-corpus off-host restore відкриті.
Наступна [локальна змішана проба](_assessments/2026-11-04-corpus-load/REPORT.md):
2 112/2 112 HTTP/content checks успішні; 10 search/s плюс у середній фазі
5 word pages/s і 0.2 shards/s. Короткий warm-cache прогін обмеженої вибірки
не закриває AT-16, crawler budget або target-server capacity.
Під час restore preparation окрема проба 16 масок із `limit=1` дала 6 HTTP 503
приблизно через 2 s; exact 16/16 успішні.
[Подальша діагностика](_assessments/2026-11-04-mask-plans/REPORT.md): 160/160 HTTP 200
без concurrent restore; 32/32 SQL-плани мають LIKE postfilter після сканування пари.
Причина попередніх 503 не доведена.
[Bounded-prefix prototype](_assessments/2026-11-04-prefix-index/REPORT.md) на окремій
копії 16 пар / 1 373 948 rows: 32/32 candidate index conditions із тотожними
результатами; synthetic pagination/Unicode/довгі поля пройшли. Prefix index
65.09 MiB, insert-sample WAL +29.1%; prototype не змінював service schema/query.
Schema 4 / `ReadPage` bounded prefix prefilter реалізовано з synthetic integration
перевірками Unicode/escaping, C seek, signed cursors і upgrade ready data.
[Фактичне покриття](TRACEABILITY.md#bounded-prefix-service-integration--2026-11-04).
[Повний локальний upgrade](_assessments/2026-11-04-full-prefix/REPORT.md):
початкова dev-БД тепер schema 4 на всіх 56 455 068 translations; index 2.428 GiB,
команда ~177 s, application hashes/sequences незмінні. 32/32 actual ReadPage
plans мають prefix Index Cond; 135 HTTP results тотожні, включно з 7 cursors.
БД тепер 66.744 GiB; backup/restored copies залишені schema 3.
Далі — schema 4 full backup/restore, ширші HTTP/cache/contention probes;
AT-16 і capacity acceptance відкриті.
До schema 4 один dataset/indexes — 64.315 GiB плюс WAL: на root ~45 GiB із профілю Oracle
2026-10-23 цей layout не поміщається. Нового server probe не було; локальні
memory caps 512 MiB → 8 → 16 GiB не є підтвердженням Micro capacity.
Окрема команда `dataset reconcile` звіряє staged dataset без exports/ready,
залишаючи known gaps та exceptions незакритими;
[synthetic перевірки](_assessments/2026-10-27-dataset-reconciliation/REPORT.md).
Наступна [повна локальна звірка](_assessments/2026-11-04-corpus-reconciliation/REPORT.md) пройшла;
[Конкретний звіт](LOSS_REVIEW.md) погоджено власником 2026-11-04; adapter
`verify --loss-approval FILE` підтримує known gaps, але не exceptions/rejections.
[Перший HTTP зріз](_assessments/2026-10-24-http-lookup/REPORT.md).
2026-10-24 [повний структурний аудит CSV](_assessments/2026-10-24-csv-audit/REPORT.md)
підтвердив 56 455 068 різних кортежів у 992 файлах, без точних дублікатів
і структурних відхилень за перевіреними правилами V1. Виявлено 118 009
додаткових написань зі спільними normalized keys, 20 source-записів із
крайніми пробілами та 15 021 зі `*`/`?`; вони зберігаються без очищення.
[Локальний прототип повної моделі](_assessments/2026-10-24-provenance/REPORT.md)
звірив усі raw translations/origins і canonical digests 992 exports із CSV.
Мовна якість, історична повнота й production migration ще не доведені.

Прямі рішення користувача мають пріоритет. Поведінку визначають requirements,
сценарії пояснюють її, architecture визначає межі й рішення, HLD — компоненти; DATA_MODEL та *_CONTRACT — їх детальні контракти.
LEGACY_COMPATIBILITY зберігає лише історичні спостереження. Виявлену суперечність виправляти разом у пов'язаних документах;
реалізація не повинна непомітно змінювати вимоги. Зміни фіксувати у Git.

Локальні БД й експорти зберігаються у `data/experiments/`, поза Git;
[опис і карта перенесення з tmp](../data/README.md). `tmp/` — лише scratch.

## Документи

Поточні підтвердження, результати огляду й точка продовження обговорення —
у [журналі рішень](DECISIONS.md). Експеримент з альтернативним engine —
[локальна проба SQLite](_assessments/2026-10-23-sqlite/REPORT.md).

| Документ | Призначення |
|---|---|
| [LEGACY.md](LEGACY.md) | Перевірені факти, джерела, обмеження дослідження |
| [USE_CASES.md](USE_CASES.md) | Актори, сценарії, альтернативи й помилки |
| [EDITING_PROPOSAL.md](EDITING_PROPOSAL.md) | Редагування й репутація: підтверджений обсяг та відкриті пропозиції |
| [REQUIREMENTS.md](REQUIREMENTS.md) | Нормативна поведінка й критерії приймання |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Межі системи, рішення, компроміси |
| [HLD.md](HLD.md) | Компоненти, модель даних, потоки та інтерфейси |
| [DATA_MODEL.md](DATA_MODEL.md) | Ключі, типи, обмеження та індекси V1 |
| [LOSS_REVIEW.md](LOSS_REVIEW.md) | Конкретний звіт про відсутні historical metadata та формат рішення власника |
| [IMPORT_CONTRACT.md](IMPORT_CONTRACT.md) | Формати, облік, retry, verify і publication CAS |
| [SEO_PLAN.md](SEO_PLAN.md) | Індексація в першому релізі; технічний план та відкриті питання |
| [DOWNLOAD_CONTRACT.md](DOWNLOAD_CONTRACT.md) | HTTP manifest, version-pinned CSV.gz, ranges, integrity і streaming limits |
| [LOOKUP_CONTRACT.md](LOOKUP_CONTRACT.md) | HTTP, Unicode, bounded cursor та пагінація |
| [LEGACY_COMPATIBILITY.md](LEGACY_COMPATIBILITY.md) | Історичні факти про старі URL; сумісність скасована |
| [BACKUP_RESTORE.md](BACKUP_RESTORE.md) | Склад backup, порядок restore та локальні synthetic/full-corpus репетиції FR-13 |
| [MIGRATION.md](MIGRATION.md) | Облік даних, перевірка, перемикання й відкат |
| [TRACEABILITY.md](TRACEABILITY.md) | Вимога → сценарій → компонент → перевірка |
| [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) | Порядок задач M1, залежності й критерії завершення; не змінює контракти |

## Пропоновані етапи

1. **M0 — контракт:** уточнити відкриті питання; зберегти фактичну базу старої системи.
2. **M1 — вертикальний зріз:** Go-сервіс, нова схема, імпорт синтетичного набору,
   точний пошук і маски, браузерна форма, тести та health/readiness.
3. **M2 — репетиція міграції:** повний аудит джерел, повторюваний staged-імпорт,
   звіряння, експорт і локалізація.
4. **M3 — запуск:** перевірка на реальному корпусі, backup/restore, узгоджене
   перемикання трафіку й перевірений відкат. Старі дані не видаляти.

Реєстрація, користувацькі правки, репутація й консиліум ШІ — окремий етап
**після запуску пошуку та завантажень** (погоджено 2026-10-23).
Статистика популярності — окремий наступний етап після рішення про її семантику.
Наявні історичні дані статистики зберігати незалежно від цього рішення.

## Відкриті питання

| ID | Питання | Що блокує |
|---|---|---|
| OQ-01 — вирішено | Основа першого релізу — функції старого сайту без нової статистики; користувач підтвердив 2026-10-21. | Не блокує |
| OQ-02 — джерела визначені, аудит відкритий | Нова окрема БД; доступні DDL + CSV (підтверджено 2026-10-23). Наявні CSV — остаточне джерело перекладів для першого запуску; старий сервіс більше не працює (підтверджено 2026-10-23). DDL — опис старої структури. Live БД/дамп даних до плану не входять. `count/status/queries` не відновлюються з CSV4. | Структурний аудит CSV завершено 2026-10-24, див. [звіт](_assessments/2026-10-24-csv-audit/REPORT.md); [Локальний прототип](_assessments/2026-10-24-provenance/REPORT.md) звірив усі translations/origins і 992 exports, але production import/retry/publication та семантична перевірка ще потрібні. Гарантія повноти стосується доступного корпусу |
| OQ-03 — частково вирішено | Розробка локально; Oracle Cloud — майбутній запуск, зайнятий Micro не навантажувати. Остаточний сервер/engine не обраний. Історичний пік до 10 запитів/с — оцінка користувача, не погоджений capacity gate; погоджені p95 звичайного пошуку <=1 s на сервері та DB deadline 2 s. [Повна модель із provenance](_assessments/2026-10-24-provenance/REPORT.md) — 25.54 GiB, без covering index під локальним CPU quota має 20/100 mask timeouts. [Оптимізований SQLite layout](_assessments/2026-10-24-sqlite-indexed/REPORT.md) — 29.66 GiB; 300 SQL-запитів при 10/с із лімітами 768 MiB/12.5% одного CPU пройшли без помилок, p95 105.955 ms. Це теплий developer cache, не Oracle/HTTP acceptance. [Compact keys](_assessments/2026-10-24-sqlite-compact/REPORT.md) з тими самими indexes — 20.85 GiB (−29.70%), всі translations/ledger/origins звірені; 0/300 SQL errors під тим самим quota, p95 103.494 ms. Дві компактні копії — 41.70 GiB, три — 62.55 GiB до artifacts; [датований Micro snapshot](_assessments/2026-10-23-server/REPORT.md) має ~41 GiB free/~45 GiB root. Storage budget та multi-dataset rehearsal потрібні; Go-каркас M1-01 ними не блокується. Платне розміщення допустиме; орієнтир близько $10/місяць на сервер і backup без консиліуму ШІ, не підтверджена жорстка межа; тариф не обраний. RTO <=24 h; повне off-host відновлення ще не перевірене. Місце для off-host backup є, сховище/процес не визначені. | Остаточний deployment profile, DB engine/layout, HTTP capacity та restore |
| OQ-04 — вирішено | Користувач підтвердив 2026-10-23: підтримка старих пошукових і download-URL не потрібна. Legacy adapter і redirects вилучені з обсягу; UC-07, FR-12 та AT-12 скасовані зі збереженням ID. | Не блокує |
| OQ-05 — вирішено | Користувач підтвердив 2026-10-23: права на словникові дані повністю належать йому. Питання прав для публікації та завантажень закрито; provenance джерел обліковується окремо. | Не блокує |
| OQ-06 — частково вирішено | Зберегти автоматичний вибір за мовою браузера. Ручний вибір запам’ятовується між відвідуваннями й має пріоритет над мовою браузера. Наявні PO — джерело перекладів; сучасний i18n дозволений. Пропущені й нові фрази доперекладати ШІ. Знайдено 31 PO-файл; див. [аудит](_assessments/2026-10-23-localization/REPORT.md). Резервна мова — англійська (`en`), погоджено 2026-10-23. Готовність локалей нового UI ще не визначена; uk/en не є погодженим обмеженням. | Остаточний перелік готових перекладів UI |

| OQ-07 — частково вирішено | Подавати додавання, редагування й видалення може будь-який зареєстрований відвідувач. Рейтинг прийнятих/відхилених правок впливає лише на пріоритет перевірки; кожна правка потребує схвалення модератором, без автоматичної публікації за рейтингом. Перевіряє консиліум ШІ, остаточне рішення ухвалює власник. Функція запускається після першого релізу пошуку й завантажень. Формула рейтингу, склад і бюджет консиліуму та збереження внесків при відкаті ще не визначені. Див. [пропозицію](EDITING_PROPOSAL.md). Поточна baseline не покриває цей сценарій. | Контракт редагування, права, версійність і план відкату після запуску запису |

Технічний аудит OQ-02 блокує гарантію повноти міграції, але не створення Go-каркаса
та роботу на синтетичних fixtures. Не підміняти відповіді припущеннями.
