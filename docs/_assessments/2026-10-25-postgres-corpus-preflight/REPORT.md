# PostgreSQL corpus preflight

## Scope і середовище

2026-10-25, base/runtime `574ec3f3b06227be707c55ce3e989b675b32b7e5`
(main після PR #13). `make build`; application/schema/dependencies не змінено.
Повний frozen manifest попереднього [Go inspect](../2026-10-25-go-corpus-inspect/REPORT.md):
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Це **bounded початок** імпорту, не завершена міграція.

Новий локальний Compose project `slovnyk-corpus-20261025`, pinned PostgreSQL
17.11 image з `compose.yaml`, окремий bind pgdata, random loopback port,
1 CPU / 512 MiB. `fsync`, `synchronous_commit`, `full_page_writes` — on.
Schema migrations виконано admin, імпорт — окремим LOGIN у `slovnyk_operator`
із CONNECT/TEMP, без superuser/CREATEDB/CREATEROLE/replication/BYPASSRLS.
[Invocation](invocation.json), [isolation](isolation.json).
Oracle, legacy і сторонні Docker projects не задіяні.

## Actual interruption та retry

Однакова команда в обох спробах, DSN лише через `SLOVNYK_OPERATOR_DSN`:

```sh
bin/slovnyk import run \
  --manifest data/experiments/csv-manifest-2026-10-25/manifest.json \
  --source data/experiments/csv-manifest-2026-10-25/snapshot \
  --batch-size 1000
```

Harness опитував committed counts кожні 2 s і надсилав SIGINT після приросту
щонайменше 50 000; через polling можливий overshoot. Додаткові stop limits:
75 s/attempt, database size 2 GiB, free disk floor 30 GiB. Вони не спрацювали;
обидві спроби зупинено за row target. Це локальні запобіжники експерименту,
не нові service limits. [Procedure](procedure.json).

| Спостереження | Спроба 1 | Спроба 2 |
|---|---:|---:|
| Committed parsed/unique/ledger/origins/translations/pair count | 55 000 | 114 000 |
| Додано нових records | 55 000 | 59 000 |
| Duplicate / rejected / issues | 0 / 0 / 0 | 0 / 0 / 0 |
| Complete files | 0 | 0 |
| CLI exit / error | 1 / import_interrupted | 1 / import_interrupted |
| Harness elapsed, із polling/SQL snapshots | 47.701 s | 10.541 s |
| Database bytes | 79 869 619 | 154 039 987 |

Це частина одного файла `slovnyk_fr-fr_es-es.csv.gz` (171 726 records у
попередньому audit), перший у hash order. Обидві спроби мають той самий dataset ID,
різні attempt IDs; dataset `loading`, active null/generation 0. Manifest SHA
після проби той самий. Stderr порожній в обох запусках.

Перші 55 000 records після retry мають ті самі counts і SHA-256 у всіх трьох
таблицях: [before](prefix-before.json) = [after](prefix-after.json).
Digest включає всі row fields: server-side `to_jsonb(row)::text` → SHA-256,
конкатенація hex digests у порядку artifact_hash/ordinal (ledger/origins) або
id (translations) → SHA-256. Межі задає перший checkpoint. Raw texts не виходять
із БД. Counters/ledger/origins/translations/pair sums узгоджені в одному SQL snapshot.

## Недоступний report після SIGINT

В обох CLI reports `files=null`, `issues=null`, `distinct_counts_available=false`:
скасоване з'єднання непридатне для deferred DB cleanup/report. Це недоступні
counts, не нуль записів. SQL snapshots підтверджують committed state вище.
Після першого SIGINT attempt лишився `running`, але operator sessions — 0;
наступний writer після session lock позначив його
`interrupted / writer_disconnected` і відтворив prefix. Другий attempt після
свого SIGINT також лишився `running` без живого writer; це покинутий запис,
який обробить наступний запуск. Ручних UPDATE не виконано.

Початковий harness assertion помилково вимагав негайного `interrupted` і
завершився assertion failure після першої спроби. Перевірено `Run` cleanup,
`begin` abandoned-writer recovery та існуючі synthetic session-loss tests;
очікування виправлено для другої спроби, application code не змінено.
[Observation](harness-observation.json), [assertions](assertions.json).
Retry/prefix assertions — PASS, exit 0; CLI exit 1 тут навмисний, не clean-run PASS.

## Місце, збереження й межі

[Baseline](baseline.json): порожня migrated DB 8 509 107 bytes.
[Sizing](sizing.json): приріст ~1277 bytes/row від baseline або ~1257 bytes/row
між checkpoints. Грубі лінійні проєкції DB/indexes для 56 455 068 records —
72.08 або 70.98 GB (~67.1 або 66.1 GiB). WAL directory у другому checkpoint —
234 881 024 bytes (224 MiB), окремо від DB size. Вільно було ~122.8 GiB.
Одна мовна пара не репрезентує весь корпус. Проєкції не включають WAL retention,
sort/temp, exports, backup/restore copies, інші distributions/index growth.
Це не доведена місткість або дозвіл на необмежений запуск.

Час включає initialization/replay/monitoring; це не порівняння throughput,
Oracle/HTTP capacity, peak RSS measurement або міграційний RTO.
Жодного файла в цьому DB import ще не завершено; повний input inspect був окремим.
Full-corpus clean-vs-retry, normalization/provenance reconciliation, verify,
exports/ready/publication та historical completeness залишаються відкритими.

Original evidence, private credentials/config і `.pgdata/` збережено у
`data/experiments/postgres-corpus-preflight-2026-10-25/`. Власні container/network
прибрано, bind pgdata збережено; [lifecycle](lifecycle.json).
Для продовження відтворити container з тим самим pinned image, bind/config та
credentials, потім повторити command; не змінювати checkpoint/state вручну.
Це не off-host backup. Credentials, pgdata й dictionary tuples не комітяться.
У цьому assessment — точні metadata-only копії JSON без stderr (він порожній).

Первинний `make check` виявив filesystem integration issue: Go `./...` заходив
у private `pgdata/`. Після видалення container каталог перейменовано на `.pgdata/`
без зміни device/inode/mode/owner, local bind override оновлено;
[layout evidence](storage-layout.json). PostgreSQL permissions не послаблено.
Далі repository checker теж намагався обходити приватні дані. У
`tests/repository/check.go` тепер спочатку перевіряється Git-ignore самого root
без trailing slash, і лише повністю excluded parent дозволяє пропустити subtree.
Contents-only glob із unignored child не вважається виключеним root; окремі
files далі перевіряються. Source ignore checks не пропускаються.

`TestPrivateDataRootAndUnignoredChild` перевіряє private directory та виявлення
unignored child; збережено direct newline-path і tracked-source checks.
Report поле `ignored_data_roots` показує verified subtree, `ignored_data_files`
рахує тільки фактично перелічені files. Це зміна developer check, не runtime.

## Review і validation

Послідовний self-review, не незалежне рев’ю: inspected runtime semantics,
ізоляція, interruption/recovery, SQL counts/prefix digests, projection exclusions
і claims активних docs. [Reviewed inputs](reviewed-inputs.sha256) фіксують активні
docs, checker та metadata; report і сам inventory виключені. Підтверджених
невиправлених findings немає. Actual CLI reports, hashes/copy та SQL assertions
перевірено. Після layout/checker виправлень `go test -race ./tests/repository
-count=1 -v`, `make check test`, `git diff --check` — PASS, exit 0.
Local disposable integration не повторено: application/schema/SQL не змінено;
actual preflight вище є окремою перевіркою. CI фіксується у PR.
AGENTS.md не змінено.
