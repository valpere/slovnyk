# Індексація словникових сторінок

## Підтверджений обсяг

2026-10-23 користувач підтвердив: сторінки окремих слів із перекладами
мають бути доступні для індексації Google та іншими пошуковиками вже
в першому релізі. Це вимога технічної придатності до індексації, а не
гарантія потрапляння всіх сторінок у пошук або позицій у видачі.
Сторінка показує вихідне слово/фразу та переклади **однією обраною мовою**.
Одна адресована словникова сторінка відповідає конкретному вихідному
написанню й напрямленій мовній парі. Явна пара в URL має пріоритет над
збереженими налаштуваннями; мова UI не змінює словникового напрямку.
FR-14 / AT-19 визначають перевірки. Synthetic pages реалізовано в M1;
це часткове покриття AT-19, без повного sitemap/crawl acceptance.
Підтверджено 2026-10-23: основний домен — `slovnyk.org`, додатковий —
`slovnyk.org.ua`. Canonical і sitemap мають використовувати основний
домен. Погоджено автоматичне перенаправлення з `slovnyk.org.ua` на
`slovnyk.org` зі збереженням шляху й параметрів нових URL. Це не
вимагає відновлення старих пошукових/download-URL: OQ-04 залишається чинним.
Перевірка перед запуском: перенаправлення веде на фіксований основний
домен, не утворює циклів, зберігає шлях і query; canonical та sitemap
посилаються на основний домен. HTTP alias використовує 308 на fixed origin
`https://slovnyk.org`, зберігає escaped path/query; forwarded headers не
визначають origin. TLS/certificates та edge configuration — deployment scope.

## Реалізований M1 контракт

`GET /word/<entry-id>` (також HEAD) повертає HTML без JS, cookies або
Accept-Language. Entry ID — 64 lowercase hex SHA-256 raw мовної пари й
вихідного написання за [DATA_MODEL](DATA_MODEL.md), не normalized key.
Ідентифікатор адресує рівно одне raw-написання в одній напрямленій парі;
написання з іншим case, NFC або крайніми пробілами мають окремі сторінки.
Fixed-size URL підтримує поля до 64 KiB, без raw text у URL або surrogate
dataset/translation IDs. Query не може змінювати пару, raw text або UI.

Bare URL pin-ить active dataset один раз, читає лише published ready data.
Наступна публікація може змінити переклади за тією самою адресою; видалене
написання → 404 без підстановки близького normalized варіанта. Сервер не
генерує індексованих історичних сторінок для кожної версії dataset.

Порядок перекладів — translation ID у pinned dataset, seek continuation:
до 50 цілих записів, conservative entry budget і final body guard 1 MiB.
`?cursor=<signed-token>` pin-ить retained published dataset та fingerprint
entry ID/пари/розміру сторінки; cursor іншої статті/пари не приймається.
Продовження не індексується (`noindex, follow`), canonical веде на bare URL.
Metadata admission/connect/SQL ділять 8 slots і загальний deadline 2 s із
search/catalog/downloads; production route вимагає readiness.

Стаття має один UI, визначений identity: підтримуваний lowercase primary
source code, інакше `en`. Raw locale не переписується. Cookies/браузер не
змінюють HTML і не записуються; toolbar дозволяє перейти на головну з
ручним вибором UI. Саме для перекладу меню окремі indexable URL/hreflang
не створюються. Мовні теги source/translation content визначаються через
BCP-47 parser; legacy-код без валідного tag отримує порожній `lang` (невідома мова).

Canonical завжди `https://slovnyk.org/word/<entry-id>`, незалежно від Host
і forwarded headers. Title/description містять короткі metadata excerpts;
видимі raw fields не обрізаються. `/search` має звичайні HTML links із
кожної raw-групи на статтю, але сам лишається `noindex, follow`.

Невідомий/невалідний ID → 404; будь-які query-параметри, окрім одного
непорожнього bounded cursor, → 400; unavailable DB/readiness/render → 503;
втрачений/unpublished cursor dataset → 410. Errors — короткий plain English,
noindex/no-store. Усі dictionary responses — no-store, без HTTP cache
версії active pointer. `robots.txt` дозволяє crawling; noindex контролює
індексацію search/continuation/error сторінок. Legacy URL не відновлюються.

Migration 000002 додає fixed-size expression index та SQL/Go identity parity,
не змінює готові raw data. До запуску HTTP оператор має виконати `migrate`;
стару фізичну schema сервер відхиляє. Додатковий disk/import cost та час
побудови індексу на повному корпусі потребують M2 rehearsal.

## Офлайн-генерація sitemap

`slovnyk dataset sitemap <dataset-id> --artifacts <existing-root>` використовує
`SLOVNYK_OPERATOR_DSN` і фізичну schema 3. Лише immutable `ready` dataset,
включно з ще не published: команда не змінює state, active pointer або generation.
Невідомий/не ready dataset відхиляється. Порожній ready dataset повертає
`sitemap_empty_dataset`, без порожнього XML index.

