# Посіпаки агента

Посіпаки — виключно особисті помічники агента. Вони скорочують його
повторювані запити й дії до одного виклику та не є залежністю сервісу,
операторських процедур або CI. Мова реалізації категорію не визначає.

| Посіпака | Виклик агента | Призначення |
|---|---|---|
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
