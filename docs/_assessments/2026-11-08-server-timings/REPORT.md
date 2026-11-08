# Локальні server-side timings — 2026-11-08

## Результат

Після merge PR30 (`78fe284097a4e5f278a37a19d8f8bf1185ca9bfe`)
додано opt-in [search timings](../../SEARCH_TIMING.md) без query/URL/DSN logging.
**1010/1010 HTTP 200**, 978/978 очікуваних timing events; client errors,
write failures, SQLSTATE 57014 й OOM/kill — 0. Усі **110/110** prior body
hash/size/Content-Type/Cache-Control oracles збігаються, включно з cursors.
900 додаткових відповідей не мають незалежних body oracles: HTTP 200 не
підміняє перевірку змісту. Application state до/після тотожний.
AT-16 лишається відкритим: це коротка локальна діагностика запропонованого профілю.

| Фаза | HTTP / timing events | Handler p50 / p95, ms | Lookup p95, ms |
|---|---:|---:|---:|
| Controls before | 55 / 39 | 15.514 / 18.388 | 7.480 |
| Ordinary 10/s, 30 s | 300 / 300 | 15.477 / 18.998 | 8.473 |
| Proposed mixed 10/s, 60 s | 600 / 600 | 15.351 / 20.365 | 9.693 |
| Controls after | 55 / 39 | 16.226 / 18.709 | 7.541 |

Percentiles — nearest rank `ceil(p*N)`, усі outcomes включені.
Ordinary admission/connect/catalog p95 — 0.0012/3.689/8.034 ms;
mixed — 0.00127/3.637/8.227 ms. Mixed handler max — 121.317 ms.
Percentiles різних етапів не підсумовуються. Lookup — весь service call,
не isolated SQL executor time; handler включає catalog/render/body write.
Client dispatch-lag p95 — 1.034/1.049 ms для paced ordinary/mixed.
Serial controls мають спільний scheduled instant, тому їхня client queue
включає очікування попередніх controls; це не server admission.

## Протокол і межі

- Лише existing restored schema 4 bind із PR29; source cluster вимкнений.
  Нові DB copies/import/restore не виконувалися. Окремий fresh LOGIN успадковував
  `slovnyk_http`, з default read-only і без owner/operator прав.
- PostgreSQL 17.11 pinned image; DB 2 CPU/8 GiB, HTTP 1 CPU/512 MiB,
  8 спільних backend slots, deadline 2 s; SQL, schema, відповіді й budget незмінні.
  Static binary з `CGO_ENABLED=0 go build -trimpath`; hash у `protocol.json`.
  `source-files.json` фіксує всі ops Go files, включно з новими файлами;
  `source_diff_sha256` у protocol охоплює лише tracked diff на момент запуску.
- HTTP target loopback; `minions/http-probe`, no reuse/proxy/redirect,
  16 workers, scheduled arrivals кожні 100 ms, client timeout 5 s,
  body cap 8 MiB. Phases неперекривні; event count звірено перед наступною.
  Client/server rows не зіставляються за порядком concurrent completion.
- Pool із попередніх deterministic 32 spaced і 8 largest pairs, без latency
  filtering. Ordinary: 150 exact positive, 90 negative, 60 short prefix.
  Mixed: 300 positive, 150 negative, 120 prefix, по 10 leading/infix/negative
  infix masks. Ordinary/prefix покривають 40 пар; 30 довільних masks — лише
  перші 10 spaced pairs у cycling schedule. Largest-mask stress лишається
  історичним evidence PR30, не повтореним цим mixed run.
- Controls: 16 exact, 16 prefix, 16 word, 7 continuations до й після.
  Word handlers не інструментовані: тому 1010 HTTP відповідають 978 events.
- Fresh PostgreSQL process/shared buffers, OS cache неконтрольований;
  cache не скидали, host shared. Тести завершені перед workload.
  Це не cold-host або тривалий soak/crawler/download benchmark.
- HTTP lifetime peak ~512 MiB й 13044 memory.max events виникли до controls;
  max events не зростали під paced phases, OOM/kill — 0. Peak включає
  readiness/artifact/file-cache charging; це не Go heap або гарантований
  production minimum. Cgroup snapshots не дають повного розподілу host cache.
- State/ledger/index readiness перевірено; **повний 17-table inventory не
  повторювали**. Два повні збіги PR30 залишаються історичними доказами.
  Поточний read-only run підтверджує незмінний application state, не новий
  повний content audit. SQLSTATE 57014 рахується за приватними PG logs,
  не виводиться з timing labels. Причину історичних 503 не встановлено.

## Запропонований профіль приймання — потребує рішення власника

Search mix: **50% exact positive / 25% exact negative / 20% short prefix /
5% інших masks**, до 10 запитів/с; ordinary subset — перші три категорії.
Для майбутнього target-server acceptance пропонується 5 хвилин такого
навантаження після readiness і controls, з handler p95 ordinary <=1 s
та окремою перевіркою DB deadline 2 s за REQ-NFR-03.
Crawler budget як початкова пропозиція: 1 word page/s, максимум 2 concurrent
crawler requests і один artifact download stream; actual bandwidth/storage
budget та остаточні resource caps визначити після вибору сервера.
Цей run не містить concurrent crawler/download traffic і не приймає цей budget.

Повторні широкі локальні import/restore/workload серії без нового ризику
не потрібні. Далі — рішення про профіль і сервер, одна адресна перевірка
на обраному сервері та owner UI/translation acceptance. Зайнятий Oracle
не використовувався; off-host backup/RPO/escrow/retention окремо відкриті.

## Перевірки й review

`make check test test-race test-smoke` і `make test-integration` — PASS.
Пізніше доданий admission test окремо пройшов `go test -race ./internal/ops`.
`TestTimingPreservesHTTPAndExcludesPrivateData` перевіряє GET/HEAD success/error
parity і відсутність query/pair/cookie/agent/error strings у strict field allowlist.
Scope/INFO switch, writer errors, connect deadline та admission deadline
перевірені unit tests; real SQL phases — `TestReadinessRecoveryWithHTTPRole`.
Default-off process smoke лишається чинним; реальний flagged binary використано
в цьому corpus run. Нові events містять лише фіксовані labels; довільні request strings відсутні.

Послідовне self-review за code-review: contracts/privacy, named-return/defer
ordering, per-request collector isolation, slot/deadline behavior, writer
compatibility та документація. Перевірених дефектів у фінальному diff не виявлено;
незалежного reviewer не було. Обсяг — branch diff від зазначеного base,
включно з новими Go files та evidence. Фінальна перевірка manifest/checks
зафіксована в `receipt.json`; CI результат фіксується в PR на exact head.

## Збереження й cleanup

Run зайняв 142.578 s. Власні HTTP/PG containers, network і reader LOGIN прибрані;
retained bind/artifacts збережені; сторонні containers незмінні.
Залишилося близько 84.6 GiB. Raw PG logs, DSN/keyring, paths/terms і inputs
у ignored `data/experiments/server-timings-2026-11-08/`; one-off harness —
`tmp/server-timings/`. Тут опубліковані лише агрегати, allowlisted timing/client
measurements без body/hash/header/URL data, ресурси, state та lifecycle receipts.
`receipt.json` містить SHA-256 усіх інших файлів цього evidence каталогу.
