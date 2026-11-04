# Bounded-prefix index: окрема локальна копія

## Висновок

Після [PR24 diagnostics](../2026-11-04-mask-plans/REPORT.md) перевірено
B-tree над `left(source_key,64)` із повним LIKE recheck. На окремій копії
**16 повних мовних пар / 1 373 948 translations** усі 32 candidate plans мають
prefix expression у Index Cond; упорядковані результати збігаються з baseline.
Синтетичні Unicode/escaping/seek/довгі значення також пройшли.

Це аргумент для наступної реалізації, **не зміна сервісу чи production migration**.
Старі шість HTTP 503 не відтворювали й не оголошуємо виправленими. AT-16 відкрито.

## Дані та ізоляція

База revision: `6ef948e309d8641794a686024572766cc30e09ff` (PR24 merge).
Source — retained new-service corpus, тільки SELECT/COPY через тимчасовий
LOGIN із `slovnyk_http`. Target — інший порожній PostgreSQL 17.11 cluster,
окремий `.pgdata`; кожен DB container 2 CPU/8 GiB, loopback only.
[Startup](startup.json). Oracle, legacy й backup/restored copies не змінювали.

Скопійовано всі рядки пар тих самих перших 16 workload cases, без відбору
лише успішних. [Copy receipts](copy.json): per-pair counts = dataset_pairs,
SHA-256 binary COPY усіх дев'яти колонок в ID order однакові між source/target.
[Повторна перевірка](final-copy-verification.json) після експериментів підтвердила
незмінність усіх скопійованих рядків. Це окремий oracle цієї копії, не заміна
canonical migration digest.

Прототип містить лише translations, PK та exact index, потім prefix index.
Немає provenance, FK, tuple-hash UNIQUE, entry index, ready triggers або повного
importer. Domain types замінено базовими text/bytea; collation усіх п'яти text
колонок — `C`, [фактична звірка](actual-collations.json). Тому фізична щільність,
плани й write cost не тотожні повній service DB. Source application schema незмінна.

Початкова проба помилково мала default collation на чотирьох колонках. Після
[вирівнювання](collation-alignment.json) matrix, synthetic й insert samples
повторено; нижче лише повторні результати. Початкові receipts збережені приватно
в `initial-default-collation/`, не використовуються як acceptance evidence.

## Запит і коректність

[DDL і first-page SQL](PROTOTYPE.sql). До наявного повного LIKE додається
консервативний prefilter за першим literal prefix compiled LIKE pattern:

