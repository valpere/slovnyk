# FR-13: локальна backup/restore репетиція

## Scope і review

Base: `5817605c151e18046c0c31700399e10bd9432f1d` (main після PR #9).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
[Reviewed input hashes](reviewed-inputs.sha256) фіксують tests і активні docs;
report та його hash inventory виключені зі свого scope.

Переглянуто FR-13/AT-13, повноту DB/files snapshot, fresh target isolation,
subprocess exit status/deadlines, privileges після restore, cleanup, захист
credentials і відповідність документації фактичним assertions. Підтверджених
невиправлених findings у цьому зрізі немає. Перед фінальним integration run
додано byte-identical mask search після restore. AGENTS.md, application runtime
і schema/migrations не змінено.

## Реалізовано

Після existing A→B→rollback A/restart scenario `TestServiceLifecycle` зупиняє
fixture writers. `restoreFixture` знімає custom dump через PostgreSQL tools
закріпленого runner-контейнера; host tools не потрібні. Archive й inventory
зберігаються приватно у власному scratch, credentials не передаються через
command arguments і не друкуються при помилках PostgreSQL tools.

Нова UTF-8 БД створюється без `--clean`; restore single-transaction/fail-fast.
Counts/digests усіх **17 таблиць** збігаються, включно зі schema ledger,
embedded manifests, raw data, provenance, reports, active pointer/generation та
publication history. Також збігаються paths/bytes/SHA-256 **10 artifacts** і
**4 source files** після копіювання у нові roots. Послідовностей у schema немає.

На час HTTP-приймання оригінальна БД не приймає connections, її sessions
закрито. DB-only restore з порожнім artifact root лишає health 200, але
readiness/sitemap/download повертають 503. Повний комплект із тим самим release
та synthetic keyring проходить readiness; exact/mask search, старі search/article
continuations і active download збігаються з попередніми bytes. Active sitemap
та retained B download/shard доступні, HTTP LOGIN не може змінити active pointer,
операторський CLI повторює sitemap registration без нового report.

Cleanup зареєстровано одразу: processes зупиняються перед їхньою БД, restored DB
видаляється перед fixture LOGINs; runner прибирає власний container/network/volume.
Це developer/CI test, не runtime backup utility та не посіпака агента.

## Фактична валідація

- `make build check test test-race test-smoke` — PASS, exit 0.
- `go vet -tags integration ./tests/serviceflow` — PASS, exit 0.
- `make test-integration` — PASS, exit 0, включно з race-enabled lifecycle.

Final integration перевіряє source-file inventory та restored mask search.
Logs: `tmp/backup-restore/checks.log`, `tmp/backup-restore/final-integration.log`
(ignored; dumps/fixtures не публікуються). [Mapping](../../TRACEABILITY.md).
[Operator guide](../../BACKUP_RESTORE.md) описує весь комплект і паузу всіх
operator writers для узгодженого snapshot БД/файлів. CI outcome буде окремо
у PR; локальний звіт його не передбачає.

## Межі

Це той самий disposable cluster: group roles і fixture LOGINs уже існують.
Fresh-cluster bootstrap, off-host transfer/storage durability, secret/config
escrow, повний корпус, backup schedule/RPO/retention та RTO <=24 h не перевірені.
Сталий keyring заданий тестом, не відновлений із secret backup. Table JSON digest
є локальним test oracle, не переносним public migration format. Немає production
backup utility, deployment або UI change. Oracle, legacy й реальні дані не задіяні.
FR-13/AT-13 та M1 UI/мовне acceptance лишаються відкритими в неперевіреній частині.
