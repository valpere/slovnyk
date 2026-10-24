# Локальна проба повної моделі, provenance та exports — 2026-10-24

## Межі експерименту

Усі операції виконуються на developer machine; Oracle `psysoft` не задіяний.
Старий код і CSV не змінюються. Це дослідницька SQLite-схема за логічною
[DATA_MODEL](../../DATA_MODEL.md), не production migration чи вибір DB engine.
Go-застосунку, HTTP та розгортання ще немає.
Середовище: AMD Ryzen 7 7700, 16 logical CPUs, ~61 GiB RAM,
Python 3.12.3, SQLite 3.45.1, Unicode 15.0.0; Linux x86-64.
SQLite cache 8 MiB/connection, mmap=0, temp_store=FILE; кеш ОС не очищався.

## Повне звіряння та обсяг

[result.json](result.json): експеримент завершений успішно; усі 992 пари
мають однакові source/DB/export canonical digests. Кожен raw translation
та origin відповідає вихідному логічному CSV ordinal.

| Метрика | Результат |
|---|---:|
| translations / import_records / translation_origins | по 56 455 068 |
| source_files / dataset_pairs / exports | по 992 |
| dataset_locales | 32 |
| import_issues / historical query rows | 0 / 0 |
| БД до secondary indexes | 20 018 806 784 bytes / 18.64 GiB |
| БД з UNIQUE tuple + exact indexes | 27 424 051 200 bytes / **25.54 GiB** |
| Після ANALYZE | 27 424 055 296 bytes |
| Усі CSV.gz exports | 604 798 333 bytes / 0.56 GiB |
| Bulk load / secondary indexes | 376.183 s / 195.973 s |
| Повний foreign_key_check | ok, 84.127 s |
| Source/origins reconciliation та exports | 456.300 s |
| Загалом до ANALYZE / peak process RSS | 1 130.571 s / 565 244 KiB (~552 MiB) |

RSS включає Python reconciliation/sorting, не кеш ОС і не майбутній Go HTTP
сервіс. Це імпорт через serving seed, не вимір production gzip importer.

**Наслідок для диска:** два datasets такого layout — щонайменше 51.08 GiB,
три — 76.62 GiB, ще без додаткового covering index, exports, журналів,
temporary sort та запасу ОС. Це перевищує ~41 GiB вільного місця з датованого
[read-only snapshot Micro](../2026-10-23-server/REPORT.md). Стан диска сервера
повторно не перевірявся. Потрібен інший storage budget або окремо погоджений
компактніший фізичний layout; видалення provenance не є прийнятим рішенням.

## Метод

[schema.sql](schema.sql) містить 14 логічних таблиць: datasets, active pointer,
locales/pairs, source files, translations, import ledger, origins, runs/issues,
query snapshots, publication events та exports. Складені ключі з dataset UUID
і SHA-256 artifact збережені явно; великі таблиці — `WITHOUT ROWID`.
Це один можливий фізичний layout, не мінімально можливий розмір SQLite.

[probe.py](probe.py) використовує закриту read-only serving-БД попередньої
[проби](../2026-10-24-sqlite-full/REPORT.md) як seed для bulk insert.
Після цього незалежно читає **всі вихідні CSV**: звіряє кожен translation
та origin за порядковим номером, raw-полями й ID; повторно обчислює
NFC/full casefold/NFC, key hash і tuple hash (Unicode 15.0.0). Відсутні legacy count/status
залишає NULL. Не відновлює статистику, якої немає в CSV4.

UNIQUE tuple-hash index будується після bulk load, перед validation;
колізія зупиняє експеримент, не зливає записи. FK enforcement вимкнений
лише під час bulk load; потім виконується повний `foreign_key_check`.
Є exact hash index; covering lookup index у цій пробі відсутній.

Для кожної пари вихідні кортежі сортуються за UTF-8 bytes усіх чотирьох
полів. Canonical stream: кожне поле має uint64 big-endian byte length,
потім UTF-8 bytes. SHA-256 цього потоку порівнюється між source, SQL-result
та повторно прочитаним CSV.gz. Експорт: UTF-8, без header, LF, gzip level 1,
mtime 0; literal CRLF/лапки зберігаються всередині CSV-полів.
Compressed-file hashes не повинні збігатися з оригінальними архівами.

[validate.py](validate.py) окремо перевіряє фактичний стан dataset/pointer,
відсутність publication events та кожне ledger-посилання за artifact/ordinal.
Експериментальний manifest/fingerprint не є production canonical manifest.
Dataset залишається `validating`, active pointer — NULL, generation — 0.

## Відтворення

Запускати з кореня репозиторію, використовуючи **нові** output paths:
інструменти відмовляються перезаписувати результати. Потрібні CSV та локальні
артефакти попередніх проб; вони не входять у чистий checkout.

```sh
python3 -m unittest discover -s docs/_assessments/2026-10-24-provenance -v
python3 docs/_assessments/2026-10-24-provenance/probe.py \
  ../slovnyk1.0/csv data/experiments/sqlite-full/corpus.sqlite3 \
  docs/_assessments/2026-10-24-csv-audit/result.json \
  docs/_assessments/2026-10-24-sqlite-full/result.json \
  data/experiments/provenance-new/model.sqlite3 data/experiments/provenance-new/exports \
  data/experiments/provenance-new-result.json
```