SQL читає DISTINCT raw entry hashes у byte order; Go потоково пише унікальні
`https://slovnyk.org/word/<entry-id>`, без cursor/dataset/UI query. Переклади
одного raw source/пари дають одну адресу; case/NFC/пробіли та інша пара — окремі.
UTF-8 XML містить стандартний namespace; timestamps/lastmod не вигадуються.
До 50 000 URL на shard, до 50 000 shards на index, кожен XML не більше
52 428 800 bytes. Фіксована довжина адрес утримує shard нижче 6 MiB.
Ці межі та root scope відповідають [Sitemaps protocol](https://www.sitemaps.org/protocol.html).
Index посилається на root-level адреси
`https://slovnyk.org/sitemap-<dataset-id>-<shard-sha256>.xml`: вкладений sitemap
за базовим протоколом не може охоплювати сусідній `/word/`.

Локальні файли: `datasets/<dataset-id>/sitemaps/<sha256>.xml` у artifact root.
Після всіх shards/index останнім встановлюється `<sha256>.json` — manifest v1
із dataset, URL count, counts та path/bytes/SHA256 кожного shard і index.
CLI JSON повертає ці поля й `report` ref. Exit 0 — повний inventory, 1 — збій,
2 — аргументи/config; помилки не містять DSN, source text або SQL details.
Файли встановлюються через shared no-clobber/fsync store; незмінний повторний
запуск дає ті самі bytes/hashes та повторно перевіряє наявні файли. Після збою
можливі unreferenced fragments; повторіть команду. Після durable files команда транзакційно реєструє inventory у БД; auto-delete немає.

Go тримає поточний hash, write buffer та bounded inventory shards, без списку
всіх URL. DISTINCT/ORDER BY можуть вимагати PostgreSQL sort/spill; повний корпус,
DB RAM/temp disk, час та загальний disk cost ще не виміряно. Команда підтримує
скасування через SIGINT/SIGTERM; фіксованого total execution deadline немає.

## HTTP sitemap discovery

Schema 3 [migration](../migrations/000003_sitemaps.sql) додає immutable
`sitemap_inventory` (dataset, report/index refs, total counts) і `sitemap_shards`
(dataset/hash, bytes, URLs). Шляхи виводяться лише з перевірених UUID/hash.
Команда реєструє їх через вузьку SECURITY DEFINER SQL-функцію після durable
files; row lock серіалізує retry, deferred constraint звіряє shard/URL totals.
Operator має SELECT/EXECUTE, HTTP — лише SELECT; direct INSERT/UPDATE/DELETE
не дозволені. Повторний report SHA збігається або повертається failure;
реєстрація не змінює ready data чи publication generation. На SQL failure
файли/report можуть лишитися, але незареєстрований inventory не видається.

`GET /sitemap.xml` (також HEAD) одним SQL snapshot вибирає index active
published ready dataset. `Cache-Control: no-store`; conditional ETag перевіряється
після нового resolve, тому publish/rollback змінює index без filesystem pointer.
`GET /sitemap-<dataset-id>-<sha256>.xml` видає лише зареєстрований shard published
ready dataset, включно з retained версіями: `public, max-age=31536000, immutable`.
Обидва маршрути: XML UTF-8, strong SHA ETag, HEAD/Range/conditional requests,
без cookies, Accept-Language або forwarded-origin; будь-який query → 400.
Некоректний/unknown/unpublished shard → 404, відсутній active inventory,
DB/readiness/file failure → 503. Помилки короткі English, no-store/noindex.
`robots.txt` рекламує fixed `https://slovnyk.org/sitemap.xml`; без БД лишається 200.

Metadata ділить чинні 8 DB slots і deadline 2 s, після resolve connection
звільняється. Handler має до 8 відкритих sitemap transfers і rolling write
deadline 30 s; HTTP не сканує translations/файловий корпус, не перебирає весь
inventory у Go та не генерує XML. File open перевіряє confinement, regular-file та exact byte size.
Startup/recovery/нова generation додатково звіряє SHA всіх зареєстрованих active
sitemap files; HTTP filesystem має бути immutable/read-only, як exports.
Same-size mutation після цих перевірок не виявляється на кожен HTTP-запит.

Inventory поки optional для сумісності з попередніми ready datasets:
publication не вимагає його, readiness допускає відсутній реєстр, а `/sitemap.xml`
тоді повертає 503. Якщо inventory є, missing/corrupt файли блокують readiness.
Операторський порядок перед release: verify → sitemap → publish; готові файли
без реєстрації зі старого binary треба повторно згенерувати новою командою.
Реєстрація після publication не змінює generation: файли перевірені генератором,
наступна startup/recovery перевірка звірить їх повторно. Auto-GC/retirement немає;
retained DB inventory та files зберігаються для stable shard URLs/rollback.
Повний release acceptance має перевірити inventory кожної публікованої версії.

## Наступна робота

- Full-corpus rehearsal sitemap generation/discovery та release checks готовності
  inventory; capacity/readiness hash cost на повному corpus ще не виміряно.
- Виміряти навантаження роботів разом із пошуком. Бюджет індексації не
  випливає з оцінки користувацького піку 10 запитів/с. Правила кешування,
  обмеження частоти та витрати RAM/диска ще не визначені.

Crawler budgets та release-політика обов’язкового inventory ще відкриті;
це не нові продуктові погодження.

## Деталі, що залишаються відкритими

Склад сторінки погоджено: переклади лише однією обраною мовою. M1 визначає
stable URL, raw identity, active/retained behavior, canonical та continuation.
Потрібні full-corpus rehearsal, ресурсний budget індексу й
crawling, фінальні мовне/дизайн/accessibility приймання та deployment redirects.
Поточний `/search` залишається пошуком в одній напрямленій парі; його
поведінка цим документом не змінюється.

## Джерела

Googlebot зазвичай не надсилає `Accept-Language`; покладатися лише на
автолокалізацію недостатньо для виявлення всіх мовних варіантів.
[Google: locale-adaptive pages](https://developers.google.com/search/docs/specialty/international/locale-adaptive-pages).
[Google: multilingual sites](https://developers.google.com/search/docs/specialty/international/managing-multi-regional-sites).
Sitemap допомагає виявляти URL, але не гарантує індексації.
[Google: recrawling](https://developers.google.com/search/docs/crawling-indexing/ask-google-to-recrawl).
