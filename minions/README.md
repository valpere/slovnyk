# Посіпаки агента

Посіпаки — виключно особисті помічники агента. Вони скорочують його
повторювані запити й дії до одного виклику та не є залежністю сервісу,
операторських процедур або CI. Мова реалізації категорію не визначає.

| Посіпака | Виклик агента | Призначення |
|---|---|---|
| `http-probe` | `go run ./minions/http-probe PRIVATE_CONFIG_JSON NEW_OUTPUT_DIRECTORY` | Обмежені loopback HTTP probes із чергою, latency та response hashes; без URL і тіл у результатах |
| `corpus-inventory` | `go run ./minions/corpus-inventory PRIVATE_JSON OUTPUT_JSON` | Агентська повна звірка 17 таблиць dev-БД і sequences через streaming binary COPY; лише metadata/hash output |
| `corpus-progress` | `go run ./minions/corpus-progress RUN_DIRECTORY MANIFEST_JSON` | Показує elapsed, free disk та останній спостережений файл локальної звірки; читає лише progress/manifest metadata, без DB-запитів |
| `check-repository` | `go run ./minions/check-repository` | Знаходить checkout і одним викликом запускає проєктну перевірку з докладним звітом |
| `ui-preview` | `go run ./minions/ui-preview [-port 8787]` | Loopback preview actual HTTP templates на fixed synthetic callbacks, без БД; для агентських UI/мовних review |
| `pr-status` | `go run ./minions/pr-status 36` | Read-only статус точного head PR: по два jobs push/PR, без merge чи rerun |
| `ui-review-cdp.mjs` | `node minions/ui-review-cdp.mjs http://127.0.0.1:8788 minions/ui-review.example.json tmp/ui-review-NEW` | Матриця сторінок/ширин: HTTP 200, html lang, overflow, optional full-page screenshots |
| `capture-cdp.mjs` | `node minions/capture-cdp.mjs http://127.0.0.1:PORT/path tmp/preview.png 390 844` | Знімок loopback preview через наявний Chromium CDP 9222; JS сторінки вимкнено, створює й прибирає лише власний ізольований context |

Потрібні Go та Git. Для виклику поза checkout можна зібрати binary у `bin/`;
root визначається за модулем у cwd/ancestors, шляхом binary або source.
Посіпака повертає exit status проєктної перевірки. Звіти — `tmp/tests/`.
Для CDP-посіпаки потрібен Node.js 22+ з built-in fetch/WebSocket та вже
запущений Chromium на loopback port 9222. Він не запускає browser process,
не використовує Playwright та не залежить від сервісу або CI. Output directory
має існувати; одноразові captures зберігайте в `tmp/`,
review screenshots можна включати до відповідного assessment, лише якщо дані synthetic.
`capture-cdp.mjs` відмовляється перезаписувати існуючий output файл.

Колишні Python-посіпаки переписано на Go та розділено за призначенням:
`prepare-artifacts` — [утиліта сервісу](../utilities/README.md);
repository/HTTP smoke — [проєктні тести](../tests/README.md).
Shell test runners також перенесено в `tests/`, оскільки їх використовує CI.

Нові особисті повторювані дії агента — тут; одноразові речі — в `tmp/`.
Історичні інструменти й hashes у `docs/_assessments/` не переписуйте.

`corpus-progress` читає `progress.jsonl` у вказаному локальному run directory.
Незавершений останній рядок пропускає. Номер файла відповідає порядку SHA-256
у manifest та є лише спостереженням, а не checkpoint або доказом завершення.
Після перезапуску використовуйте нову run directory; результати попередніх спроб
не перезаписуйте. Посіпака не запускає, не зупиняє й не змінює процеси.

`corpus-inventory` перенесений із повторно використаного scratch harness.
Це особистий діагностичний помічник агента, не команда сервісу/оператора чи CI.
Запускайте лише для явно обраної локальної dev-БД із зупиненими writers;
послідовні table reads не створюють спільного snapshot. JSON input (mode 0600)
містить `address`, `db`, `admin`, `admin_password`; не виводьте його вміст.
Output створюється ексклюзивно з mode 0600; новий запуск потребує нового filename.
Потрібні SELECT/COPY та читання sequence state. Процес має context deadline 12 h,
не зберігає raw rows і не змінює дані. Повне читання створює I/O навантаження;
не запускайте його на зайнятому Oracle. Count + sum/XOR row SHA-256 —
probabilistic multiset oracle, не canonical source reconciliation.

`http-probe` читає приватний JSON (mode 0600, до 2 MiB): `base` — числова loopback IP
з портом, `phase` — безпечна мітка, `workers` (1–16), `interval_ms` (0 або
50–10000), `timeout_ms` (1–10000), `maximum_seconds` (1–1800), `body_limit`
(до 8 MiB), `cases` і `schedule`. Кожен case має числовий `id`, безпечний
`kind`, приватний relative `path` та необов’язковий `expected` із `status`,
`bytes`, `sha256`, `content_type`, `cache_control`. `schedule` містить до
5000 case IDs. Реальні paths/терміни тримайте тільки в ignored inputs.

