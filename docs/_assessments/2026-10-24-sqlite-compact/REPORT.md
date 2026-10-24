# SQLite: компактні внутрішні ключі — 2026-10-24

## Метод і межі

Локальне продовження [проби covering index](../2026-10-24-sqlite-indexed/REPORT.md).
Окрема БД `data/experiments/provenance-compact/model.sqlite3` створюється з
read-only [звіреного прототипу](../2026-10-24-provenance/REPORT.md)
`data/experiments/provenance/model.sqlite3`. Попередні БД та CSV незмінні.
Oracle не задіяний. Середовище: Ryzen 7 7700, ~61 GiB RAM,
Python 3.12.3 / SQLite 3.45.1, Linux x86-64.

[schema.sql](schema.sql) зберігає 14 логічних таблиць та їхні дані.
Фізичні відмінності:

- Dataset UUID зберігається один раз у `datasets.uuid`; `datasets.id` та
  dataset FK — integer surrogate. Зовнішня ідентичність набору не змінюється.
- SHA-256 artifact зберігається один раз у `source_files.artifact_hash`;
  ledger/origins використовують integer artifact_id. Пара dataset/artifact
  залишається частиною ключів і FK, без cross-dataset references.
- Raw source/target/locale, source_key, key_hash, tuple_hash, logical ordinals,
  outcome та nullable legacy metadata зберігаються без змін.
- Є UNIQUE tuple hash, exact hash index та ordered covering lookup index.
  Жодні записи чи metadata не видаляються заради зменшення розміру.

Manifest, fingerprint і UUID копіюються дослівно для звіряння логічної
ідентичності. Фізична схема цього експерименту не є production schema version
і не затверджує canonical manifest/migration. PostgreSQL baseline та
зовнішні контракти не змінюються. Dataset залишається validating,
active=NULL/generation=0, publication не виконується.

## Перетворення та перевірка

[probe.py](probe.py) копіює translations, потім по кожному artifact ledger
та origins. Під час bulk FK enforcement вимкнений; після побудови індексів
виконується повний `foreign_key_check`. `ANALYZE main` обмежено новою БД;
attached source залишається read-only.

Повне streaming порівняння з базою: кожен translation із raw-полями,
нормалізованим ключем та обома hashes; кожен ledger record; кожен origin
з raw-полями та metadata. Компактні IDs розв'язуються через SHA-256/UUID
каталогу. Порядок порівняння — ID для translations і artifact/ordinal для
ledger/origins. Звіряються також каталоги, manifest, checkpoints та exports
metadata. Вхід цієї проби — один validating dataset із порожніми issues,
query snapshots та publication events; інші стани явно відхиляються.

Це доказ рівності вже звіреній базі, не повторний production CSV import.
Попередні 992 source/DB/export digests не обчислюються повторно;
файли exports залишаються в `data/experiments/provenance/exports/`.
Production retry/crash, export path protocol, CAS/retention, HTTP та
off-host restore не реалізовані цим інструментом.

Три [synthetic tests](test_probe.py) пройшли: повне логічне звіряння після
перетворення, виявлення пошкодженого raw origin та rejection cross-dataset
artifact reference. Команда та hashes — у
[synthetic-validation.json](synthetic-validation.json).

## Повний корпус: результат звіряння та обсяг

[result.json](result.json): завершено успішно, foreign_key_check=ok.
Порівняно по **56 455 068** translations, import_records і translation_origins;
32 locales, по 992 pairs/source files/exports, один import_run. Dataset metadata
ідентичні; issues/query snapshots/publication events порожні в обох БД.

| Метрика | Результат |
|---|---:|
| Компактна БД без secondary indexes | 13 327 474 688 bytes / 12.41 GiB |
| Компактна БД з усіма трьома indexes | 22 388 301 824 bytes / **20.85 GiB** |
| Попередній layout із тими самими indexes | 31 848 861 696 bytes / 29.66 GiB |
| Економія від compact keys | **8.81 GiB / 29.70%** |
| Перетворення | 241.157 s |
| Індекси та ANALYZE | 289.871 s |
| Повний foreign_key_check | 62.762 s |
| Повна логічна рівність | 227.755 s |
| Загалом / rusage peak process RSS | 821.666 s / 565 244 KiB (~552 MiB) |

