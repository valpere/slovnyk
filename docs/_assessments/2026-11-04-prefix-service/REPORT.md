# Bounded prefix у сервісі — 2026-11-04

## Зміна

Після merge PR25 (`00725b4bd573bbef2774373e38f6e96f2b65e894`) додано
migration 4: B-tree `(dataset_id,source_locale,target_locale,left(source_key,64))`
із C collation для expression. Старі migrations/checksums незмінні.
`ReadPage` бере до 64 буквальних Unicode codepoints із compiled LIKE pattern,
екранує SQL metacharacters і додає параметризований prefix prefilter.
Повний LIKE перевіряє suffix; C order, lookahead і seek збережені. Anchor проходить
обидва predicates. Exact та mask із leading wildcard зберігають попередні predicates.

Owner migration атомарна, без `CONCURRENTLY`: writers треба зупинити;
звичайні читання доступні. Index statement timeout — 30 min, загальний CLI —
31 min, решта statements — 30 s, lock wait — 5 s. Значення — обмежений
операторський бюджет, не виміряна гарантія часу побудови на повному корпусі.
HTTP DB deadline лишається 2 s. До запуску нового binary потрібна schema 4.

## Перевірки

- `make check test test-race test-smoke`: PASS.
- `make test-integration`: PASS на disposable PostgreSQL 17.11, включно з
  повним synthetic lifecycle і backup/restore у другий порожній cluster.
- `TestPrefixPattern`: literals, escaping, leading wildcard, Unicode width,
  boundary 64, malformed trailing escape fallback.
- `TestPrefixReadPageMatchesFullLike`: 17 masks × limits 1/2/50, повна seek
  pagination проти початкового LIKE/C-order запиту; 64-prefix collisions,
  Unicode fold/NFC, literal SQL metacharacters, long fields, invalid suffix anchor.
- `TestPrefixUpgradePreservesReadyData`: schema 3 відхиляється; owner upgrade
  та повтор ідемпотентні; raw source/target 64 KiB і folded key 96 KiB збережені,
  state ready незмінний. Селективна synthetic пара: prefix expression у Index Cond
  під HTTP role з `enable_seqscan=off`; це eligibility check, не benchmark.
- `TestBoundedPrefixSignedPages`: справжні import/verify/publish та signed
  cursors із limits 1/2; common-prefix suffix filtering, split groups, Unicode,
  exact і leading-wildcard fallback.

Перший plan check на one-row pair обрав PK навіть із вимкненим seq scan.
Fixture розширено 2 048 unrelated keys та ANALYZE; перевірка вимагає expression
у Index Cond, а не лише назви індексу. Це уточнення тесту, не зміна planner settings
сервісу. Оновлено також старий V1→latest upgrade test для нової inventory.

## Послідовне self-review

Scope: зміни гілки `feat/bounded-prefix-lookup` від зазначеного base, включно з
новими SQL/test files. Перевірено контракти, parameter numbering для anchor/seek,
необхідність full LIKE, Unicode/escape boundaries, bounded index size, immutable
migration inventory, maintenance/upgrade та restore instructions. Підтверджених
дефектів у фінальному scope не виявлено. Це self-review, без незалежного reviewer.

## Межі

Збережені corpus/backup/restored БД лишилися schema 3; цей зріз їх не запускає
і не змінює. Legacy та Oracle не зачеплено. Нові індекс і query треба окремо
перевірити на повному корпусі: build time, disk/WAL/temp space, dump/restore,
HTTP workloads, cold/warm caches і contention. AT-16, причина історичних 503,
production storage/capacity та off-host/target-server RTO відкриті.
