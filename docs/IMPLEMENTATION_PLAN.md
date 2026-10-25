# План реалізації M1

Статус на 2026-10-25: **M1-01–06 done; M1-07 in progress;
M1-08 done у synthetic verification scope**. M1 загалом ще не закрито.
Це порядок виконання [джерела істини](INDEX.md), а не нові поведінкові вимоги.
У разі суперечності звірити REQUIREMENTS і детальні контракти та виправити план.
Позначати задачу виконаною лише з посиланнями на реалізацію й фактичні перевірки.

## Результат етапу

На локальній ізольованій PostgreSQL оператор імпортує синтетичний CSV4.gz,
звіряє дані, готує експорти та атомарно публікує dataset. Відвідувач без JS
шукає exact/mask, переглядає стабільні сторінки й завантажує синтетичний словник.
Заміна active dataset не змішує сторінки; невдалий імпорт не стає видимим.
Це локальний робочий зріз, не завершена міграція або production release.

## Порядок і залежності

`M1-01 → M1-02 → M1-03 → M1-04 → M1-05 → M1-06 → M1-07 → M1-08`.
Тести додаються разом із відповідною поведінкою; M1-08 перевіряє весь шлях.
Мінімальні export/verify/publish потрібні вже у M1 для дотримання ready gate,
хоча повні acceptance перевірки міграції та завантажень залишаються у M2.

### M1-01 — Go-каркас та локальне середовище

**Status: done, 2026-10-24.** Реалізація: `cmd/slovnyk/`, `internal/ops/`,
`go.mod`, `Makefile`, `compose.yaml`, `.env.example`, `minions/test-postgres.sh`.
Module відповідає remote; Go 1.26.10, PostgreSQL 17.11 із image digest,
Go dependencies — лише stdlib. Команди та обґрунтування — у [README](../README.md).
Build, unit, vet/format, race, ізольований PostgreSQL smoke та реальні
SIGTERM/SIGINT process smoke пройшли; [докази](_assessments/2026-10-24-go-scaffold/REPORT.md).
Readiness fail-closed: 503 `storage_not_initialized`; DB/schema/active/artifact
діагностика додається в M1-03/05. AT-14/17 покриті частково, пошуку ще немає.

- Визначити module path за фактичним remote або задокументувати тимчасовий;
  закріпити конкретні Go/PostgreSQL/dependency versions і причини вибору.
- Створити `cmd/slovnyk`, пакети HLD у міру потреби, config, graceful shutdown,
  структуровані логи, `/healthz`; без active dataset `/readyz` повертає 503.
- Додати відтворюваний запуск окремої dev/test PostgreSQL та `.env.example`
  без секретів. Не використовувати стару БД як dev/test target.
- Додати реальні build/test команди й оновити README. Пропонований Make interface:
  `build`, `test`, `check`, `test-integration`, `test-race`; ці targets реалізовані.

**Готово:** чистий checkout збирається за README; процес завершується коректно;
unit checks проходять; помилка config не розкриває DSN. AT-14/17 — частково.

### M1-02 — Нормалізація, формати й синтетичні fixtures

**Status: done, 2026-10-24.** `internal/normalization/`, `internal/importdata/`,
`docs/schemas/csv-manifest-v1.schema.json`, `testdata/`.
Versioned NFC/full fold/query trim, parameter-only LIKE compiler; lossless
CSV4/gzip reader/writer, rejection reasons, limits, EOF/checksum semantics;
strict manifest, canonical fingerprint і tuple/key golden hashes.
Build, unit, vet/format, race, HTTP smoke, schema validation і bounded fuzz
пройшли; [звіт](_assessments/2026-10-24-formats/REPORT.md).
Це format primitives без CLI/storage; AT-02/03/07/18 покриті частково.

- Реалізувати NFC → full case fold → NFC, query-only trim, version ID,
  literal exact та mask compiler за [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md).
- Реалізувати lossless CSV4 parser, обмеження й gzip EOF/checksum semantics
  за [IMPORT_CONTRACT](IMPORT_CONTRACT.md); не підміняти його `encoding/csv.Reader`.
- Визначити точну machine-readable schema CSV manifest, canonical serialization,
  tuple/key digest й golden bytes/hash fixtures. DB JSONL adapter поки deferred.
- У `testdata/` створити малий корпус із кількома парами, sr-rs→sr-rs,
  дублями, normalized collisions, Unicode, punctuation, edge spaces і CR/LF.
  Пошкоджені/відхилені випадки — окремі fixtures з очікуваними reasons.

