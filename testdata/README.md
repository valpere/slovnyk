# Синтетичні fixtures

Реальних словникових даних тут немає. `import/corpus.csv` — 11 occurrences,
10 різних raw tuples, чотири напрямки, включно із `sr-rs → sr-rs`.
Є точний дубль, normalized collision, `sr-sp` без mapping, composed/decomposed
Unicode, крайні пробіли, punctuation, wildcard, emoji та quoted CR/LF/CRLF.
На початку BOM, між records CRLF, останній record без newline.
`corpus.csv.gz` — ті самі bytes у gzip із нульовим mtime; hash у manifest
стосується саме збережених compressed bytes, а не нового gzip encoder.

`import/manifest.json` відповідає
[JSON Schema](../docs/schemas/csv-manifest-v1.schema.json).
`manifest.canonical.json` не має завершального LF; `manifest.sha256` — його hash.
`tuple.hex` / `tuple.sha256` фіксують length-prefixed bytes/hash першого tuple;
`key.sha256` — SHA-256 UTF-8 ключа `strasse`. Golden bytes/digests отримано
незалежно через Python `json`, `struct`, `hashlib`, а не Go implementation.
Не перегенеровувати їх автоматично після падіння тесту: спочатку пояснити зміну.

`import/invalid/expected.json` задає reason для кожного окремого rejected/fatal
fixture. Oversize, checksum/truncation і concatenated gzip cases створюють
Go tests у пам'яті; велетенські файли у Git не потрібні.

`normalization.json` — 40 golden прикладів довгих combining sequences,
отриманих незалежно Python `unicodedata` (Unicode 14; лише стабільні старі
символи) з seed 1024. Вони перевіряють NFC/folding без вставлення CGJ.
Runtime Go tables окремо закріплено як Unicode 15.0.0.

Запуск: `make test`; обмежений fuzz — наприклад
`go test ./internal/importdata -run '^$' -fuzz '^FuzzCSV$' -fuzztime=15s -parallel=2`.
