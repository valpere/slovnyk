# Підготовка CSV snapshot manifest

## Scope і review

Base: `fede007080547df22a0a1721ea43a6d14665b69c` (main після PR #11).
M2 preparation subset FR-07/08, не повна міграція. Послідовний **self-review**,
не незалежне або multi-agent рев’ю. [Reviewed input hashes](reviewed-inputs.sha256)
фіксують utility, build/CI та активні docs; цей report й inventory виключені.

Переглянуто immutable-source precondition, filenames/locale identity,
regular-file/root boundaries, source mutation checks, duplicate hashes,
canonical versions/limits, output no-overwrite/durability, CLI privacy,
inspect interoperability та точність docs. Підтверджених невиправлених findings
немає. AGENTS.md, application runtime/schema/SQL/dependencies не змінено.

## Реалізовано

Go `utilities/prepare-csv-manifest` готує канонічний V1 manifest для пласкої
замороженої директорії `slovnyk_<from>_<to>.csv.gz`. Hashes/bytes — compressed;
pair/locales — очікування з filename, без alias/normalization. Versions/limits
спільні з application parser. Authority явно задається оператором; date/rights
за замовчуванням unknown. Known gaps задаються явно; exceptions не генеруються.

Сторонні entries, directories, symlinks, FIFO, empty files, ambiguous names,
duplicate hashes і invalid metadata відхиляються. Копіювання/заморожування input
утиліта не виконує; snapshot має залишатися immutable протягом усієї операції.
Перевіряються file identity/size/mtime навколо streaming hash; це не захист від
writer, який порушує цю передумову.

Новий private output поза source створюється O_EXCL, fsync file/directory;
existing file не змінюється. Output parent має існувати. При handled write failure
новий incomplete file прибирається; після crash output потребує перевірки.
Canonical file SHA-256 = fingerprint, без host paths/run time. Success summary
має `content_verified=false`. Error diagnostics не містять paths/raw arguments.
Tool не читає gzip contents і не підключається до БД; наступний крок — окремий
`import inspect`. `make utilities` збирає binary; CI додатково виконує цей target.
Це utility для операторів/автоматизації, не посіпака агента.

## Corpus rehearsal та неочікуваний ZIP

Перший запуск на legacy `csv/` коректно відхилив неочікуваний формат: там
**993 entries**, із них 992 gzip та один `slovnyk_en-us_pl-pl.csv.zip`.
Manifest не створено. Root cause перевірено до зміни підходу; правила utility
не послаблено. Read-only stream comparison показав: ZIP має один CSV,
8 760 795 bytes, ідентичний decompressed bytes відповідного gzip.
[Hashes і доказ](zip-comparison.json). Legacy ZIP не видалено/не змінено.

Створено окрему regular-file копію тільки 992 CSV.gz у
`data/experiments/csv-manifest-2026-10-25/snapshot/`; original bytes збережені.
Built binary підготував поруч private `manifest.json`: source authority
`slovnyk-legacy-csv`, date unknown, rights owner_confirmed за рішенням користувача,
known gap про відсутні count/status/query metadata та межі доступного snapshot.
[Summary](corpus-summary.json): fingerprint
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`,
992 artifacts, 32 locales, 992 pairs. [Звірка з аудитом](audit-comparison.json):
усі filename/SHA/bytes збігаються з 2026-10-24, загалом 535 766 648 bytes.
Повторний пакет тих самих bytes не додає записів; вибір CSV4.gz не втрачає їх.
Жодних dictionary tuples або actual snapshot/manifest у Git не додано.

## Валідація

- `go test ./utilities/prepare-csv-manifest -count=1 -v` — PASS, exit 0.
- `make build utilities check test test-race test-smoke` — PASS, exit 0.
- Після review уточнено build example для чистого checkout та явно задано
  протилежний порядок створення fixtures; `go test -race
  ./utilities/prepare-csv-manifest -count=1 -v`, `make check` і
  `git diff --check` — PASS, exit 0.
- Built binary на окремому corpus snapshot — PASS, exit 0; audit comparison — PASS.

Tests перевіряють determinism/canonical round-trip, actual synthetic inspect,
Unicode/CRLF/self-pair/opaque locales, unsafe inputs, corrupt content при inspect,
CLI errors, private output, no-overwrite та output alias source boundary.
[Mapping](../../TRACEABILITY.md), [команди](../../../utilities/README.md).
Logs у `tmp/csv-snapshot-manifest/` ignored; довговічна local copy у `data/`.
Local PostgreSQL integration не повторювався: SQL/runtime не змінено. CI outcome
фіксується окремо у PR і не передбачається цим звітом.

## Межі

Utility підтримує конкретний flat filename layout, не всі V1 manifests.
Full-corpus gzip/CSV inspect новим Go шляхом, PostgreSQL staged import/retry,
verify/export/publication, semantic/historical completeness не виконані тут.
Попередній audit лишається історичним доказом; збіг hashes не є новою перевіркою
contents поточним parser. Manifest не схвалює rejections/losses і не замінює
ready gates. M1 UI/мовне acceptance та M2/M3 лишаються відкритими.
Oracle/browser/legacy runtime/production не задіяні; legacy files read-only.