**Готово:** parser зберігає raw поля побайтово після decoding; працює потоково;
invalid/NUL/oversized values обліковуються; chunking не змінює ordinals;
fold expansions і wildcard escaping підтверджені тестами. AT-02/03/07/18 — частково.

### M1-03 — Нова схема й PostgreSQL storage

**Status: done, 2026-10-24.** `migrations/000001_initial.sql`,
`internal/storage/postgres/`, `slovnyk migrate`, оновлений PostgreSQL minion.
Транзакційні embedded migrations із checksum ledger/advisory lock, composite
FK, hash/length constraints, provenance/checkpoints/events, terminal content
immutability, окремі HTTP/operator groups. Exact primitive: bounded hash index,
residual equality, C sort; application dataset pin/cursor/deadline — M1-06.
Реальна PostgreSQL 17.11: long fields, FK/privileges/rollback/publication guards,
concurrent ready/write, fresh/repeated migrations; [звіт](_assessments/2026-10-24-postgres-schema/REPORT.md).
Build/unit/vet/format/race/HTTP smoke пройшли. HTTP readiness ще fail-closed,
підключення storage/active/artifact checks — M1-05. AT-01/09/15/18 — частково.

- Перетворити [DATA_MODEL](DATA_MODEL.md) у версійні SQL migrations нової БД:
  composite FK, hashes, provenance ledger, checkpoints, publication events і generation.
- Додати bounded exact index + residual equality, C collation та захист ready
  content від змін. Реалізувати окремі HTTP read-only й operator DB privileges.
- Перевірити реальні constraints/transactions на PostgreSQL, а не лише mocks.

**Готово:** нова схема піднімається з нуля; FK відхиляють cross-dataset references;
довгі допустимі поля вставляються; HTTP role не може імпортувати/публікувати;
тести не запускають legacy SQL. AT-01/09/15/18 — частково.

### M1-04 — Inventory та повторюваний staged import

**Status: done, 2026-10-24.** `internal/importer/`, `slovnyk import inspect/run`,
COPY batches, session lock, transactional ledger/origins/issues/counters/checkpoint,
stream/hash checks і deterministic replay. Успіх — лише `validating`.
Реальна disposable PostgreSQL: retry після кожного batch, validating replay,
rollback, actual session loss, collision injection, corrupt inputs, cross-file
ownership і operator LOGIN CLI; [звіт](_assessments/2026-10-24-staged-import/REPORT.md).
Повний corpus, verify, ready, publication та capacity ще не перевірені.

- З'єднати CLI inspect/run, manifest validation, hash checks і потоковий parser
  зі storage; V1 mapping identity, жодного мовчазного sr-sp→sr-rs.
- Реалізувати single writer session lock, batch+ledger+origins+checkpoint
  в одній транзакції, deterministic replay і retry після interruption.
- Показувати unique/duplicate/rejected, completion state та issues; failed
  file не має виглядати повністю врахованим. Не вигадувати count/status для CSV4.

**Готово:** clean run та retry після кожного batch дають ті самі tuples/origins;
пошкоджений trailer блокує ready; duplicate artifact hash відхиляється;
active dataset незмінний. AT-07/08/09/11/18 — лише synthetic CSV частина.

### M1-05 — Verify, незмінні exports, publish та rollback

**Status: done, 2026-10-24 (synthetic CSV subset).** `internal/publication/`,
`internal/artifacts/`, importer reconciliation, dataset CLI і HTTP readiness.
Real PostgreSQL: ready/publish, повторний inventory, concurrency CAS, stale/ABA,
rollback і negative isolation; child-process artifact crashes та recovery tests.
[Звіт](_assessments/2026-10-24-publication/REPORT.md). Approval-loss adapter,
повний Go corpus і production durability gate залишаються відкритими.

- Реалізувати reconciliation counts/digests/origins, CSV4.gz round-trip і звіт,
  прив'язаний до fingerprint та export hashes. Жодного `force ready`.
- Готувати versioned artifacts через fsync + atomic no-clobber installation
  та fsync directories до ready/active DB commit; перевіряти startup references.
- Реалізувати publish/rollback з обов'язковим expected-generation CAS,
  publication history і відмовою для stale generation, failed та неперевірених наборів.
- Негативні fault fixtures ізолювати від clean dataset; не вимикати verify,
  щоб пропустити помилковий тестовий набір.

**Готово:** синтетичний набір проходить ready/publish; повторний export не замінює
старі bytes; stale publish/rollback та ABA відхиляються; crash/restart не робить
неповний artifact доступним. AT-06/10 — synthetic subset; це не production durability gate.

### M1-06 — Пошук і cursor

