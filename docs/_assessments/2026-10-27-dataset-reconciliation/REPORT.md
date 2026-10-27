# Звірка staged dataset до погодження втрат

## Scope

Base: `5ebb99d175f41143bb867330534fac5f10f09778` (merged PR #15).
Команда `dataset reconcile ID --source DIR` відокремлює перевірку доступних
даних від створення exports та переходу ready. Known gaps у реальному manifest
досі блокують strict verify; цей зріз не реалізує owner-approval adapter і не
погоджує жодних втрат. Повний корпус тут не запускався.

Реалізація — `internal/publication/reconcile.go`, shared validation setup та
accounting у `publication.go`, CLI у `cmd/slovnyk/dataset.go`. Schema не змінена.
AGENTS.md, historical assessments, legacy, Oracle і real corpus не змінено.

## Поведінка

Команда приймає лише validating dataset, canonical manifest/fingerprint,
operator connection та той самий незмінний snapshot. Тримає import/verify writer
lock і dataset row lock. Повторно читає всі artifacts до EOF/checksum,
звіряє ledger/ordinals/checkpoints, raw translations/origins, normalized keys
і hashes, metadata, pair counts та повноту inventory. Використовує наявний
bounded replay staging; завершення відкочує транзакцію й закриває connection.
Persistent дані, report references, exports, state й active pointer незмінні.

JSON stdout — diagnostic `version=1`, `kind=reconciliation`, dataset/fingerprint,
state, total/per-file counts та незмінені known_gaps/exceptions. Відсутність
помилки доводить узгодженість доступних included artifacts, а не історичну
повноту або згоду на виключення. Rejected records/issues і неповний import
не підтримуються як успішний diagnostic path. Готові/failed/loading datasets
потребують відповідних інших процедур; ready exports перевіряє `dataset export`.

`--artifacts`, CAS generation та operator-ref для reconcile відхиляються.
Помилки CLI — bounded codes без DSN/source paths. Наявні verify/export,
publish/rollback CAS, ready report format і HTTP поведінка збережені.

Replay тепер явно порівнює `key_hash` разом із source key та tuple hash.
Schema вже має CHECK відповідності hash/key та immutable translation trigger;
це додаткова перевірка replay staging, а не доказ наявної пошкодженої БД.
Regression fixture підміняє тільки temp key/hash: постійні guards не вимикаються.

## Перевірки

- `make test-integration` — PASS, exit 0, disposable PostgreSQL 17.11.
  Включено clean/gap/exception diagnostics, repeat, fingerprint/counts,
  unchanged validating/report/exports/generation та відмову publish/verify
  без погодження. Негативні cases: origins/metadata/counters/completion,
  missing та same-size modified compressed source, writer conflict,
  canceled operation/connection cleanup, rejected records.
- `internal/importer/importer_integration_test.go`:
  `TestReplayDetectsChangedNormalization` — temp key/hash mismatch → ErrReplay;
  решта import/retry/collision suites також пройшли.
- `cmd/slovnyk/import_integration_test.go`: actual CLI reconcile перед
  verify/export/sitemap/publish; synthetic service lifecycle і fresh-cluster
  backup/restore також пройшли у повному integration suite.

Перші corruption fixtures намагалися UPDATE immutable translations: schema
правильно відхилила їх до звірки. Fixtures скориговано без послаблення guards;
фінальний повний integration suite завершився exit 0. Власні disposable
containers, networks і volumes прибрані runner-ом.

`make check test test-race test-smoke` — PASS, exit 0, після фінальних
implementation/test змін. Build входить у test-smoke. Фінальний integration suite
також повторено після додавання state guards tests: exit 0. Фінальні docs/hash
inventory перевіряються `make check` перед commit.

Послідовний self-review за code-review skill: contract/state/lock/cleanup,
shared verify regression, CLI arguments/privacy та documentation passes.
Переглянуто base→working-tree diff і нові файли; точний scope —
[reviewed input hashes](reviewed-inputs.sha256), report/inventory виключені.
Невиправлених підтверджених findings у цьому scope немає. Це self-review,
не незалежне або multi-agent рев'ю.
CI для exact PR head буде зафіксовано в описі PR, без передбачення його результату.

## Межі та продовження

Це synthetic preparation subset FR-07/08, не full-corpus acceptance. Для
реального dataset потрібен окремий контрольований локальний запуск reconcile
з ресурсним бюджетом і збереженням metadata evidence; час/ресурси ще не виміряні.
Далі — конкретний loss report і owner-approval adapter, verify/exports/ready,
capacity/SLO та backup/restore acceptance. Manifest gaps не видаляти.
64.315 GiB layout і історичний ~45 GiB Oracle disk усе ще несумісні.

Logs (ignored): `tmp/dataset-reconciliation/`; synthetic fixtures лише у tmp
та testdata. Немає SSH, full-corpus DB startup або deployment.
