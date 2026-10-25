# Контракт завантажень V1

Реалізований synthetic зріз M1-07 для FR-06 / AT-06. Downloads не є повним
backup метаданих. Старі download-URL та загальний архів не підтримуються.

## Manifest і посилання

`GET /downloads` показує локалізований HTML без JavaScript; `format=plain` —
UTF-8 таблицю. Manifest містить active dataset ID, напрямлені мовні пари,
logical row counts, compressed byte sizes, SHA-256 compressed bytes і посилання.
Plain-формат використовує TAB між полями; коди мов завжди quoted/escaped за
Go string syntax (`strconv.Unquote`), щоб зберігати raw TAB/CR/LF та лапки.
Вибір UI/cookies працює як у LOOKUP_CONTRACT; він не змінює вміст файлів.

`GET /downloads/<dataset-id>/<export-id>` адресує окремий CSV4.gz без заголовка.
Обидва ID — canonical lowercase UUID, не файлові шляхи. Посилання не змінює
версію при новій публікації; раніше published ready datasets доступні як retained.
Ready dataset, який ще ніколи не публікували, недоступний. DB metadata визначає
файл; query-параметри не задають path, ім’я файла чи мовну пару.

Manifest один раз pin-ить active pointer і читає immutable metadata. Каталог
обмежений 256 locales / 4096 pairs; HTML/plain body — 1 MiB. Перевищення,
неповний inventory або unavailable storage дають 503 без partial manifest.

## Файли та HTTP

- `Content-Type: application/gzip`, без `Content-Encoding`; gzip — сам attachment.
- `Content-Disposition: attachment` з ім’ям `slovnyk-<dataset>-<export>.csv.gz`.
- Strong ETag — quoted SHA-256; `Cache-Control: public, max-age=31536000, immutable`.
- Стандартні HEAD, byte ranges / If-Range та ETag conditions через ServeContent;
  HEAD без body, partial GET → 206, matching If-None-Match → 304, bad range → 416.
- Binary representation не залежить від UI, cookies чи Accept-Language;
  успішне завантаження не записує preferences. `X-Robots-Tag: noindex, follow`.
- Unknown/invalid IDs, unpublished export або чужа dataset/export combination →
  404; malformed query encoding/UTF-8/NUL → 400; unavailable DB/readiness/file →
  503. До HTTP headers помилки localized, no-store, без filesystem/driver details.
  Range/conditional errors використовують стандартні HTTP messages, no-store,
  без attachment header. Жодних directory listings чи raw-path routes.

## Межі ресурсів і цілісність

Metadata admission/connect/SQL мають 2 s budget і ділять 8 DB slots із пошуком
та каталогом. Connection звільняється до відкриття/передачі файла. Окремо до
8 передач одночасно; зайняті slots → 503. Binary stream не обмежений 1 MiB
page budget. Renewed 30 s write deadline на кожен chunk дозволяє довгі передачі
з прогресом; звичайний HTTP write timeout 15 s не обриває їх за загальним часом.
Graceful shutdown лишається bounded за конфігурацією сервісу.

Production downloads потребують готового readiness monitor. Publish перевіряє
exports до commit; startup/recovery readiness звіряє hashes і CSV round-trip
активного набору. Retained artifacts незмінні та монтуються read-only для HTTP,
GC вимкнений. Запит не хешує/не генерує великий файл повторно: confined store
перевіряє trusted reference, regular file, відсутність symlink і розмір.
Цей контракт спирається на незмінність retained files. Після початку передачі
I/O failure може обірвати body; клієнт перевіряє length/SHA-256 і може повторити
з byte range. HTTP не може замінити вже передані bytes сторінкою 503.

## Перевірки й залишковий scope

HTTP тести: `internal/httpui/downloads_test.go`. Реальний SQL/HTTP/CSV/hash і
publication/retention перевірено у `internal/ops/readiness_integration_test.go`.
Process smoke включає `/downloads` GET/HEAD без storage. Точні результати —
[TRACEABILITY](TRACEABILITY.md). Synthetic coverage не замінює production corpus,
server capacity, retention/GC policy, off-host restore або SEO acceptance.