**Status: done, 2026-10-24 (synthetic service subset).** `internal/lookup/`,
`internal/storage/postgres/page.go`, golden cursor і real PostgreSQL tests.
Перевірені C seek без пропусків/повторів, Unicode masks, retained dataset після
publish, changed parameters/anchors, key rotation, long fields/byte budget
і спільний deadline з actual driver cancellation.
[Звіт](_assessments/2026-10-24-lookup/REPORT.md). HTTP parsing/rendering,
фактичний serialized body limit та acquisition deadline — M1-07; p95 — M3.

- Реалізувати exact/mask SQL, стабільний C sort/seek і загальний DB deadline.
- Додати bounded signed anchor cursor, fingerprint, key IDs, published-retained
  dataset check та dataset normalizer до locale/pair validation.
- Після publish іншого набору продовжувати стару видачу; недоступний dataset
  або normalizer → 410; altered params/token → 400.

**Готово:** кілька сторінок без пропусків/повторів, including normalized variants
і same-locale; автоматичне визначення mask за `*`/`?` та exact без них;
явний exact шукає wildcard буквально; literal `%`, `_`, backslash не розширюють
маску; timeout → 503.
Довгі записи не збільшують cursor понад контракт. AT-02/03/04/15 — synthetic coverage.

### M1-07 — Вебформа, локалізація, SEO та synthetic downloads

**Status: in progress.** Перший зріз: `internal/httpui/`, HTTP backend і keyring
у `internal/ops/`; `/`, `/search`, HTML/plain, validation precedence, escaping,
1 MiB body guard і спільний acquisition/SQL deadline. Наступний зріз додає
каталог, 31 AI-draft UI catalog, cookies та пропозицію пари без JS.
Version-pinned synthetic downloads реалізовані наступним зрізом
([контракт](DOWNLOAD_CONTRACT.md)). Synthetic dictionary pages, canonical,
discovery links і robots реалізовані наступним зрізом за [SEO_PLAN](SEO_PLAN.md).
Повний sitemap/crawl capacity, мовне приймання OQ-06 та фінальне FR-05 ще відкриті.

- Додати `/`, `/search`, `/languages`, `/downloads` та version-pinned artifact handler;
  HTML без JS і `format=plain` за контрактом. UI — лаконічний пошуковий інструмент за FR-05; конкретні макети ще не погоджено.
- Додати synthetic словникові сторінки й перевірки FR-14 / AT-19 після
  визначення URL за SEO_PLAN; сторінка показує переклади однією обраною мовою; у M2/M3 перевірити повний
  discovery/sitemap і вплив crawling на ресурси. Це обсяг першого релізу.
- Реалізувати HTTP validation precedence/status matrix, UI fallback та HTML escaping.
  Зберегти автоматичну локалізацію за мовою браузера; ручний вибір
  запам’ятовувати між відвідуваннями з вищим пріоритетом, без JavaScript. Створити каталог фраз
  нового UI; використати придатні PO-переклади, пропущені й нові доперекласти ШІ.
  Останню обрану мовну пару також зберігати між відвідуваннями й підставляти
  у форму без JS; явні `from`/`to` мають пріоритет, недоступну пару не замінювати мовчки.
  На першому візиті пропонувати мову браузера → англійську, для англомовного
  браузера — англійську → українську, лише за наявності напрямку; інакше
  показувати вибір доступних напрямків. Регіональний mapping визначено в LOOKUP_CONTRACT: точний source, інакше
  єдиний напрямок за primary languages; неоднозначність без автопропозиції.
  Сучасний i18n дозволений; резервна мова — англійська (`en`). Готовність
  локалей — OQ-06.
- Додати bounded rendering: row limit і serialized byte limit, continuation від
  останнього показаного entry, а не lookahead; без raw-text truncation.
- У M1 використовувати synthetic fixtures відповідно до scope етапу; реальний
  corpus проходить аудит і verify у M2. OQ-05 закрито: права підтверджені власником.
  HTTP не має операторських mutation routes.

**Готово:** no-JS flow працює; ui незалежний від мов словника; bad input, unavailable
pair, no matches і service failure різняться; escaping/byte budget/download hashes
перевірені через HTTP. AT-01/04/05/06/15/17 — у межах M1.

### M1-08 — Наскрізна перевірка й передача результату

