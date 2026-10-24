# M1-02: Unicode, CSV4.gz та manifest — 2026-10-24

## Реалізовано

- `internal/normalization/`: NFC → full case fold → NFC, source без trim,
  query-only Unicode White_Space trim, input/control limits, auto/exact/mask,
  LIKE parameter compiler з literal `%`, `_`, backslash escaping.
- `golang.org/x/text v0.43.0` закріплено в `go.mod/go.sum`; Go 1.26 використовує
  Unicode 15.0.0. Version ID включає versions таблиць/модуля і trim set.
  Довгі non-starter sequences нормалізуються без stream-safe вставлення CGJ;
  незалежні golden приклади перевіряють canonical ordering і composition.
- `internal/importdata/`: потокові bounded CSV4/gzip reader/writer, literal
  CR/LF/CRLF, BOM тільки на початку, logical ordinals/offsets, admission reasons,
  sticky fatal syntax/record/artifact errors, gzip EOF/checksum/trailing validation.
- Strict manifest V1: required/unknown/duplicate JSON keys, UTF-8/surrogates,
  integer-only numbers, versions, inventory/pair/path/hash/limit validation;
  root-confined відкриття regular artifacts із перевіркою symlink components.
- [JSON Schema](../../schemas/csv-manifest-v1.schema.json), canonical JSON
  fingerprint, uint64-BE length-prefixed tuple digest, key digest;
  [synthetic fixtures](../../../testdata/README.md) і незалежні Python goldens.

## Фактичні перевірки

| Команда / сценарій | Результат |
|---|---|
| `make build test check test-race test-smoke` | exit 0; build, чотири Go packages, vet/gofmt, repository checks, race, реальні SIGINT/SIGTERM smoke |
| `go test ./internal/importdata -run '^$' -fuzz '^FuzzCSV$' -fuzztime=15s -parallel=2` | exit 0; 410 652 executions, жодного failing input |
| `go test ./internal/importdata -run '^$' -fuzz '^FuzzManifest$' -fuzztime=10s -parallel=2` | exit 0; 1 010 executions, жодного failing input |
| Python `jsonschema.Draft202012Validator.check_schema` + fixture validation | pass; Draft 2020-12 і date format checker |
| Synthetic gzip/hash reconciliation | 11 occurrences = 10 unique + 1 duplicate; усі raw fields і compressed SHA-256 збігаються |
| Rejection/error fixtures | pass: blank/columns/empty/NUL/UTF-8/locale/field limits, malformed quotes/CR, truncated/checksum/trailing gzip, duplicate manifests/hashes, unsafe paths/symlinks |

Fuzz запускався обмежений час й не є доказом відсутності помилок.
16 GiB limit перевірено boundary state тестом, а не читанням 16 GiB fixture.
Runtime logs/check results — `tmp/m1-02/`; ці файли не потрібні для запуску tests.

## Межі результату

Немає operator import CLI, SQL lookup, migrations, ledger/retry/verify/publish,
cursor або UI. Parser callers повинні читати gzip до EOF і передавати
manifest locale/pair admission sets; rejected previews не є raw source для insert.
Snapshot повинен лишатися immutable протягом усієї роботи з artifacts.
Exceptions у manifest не є owner approval і не дозволяють публікувати втрати.
Повні AT-02/03/07/18 ще не виконані; M1-02 закриває лише format primitives.

Робота виконана локально. Oracle, старий проєкт і реальний корпус не змінювалися;
браузери не запускалися, Playwright не встановлювався. `AGENTS.md` не змінювався.
Наступне завдання — M1-03, нова PostgreSQL schema і реальні constraint tests.