Після закриття БД — [load.py](load.py), 100 SQL-запитів із частотою 10/с,
4 worker slots, 80% exact із misses / 10% prefix / 10% infix, LIMIT 51,
samples із чотирьох найбільших пар. TEMP view лише адаптує назви колонок
повної моделі до запитів попереднього експерименту. Latency включає queue.
Progress callback перериває SQL після приблизно 2 s, але не є жорстким
wall-clock I/O timeout. Результати помилок також входять до latency metrics.

```sh
python3 docs/_assessments/2026-10-24-provenance/analyze.py \
  data/experiments/provenance-new/model.sqlite3 data/experiments/provenance-new-analyze.json
python3 docs/_assessments/2026-10-24-provenance/validate.py \
  data/experiments/provenance-new/model.sqlite3 data/experiments/provenance-new-result.json \
  docs/_assessments/2026-10-24-csv-audit/result.json \
  data/experiments/provenance-new-validation.json
python3 docs/_assessments/2026-10-24-provenance/load.py \
  ../slovnyk1.0/csv data/experiments/provenance-new/model.sqlite3 \
  data/experiments/provenance-new-load.json --queries 100 --rate 10
```

Обмежений запуск — через локальний `systemd-run --user --wait --pipe --collect`
з `MemoryMax=768M`, `CPUQuota=12.5%` та абсолютними шляхами до того самого
скрипта, CSV, БД і нового output JSON. Це 768 MiB та 1/8 **одного CPU Ryzen**;
не еквівалент конкретного Oracle OCPU. Фактичні `memory.max`/`cpu.max`
записуються самим процесом у result JSON. Transient unit видаляється після
завершення; налаштування інших локальних сервісів не змінюються.

Реальні БД та exports зберігаються лише в ignored `data/experiments/provenance/`.
У репозиторії — інструменти, synthetic tests та результати без словникових рядків.

## Виявлена проблема планувальника

Перший [load без statistics](load-unrestricted.json): 100/100 запитів
перервані progress callback, elapsed 50.302 s, сукупний p95 із queue
38 802.286 ms. Це **невдала** проба, не показник швидкості успішного пошуку.
EXPLAIN обрав primary-key scan лише за dataset_id навіть для exact,
ігноруючи більш селективний наявний exact index. Прямий dataset parameter
замість scalar subquery не виправив план; проблема не лише TEMP view.

[analyze.py](analyze.py) виконує локальний `ANALYZE` та записує плани до/після
в [analyze-result.json](analyze-result.json). Змінюються лише SQLite optimizer
statistics, не словникові рядки, schema contracts чи historical artifacts.
Перший невдалий result збережено; повторні workloads мають окремі JSON.
Перед використанням такого layout потрібні актуальні statistics і перевірка
реальних планів. Самого створення індексів виявилося недостатньо.

Повний `ANALYZE` зайняв 31.518 s і додав одну SQLite page (4 KiB).
Exact після нього використовує всі чотири колонки exact index; prefix/infix
сканують лише потрібну мовну пару через цей index та сортують у temp B-tree.
Без covering source-key index навіть prefix тут не має range access.

## SQL-навантаження після ANALYZE

| 100 queries, offered 10/s | Без quota | 768 MiB / 12.5% CPU |
|---|---:|---:|
| Elapsed | 10.236 s | 12.189 s |
| Exact p95, queue включено | 1.726 ms | 889.720 ms |
| Prefix p95 | 442.363 ms | 2 288.747 ms |
| Infix p95 | 432.987 ms | 2 288.881 ms |
| Сукупний p95 | 430.974 ms | 2 194.194 ms |
| Помилки | 0/100 | **20/100** |

Докази: [без quota](load-analyzed.json), [із quota](load-constrained.json).
У constrained run всі 20 mask-запитів перервані deadline callback; exact
завершилися, але queue виросла. Mask latency тут — час до помилки,
а не успішна відповідь. Фактичні cgroup limits: `cpu.max=12500 100000`,
`memory.max=805306368`. Цей layout із pair scans не підтвердив придатність
для малого CPU. Потрібні source-key/covering index та повторний workload
або інший ресурсний профіль. Жоден запуск не є проходженням AT-16/HTTP SLO.

## Перевірки інструментів

Чотири synthetic tests пройшли: raw Unicode/whitespace/CRLF round trip,
відсутність publication, cross-dataset FK rejection, зупинка на hash collision,
length-prefixed tuple encoding. Команда й hashes —
[synthetic-validation.json](synthetic-validation.json).
Окрема перевірка ledger та фактичного publication state —
[validation.json](validation.json): усі 56 455 068 ledger records у 992 файлах
мають правильний ordinal → translation_id mapping; 9.393 s, active=NULL,
generation=0, publication events=0. Це не production acceptance tests.

## Що ще не доводить ця проба

Не перевірені production parser/retry/checkpoints, duplicates/reject quarantine,
crash recovery, immutable-state enforcement, atomic publish/CAS, GC,
production export path/fsync protocol, off-host backup та повний restore сервісу.
Файли експорту локальні, не опубліковані. Одночасні retained/staging datasets
не створювалися. Семантична якість перекладів і історична повнота поза
доступним корпусом не оцінюються.

Навантаження SQL на Ryzen із неконтрольованим кешем ОС не відтворює Oracle
CPU/I/O чи сусідні процеси. Локальний cgroup limit також не дає такої гарантії:
спільні cached pages можуть обліковуватись поза cgroup. HTTP/rendering,
cursors, bot traffic та погоджений acceptance workload ще потрібні.

## Розташування локальних даних

2026-10-24 БД та exports перенесені з `tmp/` до `data/experiments/`.
Поточні шляхи й команди вище оновлено; вміст даних та результати вимірів
збережені. Опис директорій — [data/README.md](../../../data/README.md).
