# Slovnyk

Перенесення багатомовного словникового сервісу на нову реалізацію на Go
з окремою новою БД, наповненою даними старого сервісу. Старий код і дані
доступні у `../slovnyk1.0/`; структура нової БД може відрізнятися.

Джерело істини та порядок читання: [docs/INDEX.md](docs/INDEX.md).

Підтверджені рішення та наступні питання: [журнал](docs/DECISIONS.md).
Розробка локально; кандидат для запуску в Oracle Cloud досліджено read-only:
[оцінка сервера й масштабу даних](docs/_assessments/2026-10-23-server/REPORT.md).

Технічні перевірки 2026-10-24: [аудит усіх CSV](docs/_assessments/2026-10-24-csv-audit/REPORT.md)
та [SQLite на повному корпусі](docs/_assessments/2026-10-24-sqlite-full/REPORT.md).
[Повна модель із provenance](docs/_assessments/2026-10-24-provenance/REPORT.md):
усі записи й 992 експорти звірені; 25.54 GiB, виявлено обмеження диска/CPU.
[Оптимізація SQLite-індексу](docs/_assessments/2026-10-24-sqlite-indexed/REPORT.md):
29.66 GiB, 300 SQL-запитів при 10/с під CPU quota без помилок;
storage budget та перевірка HTTP/цільового сервера залишаються відкритими.
[Компактні фізичні ключі](docs/_assessments/2026-10-24-sqlite-compact/REPORT.md):
20.85 GiB (−29.70%), всі translations/ledger/origins звірені,
300 SQL-запитів під quota без помилок. Це експериментальні БД, не реалізований сервіс.

Локальні БД та експорти: `data/experiments/` (ignored);
[розташування даних](data/README.md).

Початок реалізації: [план M1](docs/IMPLEMENTATION_PLAN.md).
Локальна інструкція агенту — `tmp/CODEX_START.md` (ignored, не входить у checkout).

## Локальний запуск

