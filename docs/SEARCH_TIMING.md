# Діагностичні вимірювання пошуку

## Увімкнення

`SLOVNYK_SEARCH_TIMING=true` додає JSON-подію `search_timing` у stderr
після завершення кожного GET/HEAD handler для `/` або `/search`.
Default — `false`; приймаються лише `true`, `false` або порожнє значення.
Потрібен рівень логування `info` або `debug`. Перезапустіть процес для зміни.
Це тимчасова діагностика тривалостей, не статистика популярності запитів.
Health/readiness, word, sitemap і download handlers не вимірюються.
Відхилення до wrapper (наприклад, зовнішній ліміт HTTP target) не створюють події.

## Поля й межі вимірювань

Усі тривалості — монотонний elapsed time у мілісекундах, включно з очікуванням CPU.

| Поле | Значення |
|---|---|
| `handler_ms` | Handler, rendering і виклики запису body; без читання HTTP headers, мережевої доставки та запису події в log sink |
| `search_ms` | Search backend разом з admission, connection, lookup і cleanup |
| `admission_ms` | Очікування одного з восьми спільних backend slots |
| `connect_ms` | Встановлення нового PostgreSQL connection |
| `lookup_ms` | Увесь `lookup.Service.Search`, включно з pin/catalog/anchor/query; не чистий час SQL executor |
| `cleanup_ms` | Закриття search connection і повернення slot |
| `catalog_ms` | Окремий UI catalog backend, разом з його admission/connection/SQL/cleanup |
| `status` | HTTP status, переданий writer, або implicit 200 |
| `query_kind` | `exact`, `mask` або `not_run`; prefix також `mask` |
| `search_result`, `catalog_result` | `ok`, `not_run`, `invalid`, `pair_unavailable`, `dataset_unavailable`, `unavailable`, `deadline`, `canceled` |
| `failure_stage` | Останній search stage при помилці: `admission`, `connect`, `lookup`; `none` для success/not-run |
| `write_failed` | Writer повернув error або short write; не доказ отримання body клієнтом |

Також присутні фіксовані `event`, `msg`, `level` та час `time`.
Контекстні labels описують спостережений стан після операції, не SQLSTATE
і не доведену причину попередніх 503. Catalog failure може дати notice при HTTP 200.
Нуль означає невиконаний етап або дуже коротку операцію; `not_run` відрізняє
відсутність backend виклику. Handler panic не створює completion event.
`search_ms` і `catalog_ms` уже входять у `handler_ms`: не підсумовуйте всі колонки.

## Приватність і використання

Подія не містить query, URL, мовної пари, cookies, IP, User-Agent, cursor,
DSN або тексту backend error. Немає request identifier: порівнюйте агрегати
окремих неперекривних фаз, не зіставляйте concurrent requests за порядком рядків.
Client queue та повний HTTP round trip вимірюються окремо.

Deadline, SQL, slot count, відповіді й default logging поведінка не змінені.
Запис INFO синхронний: log sink може затримувати goroutine після body write;
його затримка не входить у `handler_ms`. Для тривалого ввімкнення потрібні
окремі рішення про обсяг, rotation і retention журналу.
