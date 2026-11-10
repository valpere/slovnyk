# Підготовка розгортання

Це шаблони для **нового виділеного Linux-хоста**, а не команда встановлення
на нинішній Oracle. Нічого з цього не встановлено в host systemd/NGINX.
[Ресурсний профіль](../docs/SERVER_PROFILE.md) — кандидат для приймання.
Сервіс: native Go binary під systemd, окремий PostgreSQL 17.11 у Docker,
NGINX завершує TLS. Застосунок має лише читання БД та artifacts.

## 1. Підготувати хост і release

Приклади host paths/logrotate орієнтуються на Ubuntu 24.04; для іншого
distribution перевірити групу `adm`, NGINX binary/pid і package defaults.
Потрібні systemd, Docker із Compose v2 (`up --wait`), NGINX, TLS/ACME client,
SSH/rsync та SHA-256 tools. До встановлення перевірити ресурси, вільне місце,
архітектуру й відсутність конфліктів портів. Не застосовувати до shared host
без окремої адаптації. Firewall: зовні лише 80/443 і контрольований SSH;
не відкривати 55432/8080. Docker socket доступний лише адміністратору.

На локальному checkout зафіксованого release:

```sh
make build utilities
sha256sum bin/slovnyk bin/check-deployment
```

Збирати під архітектуру цілі; host-build не вважати автоматично сумісним.
Для cross-build використовувати `CGO_ENABLED=0 GOOS=linux GOARCH=<target>`
і виконати smoke на цілі. Go runtime на сервері не потрібний.
Зберегти commit, Go version, build flags і hashes в release receipt.

На **обраному** хості адміністратор створює:

- непривілейованого системного користувача/групу `slovnyk`, без Docker group;
- root-owned `/opt/slovnyk/releases/<commit>/` з обома binaries і symlink
  `/opt/slovnyk/current`; release не може змінювати app user;
- root-owned `/opt/slovnyk/deploy/` з `compose.yaml` і `postgresql.conf`;
- `/srv/slovnyk/postgres` для PostgreSQL і `/srv/slovnyk/artifacts` для файлів;
  каталог PG має належати користувачу postgres **закріпленого image**,
  artifacts доступні `slovnyk` для читання/traversal, не запису;
- приватні каталоги для source CSV і backup staging, недоступні HTTP user;
- `/etc/slovnyk/` mode 0700, root-owned env files mode 0600.

Не виконувати recursive chown над існуючими даними. Compose відмовляється
створювати відсутні bind paths; підготувати їх свідомо, з правильними ACL.
При окремих filesystem контролювати кожну, а не лише `/srv/slovnyk`.

## 2. Секрети, БД і restore

Скопіювати `*.env.example` у `/etc/slovnyk/{postgres,http,alerts}.env` та
замінити **всі** placeholders. Генерувати незалежні bootstrap/HTTP passwords;
DSN password percent-encode. Cursor key — випадкові 32 bytes (64 hex chars),
активний KID має бути в keyring. Зберігати старі ключі для continuations.
Секрети вводити через захищений редактор/secret store, не CLI/history/logs.
Окремо підготувати зашифровану копію keyring/config та ключ розшифрування поза
хостом; втрата локального комп’ютера не повинна знищити єдиний recovery key.

Встановити units із `systemd/` у `/etc/systemd/system/`, `daemon-reload`.
Спочатку запускати **лише** `slovnyk-postgres.service`. POSTGRES_PASSWORD
ініціалізує тільки порожній PGDATA; зміна env не обертає пароль існуючої БД.
DB container публікує 55432 тільки на 127.0.0.1, credentials доступні Docker
адміністратору; application DSN ніколи не використовує bootstrap account.

Відновити повний schema 4 bundle за [BACKUP_RESTORE](../docs/BACKUP_RESTORE.md):
порожня окрема БД, NOLOGIN non-superuser owner, `slovnyk_http`/`slovnyk_operator`
groups, окремі LOGINs, restore з ACL під owner, `ANALYZE`, hashes і controls.
До restore не створювати tables через `migrate`; dump уже містить ledger.
`temp_file_limit=1GB` — serving guard на backend, не виміряний restore budget.
Якщо index restore потребує більше, адміністратор задає обґрунтований session
`temp_file_limit` для restore, попередньо перевіривши вільне місце; HTTP
параметри не послаблювати. Не повторювати повний import незмінених CSV.

Запустити `slovnyk.service`. Перевірити loopback health **і** readiness:

```sh
/opt/slovnyk/current/check-deployment -url http://127.0.0.1:8080 \
  -data /srv/slovnyk -min-free-gib 30
```

