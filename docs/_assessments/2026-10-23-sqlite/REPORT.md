# Локальна перевірка SQLite як кандидата для Slovnyk

Дата: 2026-10-23. Користувач запропонував спробувати SQLite для слабкого
Oracle Micro. Остаточний engine не обрано; сервер не змінювався.

## Передумови

Публічний сервіс читає опубліковані дані; операторський імпорт — окремий процес.
Погоджено p95 звичайного пошуку <=1 s серверної обробки та загальний DB deadline
2 s. Орієнтовний історичний пік — до 10 запитів/с; профіль навантаження ще відкритий.

SQLite документує застосування для вебсайтів і підтримку багатьох читачів
при одному writer. Це відповідає напрямку дослідження, але не доводить capacity
конкретної машини: [appropriate uses](https://www.sqlite.org/whentouse.html).

## Метод

Локальний Python 3.12.3 / SQLite 3.45.1, Unicode tables 15.0.0, x86-64 Linux.
Використано чотири найбільші за стисненим розміром CSV.gz. Створено окрему
SQLite-базу у `tmp/sqlite-probe/`; старі файли не змінювалися. Експеримент
не встановлює Go driver або production normalization version.

Нова таблиця зберігає source/target locale, raw source/target, нормалізований
ключ NFC → full casefold → NFC, SHA-256 ключа та integer ID. Базові індекси:
pair та pair+key_hash; exact перевіряє residual equality. Додатково випробувано
індекс `(sl,tl,key,source,target,id)` з BINARY collation для seek/sort.
Це експериментальна схема без production provenance, deduplication і publication.

По 50 serial SQL-запитів exact/prefix/infix, розподілених між усіма чотирма
парами; LIMIT 51, стабільний порядок key/source/target/id. Параметризований
GLOB використано для mask-проби. Production compiler ще має довести весь
контракт, включно з буквальними brackets та Unicode wildcard cases.

SQLite cache — 8 MiB, mmap вимкнено, temp_store=FILE. ОС page cache не обмежено
й не очищено; це не cold-cache тест. Конкурентність і HTTP не перевірялися.

## Результати

Усього **1 284 617 записів**: en-us→fr-fr 358 927, en-us→de-de 360 698,
fr-fr→en-us 290 888, de-de→en-us 274 104. Імпорт у спрощену таблицю — 6.636 s;
цей час не включає створення базових індексів або production verify.

| Метрика | Базові hash/pair індекси | З додатковим lookup index |
|---|---:|---:|
| Розмір файлу | 220 966 912 bytes (~211 MiB) | 309 301 248 bytes (~295 MiB) |
| Exact p95, лише SQL | 0.046 ms | 0.043 ms |
| Prefix p95, лише SQL | 35.870 ms | 0.084 ms |
| Infix p95, лише SQL | 40.073 ms | 27.756 ms |

Peak RSS Python-процесу — 40 428 KiB (~39.5 MiB); це не повний бюджет
Go HTTP-сервісу, не пам'ять ОС cache і не характеристика Oracle Micro.
Повторне відкриття завершеної БД з `mode=ro&immutable=1` успішне, кількість
записів збігається. Це не перевірка backup/restore або concurrent publication.

EXPLAIN підтвердив: hash index для базового exact; pair scan + temporary sort
для базових масок; key range для prefix з lookup index; pair scan по covering
index для infix. Додавання B-tree не усуває scan довільної infix-маски.
Вплив індексів узгоджується з [SQLite query planner](https://www.sqlite.org/queryplanner.html).

## Висновок та межі

SQLite — перспективний кандидат для подальшої репетиції. Попередня рекомендація
шукати потужніший сервер стосувалася Go + PostgreSQL; на підставі цієї проби
Oracle Micro з SQLite поки не відкидається, але його придатність не підтверджена.
Не можна екстраполювати локальні latency/RSS на 56.5 млн записів, 1 GB RAM,
1/8 OCPU і 10 HTTP-запитів/с. Розмір спрощеної таблиці не є розміром повної БД
з origins, ledger, exports, staging та retained datasets.

Якщо обрати SQLite, потрібно конкретизувати:

- Побудову й verify окремої нової dataset-БД, незмінні versioned filenames
  та публікацію з generation CAS; не замінювати файл, який читається як immutable.
- Writer exclusion, atomicity checkpoint/origins та контроль publication
  замість PostgreSQL session advisory locks.
- Файлові права й read-only connections замість PostgreSQL DB roles;
  [`immutable=1`](https://www.sqlite.org/uri.html) допустимий лише для справді незмінного файла.
- Versioned Unicode normalization у застосунку, повну семантику masks,
  deadline/cancellation, довгі поля та їхній вплив на індекси.
- Backup/restore, crash durability, cursor pinning і повний provenance.

Зміна engine не скасовує поведінкових вимог. Go driver, schema і протокол
publication не обрано. Потрібна повна локальна репетиція, потім перевірка
на цільовій машині після окремого дозволу на розгортання.

## Відтворення

Скрипт відмовляється перезаписувати наявну БД. Запускати з кореня проєкту,
використовуючи нове ім'я файла:

```bash
mkdir -p tmp/sqlite-probe
python3 docs/_assessments/2026-10-23-sqlite/probe.py \
  ../slovnyk1.0/csv tmp/sqlite-probe/fresh.sqlite3 \
  > tmp/sqlite-probe/fresh-result.json
```

Зафіксований результат: [result.json](result.json). Великий SQLite-файл —
відтворюваний локальний артефакт у ignored `tmp/`, не матеріал репозиторію.
