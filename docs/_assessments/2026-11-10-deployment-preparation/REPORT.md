# Локальна підготовка deployment — 2026-11-10

## Обсяг

Base: merge PR #37 `0bac9ef834fcaaf73e8817eb7f80deb7ba9fdfb4`.
Додано серверний профіль-кандидат, templates PostgreSQL/systemd/NGINX,
logrotate, env examples без secrets і Go operator utility `check-deployment`.
Власник обрав backup після оновлень на локальний комп’ютер та Telegram.
Oracle/legacy не чіпали; production deployment не виконано.

## Конфігураційні перевірки

- `docker compose --env-file deploy/postgres.env.example -f deploy/compose.yaml config --quiet`:
  PASS; це parsing/interpolation, не запуск production bind mounts.
- PostgreSQL із фактичним `deploy/postgresql.conf` стартував у власному
  network-none container з ephemeral tmpfs PGDATA. Pinned image відповідає
  Compose. `pg_isready` та SQL settings підтвердили max_connections 32,
  shared_buffers 1 GiB, temp_file_limit 1 GiB, log_min_messages fatal.
  Synthetic cluster видалено; реальні БД не використовувалися.
- `systemd-analyze verify --root=<isolated-fixture> --man=no --generators=no`:
  перевірка units із executable/dependency stubs. Це не installation/boot test.
  Прямий host verify очікувано не знаходить ще не встановлених release paths;
  його не подаємо як успіх цільового середовища.
- NGINX `-t` і synthetic HTTPS proxy run: PASS. Використано test image
  `nginx@sha256:583b7f2fce1cdb8da5ad5c39ebd3c33b98b87d493190e548bdee1fa8c619c311`,
  localhost high ports, тимчасовий self-signed cert і synthetic preview.
  Production site config змінено лише для test listen/backend/cert/log paths;
  wrapper задає writable temp directories для read-only контейнера.
  Перша спроба wrapper не мала writable fastcgi/uwsgi/scgi temp dirs; після
  виправлення fixture повтор пройшов. Це не дефект production site config.
- Перевірено: HTTP→HTTPS 308 і alias 308 зі збереженням escaped query,
  HTTPS основної сторінки 200, `Secure` cookie, `/healthz`/`readyz` через proxy 404;
  metrics log не містить synthetic request text. Публічний DNS/TLS не змінено.

Scratch scripts, cert/key і детальні локальні logs — в ignored `tmp/`,
не в публічному receipt. Жодних реальних Telegram HTTP requests: API tests
використовують fake RoundTripper, HTTP probes — httptest loopback.

## Go перевірки та review

`make check test test-race test-smoke utilities` і `make test-integration`: PASS.
Repository checks охоплюють deployment README/ignore rules; CI також перевіряє
Compose interpolation без production запуску. `check-deployment`
покриває health/readiness, disk reserve/error, loopback/redirect guards;
Telegram — API acknowledgement, safe errors, initial failure, private state,
corrupt-state rejection, dedup/reminder/recovery, retry після delivery failure
та invalid config при healthy startup. Scratch tests — у `tmp/utilities-tests/`.

Зібрана утиліта без `-notify-telegram` перевірила поточний локальний preview:
health/readiness 200, free-space reserve >=30 GiB. Preview не перезапускали.
Послідовне self-review: contracts/privacy, failure/state transitions,
installation/restore assumptions і документація. Незалежного reviewer не було.
Перед фінальним commit не залишилося підтверджених findings у цьому scope;
обмеження target/live tests наведено нижче.

## Межі

CPU/RAM/storage profile — пропозиція, не виміряний мінімум або тариф.
Немає target capacity/boot/TLS-renewal acceptance, off-host transfer/restore,
реальної доставки Telegram або external outage watchdog. Backup completeness,
secret escrow і failure recovery перевіряються окремо до публічного запуску.
Нових повних corpus/load/restore прогонів не робили; повтор не потрібний для
цих локальних конфігурацій. UI/31 ai-draft catalogs не змінено.
