# Контракт пошуку V1

Конкретизація FUN-LOOKUP/UI. M1-06 реалізує application search/cursor service;
HTTP parsing і bounded HTML/plain rendering реалізовані в першому зрізі M1-07;
локалізація, каталог/налаштування форми залишаються відкритими.
Основний результат — server-rendered HTML без JavaScript.

## HTTP та помилки

`GET /search?from=uk-ua&to=en-us&q=слово&limit=50&ui=uk`

| Вхід / подія | Результат |
|---|---|
| Немає `q` або q після trim порожній | 200 форма без SQL lookup; cursor з порожнім q → 400 |
| Немає from/to при непорожньому q | 400 з формою вибору; new endpoint не вгадує пару |
| Немає mode / limit | auto / 50; explicit empty або невідоме значення → 400 |
| limit поза 1..100, не decimal integer | 400 |
| Duplicate відомого параметра, malformed percent/UTF-8, U+0000 | 400; parsing тільки один раз |
| Невідома locale | 400; відома, але відсутня пара → 404 з поясненням |
| Немає active dataset / DB unavailable / DB timeout | 503; це не порожній результат |
| Немає збігів | 200 з повідомленням і локалізованою підказкою використати маску `*` / `?`; без автоматичних пропозицій схожих слів або розширення запиту |
| Cursor invalid/signature mismatch/parameters changed | 400 |
| Cursor dataset або його normalizer недоступний | 410; запропонувати почати заново |
| Raw request-target понад 32 KiB | 414 до parsing |
| Decoded q понад 8 KiB або 1024 code points | 400 до lookup |

Пріоритет перевірок: transport/encoding/size → структура та явно передані mode,
limit, format, cursor/empty from/to → порожня форма → dataset/catalog/search.
Тому `q=&mode=invalid` → 400; unknown locale перевіряється лише для непорожнього
пошуку після dataset pin. UI fallback не перетворює unsupported ui на помилку.

Empty from/to і cursor — invalid, якщо передані. Unknown query keys ігноруються
після перевірки encoding і загального розміру. ASCII controls U+0001..001F/007F
у q заборонені, **крім TAB/LF/CR**, які можуть бути частиною raw field.
Відомі текстові параметри перевіряються як UTF-8; жодного другого URL decode.
UI: explicit supported ui → persisted supported manual choice →
Accept-Language supported match → English (`en`);
ручний вибір зберігається між відвідуваннями без JavaScript. Невідомий або
більше не підтримуваний збережений код пропускається;
unsupported/empty ui переходить до fallback, відображається фактично обрана мова.
`format` відсутній/html → HTML, plain → text/plain, інший/empty → 400;
format не входить у cursor fingerprint. UI не змінює словникову пару. locale aliases для нового endpoint не застосовуються.

Остання обрана мовна пара зберігається між відвідуваннями й використовується
для заповнення форми без JavaScript. Явні `from`/`to` мають пріоритет;
недоступна збережена пара потребує нового вибору без мовчазної заміни.
На першому візиті без збереженої пари форма пропонує мову браузера →
англійську; для англомовного браузера — англійську → українську. Пропозицію
можна змінити; вона доступна лише за наявності напрямку в активному dataset.
Інакше форма пропонує обрати доступний напрямок. Mapping регіональних
кодів у цій пропозиції ще потребує визначення й не змінює raw locale codes. Форма передає пару явно: `/search` не підставляє відсутні `from`/`to`
зі збережених налаштувань і не змінює пару з cursor.

## Нормалізація та маски

`mode` підтримує `auto`, `exact`, `mask`; відсутній параметр означає `auto`.
В `auto` наявність `*` або `?` у декодованому запиті означає mask, інакше exact.
Явний `exact`/`mask` має пріоритет. Основна форма використовує автоматичний
вибір без обов’язкового перемикача. `mode=exact` зберігає можливість шукати
буквальні `*`/`?`; це не скасовується автоматичним визначенням.

Обрізати крайні Unicode White_Space **тільки запиту**, потім NFC → full case fold
→ NFC. Normalization version фіксує реалізацію, Unicode tables і trim set.
Source key не обрізається. Тому raw ` слово ` exact-формою не знаходиться;
mask `*слово*` знаходить його. Оригінал доступний в експорті. Це свідомий контракт
сумісний з FR-02, а не прихована чистка даних.

M1-02 реалізує ці primitives у `internal/normalization/`: `golang.org/x/text
v0.43.0`, NFC/fold tables Unicode 15.0.0 на Go 1.26; trim — явно заданий
White_Space 15.0. Version ID включає алгоритм, dependency/table versions і trim set.
Для довгих non-starter sequences застосовується NFC без stream-safe вставлення
CGJ; golden fixtures перевіряють canonical ordering/composition окремо.
Оновлення Go/x/text потребує перегляду version/fixtures, а не тихої заміни ключів.

