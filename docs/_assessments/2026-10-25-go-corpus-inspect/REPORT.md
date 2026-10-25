# Повний Go inspect CSV-корпусу

## Scope та результат

Дата: 2026-10-25. Base/runtime revision:
`7da1a4915b268e75d7f389487301f0cc55d7d7e9` (main після merge PR #12).
Фактичний binary зібрано `make build`, без змін application code/dependencies.
Input — окрема frozen copy з [попередньої репетиції](../2026-10-25-csv-manifest/REPORT.md),
не legacy directory. БД, Oracle, browser і publication не задіяні.

| Перевірка | Результат |
|---|---:|
| Файлів / complete / checksum-verified | 992 / 992 / 992 |
| Parsed occurrences / rejected occurrences | 56 455 068 / 0 |
| Issues / error | відсутні / відсутня |
| State / exit code | inspected / 0 |
| Compressed bytes / locales / expected pairs за manifest | 535 766 648 / 32 / 992 |
| Набір hashes та per-file record counts проти audit 2026-10-24 | повний збіг |

Fingerprint і SHA-256 manifest до/після запуску:
`ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Inspect перевіряє compressed identity, gzip/CSV до EOF, UTF-8/admission limits,
наявність локалей і відповідність artifact pair. Файли snapshot лише читаються;
кожен compressed stream hash перевіряється перед parsing і повторно під час нього.

## Команда та збережені докази

Після `make build` виконано:

```sh
bin/slovnyk import inspect \
  --manifest data/experiments/csv-manifest-2026-10-25/manifest.json \
  --source data/experiments/csv-manifest-2026-10-25/snapshot
```

Для повторення потрібні саме цей immutable snapshot і manifest; їх немає у
чистому checkout. Зберігайте stdout/metrics у новій директорії, не перезаписуйте
результати попереднього запуску. Не створюйте output усередині snapshot.

- [inspect.json](inspect.json) — точні bytes CLI stdout; лише counts/hashes/status,
  без dictionary tuples, source paths або secrets.
- [invocation.json](invocation.json) — command, revision, binary SHA, Go/platform,
  час початку й manifest SHA перед запуском.
- [execution.json](execution.json) — exit, elapsed, manifest/report SHA; stderr порожній.
- [time.txt](time.txt) — фактичні `/usr/bin/time -v` metrics: wall 24.70 s,
  peak RSS 20 020 KiB (~19.6 MiB). Python monotonic wrapper: 24.708 s.
- [comparison.json](comparison.json) — звірка всіх 992 report artifact hashes
  з manifest та [audit](../2026-10-24-csv-audit/result.json): filenames/bytes,
  per-file parsed counts, завершеність, checksum та нульові rejects.

Оригінальні результати зберігаються локально у
`data/experiments/go-inspect-2026-10-25/`. Metadata-only копії тут побайтово
тотожні; stderr не копіюється, бо порожній. SHA exact stdout:
`57d5a58c1752448c603d6adeeabdb558d8caf6cfe39fa8ab7ca76eb60252a332`.

## Межі та review

`distinct_counts_available=false`: нульові accepted_unique/duplicate fields
не є результатом дедуплікації. Parsed occurrences не видаються за нову перевірку
distinct tuples. Inspect не нормалізує/звіряє DB keys та не перевіряє transaction
retry, staged PostgreSQL writes, provenance reconciliation, exports, ready або
publication. Окремі попередні synthetic negative/integration tests не повторено
цим corpus run. Семантична якість і повнота втраченої старої БД не доведені.

Це один локальний запуск із доступним page cache, без resource quota;
час і RSS не є Oracle capacity gate, HTTP SLO, PostgreSQL budget чи міграційним RTO.
Known gaps snapshot лишилися; запуск не є погодженням втрат або production launch.

Послідовний self-review, не незалежне рев’ю: перевірено CLI/Inspect implementation,
методику звірки, hashes metadata-only копій і відповідність активних docs.
[Reviewed inputs](reviewed-inputs.sha256) фіксують змінені активні docs і evidence;
цей report та сам inventory виключені. Application/schema/SQL/AGENTS.md не змінено.
Підтверджених невиправлених findings немає. `make check`, `git diff --check`
та assertions exact-copy/hash/count/completion — PASS, exit 0. Local integration
не повторено: code/schema не змінено; CI outcome фіксується окремо у PR.