1. Розібрати SQL escape sequences; перший unescaped `%`/`_` завершує prefix.
2. Взяти щонайбільше **64 Unicode runes**, повторно escape `%`, `_`, `\`, додати `%`.
3. Для порожнього prefix залишити базовий запит. **Повний LIKE не прибирати**.
4. Зберегти C-order усього `(source_key,source_text,target_text,id)`, lookahead
   та seek tuple; не переходити до сортування тільки за коротким prefix.

Індекс містить до 256 UTF-8 bytes текстового prefix, не весь нормалізований ключ.
Normalized key не обрізається і не перенормалізовується після виділення prefix.
Leading-wildcard masks залишаються на fallback; загальний timeout потрібний.

[96 EXPLAIN records](matrix.json): 32 baseline до індексу, потім на indexed table
32 baseline + 32 candidate, порядок чергується за case. Cases 0–15, limits 1/50,
actual LIMIT = UI limit + 1. Після copy та index build виконано ANALYZE **лише target**.
Planner не примушували. Діагностичний connection statement_timeout 120 s;
HTTP/application deadline не змінювався, HTTP в цьому експерименті не запускався.
Окремі SELECT після EXPLAIN хешували ordered ID/key/raw source/target results;
усі 32 case/limit digests однакові в трьох варіантах.

[Синтетичний набір](synthetic.json): 314 rows, 30 masks × limits 1/2/50,
90 baseline/candidate comparisons, 3 680 page queries. Перевірено full seek traversal,
casefold/NFC, non-ASCII, emoji, `%`/`_`/`\`, broad/negative/middle masks,
prefix межі 63/64/65, leading wildcard fallback (5 masks), 64 KiB raw fields.
[Додаткова boundary проба](long-fields.json): 64-byte locales, source/target
по 65 536 bytes і folded source_key **98 304 bytes**; insert із prefix index
та baseline/candidate query збігаються. Це не перевірка admission constraints:
їх реалізує сервіс, тут перевіряли index tuple safety та SQL semantics.

## Вимірювання і ціна

[Summary](summary.json); SQL execution із EXPLAIN, не HTTP duration/SLO.

| Запит на копії | Plans | Median / max execution |
|---|---:|---:|
| Baseline до index | 32 | 13.504 / 40.919 ms |
| Baseline на indexed table | 32 | 9.9025 / 23.054 ms |
| Candidate із prefilter | 32 | 0.0555 / 0.073 ms |

Baseline після побудови також може обрати prefix index для dataset/pair,
але без prefix range. Candidate: **32/32 prefix index conditions**.
Порівняння confounded warming, компактнішою копією й іншою статистикою;
не переносити timings або коефіцієнт прискорення на весь корпус чи сервер.

[Build](index-build.json): **68 247 552 bytes (65.09 MiB)**, 1 347.451 ms,
cluster WAL delta 61 788 896 bytes. Це тільки 16 пар; full-corpus розмір,
побудова migration та restore overhead не виміряні.

[Insert samples](write-cost.json): 3 × 100 000 рядків на варіант,
ідентичні PK/exact indexes, optional prefix; порядок чергується, TRUNCATE й CHECKPOINT
перед кожною виміряною вставкою. WAL median **46 217 608 → 59 671 464 bytes
(+29.1%)**. WAL delta — cluster-wide insert LSN, може включати background work.
Час нестабільний: baseline 373.609–1 219.508 ms, candidate 593.859–1 654.387 ms;
median 1 128.967 / 636.823 ms **не доводить здешевлення запису**. Три noisy samples
не визначають надійну latency penalty. Це не benchmark повного importer.

## Збереження та наступний зріз

[Source baseline](source-baseline.json) = [final](source-final.json);
ready/generation 1/report/counters незмінні. [Lifecycle](lifecycle.json):
source reader і власні containers/networks прибрані, OOMKilled false;
original corpus, усі backup/restore copies й окрема experiment DB збережені.
Final experiment database size: 579 204 787 bytes, WAL окремо.

Далі — реалізувати prefix extraction, версійну migration й ReadPage prefilter
з integration tests у реальній service schema, без зміни exact/fallback semantics.
Потім окремий full-corpus index/storage/build/restore gate та повтор HTTP probes.
Не використовувати цей prototype як виконану міграцію. Generic/prepared plan
поведінку при майбутньому pooling теж треба перевірити; поточний HTTP borrow
створює нове з'єднання на запит. Широкі маски, contention, cache control,
AT-16/crawler budget, target-server capacity та причина старих 503 відкриті.

## Відтворення та review

Ignored `data/experiments/prefix-index-2026-11-04/` містить private credentials,
raw plans і receipts; one-off Go harness — `tmp/prefix-index/probe/`, orchestration —
поряд у `tmp/`. Clean checkout не має private workload/harness. Public SQL і
JSON дозволяють перевірити протокол/арифметику; повне незалежне відтворення
потребує цих локальних входів. Реальні терміни, rows, query paths та raw plans
не опубліковані. [Validation](validation.json): перевірки й sequential self-review,
без незалежного reviewer.

PostgreSQL docs: [expression indexes](https://www.postgresql.org/docs/17/indexes-expressional.html)
пояснюють обчислення виразу під час запису;
[B-tree/LIKE](https://www.postgresql.org/docs/17/indexes-types.html) — anchored pattern
і collation. Висновки про цей candidate спираються на фактичні плани копії.