Перший startup сканує files; готовність не миттєва. JSON/exit 0 означають
лише ці probes і поточний резерв диска, не повне приймання цілі.

## 3. TLS і журнали

Видати certificate для **обох** `slovnyk.org` і `slovnyk.org.ua`. Налаштувати
renewal і перевірити його dry run. Для HTTP-01 потрібні доступний порт 80,
DNS на нову ціль і ACME webroot `/var/lib/slovnyk-acme`; перед першим cert
використати тимчасовий HTTP-only virtual host, адже TLS config потребує вже
наявних certificate files. DNS-01 дозволяє попередній випуск; credentials
захищати окремо. Самопідписаний cert використовувався тільки в локальному тесті.

Включити `nginx/slovnyk.conf` всередині `http {}`, вимкнути default/demo site
виділеного хоста, перевірити `nginx -t` перед reload. Proxy передає оригінальний
Host, не переписує URI; alias перенаправляє застосунок. Це відповідає
[NGINX proxy contract](https://nginx.org/en/docs/http/ngx_http_proxy_module.html).
Перед DNS cutover перевірити обидва TLS virtual hosts через адресний resolve,
escaped redirects, Secure cookies, downloads, canonical/robots/sitemaps.
`/healthz` і `/readyz` доступні лише напряму на loopback.

Request log містить лише status/timings/bytes. Request error log навмисно
вимкнений: стандартні помилки proxy можуть містити query. PostgreSQL також
не журналює statements, durations і звичайні SQL errors; app events лишаються
безпечним джерелом діагностики. Не вмикати стандартний access/error/query log
для пошуку. Узгодити це з global NGINX config: default vhosts, global error log
і ACME tooling теж перевірити на витік URL. При розслідуванні використовувати
synthetic requests, resource metrics і безпечні error codes.

Встановити `logrotate/slovnyk`, перевіривши шлях binary/pid NGINX. Docker DB
logs обмежені 3×10 MB; на виділеному хості окремо обмежити journald/OS logs
(наприклад, сумарно 512 MB), щоб вони не зайняли OS filesystem. Ротація logs
не видаляє backup. Жодних host logging settings локально не змінено.

## 4. Telegram і автозапуск

Створити власного bot, дозволити йому писати в потрібний chat; token/chat ID
покласти в `/etc/slovnyk/alerts.env`. Не додавати secrets у Git або receipt.
Під час приймання перевірити реальну тестову доставку, доступ API й
правильність chat. Валідація формату credentials не доводить доставку.

`slovnyk-check.timer` запускає утиліту раз на хвилину: збій readiness/health або
free <30 GiB → Telegram; recovery → Telegram; незмінний збій → нагадування
через 6 h. Здорова перша перевірка мовчить. Failed delivery не підтверджується
у state і буде повторена. Непридатний state file/config дає failure у journal;
після перевірки причини оператор відновлює state, не підмінює його успіхом.
State зберігається у `/var/lib/slovnyk-monitor/` приватно. Повідомлення не
містять URL, пошукових слів, користувачів або секретів.

На цілі після перевірок увімкнути автозапуск:

```sh
systemctl enable slovnyk-postgres.service slovnyk.service
systemctl enable --now slovnyk-check.timer
```

Timer на сервері не сповістить про вимкнений host/мережу/Telegram. До відкриття
обрати незалежний зовнішній watchdog із Telegram, перевірити сценарій outage.
Поточний локальний комп’ютер придатний лише коли він увімкнений і в мережі.
Зараз реальні Telegram requests не виконувалися.

## 5. Backup, приймання й відкат

Виконати [backup policy](../docs/BACKUP_RESTORE.md#політика-першого-read-only-запуску-2026-11-10):
повний комплект до відкриття й після кожного оновлення, off-host на локальний
комп’ютер, hashes та приватна копія recovery secrets. Щоденного timer немає.
Потім пройти target load і off-host restore <=24 h за [LAUNCH_PLAN](../docs/LAUNCH_PLAN.md).
Лише після receipt, мовного приймання й рішення власника — відкриття DNS.

Зберігати попередній release і dataset. Відкат binary: зупинити app, повернути
`current` на перевірений сумісний release, запустити, перевірити readiness і
controls. Dataset rollback — штатна CAS-команда з operator LOGIN за
[MIGRATION](../docs/MIGRATION.md#5-відкат), не ручний SQL. За відсутності
сумісного попередника зупинити запуск і відновити перевірений bundle.
Нічого з retained datasets/backups автоматично не видаляти.