Потрібні Go **1.26.10** і Make; для окремої dev/test PostgreSQL — Docker із
Compose v2+. Module path `github.com/valpere/slovnyk` відповідає Git remote.
Unicode NFC/full case folding використовують `golang.org/x/text v0.43.0`,
закріплений у `go.mod` / `go.sum` (Unicode 15.0.0 для Go 1.26).
PostgreSQL driver — `github.com/jackc/pgx/v5 v5.11.0`; транзитивні
залежності також закріплені в module files.
Go закріплено в `go.mod` і `.go-version`; стандартна бібліотека забезпечує
HTTP, JSON-логи й завершення процесу.
[Go 1.26.10](https://go.dev/doc/devel/release#go1.26.10) включає виправлення безпеки.

```sh
make build
./bin/slovnyk serve
# В іншому терміналі:
curl -i http://127.0.0.1:8080/healthz
curl -i http://127.0.0.1:8080/readyz
```

`/healthz` → 200; без storage config `/readyz` → 503 із `storage_not_initialized`.
З read-only HTTP DSN readiness перевіряє schema, active dataset та artifacts
(інструкція нижче). Без published dataset форма доступна, пошук і словникові
дані повертають 503.
Ctrl+C або SIGTERM припиняє приймання запитів і очікує активні до 10 s;
перевищення deadline закриває з'єднання та завершує процес із кодом 1.
Неправильна команда/config → exit 2; коректне завершення → exit 0.

За потреби скопіюйте `.env.example` у `.env`, відредагуйте й завантажте явно:

```sh
cp .env.example .env
set -a
. ./.env
set +a
make run
```

Застосунок читає `SLOVNYK_HTTP_ADDR` (default `127.0.0.1:8080`),
`SLOVNYK_LOG_LEVEL` (`debug/info/warn/error`, default `info`) та
`SLOVNYK_SHUTDOWN_TIMEOUT` (додатна Go duration, default `10s`).
`.env` автоматично читає лише Compose; Go-процес використовує environment.
Логи — JSON у stderr, без access log і текстів пошуку. HTTP та CLI використовують окремі явні DSNs; credentials не логуються.

## Окрема dev/test БД

```sh
make dev-db-up
docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
make dev-db-down
```

Це новий локальний контейнер, volume та мережа проєкту `slovnyk-local-dev`,
без доступу до старих даних. PostgreSQL **17.11 Alpine** закріплено також за
image digest у [compose.yaml](compose.yaml): відтворювана PostgreSQL baseline,
остаточний production engine ще відкритий.
[Release notes](https://www.postgresql.org/docs/release/17.11/).
Порт — лише `127.0.0.1:55432`; змінюється через `SLOVNYK_DEV_DB_PORT`.
Облікові дані в example призначені для локальної розробки. `dev-db-down`
зберігає volume.

### Міграції нової БД

Лише для окремої нової dev/test БД, без legacy DSN:

```sh
make build
export SLOVNYK_MIGRATION_DSN='postgres://slovnyk_dev:local-development-only@127.0.0.1:55432/slovnyk_dev?sslmode=disable'
./bin/slovnyk migrate
unset SLOVNYK_MIGRATION_DSN
```

Приклад відповідає незміненим Compose defaults; пароль призначений лише для
локального середовища. Міграційний login повинен володіти новою схемою і мати
`CREATEROLE` для першого створення груп. Міграції з [migrations/](migrations/)
вбудовані в binary, виконуються атомарно під advisory lock і перевіряють SHA-256
історії; застосовані SQL не редагувати. Ліміт команди — 45 s.

Схема `slovnyk` має групи `slovnyk_http` і `slovnyk_operator` без LOGIN/паролів.
HTTP читає лише runtime tables; оператор імпортує/публікує, але не змінює DDL
чи migration ledger. Окремі LOGINs і членство створює адміністратор; migration
login не використовувати для HTTP. Небезпечні наявні групи відхиляються.
Жодних автоматичних down migrations, GC або видалення datasets немає.

Для HTTP storage задайте разом `SLOVNYK_HTTP_DSN` (LOGIN групи `slovnyk_http`)
і `SLOVNYK_ARTIFACT_ROOT`, а також cursor keyring (див. нижче). На startup, recovery та новій generation перевіряються
schema, ready report, hashes і round-trip усіх активних exports. До успіху readiness — 503.
Pointer перевіряється щосекунди; втрата БД скидає readiness. HTTP mount artifacts
має бути read-only, final files незмінні, GC вимкнений. Перевірені bytes
кешуються до recovery/нової generation, тому зовнішні зміни файлів заборонені.

## Інвентаризація та staged import

Спершу підготуйте manifest V1 за [контрактом](docs/IMPORT_CONTRACT.md) і
незмінну директорію snapshot. Synthetic приклад без БД:

```sh
make build
mkdir -p tmp/import
./bin/slovnyk import inspect --manifest testdata/import/manifest.json --source testdata/import > tmp/import/inspect.json
```

Після `migrate` на окремій новій БД задайте `SLOVNYK_OPERATOR_DSN` для
окремого LOGIN із правами групи `slovnyk_operator` і database TEMP:

```sh
./bin/slovnyk import run --manifest testdata/import/manifest.json --source testdata/import --batch-size 100 > tmp/import/run.json
```

DSN передається лише environment. `inspect` звітує parsed/rejected occurrences,
без distinct counts (`distinct_counts_available=false`). `run` звітує
unique/duplicate/rejected, completion, fingerprint, dataset/attempt IDs та issues.
JSON — stdout; exit 0 — staging виконано, 1 — failure, 2 — аргументи/config/manifest.
Exit 0 із rejected records не є погодженням втрат або готовністю до публікації.

Типовий пакет — 100 records, дозволено 1–1000, додатковий ліміт decoded fields
4 MiB. Повторіть ту саму команду після interruption: fingerprint і committed
prefix перевіряються, origins не дублюються. Lock wait — до 5 s. Для terminal
`failed` потрібен новий manifest/версія правил; source не змінювати в процесі.
Успіх лишає набір у `validating`, active dataset незмінний;
[реалізація й результати](docs/_assessments/2026-10-24-staged-import/REPORT.md).

## Verify, exports та публікація

Потрібні operator DSN і незмінний snapshot, використаний для import. Усі flags
ідуть після dataset ID. Artifact root — окрема надійна локальна filesystem
з hard links і fsync (Linux baseline); не object/network storage.

```sh
go run ./utilities/prepare-artifacts data/artifacts
./bin/slovnyk dataset verify <dataset-id> --source testdata/import --artifacts data/artifacts
./bin/slovnyk dataset export <dataset-id> --artifacts data/artifacts
./bin/slovnyk dataset publish <dataset-id> --artifacts data/artifacts --expected-generation 0 --operator-ref local-owner
./bin/slovnyk dataset rollback <previous-id> --artifacts data/artifacts --expected-generation <current-generation> --operator-ref local-owner
```

`verify` звіряє source hashes/EOF, ledger, origins, metadata та counters; створює
окремий lossless CSV4.gz для кожної пари й immutable JSON report, тоді ставить
`ready`. Файли й батьківські directories синхронізовані до DB commit. Повторний
verify/export ready dataset повертає той самий перевірений inventory, без заміни
bytes. Збій до commit лишає `validating` і допустимі orphan files; повторіть verify.
Root і артефакти не видаляти та не перезаписувати; `data/artifacts/` ignored.

`publish`/`rollback` вимагають фактичну поточну generation із `slovnyk.active_dataset`;
0 — лише перша публікація. Конфлікт повертається без зміни pointer/history.
Rollback дозволено лише до раніше опублікованої збереженої версії. JSON — stdout;
exit 0 — успіх, 1 — невдала операція, 2 — неправильні аргументи/відсутня config.
Нині будь-які rejected records, issues, known gaps чи exceptions блокують ready:
адаптер окремого погодження власником звіту втрат ще попереду, `force ready` немає.
Це synthetic M1; повний Go corpus import та production durability не перевірені.
[Реалізація, рев’ю та обмеження](docs/_assessments/2026-10-24-publication/REPORT.md).

## Посіпаки й тимчасові файли

Утиліти для роботи сервісу, доступні всім, живуть у [utilities/](utilities/README.md).
Проєктні перевірки для людей і CI — у [tests/](tests/README.md).
[minions/](minions/README.md) містить виключно особистих помічників агента;
сервіс і CI від них не залежать. Мова реалізації не визначає категорію.
Python runtime для цих команд більше не потрібний. Тимчасові речі — `tmp/`. Дані експериментів
залишаються у постійному ignored `data/experiments/`. Історичні інструменти
у `docs/_assessments/` зберігаються разом зі своїми звітами.

## Перевірки

| Команда | Дія |
|---|---|
| `make build` | Збирає `bin/slovnyk` |
| `make run` | Запускає HTTP через `go run` |
| `make test` | `go test ./...` |
| `make check` | Перевіряє gofmt, vet, whitespace staged/unstaged diff, активні Markdown-посилання та Git ignore локальних даних |
| `make test-smoke` | Збирає та перевіряє HTTP-процес, health/readiness і SIGINT/SIGTERM |
| `make test-race` | `go test -race ./...`; потрібен C compiler |
| `make test-integration` | Створює окрему PostgreSQL, запускає schema/import/retry/publication та наскрізний CLI/HTTP lifecycle, видаляє власні ресурси |

Integration target використовує унікальний Compose project, synthetic
credentials і випадковий loopback port; до наявної БД не підключається.
`-tags integration` tests окремо вимагають явний `SLOVNYK_TEST_DSN`;
використовуйте `make test-integration`, який задає його лише для власної disposable БД.
Фактичні результати M1-01 — у [TRACEABILITY](docs/TRACEABILITY.md) та
[звіті](docs/_assessments/2026-10-24-go-scaffold/REPORT.md).

### Наскрізне відтворення на порожній БД

У чистому checkout із Go/Make, C compiler і Docker Compose виконайте:

```sh
make build check test test-race test-smoke
make test-integration
```

Runner сам створює контейнер на випадковому loopback port. Новий
`tests/serviceflow/TestServiceLifecycle` збирає binary в `tmp/serviceflow/`,
створює окрему порожню БД й два restricted LOGINs. Через справжній CLI
виконує migrate → synthetic import/verify/publish A; через HTTP перевіряє
exact/mask, статтю та CSV.gz із SHA-256/CSV перевіркою. Далі — stale CAS,
publish B, retained search/article/download A, rollback A, втрата з’єднань
і recovery readiness, restart із тим самим keyring. Прогін має завершитися
exit 0 та `PASS: empty DB …`; `-race` увімкнено для lifecycle test harness,
child service — звичайний build. Unit race tests виконуються окремо вище.

Сценарій видаляє власні процеси/БД/LOGINs/scratch, runner — container/volume.
Він не використовує `.env`, application DSNs, legacy, browser або Oracle.
Це synthetic M1-08 verification, не production/capacity чи мовне приймання.
[Перелік перевірок і обмежень](tests/README.md).

Оцінювання специфікації: [tumanomir, 2026-10-21](docs/_assessments/2026-10-21/REPORT.md).

Уточнення контрактів та незалежне рев’ю: [консиліум, 2026-10-21](docs/_assessments/2026-10-21-contracts/REPORT.md).

M1-01 реалізовано: Go-модуль, HTTP-каркас і локальне dev/test середовище.
M1-02 реалізовано: versioned normalization/mask compiler, lossless CSV4.gz,
strict manifest V1 і [синтетичні fixtures](testdata/README.md).
M1-03 реалізовано: схема PostgreSQL, міграційний CLI, constraints, ролі
та реальні transaction tests. M1-04 реалізовано: inspect/run CLI, staged import,
транзакційний ledger/provenance і retry. M1-05 реалізовано: reconciliation,
durable exports/report, publication/rollback CAS та HTTP readiness.
M1-06 реалізовано: application service exact/mask і підписана cursor-пагінація.
M1-07 in progress: HTTP lookup, каталог, локалізація й налаштування форми реалізовані; synthetic downloads і словникові SEO-сторінки реалізовані; повний sitemap/crawl acceptance ще відкриті.
M1-08: наскрізне synthetic CLI/HTTP відтворення реалізовано; M1 загалом ще не
закрито через відкриті мовне/дизайн-приймання та повний SEO discovery.
Старий проєкт залишається незмінним; Oracle не задіяний.

## Рев’ю, коміти та CI

Порядок роботи: [review → PR → CI](docs/CONTRIBUTING.md).
Після кожного завершеного етапу: рев’ю змін, потрібні локальні перевірки,
сфокусований локальний commit. Результат рев’ю та обмеження зберігати разом
із документацією етапу; виправлення повторно перевіряти до commit.

[GitHub Actions CI](.github/workflows/ci.yml) запускає на push/PR та вручну
`make check`, unit/race tests, HTTP process smoke і disposable PostgreSQL
integration tests. Go береться з `.go-version`, actions закріплені за SHA;
workflow має лише `contents: read`, без secrets, production чи browser доступу.
Перевірка committed diff виключає історичні `docs/_assessments/`, bytes яких
потрібні для відтворення hashes; активні документи перевіряються окремо.
Зелений CI не замінює рев’ю; branch protection не налаштовується цим workflow.
[Рев’ю початкової реалізації](docs/_assessments/2026-10-24-bootstrap-review/REPORT.md).

## Пошук і cursor: реалізований service layer

`internal/lookup.Service.Search` приймає typed request і caller-owned PostgreSQL
connection під HTTP read-only role. Exact/mask, C sort/seek, raw grouping,
retained dataset pin і спільний двосекундний SQL deadline перевірені на synthetic
даних. `serve` надає `/` та `/search`: HTML-форму без JS і `format=plain`.
`/languages` показує наявні напрямки та кількість перекладів. UI має 31
вбудований [каталог перекладів](web/README.md): придатні PO-фрази та нові
AI-переклади, поки зі статусом `ai-draft`; мовне приймання OQ-06 відкрите.
Явний `ui` → збережений ручний вибір → мова браузера → `en`. Ручний UI
і остання наявна пара зберігаються окремими cookies на рік, без JS. Форма
пропонує browser → English або English → Ukrainian лише за наявності
однозначного напрямку; `/search` завжди потребує явної пари. Недоступна
збережена пара показує підказку вибрати іншу. Словникові сторінки та downloads описано нижче; повний sitemap ще не реалізовано.

`lookup.NewSigner` приймає active key ID і keyring: 1–16 ключів по 32–64 bytes;
використовуйте випадкові секретні ключі з щонайменше 256 bits entropy.
Попередні ключі зберігайте протягом підтримки їхніх cursors. З HTTP storage
обов'язково задайте `SLOVNYK_CURSOR_ACTIVE_KID` і `SLOVNYK_CURSOR_KEYS`:
список `kid:hex,kid:hex` без пробілів, 32–64 decoded bytes на ключ.
Для нового секрету: `openssl rand -hex 32`; не комітьте результат.
Наприклад, keyring із новим і попереднім ключами: `v2:<hex>,v1:<hex>`,
active KID — `v2`. Synthetic ключ golden fixture не призначений для запуску.
Cursor містить ID останнього включеного запису, без raw texts, максимум 2048 bytes.

Service резервує 896 KiB для escaped entries і 128 KiB для UI. Renderer
буферизує весь HTML/plain body до headers, не більше 1 MiB; перевищення
повертає коротку 503, без часткових записів. HTTP backend обмежує одночасні
search/catalog connections до 8; slot, connect, lookup і каталог ділять deadline 2 s.
Typed `Limit=0` означає default 50; explicit HTTP `limit=0` відхиляється.
Raw request-target понад 32 KiB → 414; невалідні/дубльовані параметри → 400.
Search responses мають `no-store` і `noindex, follow`; probes залишаються JSON.
Без storage працюють локалізована форма й health; непорожній пошук, каталог
і readiness → 503. Порожня форма залишається 200 з повідомленням про каталог.
`make test-integration` включає lookup tests; bounded fuzz:

```sh
go test ./internal/lookup -run '^$' -fuzz '^FuzzCursor$' -fuzztime=10s -parallel=2
```

[Рев’ю та перевірки M1-06](docs/_assessments/2026-10-24-lookup/REPORT.md).
Це synthetic coverage, без підтвердження повного corpus чи production p95.

## Завантаження словників

`/downloads` показує версію активного набору, наявні напрямки, кількість CSV
записів, compressed bytes, SHA-256 і посилання на окремі CSV4.gz. Доступний
також `?format=plain`. Локалізація сторінки й ручні preferences працюють без JS;
шість нових фраз доповнено ШІ в усіх 31 каталогах зі статусом ai-draft.

Файл адресований `/downloads/<dataset-id>/<export-id>`; URL лишається чинним
для retained published версії після наступної публікації. Немає raw-path route,
архіву всього корпусу або доступу до непублікованих наборів. Файли видаються
як gzip attachments, з ETag/SHA-256, HEAD і byte-range resume. DB connections
звільняються до streaming; окремо до 8 передач із поновлюваним 30 s write
budget на chunk. Великі файли не підпадають під 1 MiB page limit.

Downloads потребують readiness і immutable read-only artifact mount. Перевірки
hashes виконуються publication/startup/recovery, без повторного full-file scan
на кожне завантаження; file open перевіряє confinement/type/size. Деталі й
помилки — [DOWNLOAD_CONTRACT](docs/DOWNLOAD_CONTRACT.md). `make test-integration`
перевіряє synthetic downloads/CSV/hash/retained URLs під HTTP role; production
corpus, capacity та restore цим зрізом не підтверджено.

## Постійні словникові сторінки

`/word/<entry-id>` показує одне raw-написання та переклади однією мовою
призначення. Посилання є в заголовку кожної групи результатів пошуку.
64-character hash враховує raw source, from і to; URL лишається сталим між
публікаціями. Bare URL читає active dataset; signed continuation pin-ить
retained dataset і має noindex. Cookies/Accept-Language не змінюють статтю.
Canonical використовує `https://slovnyk.org`, UI визначається source code з
fallback en. Ручний вибір UI зі статті веде на головну сторінку.

Після оновлення binary виконайте `migrate`: фізична schema version 2 додає
expression index, не змінюючи V1 manifest/raw data. HTTP не мігрує автоматично.
`/robots.txt` дозволяє crawling, `/search` залишається noindex. Додатковий
host `slovnyk.org.ua` отримує 308 на fixed primary origin зі збереженням path
і query. Налаштування TLS/edge перевіряються при deployment.

Це synthetic M1 subset FR-14 / AT-19: повний offline sitemap, crawl/resource
budget і production acceptance ще відкриті. [Контракт та джерела](docs/SEO_PLAN.md).