**Status: done (synthetic verification), 2026-10-25.**
`tests/serviceflow/lifecycle_test.go` збирає фактичний binary, створює окрему
порожню БД та restricted LOGINs; CLI/HTTP проходять publish A/B, retained
search/article/download A, rollback, DB outage/recovery та restart. Runner
`make test-integration` включає сценарій з race detector для test harness.
README містить відтворювані команди; [звіт](_assessments/2026-10-25-service-lifecycle/REPORT.md)
і [TRACEABILITY](TRACEABILITY.md) фіксують фактичні результати й межі.
Мовне/дизайн-приймання M1-07, повний SEO discovery та M2/M3 залишаються відкритими.
Наведені нижче пункти визначають verification scope, не повне production acceptance.

- Відтворити: empty DB → migrations → synthetic import → verify/exports → publish
  → exact/mask/pages/download → publish B → continuation A → rollback.
- Перевірити interruption/retry, corrupt gzip, invalid cursor, unavailable DB,
  readiness після restart, stale CAS, artifact no-clobber й byte-limited pages.
- Прогнати build, unit, реальні PostgreSQL integration, vet/format та race checks,
  що підтримуються середовищем. Не позначати відсутній integration runner як pass.
- Оновити README фактичними командами та TRACEABILITY шляхами тестів,
  покритою частиною acceptance і результатами; не оголошувати всі AT виконаними.

**Готово:** інший агент відтворює весь сценарій за README на чистій dev/test цілі.
Підсумок містить зміни, команди й exit statuses, невиконані перевірки та залишки M2.
`tumanomir check` перевіряє структуру вимог окремо від app tests; зміна його метрики
не замінює перевірку поведінки.

## Поза M1 та відкриті рішення

M2: повний audit реального CSV-корпусу, імпорт і повна міграційна звірка.
Доступні DDL + CSV; DB snapshot adapter і відсутні historical metadata
не є обов’язковими задачами першого релізу.
M3: capacity/SLO, backup/restore rehearsal, production configuration, cutover/rollback.
Уточнення 2026-10-24: Oracle Micro зайнятий іншим проєктом. До зміни
умов доступу всі експерименти виконувати локально; на `ssh psysoft` лише
легкі read-only перевірки, без імпорту, benchmarks, інсталяцій чи змін сервісів.
Після запуску пошуку й завантажень — окремий етап реєстрації, правок,
репутації та консиліуму ШІ з остаточним рішенням власника (OQ-07).
Ці функції не входять у M1–M3 і не блокують перший реліз.
Для OQ-02 погоджено окрему нову БД та остаточні джерела DDL + CSV.
[Структурний аудит](_assessments/2026-10-24-csv-audit/REPORT.md) завершено
2026-10-24; семантична перевірка та production reconciliation ще відкриті.
[SQLite-проба](_assessments/2026-10-24-sqlite-full/REPORT.md) виконана
на всьому корпусі, але не реалізує M1 або повну production-модель.
[Прототип із ledger/origins](_assessments/2026-10-24-provenance/REPORT.md)
звірив усі записи/992 exports; 25.54 GiB, локальний CPU-limited workload
має 20 mask timeouts зі 100 queries. Optimizer statistics суттєві для планів.
Це діагностика, не виконані M1-задачі; engine/layout і resource budget відкриті.
[Подальший covering index](_assessments/2026-10-24-sqlite-indexed/REPORT.md)
усунув помилки для локального профілю: 0/300 SQL errors при 10/s під quota;
БД зросла до 29.66 GiB. [Compact keys](_assessments/2026-10-24-sqlite-compact/REPORT.md)
зменшили її до 20.85 GiB; повне логічне звіряння пройшло, 0/300 errors
під тим самим quota. Storage budget та production behavior ще відкриті.
Цих досліджень достатньо для початку незалежного M1-01; подальші варіанти
зберігання не є передумовою Go-каркаса.
Для OQ-03 погоджено локальну розробку й Oracle Cloud як середовище майбутнього
запуску; остаточний сервер і capacity gate не визначені. OQ-04 закрито: сумісність старих URL не потрібна. OQ-06 відкрите; актуальний стан — у [INDEX](INDEX.md). Агент фіксує блокер конкретної
частини та продовжує незалежну роботу. Немає потреби чекати їх для synthetic M1.

## Облік прогресу

Для кожної M1-задачі записувати status (`open`, `in progress`, `done`, `blocked`),
implementation/test paths, перевірені сценарії та нерозв'язані питання.
Commit hashes додавати лише якщо commits фактично створені.
Поточний status: **M1-01–06 done; M1-07 in progress; M1-08 done (synthetic)**.
Далі — відкриті приймання M1-07 та повний corpus/discovery у M2; це не дозвіл на deployment.
Runtime звіти можна тримати в `tmp/`;
довговічний стан задач і підтверджене покриття — тут і в TRACEABILITY.
