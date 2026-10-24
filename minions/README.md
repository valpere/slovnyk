# Посіпаки

Регулярні локальні перевірки та допоміжні скрипти проєкту. Запускаються
з будь-якої директорії; шляхи до репозиторію визначають за своїм розташуванням.
Потрібен Python 3.10+ (лише стандартна бібліотека), для PostgreSQL — Docker Compose.
Make targets запускаються з кореня репозиторію.

| Посіпака | Запуск | Призначення |
|---|---|---|
| `check-patch.sh` | `make check` або `./minions/check-patch.sh [base-commit]` | Whitespace staged/unstaged і committed CI diff; історичні assessments зберігаються byte-for-byte |
| `check-repository.py` | `make check` або `python3 minions/check-repository.py` | Активні Markdown-посилання, кінцеві пробіли, Python syntax посіпак, ignore локальних даних, доступність scripts для Git |
| `smoke-http.py` | `make test-smoke` або `python3 minions/smoke-http.py` після build | Власний процес на випадковому loopback port; GET/HEAD health, readiness, SIGTERM/SIGINT, відсутність query text у lifecycle logs |
| `prepare-artifacts.py` | `python3 minions/prepare-artifacts.py data/artifacts` | Створює root і fsync ancestor directories до verify; не змінює наявні files |
| `test-postgres.sh` | `make test-integration` | Ізольований PostgreSQL: version, migrate CLI, schema/privileges/transaction/import/retry/publication/lookup/readiness/CLI tests; видалення власних контейнера/мережі/volume |

Успіх → exit 0, невдала перевірка → ненульовий exit. JSON останніх запусків:
`tmp/minions/repository-check.json` і `tmp/minions/http-smoke.json`;
вони перезаписуються й не входять у Git. HTTP minion завершує тільки власний
процес. PostgreSQL minion не використовує application/legacy DSN.

Markdown-перевірка за замовчуванням охоплює `README.md`, `data/README.md`,
цей файл, `testdata/README.md` і `docs/*.md`. Історичні snapshots у `docs/_assessments/` не входять:
їхні посилання можуть відображати старе розташування. Окремі документи можна
передати явно (шляхи від кореня репозиторію):

```sh
python3 minions/check-repository.py docs/INDEX.md docs/TRACEABILITY.md
```

Перевіряється існування локальних цілей посилань, без мережі й перевірки anchors.
HTTP smoke без storage config перевіряє readiness=503,
`storage_not_initialized`; storage readiness/recovery — окремий Go integration test.
Він не перевіряє пошук, graceful drain активного запиту чи production capacity;
drain/deadline перевіряють Go unit tests. PostgreSQL tests перевіряють
schema/transactions, staged import/retry, verify/export/publication/CAS і
lookup/cursor/deadline і readiness recovery під HTTP role. Жоден
посіпака не підключається до Oracle. Для PostgreSQL runner також потрібен Go;
його DSN завжди генерується для власного контейнера, без fallback на наявний.

Нові одноразові скрипти й fixtures створювати в `tmp/`. Якщо скрипт повторно
використовується або придатний для цього — перенести до `minions/`, описати
в цій таблиці й за потреби додати Make target. Довговічні БД залишаються
в `data/experiments/`; історичні докази й hashes у звітах не переписувати
після перенесення інструментів. `test-postgres.sh` перенесено зі `scripts/`;
inline Markdown/ignore та process-smoke перевірки оформлено окремими посіпаками.

Schema, importer, publication, lookup, ops та CLI suites запускаються послідовно: schema tests
перебудовують лише власну disposable схему. Healthcheck використовує TCP,
щоб не прийняти тимчасовий Unix-socket init-сервер за готову БД.