RSS не включає кеш ОС; це не бюджет майбутнього HTTP-сервісу.

Розподіл сторінок за [storage-stats.json](storage-stats.json):

| Об'єкт | GiB |
|---|---:|
| translations | 7.57 |
| import_records | 1.46 |
| translation_origins | 3.37 |
| tuple_identity | 2.27 |
| exact_lookup | 2.91 |
| lookup_order | 3.26 |

Решта — невеликі каталоги та службові сторінки. Вільних сторінок немає;
повний облік включає одну зарезервовану сторінку 4096 bytes
([SQLite lock-byte page](https://www.sqlite.org/fileformat.html#the_lock_byte_page)).
Початкову помилку діагностичного обліку цієї сторінки виправлено;
історія — у [storage-stats-initial-failure.json](storage-stats-initial-failure.json).
Prefix/infix використовують covering lookup_order без temporary ORDER BY sort.

Дві копії — **41.70 GiB**, три — **62.55 GiB**, без exports, джерел,
temporary sort та ОС. Дві майже вичерпують ~41 GiB free з датованого
Micro snapshot; три перевищують усю його ~45 GiB root filesystem.
Економія значна, але запас для active/staging/retained versions цим
експериментом не забезпечено; multi-dataset publication не репетирувалася.

## SQL-навантаження

Незмінений load.py попередньої проби, запуски послідовні після перетворення
та звіряння; storage inspection виконується після обох workloads.

| Метрика, queue включено | Без quota, 100 queries | 768 MiB / 12.5% CPU, 300 queries |
|---|---:|---:|
| Elapsed | 9.921 s | 30.000 s |
| Exact p95 | 0.420 ms | 0.395 ms |
| Prefix p95 | 2.671 ms | 0.535 ms |
| Infix p95 | 38.869 ms | 199.938 ms |
| Сукупний p95 | 24.349 ms | **103.494 ms** |
| Максимум | 38.869 ms | 203.837 ms |
| Помилки | 0/100 | **0/300** |

Докази: [load-unrestricted.json](load-unrestricted.json),
[load-constrained.json](load-constrained.json). Фактичні limits:
`cpu.max=12500 100000`, `memory.max=805306368`.
У попередньому indexed layout constrained p95 — 105.955 ms, infix p95 —
202.129 ms. Тут пошук має подібні результати; це окремі короткі warm-cache
запуски, не доказ статистично значущого прискорення. Скорочення storage
не призвело до помилок або очевидної деградації на цьому профілі.

## Відтворення

З кореня репозиторію, з новими output paths; потрібні локальні базова БД
та CSV, які не входять у чистий checkout.

```sh
python3 -m unittest discover -s docs/_assessments/2026-10-24-sqlite-compact -v
python3 docs/_assessments/2026-10-24-sqlite-compact/probe.py \
  data/experiments/provenance/model.sqlite3 \
  data/experiments/provenance-compact-new/model.sqlite3 \
  data/experiments/provenance-compact-new-result.json
python3 docs/_assessments/2026-10-24-provenance/load.py \
  ../slovnyk1.0/csv data/experiments/provenance-compact-new/model.sqlite3 \
  data/experiments/provenance-compact-new-load.json --queries 100 --rate 10
```

Обмежений запуск того самого load.py — 300 запитів при 10/s через локальний
`systemd-run --user --wait --pipe --collect`, `MemoryMax=768M`,
`CPUQuota=12.5%`, унікальний unit, абсолютні шляхи та окремий output JSON.
Профіль: 80% exact із misses, 10% prefix, 10% infix; samples чотирьох
найбільших пар, 4 worker slots, LIMIT 51. Latency включає queue.
SQLite cache 8 MiB/connection, mmap=0, temp_store=FILE. Кеш ОС не очищався;
shared pages можуть обліковуватися поза cgroup. Ryzen quota не відтворює
Oracle CPU/I/O. Це SQL experiment, не HTTP/AT-16 acceptance workload.

Підсумкова перевірка артефактів, hashes інструментів, посилань та Git ignore —
[checks.json](checks.json). Наступний крок — локальний Go scaffold M1-01;
остаточний вибір engine і схема production publication залишаються відкритими.
