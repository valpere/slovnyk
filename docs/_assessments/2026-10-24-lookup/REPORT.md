# M1-06: пошук і signed cursor

Дата: 2026-10-24. Baseline: `05fdc91c92931bdfb23ce271c47b80bbf5edb30e`.
Scope: application service і SQL на синтетичних даних; HTTP adapter — M1-07.

## Реалізація

- `internal/lookup/`: typed search, active pin один раз, published/retained dataset
  і version gate, locale/pair checks, canonical HMAC-SHA256 cursor та key rotation.
- `internal/storage/postgres/page.go`: parameterized exact hash + residual equality,
  mask LIKE escaping, C sort/seek, перевірка anchor через той самий pair/filter.
- Cursor містить лише bounded IDs/fingerprint/limit/version/kid. Незалежний Python
  golden фіксує serialization, length-prefixed fingerprint і signature.
- Цілі entries обмежені row limit та conservative escaped-byte budget; cursor
  походить від останнього включеного entry. Raw написання групуються без злиття.
- Спільний двосекундний context охоплює pin/catalog/anchor/search SQL і драйвер.
  Validation відділено від query normalization для version resolution до Compile.

## Рев’ю

Послідовне self-review за `code-review`: contracts, parameterization/privileges,
publication/version isolation, cursor tampering/canonicalization/key rotation,
seek/lookahead/byte boundaries, cancellation та documentation claims.
Це не незалежне й не multi-agent рев’ю. Точний склад і SHA-256 змінених inputs
фіксує `reviewed-inputs.json`; звіт та сам inventory виключені з його hashes.
Підтверджених невиправлених дефектів у цьому scope не виявлено.

## Перевірені сценарії

`internal/lookup/lookup_integration_test.go` запускається послідовно з іншими
suites на власній disposable PostgreSQL 17.11 під HTTP read-only role:

- `TestSearchModesAndStablePages`: C ordering, normalized variants без втрат
  і повторів, raw grouping, Unicode codepoint masks, literal SQL metacharacters,
  explicit exact wildcard, same-locale та відмінні 400/404.
- `TestCursorPinAuthorizationAndChanges`: publish нового набору зі зміненим
  каталогом, continuation старого, altered params/limit, absent/unpublished
  dataset, wrong pair/filter anchor, unavailable normalization version.
- `TestByteBudgetAndLongAnchor`: 64 KiB raw fields, escaped bytes скорочують
  сторінки без truncation; довгий normalized key не потрапляє в bounded cursor.
- `TestWholeLookupDeadlineAndRecovery`: 1 s затримки початкового pin і реальний
  PostgreSQL lock; наступний SQL не отримує нові 2 s, cancellation → 503,
  після звільнення lock і нового connection пошук відновлюється.

Звичайні datasets проходять справжні import → verify → publish. Окремий
SQL fixture майбутнього normalizer моделює metadata publication: він проходить
чинні DB transitions/CAS guards, але не має verified production artifacts;
використовується лише для відхилення непідтримуваної версії до search/catalog.
Unit tests перевіряють golden token, tampering/noncanonical/oversized tokens,
rotation/copy keyring, fingerprint, validation до SQL і mapping помилок.

## Межі

HTTP parsing precedence, routes, localization, XSS-safe templates, plain output,
фактичний whole-response 1 MiB bound і acquisition deadline — ще M1-07.
Service budget резервує 896 KiB entries і 128 KiB UI; renderer повинен перевірити
остаточні bytes до headers. Ключі зараз передаються Go API; HTTP provisioning
ще не підключено. Підтримується одна compiled normalization version.
Повний corpus, mask query plans/capacity, p95 і production/Oracle не перевірялися.
Legacy, експериментальні дані та AGENTS.md не змінені; браузер не запускався.

## Фактичні результати

Усі команди завершилися exit 0:

| Команда | Результат |
|---|---|
| `make check` | PASS: gofmt, vet, patch whitespace, repository links/ignore checks |
| `make test` | PASS: усі звичайні Go tests |
| `make test-race` | PASS: звичайні suites під race detector; integration suite окремо без race |
| `make test-smoke` | PASS: build, health/readiness і process shutdown regression |
| `make test-integration` | PASS: усі sequential suites на власній PostgreSQL 17.11, включно з чотирма lookup scenarios; ресурси прибрані |
| `go test ./internal/lookup -run '^$' -fuzz '^FuzzCursor$' -fuzztime=10s -parallel=2` | PASS: 69 183 executions, valid signed seed та malformed/oversized seeds |

Runtime logs: `tmp/m1-06/checks-final.log`, `checks.log` (build/smoke),
`integration-final.log`, `fuzz-final.log`; ignored і не є переносними artifacts.
Остання зміна після integration — valid fuzz seed і виправлення старого comment
про межу service/HTTP; unit/race/check та fuzz повторно пройшли.
M1-01–06 done; наступний етап — M1-07.
