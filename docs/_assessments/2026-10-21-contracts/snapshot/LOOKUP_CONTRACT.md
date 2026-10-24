# Контракт пошуку V1

Конкретизація FUN-LOOKUP/UI; API нижче — цільовий, ще не реалізований.
Основний результат — server-rendered HTML без JavaScript.

## HTTP та помилки

`GET /search?from=uk-ua&to=en-us&q=слово&mode=exact&limit=50&ui=uk`

| Вхід / подія | Результат |
|---|---|
| Немає `q` або q після trim порожній | 200 форма без SQL lookup; cursor з порожнім q → 400 |
| Немає from/to при непорожньому q | 400 з формою вибору; new endpoint не вгадує пару |
| Немає mode / limit | exact / 50; explicit empty або невідоме значення → 400 |
| limit поза 1..100, не decimal integer | 400 |
| Duplicate відомого параметра, malformed percent/UTF-8, U+0000 | 400; parsing тільки один раз |
| Невідома locale | 400; відома, але відсутня пара → 404 з поясненням |
| Немає active dataset / DB unavailable / DB timeout | 503; це не порожній результат |
| Немає збігів | 200 з явним empty result |
| Cursor invalid/signature mismatch/parameters changed | 400 |
| Cursor dataset або його normalizer недоступний | 410; запропонувати почати заново |
| Raw request-target понад 32 KiB | 414 до parsing |
| Decoded q понад 8 KiB або 1024 code points | 400 до lookup |

Empty from/to і cursor — invalid, якщо передані. Unknown query keys ігноруються
після перевірки encoding і загального розміру. ASCII controls U+0001..001F/007F
у q заборонені, **крім TAB/LF/CR**, які можуть бути частиною raw field.
Відомі текстові параметри перевіряються як UTF-8; жодного другого URL decode.
UI: explicit supported ui → Accept-Language supported match → configured default;
unsupported/empty ui переходить до fallback, відображається фактично обрана мова.
`format` відсутній/html → HTML, plain → text/plain, інший/empty → 400;
format не входить у cursor fingerprint. UI не змінює словникову пару. locale aliases для нового endpoint не застосовуються.

## Нормалізація та маски

Обрізати крайні Unicode White_Space **тільки запиту**, потім NFC → full case fold
→ NFC. Normalization version фіксує реалізацію, Unicode tables і trim set.
Source key не обрізається. Тому raw ` слово ` exact-формою не знаходиться;
mask `*слово*` знаходить його. Оригінал доступний в експорті. Це свідомий контракт
сумісний з FR-02, а не прихована чистка даних.

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
перевіряти locales/pair та нормалізувати q. Новий active не впливає на продовження.
Dataset з token має бути раніше опублікований і retained; довільний staging ID
не дає доступу до даних. Publication history зберігає факт publish.

Порядок C/bytewise: `(source_key, source_text, target_text, id)`; seek predicate
використовує ту саму collation і strict greater-than. Source grouping лише
всередині сторінки; велика група продовжується на наступній сторінці.

Cursor — base64url canonical JSON payload + HMAC-SHA256, максимум 2048 bytes:
`version, kid, dataset_id, fingerprint, anchor_translation_id, limit`.
Fingerprint — SHA-256 length-prefixed UTF-8 values `normalization_version, from,
to, mode, normalized_q, decimal_limit`. UI не входить. Token не містить raw texts
чи повного sort tuple. Сервер отримує boundary tuple з immutable translation
за dataset+anchor ID та перевіряє, що anchor належить парі й відповідає фільтру.
HMAC constant-time check; unknown kid → 400. Old signing keys зберігаються на
весь строк підтримки cursors; заміна ключа без retention робить старі tokens invalid.
Формат token і serialization мають golden fixtures до реалізації.

Page обмежена limit **і 1 MiB serialized response body**. Формувати bounded buffer
до headers, включати лише цілі entries; reserve для templates/continuation/error
UI — 128 KiB, а один entry з двома полями до 64 KiB має вміститися з HTML escaping.
Renderer зобов'язаний перевіряти цю верхню межу; oversized шаблон/entry — явна 503,
не обрізання тексту. Коли byte limit зупинив видачу, cursor вказує на останній
включений entry, а не на прочитаний наперед. Наявність наступного entry визначає
has_more. Page може бути коротшою за limit. Загальний DB deadline — 2 s від початку
lookup, не 2 s на кожен SQL; cancellation доходить до драйвера.

## Індекси й перевірки

Exact: bounded key_hash index + residual equality source_key; normalized collisions
не зливають raw variants. Mask може сканувати pair під deadline; швидкість infix
не обіцяється без вимірювання. План та memory/sort budget перевірити у rehearsal.

AT-02/03/04/05/15/16 доповнюються fixtures: fold expansions, composed/decomposed,
ZWJ, literal SQL metacharacters, edge spaces, long fields, byte-limited pages,
forged/oversized cursor, changed params/limit, key rotation, new active зі зміненим
каталогом/normalizer, retained old dataset, відсутній old dataset → 410.
