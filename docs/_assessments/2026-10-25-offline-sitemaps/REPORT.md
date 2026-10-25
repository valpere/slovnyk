# Offline sitemap artifacts

## Scope і review

Base: `84e624d823f8414bbe0c6c20325d097c1f9f070a` (main після PR #6).
Послідовний **self-review**, не незалежне або multi-agent рев'ю, за code-review
та verification-before-completion skills. Scope фіксує
[reviewed input hashes](reviewed-inputs.sha256); report/hash inventory не
включені у власний scope. AGENTS.md, migrations, legacy та Oracle не змінено.

Перевіряються FR-14 / AT-19, immutable ready state, SQL/Go identity parity,
protocol limits/root scope, deterministic streaming, partial failures,
artifact confinement/no-clobber/integrity, CLI arguments/errors та точність docs.

## Реалізовано

`internal/sitemaps` та `slovnyk dataset sitemap ID --artifacts EXISTING_ROOT`:
DISTINCT ordered raw hashes, 50 000 URL/shard, immutable index/report. Shared
artifact store також приймає `.xml`. Лише ready dataset, active pointer незмінний;
не ready/unknown/empty dataset і пошкоджені файли відхиляються. JSON inventory
встановлюється останнім, повторні файли перевіряються без заміни inode.

## Перевірки

- `make build check test test-race test-smoke` — PASS, exit 0.
- `go vet -tags integration ./internal/sitemaps ./cmd/slovnyk` — PASS, exit 0.
- `make test-integration` — PASS, exit 0, включно з CLI та service lifecycle.

Unit boundaries: 1/50 000/50 001 URL, parsed XML namespace/loc, counts/size/SHA,
JSON inventory, stable retry/inode. Fault cases: empty/invalid/duplicate/decreasing
hash, iterator error, cancellation, shard limit, existing corrupt shard; немає
success result/report на збій або overwrite. Shared artifact-store suites також
повторно перевірено. PG test: ready pre-publication, raw variants/pair identities,
кілька translations одного source, state/unknown rejection, active pointer
незмінний, bytes після publish ті самі. CLI JSON та secret-safe arguments перевірено.

Перший integration run виявив помилкову fixture assertion: existing corpus не
містив різних translations одного source. Додано спеціальні synthetic records;
після цього повний suite пройшов. Під час self-review виправлено P2 protocol
scope: початкові майбутні `/sitemaps/<dataset>/<hash>.xml` обмежували б shard
піддеревом `/sitemaps/`; тепер root-level URLs охоплюють `/word/`. Після buffered
writes і зміни адрес повторено повні local checks та integration. Невиправлених
підтверджених findings у цьому scope немає; це не незалежне рев'ю.

Логи (ignored): `tmp/offline-sitemaps-checks.log`,
`tmp/offline-sitemaps-final-integration.log`; фінальний runner прибрав власні
container/network/volume. Mapping — [TRACEABILITY](../../TRACEABILITY.md).
Окремий фінальний `make check` перевіряє дописані docs/hash inventory;
PR merge-revision CI evidence фіксується в GitHub після відкриття PR.

## Межі

HTTP sitemap routes/robots discovery, DB registration, publication/rollback
integration та full-corpus rehearsal ще не реалізовано. Go memory bounded,
але SQL DISTINCT/ORDER BY може споживати DB RAM/temp disk; capacity не доведена.
Synthetic local PostgreSQL; browser/Oracle/legacy/real corpus не використовуються.
Це часткове FR-14 / AT-19, M1-07 залишається in progress.
