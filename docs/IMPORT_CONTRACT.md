# Контракт імпорту та публікації V1

Контракт для FUN-IMPORT/PUBLISH. M1-04 реалізує `import inspect/run`
у `internal/importer/`; [команди](../README.md#інвентаризація-та-staged-import),
[перевірки](_assessments/2026-10-24-staged-import/REPORT.md).
Успішне staging закінчується `validating`; verify/publish — M1-05.
M1-02 реалізує format primitives у `internal/importdata/`;
точна [machine-readable schema](schemas/csv-manifest-v1.schema.json),
[synthetic/golden fixtures](../testdata/README.md).
Модель і ключі — [DATA_MODEL](DATA_MODEL.md). Джерело ніколи не змінюється.

## Маніфест та fingerprint

Оператор готує immutable snapshot і manifest JSON schema version 1: source kind,
authority/snapshot date або unknown, rights status, список artifacts, locales,
expected pairs, parser/mapping/normalization versions, admission limits,
відомі пропуски, exceptions. Кожен artifact має SHA-256 вихідних bytes, size,
relative name, format ID, очікувану пару або явне multi-pair позначення.
Невідомі поля, дубльовані JSON keys, невідомий format/version — помилка.
Paths лише відносні, без `..`, symlinks чи виходу зі snapshot root.
Однаковий artifact hash двічі в manifest — помилка, навіть під різними іменами.

Fingerprint = SHA-256 canonical manifest bytes: UTF-8 JSON без whitespace,
keys лексикографічно, без float values; artifacts відсортовані за hash,
локалі/пари/винятки — за канонічним JSON представленням. Значущі **всі** поля,
включно з іменами та exceptions. Канонічний encoder version входить у schema version;
реалізація постачає golden bytes/hash fixture. Host-specific absolute paths
і run timestamps у fingerprint не входять і зберігаються окремо.
Зміна bytes, правил, лімітів чи exceptions — новий fingerprint/dataset.

Конкретний V1 CSV schema використовує `source.kind=csv_snapshot`,
`source.authority` і `snapshot_date` (`YYYY-MM-DD` або `unknown`),
`rights_status=owner_confirmed|unknown`; усі поля schema обов'язкові.
`parser_version=lossless-csv4-v1`, `mapping_version=identity-v1`.
Артефакт: `sha256`, `bytes`, `name`, `format=csv4.gz`, `pair={from,to}`
та `multi_pair=false`, або `pair=null` і `multi_pair=true`.
`admission_limits` фіксує `field_bytes=65536`, `record_bytes=1048576`,
`artifact_bytes=17179869184`; інші значення потребують нового контракту.
Відомі пропуски — `known_gaps`, список непорожніх пояснень.
Exception має `artifact_sha256`, `ordinal`, `reason`, `decision`:
`exclude_record` адресує ordinal >=1 included artifact; `exclude_artifact`
адресує ordinal=0 artifact, який уже вилучено зі списку included artifacts.
Наявність exception у JSON **не є погодженням власника** і не обходить verify.

Canonical encoder V1 використовує Go JSON string escaping без HTML escaping:
`<`, `>` і `&` буквальні, U+2028/U+2029 — `\u2028`/`\u2029`;
інших Unicode normalization чи перетворень рядків немає. Цілі числа — decimal,
без float/exponent; keys відсортовані за UTF-8 bytes. Локалі, пари,
known gaps та exceptions сортуються за canonical JSON bytes; artifacts — за hash.
JSON без завершального LF; golden fingerprint зафіксований окремо.
Непарні UTF-16 surrogate escapes відхиляються, а не замінюються на U+FFFD.
Захисний ліміт manifest JSON — 4 MiB, вкладеність — до 32; більший inventory
потребує явного перегляду admission policy. Snapshot залишається immutable
протягом відкриття/читання; root-confined filesystem API не дозволяє вийти
за його межі, а symlink components відхиляються до відкриття.

## CSV4.gz: lossless-csv4-v1

Чотири поля без заголовка: source locale, target locale, source text, target text.
UTF-8, comma delimiter; quoted field починається `"`, literal quote — `""`.
LF або CRLF поза лапками завершують record; усередині лапок LF/CRLF/CR
зберігаються буквально. Bare CR поза лапками — fatal syntax error.
Після closing quote допустимі лише delimiter, record end або EOF.
Final record без newline допустимий. Порожній фізичний рядок — один record
з неправильним field count, не silently skipped. UTF-8 BOM дозволений лише
на початку decompressed file і знімається як file marker; у полях це звичайний символ.

**Не використовувати `encoding/csv.Reader` як lossless parser:** він нормалізує
CRLF усередині поля. Потрібен parser з цим контрактом і відповідними fixtures.
Вихідні compressed artifacts зберігаються окремо, щоб відтворити оригінальні bytes.
Export writer також мусить пройти CR/LF round-trip: декодовані поля незмінні.

Поле понад 64 KiB, порожня мова/текст, NUL, invalid UTF-8, не 4 поля,
код/пара поза manifest — rejected з reason і locator. Ліміт parser record —
1 MiB decompressed bytes, artifact — 16 GiB decompressed bytes у V1; ці limits
версійні, не доказ відповідності corpus. Якщо parser не може безпечно знайти
кінець oversized/malformed record, файл failed, повний parsed count unknown.
Gzip читається до EOF з перевіркою checksum/trailer; concatenated members
декодуються як один потік, trailing non-gzip bytes — failure. Усі input bytes
звіряються з manifest до завершення verify. Fatal file failure блокує ready.

API parser: `Next` повертає logical ordinal/byte offset і reason відхилення;
rejected fields можуть містити лише bounded preview й не використовуються
для insert/export. Оригінал лишається в immutable artifact. Fatal errors sticky;
після них читання не відновлюється. `Close` gzip reader не замінює читання до EOF.

## DB snapshot adapter — можливе розширення

Погоджені джерела — DDL + CSV. Цей adapter не потрібен для поточної міграції
й не вважається підтвердженням наявності дампу або доступу до робочої БД.

CSV4 не відновлює metadata. Окремий `legacy-db-jsonl-v1` вимагає header з source
schema hash, snapshot ID, encoding і явним table-to-locale mapping. Наступні
JSONL records мають kind translation/query/locale, source table + row locator,
типізовані nullable поля і raw metadata. Int64 передаються decimal strings;
unknown відокремлений від 0; timestamps — raw string + source type + timezone
status. JSON parser відхиляє duplicate keys. Raw export зберігається незалежно
від SQL admission. Формат не вважається готовим adapter до затвердження точної
JSON Schema та перевірки реальної БД (OQ-02); не вгадувати її з saved DDL.

## Retry та облік

`created → loading → validating → ready`; terminal validation/syntax failure →
`failed`. Ready незмінний. Один writer на fingerprint під DB lock; run має
attempt ID. Lock — session-level PostgreSQL advisory lock на виділеному connection
на весь attempt, включно з batch transactions; fingerprint-to-lock-key mapping
версійний. Реалізація V1 бере перші 8 bytes SHA-256
(`slovnyk-import-lock-v1\x00` + 32 fingerprint bytes) як signed big-endian int64;
типовий lock wait — 5 s. Колізія advisory key лише серіалізує зайву роботу, не зливає datasets.
Connection не повертається у pool до unlock; втрачений lock connection припиняє
writer. Новий attempt має bounded lock wait і не стартує, поки PostgreSQL не
звільнив попередню session. Втрата процесу/DB connection — retryable interruption: lock звільняється,
новий attempt продовжує loading/validating лише з незмінним manifest. Fatal failed
не resume-иться: потрібна нова версія manifest/правил; повтор діагностики допустимий
без запису і без переходу failed → ready.

Record ordinal — детермінований логічний номер від 1 в artifact за версійним parser,
незалежний від фізичних рядків і chunking. При tuple_hash match до insert/merge
порівняти всі чотири raw поля; різниця → явний collision failure. UNIQUE constraint
є backstop, а не доказом рівності. Batch records, origins,
issues і checkpoint комітяться однією транзакцією. Replay gzip від початку
перевіряє prefix bytes/identity та пропускає вже committed ordinals; не додає origins
повторно. `accepted_unique` належить першому входженню за artifact hash/ordinal;
порядок обробки детермінований. Metadata конфліктних origins не усереднюються.

Для повністю розібраного artifact:
`parsed = accepted_unique + duplicate_occurrences + rejected`.
Для failed artifact totals — лише підтверджений prefix, completion=false.
Погоджено 2026-10-23: запуск без записів, які неможливо коректно
імпортувати, допустимий лише після погодження **власником** конкретного
звіту про втрати. Загальна згода на такий сценарій не схвалює ще невідомі
втрати й не дозволяє ігнорувати fatal file failure. Погодження пов’язується
з відповідним набором і переліком exceptions.

Exceptions мають перелічувати конкретні hashes/locators/reasons і рішення;
загальне «ігнорувати помилки» недопустиме. Виключення цілого artifact потребує
нового manifest зі збереженим описом excluded source та звітом втрати.

## Verify та active pointer

Verify звіряє всі artifacts, tuple bytes, counts, origins, FK, per-pair sorted
length-prefixed tuple digests, metadata та контрольні запити. Експорти і report
готуються до ready: complete temp file → flush/fsync file → SHA-256 → atomic
no-clobber installation → fsync parent directories на одному filesystem.
Final artifact reference — `datasets/<dataset-id>/exports/<export-id>/<sha256>.csv.gz`;
report також має immutable versioned reference. Ніколи не overwrite retained artifact,
навіть через atomic rename. Якщо final path існує, перевірити bytes/hash і використати
лише тотожний файл; інакше failure. Спосіб no-clobber installation залежить від OS
і мусить бути перевірений інтеграційним crash test. Підтримуються filesystem з
локальними atomic installation та fsync guarantees; інші storage adapters потребують
окремого durability contract. До DB ready/active commit artifacts вже durable.
На startup/recovery readiness=503 до перевірки references і hashes активного набору.
HTTP mount — readonly; CLI не змінює final bytes; GC у V1 вимкнений.
Verify report прив'язаний до dataset fingerprint і hashes усіх exports.
Неперевірений/незавершений файл не може бути посиланням ready dataset.

Операторські команди publish/rollback обов'язково мають `--expected-generation N`.
Транзакція блокує singleton active row, порівнює generation, перевіряє ready/report/
artifact availability, перемикає ID та збільшує generation на 1. Stale N → conflict,
без зміни pointer; це захищає і від ABA-перемикань. Rollback — така сама CAS-операція
до retained ready version, не редагування даних. Перший publish очікує generation=0. Артефакти перевіряються під publication
lock перед pointer commit; immutability/no-GC забороняють їх заміну між verify і commit.
Cursor readers бачать immutable old або new dataset. GC/retention не виконується
автоматично у V1; будь-яке видалення потребує окремої процедури та backup.

## Обов'язкові fixtures до реалізації

Quoted CRLF/LF/CR, final record без LF, blank record, BOM, doubled quotes,
invalid UTF-8/NUL, oversized field, wrong columns, truncated gzip/trailer,
duplicate artifact hashes, repeated tuple з різним metadata, hash collision injection,
retry після кожного batch, retry validating, stale publish/rollback та ABA.
Для DB adapter: NULL/0/negative metadata, timestamp без timezone, schema mismatch.
Це додаткові критерії AT-06–11/18, не твердження про пройдені тести.
