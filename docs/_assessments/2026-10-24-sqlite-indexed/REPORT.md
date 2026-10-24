# SQLite: індекс для пошуку на повній моделі — 2026-10-24

## Мета та метод

Продовження [повної проби provenance](../2026-10-24-provenance/REPORT.md):
перевірити маски під локальним CPU quota після додавання ordered covering
index. У попередньому layout усі 20 mask queries зі 100 завершилися deadline
помилкою під quota. Дані містять 56 455 068 перекладних зв’язків.

Усе виконується локально: AMD Ryzen 7 7700, 16 logical CPUs, ~61 GiB RAM,
Python 3.12.3 / SQLite 3.45.1. Oracle не задіяний. Базова БД
`data/experiments/provenance/model.sqlite3` залишається без змін.
Нова окрема копія: `data/experiments/provenance-indexed/model.sqlite3`.
[prepare.py](prepare.py) перевіряє SHA-256 копії до зміни індексів,
додає індекс та виконує `ANALYZE lookup_order`:

```sql
CREATE INDEX lookup_order ON translations(
    dataset_id, source_locale, target_locale,
    source_key COLLATE BINARY, source_text COLLATE BINARY,
    target_text COLLATE BINARY, id
);
```

Логічні рядки, FK, ledger/origins, raw-написання й правила нормалізації
зберігаються. Dataset залишається `validating`, active=NULL/generation=0;
publication не виконується. Експорти залишаються у базовій директорії,
не копіюються повторно. Новий індекс — фізична оптимізація експерименту,
не затверджена production migration чи остаточний вибір engine.
PostgreSQL baseline у DATA_MODEL залишається чинною; цей SQLite text index
та GLOB не переносити до неї як готову реалізацію контракту масок.

## Обсяг і SQL-плани

[prepare-result.json](prepare-result.json): копія перед змінами має однаковий
SHA-256 із базовою БД:
`126572105baf8ef7030559a81a58f03820ed92be04bb136f7486b95ef57ded75`.
Після index build підтверджено 56 455 068 translations та незмінний
publication state. Повний source/DB/export reconciliation базової проби
не повторювався; ця операція додає лише index та optimizer statistics.

| Метрика | Результат |
|---|---:|
| Базова БД | 27 424 055 296 bytes / 25.54 GiB |
| БД з lookup_order | 31 848 861 696 bytes / **29.66 GiB** |
| Приріст | 4 424 806 400 bytes / **4.12 GiB** |
| Копіювання, fsync та перевірка SHA-256 | 368.191 s |
| Побудова індексу | 103.513 s |
| ANALYZE нового індексу | 6.845 s |

EXPLAIN: exact використовує `lookup_order` за pair/source_key із residual
hash check; prefix — covering range за source_key; infix — covering scan
лише потрібної мовної пари. Temporary ORDER BY B-tree більше не потрібний.
Довільна infix-маска залишається лінійним scan пари, не стає range lookup.

Два datasets цього layout — **59.32 GiB**, три — **88.98 GiB**, без exports,
інших робочих файлів та ОС. Індекс поліпшує пошук ціною збільшення storage;
~41 GiB free з датованого Micro snapshot не вміщують навіть дві такі версії.
Це розрахунок за однією виміряною БД, не репетиція multi-dataset publication.

## Профіль вимірювання

Використовується незмінений [load.py](../2026-10-24-provenance/load.py)
попередньої проби: samples з тих самих чотирьох найбільших пар, 80% exact
із misses / 10% prefix / 10% infix, offered rate 10/s, 4 worker slots,
LIMIT 51, той самий ORDER BY, TEMP view та progress callback ~2 s.
Latency включає scheduling/queue; помилки також входять у метрики.
Cache 8 MiB/connection, mmap=0, temp_store=FILE; кеш ОС не очищався.
Unrestricted run — 100 запитів; constrained — 300. Попередня constrained
проба без covering index мала 100 запитів; типи, samples, rate й ліміти
ті самі, але кількість спостережень різна. Constrained run послідовно
виконаний після unrestricted, не паралельно з index build чи копіюванням.

Обмеження локального transient systemd unit: `MemoryMax=768M`,
`CPUQuota=12.5%`. Це 1/8 одного CPU Ryzen, не відтворення Oracle OCPU.
Shared cached pages можуть бути charged поза cgroup. Тут немає HTTP,
UI rendering, cursor pagination, ботів або погодженого acceptance workload.
Це короткий SQL experiment із теплим кешем, не проходження AT-16.

## Результати навантаження

| Метрика, queue включено | Без quota, 100 queries | 768 MiB / 12.5% CPU, 300 queries |
|---|---:|---:|
| Elapsed | 9.922 s | 30.005 s |
| Exact p95 | 1.554 ms | 0.395 ms |
| Prefix p95 | 0.440 ms | 0.302 ms |
| Infix p95 | 31.235 ms | 202.129 ms |
| Сукупний p95 | 25.749 ms | **105.955 ms** |
| Максимум | 31.235 ms | 202.677 ms |
| Помилки | 0/100 | **0/300** |

Докази: [load-unrestricted.json](load-unrestricted.json),
[load-constrained.json](load-constrained.json). Обмежений процес сам зафіксував
`cpu.max=12500 100000`, `memory.max=805306368`.
На цьому профілі covering index усунув попередні mask timeouts;
всі 30 prefix та 30 infix запитів завершилися успішно.

SQLite залишається технічно можливим кандидатом для подальшої реалізації.
Вибір engine, compact layout і target hosting ще відкритий. Потрібні
production mask compiler/Unicode fixtures, HTTP та реалістичний workload,
включно з ботами й складними масками, publication/retry і off-host restore.
Індекс не вирішив storage budget active/staging/retained versions.
Підсумкові перевірки invariants, plans, cgroup limits, hashes інструментів
і документації зафіксовані у [checks.json](checks.json).

## Відтворення

З кореня репозиторію; output paths мають бути новими. Потрібні локальна
базова БД і CSV, які не входять у чистий checkout.

```sh
python3 docs/_assessments/2026-10-24-sqlite-indexed/prepare.py \
  data/experiments/provenance/model.sqlite3 \
  data/experiments/provenance-indexed-new/model.sqlite3 \
  data/experiments/provenance-indexed-new-prepare.json
python3 docs/_assessments/2026-10-24-provenance/load.py \
  ../slovnyk1.0/csv data/experiments/provenance-indexed-new/model.sqlite3 \
  data/experiments/provenance-indexed-new-load.json --queries 100 --rate 10
```

Для constrained run виконати останню команду через локальний
`systemd-run --user --quiet --wait --pipe --collect` із властивостями вище,
`--queries 300 --rate 10`, окремим output JSON, унікальною назвою unit
та абсолютними шляхами до скрипта/input/output.
Жодних операцій на сервері для відтворення цієї проби не потрібно.
