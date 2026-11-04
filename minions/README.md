# Посіпаки агента

Посіпаки — виключно особисті помічники агента. Вони скорочують його
повторювані запити й дії до одного виклику та не є залежністю сервісу,
операторських процедур або CI. Мова реалізації категорію не визначає.

| Посіпака | Виклик агента | Призначення |
|---|---|---|
| `corpus-inventory` | `go run ./minions/corpus-inventory PRIVATE_JSON OUTPUT_JSON` | Агентська повна звірка 17 таблиць dev-БД і sequences через streaming binary COPY; лише metadata/hash output |
| `corpus-progress` | `go run ./minions/corpus-progress RUN_DIRECTORY MANIFEST_JSON` | Показує elapsed, free disk та останній спостережений файл локальної звірки; читає лише progress/manifest metadata, без DB-запитів |
| `check-repository` | `go run ./minions/check-repository` | Знаходить checkout і одним викликом запускає проєктну перевірку з докладним звітом |
| `capture-cdp.mjs` | `node minions/capture-cdp.mjs http://127.0.0.1:PORT/path tmp/preview.png 390 844` | Знімок loopback preview через наявний Chromium CDP 9222; JS сторінки вимкнено, створює й закриває лише власну вкладку |

Потрібні Go та Git. Для виклику поза checkout можна зібрати binary у `bin/`;
root визначається за модулем у cwd/ancestors, шляхом binary або source.
Посіпака повертає exit status проєктної перевірки. Звіти — `tmp/tests/`.
Для CDP-посіпаки потрібен Node.js 22+ з built-in fetch/WebSocket та вже
запущений Chromium на loopback port 9222. Він не запускає browser process,
не використовує Playwright та не залежить від сервісу або CI. Output directory
має існувати; одноразові preview servers та captures зберігайте в `tmp/`,
review screenshots можна включати до відповідного assessment.

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
