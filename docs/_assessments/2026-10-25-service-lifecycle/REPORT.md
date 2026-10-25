# M1-08: наскрізна перевірка сервісу

## Scope і review

Base: `c86dd6e7abf6a6b398e9101fd65a5535394a6e8c` (main після PR #5).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
[Reviewed input hashes](reviewed-inputs.sha256) фіксують змінені tests та
активні docs; report і hash inventory не включені у власний scope.
AGENTS.md, application implementation та migrations не змінено.

Переглянуто M1-08/FR та acceptance mapping, subprocess environment/deadlines,
process/connection cleanup, ізоляцію DB outage, role privileges, CLI JSON/status,
HTTP body limits, publication/CAS, retained cursor/file behavior та точність docs.
Підтверджених невиправлених findings у цьому зрізі немає. Під час self-review
посилено HTTP checks: manifest link, stable article A→B→A; cleanup зареєстровано
так, щоб server processes прибиралися перед власною БД/LOGINs.
README більше не стверджує, що пошуку/UI немає, і правильно називає test runner.

## Реалізовано

`tests/serviceflow/lifecycle_test.go: TestServiceLifecycle` будує сервіс у
`tmp/serviceflow/run-*`. Лише explicit DSN disposable runner, нова порожня
БД та два окремі LOGINs із operator/HTTP group membership. HTTP login не
superuser, write denial перевірено. Runtime services не викликаються напряму.

Фактичні CLI subprocesses: migrate → import batches 7 → verify/exports →
publish A → stale CAS rejection → publish B → rollback A. Loopback HTTP:
exact/mask, search cursor, dictionary article/continuation, manifest та 53-row
CSV.gz із size/SHA/EOF validation. Retained A лишається після publication B;
bare article/search слідують B, rollback відновлює A. Ізольований outage лише
власної БД відхиляє нові connections і закриває її sessions: health лишається
200, readiness/search → 503; після recovery readiness та результати відновлюються.
Restart зі сталим keyring зберігає cursor і download. Graceful shutdown та
відсутність query/password/DSN у process logs перевірено. Cleanup видаляє
тільки власні процеси, БД, LOGINs і scratch; runner — container/volume.

Сценарій додано останнім до чинного PostgreSQL runner/CI job, із `-race` для
harness. Child binary — звичайний build; окремі `make test-race` перевіряють
application unit suites. Це developer/CI test, не посіпака чи runtime utility.

## Фактичні перевірки

- `make build check test test-race test-smoke` — PASS, exit 0.
- `go vet -tags integration ./tests/serviceflow` — PASS, exit 0.
- `make test-integration` — PASS, exit 0, включно з race-enabled lifecycle.

Перший lifecycle прохід також пройшов; після додаткових manifest/article
assertions та race flag повторено повний integration suite. PostgreSQL 17.11,
локальні synthetic data; container/network/volume прибрано. Logs:
`tmp/serviceflow/checks.log`, `tmp/serviceflow/final-integration.log` (ignored).
Точний mapping та перевірки interruption/retry/corrupt streams/no-clobber,
які виконуються окремими suites, — [TRACEABILITY](../../TRACEABILITY.md).
Команди для чистого checkout наведено в [README](../../../README.md).

## Межі

M1-08 done у synthetic verification scope. Незалежне відтворення іншою людиною
чи агентом не виконувалося; надано automated clean-database scenario та команди.
M1 загалом не закрито: M1-07 мовне/дизайн/accessibility приймання лишається
відкритим. Full sitemap/crawl discovery, production corpus/capacity, off-host
restore і deployment належать до наступної роботи. Oracle/legacy/browser не
задіяні. PR CI результат фіксується окремо у GitHub, звіт його не передбачає.
