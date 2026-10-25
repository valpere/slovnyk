# HTTP sitemap discovery

## Scope і review

Base: `19dfb2d182e456cc9c0544f4c36390195d4a864b` (PR #7, ще open на початку).
Залежний branch/PR, без merge попереднього PR від імені власника.
Послідовний self-review, не незалежний чи multi-agent; scope:
[reviewed input hashes](reviewed-inputs.sha256), report/inventory hashes виключені.
AGENTS.md, legacy, Oracle та real corpus не змінено/не використано.

## Реалізація

Schema 3 додає immutable sitemap registry. Файли/report fsync перед атомарною
реєстрацією; operator має вузький SECURITY DEFINER write API, HTTP — SELECT.
Counts/deferred constraint відхиляють partial commit, direct writes заборонено.
Index resolve pin-ить active published ready dataset одним SQL snapshot;
shards лише registered/published, retained refs зберігаються. Handler streaming,
metadata budget/transfer limits, HEAD/ETag/Range, fixed robots discovery.
Startup/recovery/нова generation звіряє registered active file hashes.

## Review та перевірки

Виконано послідовні contract/control-flow/privilege/failure/documentation passes
за code-review skill. Перевірено immutable state, реєстрацію після fsync,
atomic counts/row locks, no direct writes, publication/history filtering,
SQL snapshot активного index, confined file open, metadata/transfer admission,
query rejection, cache/error semantics і чесні межі readiness.

Під час review використано вузьку SQL write API замість transaction-ID sealing:
немає persisted XID dependency, table INSERT недоступний operator/HTTP. Доповнено
error wrapper: Range errors також noindex/no-store. Незалежне рев'ю не виконано;
підтверджених невиправлених findings у цьому scope немає.

- `make build check test test-race test-smoke` — PASS, exit 0.
- `go vet -tags integration ./internal/sitemaps ./internal/ops ./internal/storage/postgres ./tests/serviceflow` — PASS, exit 0.
- `make test-integration` — PASS, exit 0, disposable PostgreSQL 17.11.

Фінальний PG run включає write API, privilege/partial rollback tests та actual
CLI/HTTP lifecycle: sitemap до publish, active index A→B→A, retained shard A,
DB outage/recovery, restart і same-size corrupt shard → readiness/index 503.
HTTP unit checks: GET/HEAD/304/206/416, fixed refs/ETag/cache, strict paths/query,
safe errors. Upgrade test відновлює schema 1 і мігрує до 3 без зміни ready raw
fields. Наявні importer/publication/download suites також пройшли.
Проміжні suites пройшли; після material write API/test changes повторено
повний integration suite. Local build/unit/race/smoke відповідають останньому
implementation; дописані docs/hash inventory перевіряються фінальним `make check`.

Logs (ignored): `tmp/http-sitemaps-final-checks.log`,
`tmp/http-sitemaps-final-integration.log`. Власні containers/network/volume
прибрані. Mapping — [TRACEABILITY](../../TRACEABILITY.md).
PR CI evidence буде додано до GitHub PR після commit; воно тут не передбачається.

## Межі

Inventory optional для publication: відсутній index → 503; інші функції сервісу
працюють за попереднім контрактом. Release порядок verify → sitemap → publish.
Immutable HTTP mount обов'язковий; same-size mutation не hash-иться на кожен
request. Retained GC, full-corpus build/readiness/crawler capacity, production,
мовне/дизайн/accessibility acceptance лишаються відкритими; M1-07 in progress.