Exact сприймає `*`/`?` буквально. У mask `*` — нуль або більше Unicode code points,
`?` — один code point **після** нормалізації, не grapheme. Немає escape-синтаксису
для literal wildcard у mask; для нього використовувати exact. LIKE compiler
заміняє wildcard tokens і екранує literal `%`, `_`, `\` з явним SQL ESCAPE.
Key/pattern і cursor bounds передаються лише як SQL parameters.

Приклади: `ß → ss`, тому `?` не збігається з цим ключем, `??` збігається;
`e + combining acute → é`, для нього достатньо `?`; ZWJ emoji може потребувати
кількох `?`. Діакритика, внутрішні пробіли, апострофи та punctuation значущі.

## Dataset pin, порядок і cursor

Без cursor читати active ID один раз; далі всі catalog/key/search queries
працюють тільки з ним. З cursor: спочатку перевірити bounded token і HMAC,
вибрати **dataset з cursor**, завантажити його normalizer і лише після цього
перевіряти locales/pair та нормалізувати q. Потім recompute fingerprint поточного
запиту за normalizer **pinned dataset** і constant-time compare з token; mismatch → 400
до виконання search SQL. Limit незмінний у ланцюжку cursor; новий limit починає новий
пошук без cursor. Новий active не впливає на продовження.
Dataset з token має бути раніше опублікований і retained; довільний staging ID
не дає доступу до даних. Publication history зберігає факт publish.

Порядок C/bytewise: `(source_key, source_text, target_text, id)`; seek predicate
використовує ту саму collation і strict greater-than. Переклади групуються під заголовком
оригінального `source_text`, різні raw-написання не зливаються через
спільний source key. Групування лише всередині сторінки; велика група
продовжується на наступній сторінці з повтореним заголовком. Ліміти
стосуються зв’язків і serialized bytes, а не кількості заголовків.

Cursor — base64url canonical JSON payload + HMAC-SHA256, максимум 2048 bytes:
`version, kid, dataset_id, fingerprint, anchor_translation_id, limit`.
Fingerprint — SHA-256 length-prefixed UTF-8 values `normalization_version, from,
to, effective_mode, normalized_q, decimal_limit`. `effective_mode` — визначений
`exact` або `mask`, а не `auto`; auto і явний режим з однаковим ефектом
дають той самий fingerprint. UI не входить. Token не містить raw texts
чи повного sort tuple. Сервер отримує boundary tuple з immutable translation
за dataset+anchor ID та перевіряє, що anchor належить парі й відповідає фільтру.
HMAC constant-time check; unknown kid → 400. Old signing keys зберігаються на
весь строк підтримки cursors; заміна ключа без retention робить старі tokens invalid.
V1 wire format: `base64url(payload) + "." + base64url(HMAC)`, без padding.
Canonical JSON має ключі в лексичному порядку: `anchor_translation_id`,
`dataset_id`, `fingerprint`, `kid`, `limit`, `version`; без whitespace/newline,
з escaping `encoding/json`. Length prefix fingerprint — uint64 big-endian byte
length перед кожним UTF-8 значенням. Fingerprint записаний lowercase hex.
Невідомі/повторені ключі й неканонічне кодування відхиляються. Expiry у V1 немає.
Незалежний golden fixture: `testdata/lookup/cursor-v1.json`.

Page обмежена limit **і 1 MiB serialized response body**. Формувати bounded buffer
до headers, включати лише цілі entries; reserve для templates/continuation/error
UI — 128 KiB, а один entry з двома полями до 64 KiB має вміститися з HTML escaping.
Renderer зобов'язаний перевіряти цю верхню межу; oversized шаблон/entry — явна 503
з окремим diagnostic code `render_budget_exceeded` (помилка server configuration/data),
не обрізання тексту. Коли byte limit зупинив видачу, cursor вказує на останній
включений entry, а не на прочитаний наперед. Наявність наступного entry визначає
has_more. Page може бути коротшою за limit. Загальний DB deadline — 2 s від початку
lookup, не 2 s на кожен SQL; cancellation доходить до драйвера.

Погоджений користувачем рівень сервісу: щонайменше 95% звичайних пошуків
мають server-side processing time <=1 s. Це час обробки пошуку й підготовки
відповіді на сервері, без мережевої доставки та роботи браузера. DB deadline
2 s не є гарантією повного HTTP response time <=2 s. Профіль звичайних пошуків
і навантажувальний протокол уточнюються за OQ-03; оцінка піку — до 10 запитів/с.

## Індекси й перевірки

Exact: bounded key_hash index + residual equality source_key; normalized collisions
не зливають raw variants. Mask може сканувати pair під deadline; швидкість infix
не обіцяється без вимірювання. План та memory/sort budget перевірити у rehearsal.

AT-02/03/04/05/15/16 доповнюються fixtures: fold expansions, composed/decomposed,
ZWJ, automatic mode detection, explicit exact override, literal SQL metacharacters, edge spaces, long fields, byte-limited pages,
forged/oversized cursor, changed params/limit, key rotation, new active зі зміненим
каталогом/normalizer, retained old dataset, відсутній old dataset → 410.

## Межа реалізації M1-06

`internal/lookup.Service.Search` отримує вже розібраний typed request і позичає
caller-owned connection. Порожні typed Mode/Limit означають auto/50; на HTTP
adapter M1-07 покладені parameter-presence, duplicate/transport/format checks,
відхилення explicit empty fields і `limit=0`, localization та rendering.
Service перевіряє query bounds без нормалізації до pin/version resolution.
Зараз підтримується лише скомпільована `normalization.Version`.

Для entries service рахує escaped source/target + 64 bytes markup на запис
у бюджеті 896 KiB. Це не заміна перевірки остаточного HTML/plain body; renderer
має вкластися в 1 MiB разом із UI. Двосекундний service context охоплює всі його
SQL; майбутній HTTP adapter має включати acquisition у той самий deadline.
[Фактичні перевірки](_assessments/2026-10-24-lookup/REPORT.md) — synthetic subset,
без production capacity acceptance.
