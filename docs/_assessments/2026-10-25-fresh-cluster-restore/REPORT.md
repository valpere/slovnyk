# FR-13: restore в окремий чистий PostgreSQL cluster

## Scope і review

Base: `84011f2da20d12ae9e39c3a3073af9e0e4704a53` (main після PR #10).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
[Reviewed input hashes](reviewed-inputs.sha256) фіксують tests і активні docs;
цей report та його inventory виключені зі свого scope.

Переглянуто FR-13/AT-13, bootstrap і ownership, ACL/SECURITY DEFINER, atomic
restore failure/retry, ізоляцію двох цілей, deadlines, cleanup при частковому
setup, credentials у subprocess arguments/logs та відповідність docs assertions.
Підтверджених невиправлених findings немає. Перед фінальним integration run
додано ownership types, точний SQLSTATE 42501 для permissions і cleanup обох
Compose projects навіть при помилці першого `down`. AGENTS.md, application
runtime, dependencies та migrations не змінено.

## Реалізовано

`tests/postgres.sh` створює другий PostgreSQL 17.11 container у власному Compose
project із окремим volume та random loopback port перед serviceflow. Він має
інший initial admin; source owner `slovnyk_test` і application groups відсутні.
Кожен контейнер зберігає Compose limit 512 MiB/1 CPU. `restoreFixture` перевіряє
labels, різні container IDs та PostgreSQL system identifiers.

На цілі створюються новий NOLOGIN non-superuser owner і його нова UTF-8 БД.
Restore запускається з `--no-owner --role=<new-owner> --single-transaction
--exit-on-error`. Без group grantees перша спроба має nonzero exit і не залишає
application schema. Після provision groups та restricted LOGINs той самий dump
успішно відновлюється. Globals dump/source roles не копіюються.

Owner schema/relations/functions/types — нова роль; owner, groups і LOGINs
не мають superuser/CREATEDB/CREATEROLE/replication/BYPASSRLS. Counts/digests усіх
17 таблиць, 10 artifacts і 4 source files збігаються, як у попередній репетиції.
Подальший actual HTTP binary використовує нові LOGINs другого cluster, поки
вихідна БД недоступна. Empty artifact root → readiness 503; повний комплект →
ready, byte-identical exact/mask/article continuations і download, sitemap та
retained B доступні. Runtime keyring той самий synthetic.

HTTP UPDATE/DDL/register і operator direct inventory DELETE/DDL відхиляються
з SQLSTATE 42501. Actual operator CLI повторює sitemap registration: restored
SECURITY DEFINER працює під новим non-superuser owner та не створює нового report.
Cleanup прибирає processes перед їхніми БД, потім roles, connections, scratch;
runner видаляє обидва containers/networks/volumes. Це developer/CI test,
не production backup utility або посіпака агента.

## Фактичні перевірки

- `make build check test test-race test-smoke` — PASS, exit 0.
- `go vet -tags integration ./tests/serviceflow` — PASS, exit 0.
- `make test-integration` — PASS, exit 0, включно з race-enabled harness.

Перший two-cluster run пройшов; після додаткових type ownership/permission
assertions повторено весь integration suite. Final run: dump 78 460 bytes,
локальна підготовка/restore 566 ms, весь lifecycle 7.26 s. Це synthetic diagnostic
measurements, не production RTO або capacity gate. Обидва Compose projects
прибрані. Logs: `tmp/fresh-cluster-restore/checks.log`,
`tmp/fresh-cluster-restore/final-integration.log` (ignored).
[Mapping](../../TRACEABILITY.md), [процедура](../../BACKUP_RESTORE.md).
CI outcome фіксується окремо у PR, цей звіт його не передбачає.

## Межі

Закрито локальне обмеження попередньої same-cluster репетиції щодо existing roles.
Обидва clusters залишаються на одному developer/CI host. Немає off-host transfer,
storage durability/retention, secure config/keyring recovery, full-corpus capacity
або RTO <=24 h. Нові LOGIN passwords задані fixture, старі credentials не
відновлюються. Restore administrator має superuser для provisioning/SET ROLE;
application objects належать іншому restricted owner. Відсутній production
backup utility/deployment; Oracle, legacy та реальні дані не задіяні.
FR-13/AT-13 і M1 UI/мовне приймання повністю не закриті.
