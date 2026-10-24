# Go utilities, project tests та особисті посіпаки

Дата: 2026-10-24. Baseline перенесення: `823150e127d0068ffc92a199e19686b4d0f3d9f5`.
Перед ним оформлено [PR #1](https://github.com/valpere/slovnyk/pull/1), review
reports M1-01–06 та contributor workflow review → PR → CI.

## Розділення за призначенням

Уточнення користувача: utilities потрібні для роботи сервісу й доступні всім;
minions — виключно особисті помічники агента. Мова реалізації не є критерієм.

- `utilities/prepare-artifacts`: Go operator utility, MkdirAll + directory/ancestor
  fsync; повторний запуск не змінює наявних files. `make utilities` збирає binary.
- `tests/repository`: стандартна проєктна перевірка Markdown, links та Git ignore.
  Ignore перевіряється NUL-delimited paths, також для tracked sources.
- `tests/processsmoke`: Go tests з tag smoke, власний loopback subprocess,
  SIGTERM/SIGINT, HTTP health/readiness, bounded logs та query privacy.
  Успадковані `SLOVNYK_*` видалено, proxies/redirects вимкнено.
- `tests/check-patch.sh`, `tests/postgres.sh`: проєктні shell test runners,
  які використовує Make/CI. Script bodies незмінні, перенесено шляхи.
- `minions/check-repository`: особистий Go shortcut агента, що знаходить checkout
  і запускає стандартний repository test. CI його не викликає.

Три Python-команди вилучено. Make/CI та operator documentation більше не
потребують Python; історичні Python assessment scripts/bytes збережені.

## Review та CI перед перенесенням

Перший PR run `40103464608` failed на intentional CRLF/bare-CR fixtures у повному
main→HEAD diff; попередній push diff не включав ці старі файли. Локальне
відтворення `check-patch.sh origin/main`: exit 2 до .gitattributes, exit 0 після.
Вузькі attributes дозволяють CR-at-EOL лише для двох fixtures і вимикають text
conversion; bytes/hashes самих fixtures незмінні. Commit `823150e` містить також
PR template і CONTRIBUTING. PR CI [40103682058](https://github.com/valpere/slovnyk/actions/runs/40103682058)
на цьому commit — success, обидві jobs. Це evidence workflow до Go migration.

Перенесення переглянуто послідовним self-review за code-review skill:
behavior parity, subprocess cleanup/timeouts/privacy, fsync failures,
Git ignore errors, fixtures, commands, current documentation та межі категорій.
Підтверджених невиправлених findings у цьому diff не виявлено.
Точні input hashes — `reviewed-inputs.json` (цей звіт і inventory виключені).
Рев’ю не незалежне й не multi-agent. PR/CI не означає дозволу merge/deploy.

## Перевірки та обмеження

Regression tests:
- `tests/repository/check_test.go`: missing/encoded/local links, UTF-8,
  whitespace, ignored local data, tracked-but-ignored sources і newline filename.
- `utilities/prepare-artifacts/main_test.go`: повторне provisioning, unchanged
  existing file, відмова для file замість directory.
- `tests/processsmoke/failure_test.go`: успадкована storage configuration
  не доходить до child; зовнішня адреса відхиляється, child killed/reaped.
- `tests/processsmoke/smoke_test.go`: справжній service binary, обидва signals,
  health/HEAD/readiness та lifecycle; звіт тільки із safe diagnostics.

Це local/Linux baseline. Production deployment, корпус та Oracle не задіяні.
AGENTS.md, legacy й historical assessments незмінні. M1-07 залишається наступним
функціональним етапом, його реалізація тут не починалася.

## Фактичні локальні результати

Усі перелічені перевірки завершилися exit 0:

- `make check test test-race test-smoke utilities`: format/vet/patch/repository,
  звичайні й race suites, real process smoke та build operator utility.
- `make test-integration`: усі suites через новий `tests/postgres.sh`, власна
  disposable PostgreSQL 17.11; container/network/volume видалено.
- `go test -race -tags smoke ./tests/processsmoke -count=1`: process suite,
  включно з failure cleanup, під race detector.
- `./tests/check-patch.sh origin/main`: повний PR diff, разом зі staged/unstaged.
- Binary особистого checker та `bin/prepare-artifacts` запущені з
  `tmp/go-utilities/`, поза repository root; relative artifact destination створено
  саме там. Report checker — PASS, utility повернула absolute path.
- Після останніх documentation changes повторний `make check` — PASS.

Runtime logs: `tmp/go-utilities/checks-final.log`, `integration-final.log`,
`smoke-race.log`, `check-final-docs.log`, `assistant-check.log`, `artifact-root.log`.
Remote CI для Go migration перевіряється після reviewed commit/push; цей
pre-commit звіт не підміняє його попереднім зеленим run.
