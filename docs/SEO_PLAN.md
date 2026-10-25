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

## Наступна робота

- Повний sitemap/index для наявного вмісту: version-pinned pre-generation
  та discovery всього корпусу. Не сканувати корпус на кожен HTTP-запит.
- Виміряти навантаження роботів разом із пошуком. Бюджет індексації не
  випливає з оцінки користувацького піку 10 запитів/с. Правила кешування,
  обмеження частоти та витрати RAM/диска ще не визначені.

Пропозиції щодо повного discovery, caching і crawler budgets ще не
реалізовано; вони не є новими продуктовими погодженнями.

## Деталі, що залишаються відкритими

Склад сторінки погоджено: переклади лише однією обраною мовою. M1 визначає
stable URL, raw identity, active/retained behavior, canonical та continuation.
Потрібні повний offline sitemap/index discovery, ресурсний budget індексу й
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
