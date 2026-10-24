# HTTP request-target guard: follow-up review — 2026-10-24

## Scope

Послідовний self-review follow-up до `3562c3ed3db263c3d38849039151c81b47fa72c8`.
Точний reviewed input — [hash inventory](reviewed-inputs.sha256).
Попередній [звіт](../2026-10-24-http-lookup/REPORT.md) і його hashes збережено
без змін; вони описують попередню версію, не цей delta.

## Підтверджені findings та виправлення

**P2: застосувати target guard до routing і дозволити досягти його на сервері.**
Попередній `MaxHeaderBytes=16 KiB` обмежував також request line: допустимий
24 KiB query міг отримати transport 431 раніше за adapter. Окремо стандартний
ServeMux на `//search?…` повертав redirect до перевірки довжини; локальний
minimal reproduction дав 307 замість 414 для target понад 32 KiB.

`LimitTarget` тепер стоїть перед outer і inner mux. HTTP parser ceiling піднято
до 64 KiB, щоб target boundary 32 KiB досягав application guard. Для ще більших
сумарних request line + headers залишається transport hard limit стандартного
Go server; він може повернути 431 до application handler.

`TestTargetLimitBeforeRouting` перевіряє ordinary/double-slash/dot-segment paths.
Actual binary process smoke: query 24 KiB → 200, понад 32 KiB → 414,
також double-slash → 414 без redirect; SIGTERM і SIGINT. Після виправлення
повторний contract/control-flow/docs review не знайшов інших підтверджених
невиправлених findings у delta.

## Validation

`make check test test-race test-smoke` — PASS, exit 0. PostgreSQL code/queries
не змінені; попередній isolated integration PASS залишається чинним для них.
Останню PR CI revision слід перевірити окремо перед merge.
Зовнішній вигляд не змінено; попередні CDP screenshots залишаються придатними.
M1-07 лишається in progress із попередньо зазначеними обмеженнями.
