# SQLite на повному корпусі: локальна технічна проба

Дата: 2026-10-24. Усі 992 остаточні CSV.gz, **56 455 068 записів**.
Попередня [проба чотирьох пар](../2026-10-23-sqlite/REPORT.md) збережена
як окремий історичний експеримент. Engine ще не обрано.

## Метод

Python 3.12.3, SQLite 3.45.1, Unicode 15.0.0; developer machine AMD Ryzen 7
7700, 16 logical CPUs, ~61 GiB RAM, Linux x86-64. Нова окрема БД:
`data/experiments/sqlite-full/corpus.sqlite3`; вихідні файли не змінювалися.
Скрипт адаптований з попередньої проби: всі файли, progress logs, коректний
опис меж експерименту. Схема та запити залишилися порівнюваними.

Зберігаються raw locale/source/target, NFC → full casefold → NFC source key,
SHA-256 ключа та integer ID. Немає production dataset/ledger/origins,
publication CAS, deduplication та production parser. Незалежний
[аудит CSV](../2026-10-24-csv-audit/REPORT.md) не виявив структурних відхилень
або точних дублікатів; per-file counts цієї БД збігаються з аудитом.
Це звіряння кількостей, не доказ рівності всіх імпортованих кортежів.

SQLite cache — 8 MiB на connection, mmap=0, temp_store=FILE; page cache ОС
не обмежено й не очищено. Імпорт працював паралельно з аудитом джерел,
тому його час не є ізольованим benchmark. Подальші load/restore виконані
послідовно. Жодних змін або benchmark на `ssh psysoft` не виконано.

## Обсяг і час

| Метрика | Результат |
|---|---:|
| Rows у БД / після повторного read-only відкриття | 56 455 068 / 56 455 068 |
| Імпорт без створення індексів | 255.481 s |
| БД із hash/pair indexes | 9 625 477 120 bytes / 8.96 GiB |
| БД із додатковим covering lookup index | 13 349 707 776 bytes / 12.43 GiB |
| Створення додаткового lookup index | 61.419 s |
| Peak RSS процесу базової проби | 45 044 KiB / ~44 MiB |

RSS не включає кеш ОС і не є бюджетом Go HTTP-сервісу. Час створення
базових індексів, ANALYZE й повного production verify до import_s не входить.
Файл БД не включає повний provenance та retained datasets/exports.

## SQL-пошук

По 50 serial queries exact/prefix/infix, вибірка охоплює 50 напрямлених пар;
LIMIT 51, стабільне сортування. Випробувано маски SQLite GLOB, а не готовий
production compiler; семантика Go masks і Unicode ще потребує fixtures.

| SQL p95 | Hash/pair indexes | З lookup index |
|---|---:|---:|
| Exact | 0.071 ms | 0.038 ms |
| Prefix | 29.217 ms | 0.071 ms |
| Infix | 31.427 ms | 17.196 ms |

EXPLAIN: з lookup index exact використовує рівність key, prefix — range,
infix — scan напрямленої пари по covering index. Довільна infix-маска
залишається scan, хоча B-tree покращує інші сценарії.

Окрема exploratory-проба: **300 SQL-запитів із частотою 10/с**, 4 worker
slots, 80% exact із misses, 10% prefix, 10% infix. Samples взято з чотирьох
найбільших пар; БД містить весь корпус. За 29.920 s помилок немає;
p95 exact 0.374 ms, prefix 0.402 ms, infix 24.853 ms; сукупний p95 22.746 ms,
максимум 28.353 ms, включно з scheduling/queue. Чотири slots не означають
постійно чотири одночасно активні queries. Це коротка проба з теплим кешем,
без HTTP, UI rendering, cursors, publication і ботів; профіль не є погодженим
acceptance workload. Progress callback із deadline 2 s не доводить жорсткий
wall-clock timeout під час I/O або працездатність Go cancellation.

## Відновлення

Локальна репетиція завершена: копіювання закритого immutable файла в
`data/experiments/sqlite-full/restored.sqlite3`, SHA-256 обох файлів збігаються,
`PRAGMA quick_check` повернув `ok`, відновлено 56 455 068 rows.
Копіювання — 105.241 s; увесь цикл із hashes і перевірками — **127.065 s**.
Це відновлення на тому самому developer host/filesystem; не off-host backup,
не live backup і не відновлення HTTP/DNS/secrets на Oracle. Quick_check
не замінює повний integrity_check або domain reconciliation. Погоджені
24 години для втрати production-сервера цим тестом не підтверджені.
Доказ: [restore-result.json](restore-result.json).

## Висновки та наступні перевірки

SQLite залишається придатним **кандидатом** для першого read-only релізу;
на цій машині явного обмеження SQL-пошуку не виявлено. Це не підтвердження
Oracle Micro: у нього 1 GB RAM і baseline 1/8 OCPU; локально доступно ~61 GiB
RAM для ОС cache та значно потужніший CPU. AT-16 / HTTP SLO не пройдені.

Одна serving-БД — 12.43 GiB; два datasets — 24.87 GiB; три копії
(active, retained, staging/restore) — **37.30 GiB** до provenance, exports,
вихідних архівів, журналів та запасу. Порівняно з ~41 GiB вільного root disk
у [знімку Micro від 2026-10-23](../2026-10-23-server/REPORT.md) це суттєвий
ризик. Ціни VPS з 40 GB disk не можна вважати придатною конфігурацією без
повної storage-репетиції. Індекс поки експериментальний; не закріплює
цей розмір як мінімум або як розмір production БД.

Потрібні повна schema з ledger/origins, reconciliation digests/exports,
SQLite publication/rollback протокол, crash/cancellation tests, фактичний
RAM/disk budget і HTTP-навантаження на цільовій машині. PostgreSQL baseline
цим експериментом не реалізована й не порівняна; зміна engine потребує
узгодження пов’язаних контрактів.

## Відтворення й докази

Використовувати нові output filenames; tools відмовляються перезаписувати
БД та evidence. Імпорт може створювати великі локальні файли.

```bash
python3 docs/_assessments/2026-10-24-sqlite-full/probe.py \
  ../slovnyk1.0/csv data/experiments/sqlite-full/fresh.sqlite3 --files 992 \
  > data/experiments/sqlite-full/fresh-result.json
python3 docs/_assessments/2026-10-24-sqlite-full/load.py \
  ../slovnyk1.0/csv data/experiments/sqlite-full/fresh.sqlite3 data/experiments/sqlite-full/fresh-load.json
python3 docs/_assessments/2026-10-24-sqlite-full/restore.py \
  data/experiments/sqlite-full/fresh.sqlite3 data/experiments/sqlite-full/fresh-restored.sqlite3 \
  data/experiments/sqlite-full/fresh-restore.json
```

[result.json](result.json), [load-result.json](load-result.json),
[probe.py](probe.py), [load.py](load.py), [restore.py](restore.py).
Великі БД й реальні словникові тексти залишаються в ignored `tmp/`;
у репозиторії — scripts, агрегати та hashes.

## Розташування локальних даних

2026-10-24 БД та exports перенесені з `tmp/` до `data/experiments/`.
Поточні шляхи й команди вище оновлено; вміст даних та результати вимірів
збережені. Опис директорій — [data/README.md](../../../data/README.md).
