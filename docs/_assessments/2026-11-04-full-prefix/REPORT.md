# Schema 4 на повному локальному корпусі

## Результат

Після merge PR26 (`61d221ce1751c8e8f61f704f40e0ce28d714d354`)
owner CLI оновив початкову **dev-БД** із 56 455 068 translations / 992 pairs
зі schema 3 до 4. Dataset лишився ready/active, generation 1.
Виконання команди спостерігалося через **177 s**; polling має крок 3 s,
тому це не точний SQL-only build time. Повторний `migrate` успішний.
[Migration](migration.json), [idempotence](idempotence.json).

Новий `translations_prefix64` — **2 607 292 416 bytes / 2.428 GiB**,
valid/ready, з очікуваною C expression. БД 64.316 → **66.744 GiB**,
WAL directory окремо. Cluster WAL delta за міграцію — 2 361 107 088 bytes;
це cluster-wide delta, не виключно index WAL. Cumulative database temp bytes
зросли приблизно на 3 GiB, temp files — на 3. Мінімально спостережено
**98.329 GiB** вільного місця; це samples, не точний peak disk accounting.
Disk floor 30 GiB і часовий guard не спрацювали.
[Before](migration-before.json), [after](migration-after.json),
[progress](migration-progress.json), [index definition](index-validation.json).
Після міграції виконано `ANALYZE slovnyk.translations`.

## Незмінність даних

До DDL всі 17 table inventories і sequence state збіглися з попередньою
повною backup/restore репетицією. Після DDL повторно потоково прочитано всі
колонки кожного рядка. **Усі 16 application tables і sequences тотожні**;
лише `schema_migrations` має очікувані 3 → 4 rows. Рядки ledger 1..3 незмінні,
checksum 4 збігається з embedded SQL source. Ready/report/publication state
і generation збережені. [Integrity assertions](integrity.json),
[inventory before](inventory-before.json), [after](inventory-after.json).

Oracle — count, sum row SHA-256 mod 2^256 та XOR row SHA-256 через binary COPY;
враховує field count, length/null markers і bytes. Це probabilistic multiset
check за незмінних built-in/domain encodings, не нова source reconciliation.
Writers зупинені; перед міграцією інших client backends не було.
Послідовні table reads не є спільним snapshot.

Повторно використаний Go harness перенесено в
[`minions/corpus-inventory`](../../../minions/corpus-inventory/main.go):
він є особистим помічником агента, не service/operator utility. Unit tests
перевіряють reorder, duplicates, зміну рядка, NULL/empty і malformed COPY.
Постміграційна повна звірка виконана саме цим посіпакою.

## SQL та HTTP

Pgx tracer перехопив SQL/args **справжнього `postgres.ReadPage`**;
32 EXPLAIN ANALYZE (16 раніше обраних positive cases × limits 1/50) виконано
без примусового index scan. **32/32** мають prefix expression у Index Cond
і full source-key LIKE у Filter. Entries та lookahead тотожні початковому
full-LIKE/C-order запиту на тій самій schema 4 БД.
SQL execution median/p95/max — 0.039/0.054/0.071 ms, після попередніх читань.
Це SQL diagnostic, не HTTP/server SLO. [Plans](plans.json).

Старий binary/schema 3 дав HTTP baseline; новий binary/schema 4 повторив
**135/135 HTTP 200**: exact/prefix, limits 1/50, HTML/plain та 7 continuations,
виданих до upgrade. Розміри й SHA-256 тіл тотожні, також збігаються
Content-Type і Cache-Control. Keyring однаковий приватний rehearsal keyring.
Історичних production cursors тут немає. [Before](before-http.json),
[after](after-http.json), [summary](summary.json).

Клієнтський prefix p95 на 64 responses: 257.624 → 15.831 ms; exact p95
17.793 → 17.450 ms. Це послідовна positive вибірка, без контрольованого cache
reset/randomization; baseline частково збігався з локальними Go checks.
Не виводимо з цих чисел causal speedup, cold-cache або capacity гарантій.
Baseline цього запуску також успішний; причина історичних 503 не доведена.

## Середовище, відхилення й cleanup

Pinned PostgreSQL 17.11: 2 CPU / 8 GiB, maintenance_work_mem 512 MiB,
parallel maintenance workers 0, temp_file_limit 20 GB. Cgroup memory peak
8 GiB включає page cache; memory.events `oom=0`, `oom_kill=0`, OOMKilled=false.
Це не RSS чи доказ потреби саме у 8 GiB. [Runtime](runtime-final.json).
HTTP: 1 CPU / 512 MiB, restricted LOGIN, non-root, loopback only, read-only
rootfs/mounts. Lifecycle logs перевірено без credentials/query text.

Дві помилки scratch harness виправлено зі збереженням локальних diagnostics:
`http.py` затіняв Python stdlib, тому перейменований; initial host binary
із glibc не запускався в Alpine, HTTP binary перезібрано з `CGO_ENABLED=0`.
Міграція вже успішно завершилася до другого збою; після виправлень повторний
owner migrate, HTTP і full inventory пройшли. Application code не змінювався.
[Build variants](binary-builds.json).

Тимчасовий reader, власні HTTP/DB containers/network прибрані;
початкова bind dev-БД збережена на schema 4. **Backup bundle і restored cluster
залишилися schema 3**, їх не запускали й не змінювали. Oracle та legacy
не задіяні. [Lifecycle](lifecycle.json), [final storage](storage-final.json).
Приватні inputs/plans/DSN/terms — ignored
`data/experiments/full-prefix-2026-11-04/`; одноразові scripts — `tmp/full-prefix/`.
Clean checkout не відтворює реальний workload без цих приватних входів.

## Перевірки та межі

`make check test test-race test-smoke` — PASS, включно з новим посіпакою.
Послідовне self-review за `code-review`: invariants/ledger exception,
actual production SQL capture, HTTP result oracle, sampling/resource limits,
privacy та cleanup. Підтверджених дефектів у фінальному scope не виявлено;
незалежного reviewer не було. CI перевіряється на PR head.

Далі — schema 4 full backup/restore і розширені HTTP/cache/contention probes,
узгоджений workload, crawler budget та target storage/server capacity.
66.744 GiB без WAL/artifacts не поміщається на Oracle root ~45 GiB із
профілю 2026-10-23; нової перевірки сервера не було. Off-host RTO і AT-16 відкриті.