Інтервал задає заплановані arrivals; 0 ставить усі requests у bounded чергу
одразу. Результати розрізняють dispatch lag, client HTTP/body elapsed та час
від запланованого arrival до завершення; це не server-side timer. Keep-alive,
proxy й redirects вимкнені. Усі outcomes враховуються у nearest-rank p50/p95.
Нова output directory створюється з mode 0700, JSONL/summary — 0600; старі
результати не перезаписуються. Exit 0 означає повний облік schedule, а не
відсутність HTTP помилок: перевіряйте status/error counts та oracle matches.
Без `expected` hashes є спостереженням, а не доказом правильності відповіді.
Неповний запуск лишає partial JSONL без complete summary. Посіпака не керує
Docker/БД і не перевіряє їхні ресурси; caps, disk guard та cleanup задає агент.

`ui-preview` замінює повторно використаний scratch preview. Запускайте з root
checkout; адреса й synthetic article path друкуються одним JSON. Слухає тільки
`127.0.0.1`; порт 0 обирає вільний. Власні CSV.gz створює в новій `tmp/ui-preview-*`
і прибирає на SIGINT/SIGTERM; існуючих даних не читає й не змінює. Зупиняйте
саме свій процес. Примусове kill/reboot може залишити тільки ці scratch файли.

Синтетичні пари: `en-us → uk-ua` (`hello`), `uk-ua → en-us` (`привіт`),
`ar-sa → en-us` (`مرحبا`, RTL/multiline/довгий рядок). `*` показує fixture пари,
`missing` — no-match, `failure` — search 503; невідома пара — 404. Решта запитів
без збігів. Це fixed visual cases, **не** реалізація SQL search/masks/pagination,
readiness, sitemap чи capacity. Downloads віддають справжні, але synthetic CSV.gz
із узгодженими rows/hash/bytes. UI/localization/preferences/templates — actual
handler. Посіпака не є командою сервісу чи deployment procedure.

Для чистого першого візиту використовуйте окремий тимчасовий CDP browser context:
loopback cookies спільні між портами. Не очищайте cookies/вкладки користувача.

## Статус PR одним викликом

`pr-status` потребує authenticated `gh` і запуску в цьому checkout. Обирає
найновіший run workflow `ci.yml` для кожного event `push` / `pull_request`
на поточному head та branch PR. Перевіряє точні назви двох jobs, completed/success
для jobs і workflow; skipped/cancelled не є успіхом. Повторно читає head після
запитів: зміна head робить результат недійсним. Це одноразове спостереження,
не approval, merge gate чи автоматичний watcher; нічого не змінює у Git/GitHub.

Exit codes binary: `0` — усі чотири jobs успішні; `1` — failure, неочікуваний
набір jobs або помилка; `2` — pending/відсутній run. `go run` для nonzero
друкує `exit status N`, але сам повертає 1; для machine-readable exit code
зібрати `go build -o tmp/pr-status ./minions/pr-status`. Запити обмежені
90 секундами; список — останні 100 runs на branch/head.

## Повторюваний UI review

Спочатку агент запускає synthetic `ui-preview` на окремому порту, потім
`ui-review-cdp.mjs` із [прикладом](ui-review.example.json). Посіпака не запускає
сервер чи browser process. Спільний `cdp-session.mjs` використовує лише
наявний CDP 9222, створює окремий context і прибирає його також після помилки.
Особисті cookies/вкладки не очищає. `capture-cdp.mjs` використовує той самий код.

JSON задає `accept_language`, `widths` (1–10, кожна 200–4096), `height`
(200–4096), `cases` (1–200). Кожен case: унікальний filename-safe `name`,
relative `path` на base origin, очікуваний `lang`, optional `screenshot: true`.
Сценарії й ширини виконуються послідовно в одному context: cookies між ними
зберігаються; лише перша навігація перевіряє чистий візит. Для нового набору
browser-language headers потрібен новий запуск. Page JavaScript вимкнено,
viewport задає CSS-ширину без touch/device emulation. Це Chromium-перевірка
розмітки, не мовне, screen-reader чи capacity приймання.

Новий output directory — mode 0700, `results.json` і PNG — 0600; існуючу
директорію не перезаписує. Зупиняється на першій помилці й залишає partial
звіт із `passed: false`, виходить nonzero. PNG знімає всю сторінку, до 20000px
висоти; single capture — лише viewport. Output містить labels/metrics/lang,
без paths; PNG може містити весь видимий текст. Для публічних доказів
використовуйте виключно synthetic preview, реальні terms/paths — лише в `tmp/`.

Перевірки агента: `go test ./minions/pr-status` та
`node --test minions/cdp-session.test.mjs`; реальний browser smoke запускайте
окремо через CDP, без додавання browser dependency до CI.
