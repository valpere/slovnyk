# Backup і restore нового сервісу

FR-13 / AT-13, без користувацького запису. Production та off-host відновлення
ще не виконано. Synthetic репетиція та [повний локальний restore](_assessments/2026-11-04-corpus-restore/REPORT.md) перевірили окремий fresh cluster;
вони не доводять production/off-host RTO <=24 h для повного корпусу.
[Повторний повний restore schema 4](_assessments/2026-11-08-schema4-restore/REPORT.md)
2026-11-08 відновив також prefix index і ledger: 17 таблиць/sequences, повні
файли та HTTP/cursors тотожні; target acceptance виконано з вимкненим source.
Виміряний локальний elapsed не є off-host або target-server RTO acceptance.

## Політика першого read-only запуску — 2026-11-10

Власник погодив повний комплект **перед відкриттям і після кожного оновлення
даних**; щоденні копії поки не потрібні. Старі комплекти автоматично не видаляти.
Off-host сховище майбутнього сервера — цей локальний комп’ютер. Пропонований
каталог `~/backups/slovnyk/` приватний (0700); bundle files — 0600 або суворіші.
Cloud→local передавання ще не виконано. Нинішній backup на комп’ютері, де
зараз працює preview, не є off-host захистом цього preview.

RPO прив’язаний до оновлення dataset, а не до щодобового часу: завершеним
операторське оновлення вважати лише після перевіреної зовнішньої копії. До
передавання нова версія ще не має підтвердженого off-host захисту. Всі writers
мають лишатися призупиненими до завершення узгодженого snapshot; HTTP читає
дані. Якщо потрібно не показувати ще не захищену версію, зупинити публічний
traffic на час publication і формування/перевірки backup, потім відкрити знову.
Зміна secrets/keyring/release config також потребує оновлення recovery комплекту.

Для кожної копії:

1. Перерахувати вільне місце на сервері **та** комп’ютері: поточний bundle
   ~10.809 GiB, додатково врахувати staging/encryption workspace, retained copies
   й резерв. Цей розмір історичний, не гарантія для нового snapshot.
2. Виконати «Узгоджений snapshot» нижче у новий приватний каталог із унікальним
   ID; зберегти DB dump, artifacts, CSV, manifests, release/hash і role/config
   manifest. Зафіксувати relative-path SHA-256 inventory всіх payload files.
   Cursor keyring/DSNs/Telegram secrets не включати у публічні inventories;
   приватний recovery config передати окремо зашифрованим, зі збереженим поза
   сервером ключем розшифрування. Не вважати конфіг без secrets достатнім restore.
3. З локального комп’ютера pull через SSH/rsync з обраного cloud host у **новий**
   каталог. Перевірити SSH host key; не вимикати host verification і не
   використовувати `--delete`. Приклад після визначення host і snapshot ID:

   ```sh
   umask 077
   mkdir -p "$HOME/backups/slovnyk"
   # Спочатку створити новий приватний destination; не використовувати старий ID.
   mkdir "$HOME/backups/slovnyk/NEW_ID"
   rsync -a --no-perms --no-owner --no-group backup@HOST:/PRIVATE/NEW_ID/ \
     "$HOME/backups/slovnyk/NEW_ID/"
   cd "$HOME/backups/slovnyk/NEW_ID"
   sha256sum --quiet --check inventory.sha256
   ```

   `HOST`, source directory та account — placeholders; налаштувати account для
   читання тільки backup staging, без owner/app credentials. Джерело й inventory
   мають бути довіреними; не виконувати довільний manifest із стороннього джерела.
4. Перевірити exit status, повноту списку/path/bytes, hashes і доступність
   recovery secrets. `sha256sum -c` перевіряє перелічені файли, але не доводить
   повноту комплекту. Тільки тоді записати приватний receipt «off-host verified».
   Помилка transfer/hash/secret escrow — незавершений backup, повторити; не
   підтверджувати оновлення і не видаляти попередню копію.
5. До відкриття відновити на окремій цілі з cloud source недоступним. Виміряти
   повний elapsed включно з transfer, config/secrets і controls: <=24 h.
   Перевірка hashes не замінює restore, а локальні ~56 хвилин не є target RTO.

Додаткову незалежну копію на випадок втрати самого комп’ютера та чисельний
retention budget ще потрібно визначити. Автоматичного backup timer немає.
Telegram обрано для сповіщень; credentials і реальна доставка ще не перевірені.
[Розгортання](../deploy/README.md) описує timer readiness/disk та його межі;
він не перевіряє актуальність off-host bundle й не сигналізує про повний outage
власного хоста. До запуску потрібна окрема зовнішня перевірка доступності.

## Що зберігати разом

- Повний logical dump нової БД: schema/migration checksums, canonical manifests,
  raw translations, import ledger/origins/issues, report/export/sitemap registry,
  active pointer/generation та publication history. Лише serving tables недостатньо.
- Увесь immutable artifact root із незмінними relative paths: exports, verification
  reports, sitemap index/shards/reports усіх збережених datasets, включно з retained.
- Оригінальні compressed CSV snapshots і їхні manifest files. Embedded DB manifest
  не замінює вихідні CSV bytes; вони потрібні для повторного відтворення імпорту.
- Перевірений release binary із SHA-256 і сумісну конфігурацію. Embedded UI catalogs
  входять до binary. Cursor keyring зберігати окремо захищено: втрата попередніх
  ключів робить раніше видані посилання continuation непридатними.
- Опис schema owner, group roles, LOGIN memberships і storage paths. DB dump не
  містить cluster roles; секрети LOGIN/DSN не включати до публічного inventory.

## Узгоджений snapshot

1. Призупинити **всі** operator writers: import, verify, sitemap, publish/rollback;
   також будь-яке очищення файлів. HTTP читає дані й може продовжувати роботу.
   Зберігати цю паузу до завершення dump і копіювання файлів; один `pg_dump`
   не створює атомарного snapshot між PostgreSQL та filesystem.
2. Зафіксувати active dataset/generation, schema/release version і inventory.
   Зняти повний custom archive через `pg_dump --format=custom --no-owner` із
   сумісним PostgreSQL client. Перевірити exit status, розмір і SHA-256 архіву.
3. Скопіювати artifacts та CSV snapshots у нове окреме місце. Зафіксувати всі
   relative paths, byte sizes і SHA-256; звірити копію. Не перезаписувати попередній
   backup. Успіх копіювання не замінює перевірку durability вибраного сховища.
4. Лише після повного комплекту й перевірки відновити operator jobs. Політику
   й off-host місце визначено вище; фактичні transfer, secret escrow, restore
   та місткість retention ще потрібно перевірити.

## Відновлення на окремій цілі

1. Вибрати нову порожню UTF-8 БД та окремий artifact root. Не використовувати
   application/legacy DB і не запускати destructive `--clean` над наявною БД.
2. На новому cluster спочатку provision group roles `slovnyk_http` і
   `slovnyk_operator` з NOLOGIN/NOSUPERUSER/NOCREATEDB/NOCREATEROLE/
   NOREPLICATION/NOBYPASSRLS, як у migration 1. Створити окремі LOGINs/memberships.
   Restore owner має бути окремим від HTTP/operator; не давати їм ownership.
   Створити цільову БД з цим owner. Для перевіреного шляху owner має NOLOGIN/
   NOSUPERUSER/NOCREATEDB/NOCREATEROLE/NOREPLICATION/NOBYPASSRLS; адміністративний
   restore connection має право `SET ROLE` на нього. Не копіювати старі cluster
   globals або grants адміністратора application LOGINs.
3. Звірити backup inventory, відновити schema/data/ACL через
   `pg_restore --no-owner --role=<new-schema-owner> --exit-on-error --single-transaction`
   із явною новою database target. Успішний dump вже містить schema та migration ledger:
   попереднє створення application tables через `migrate` не потрібне. Без потрібних
   grantee roles restore має завершитися помилкою; не обходити її через `--no-acl`,
   інакше робочі privileges не відновляться. Після усунення причини повторити
   restore й перевірити відсутність часткової schema після невдалого запуску.
   Для переходу зі старого release спочатку перевірте restore тим самим release;
   перед запуском поточного binary виконайте owner `migrate` до schema 4 за
   [процедурою](../README.md#міграції-нової-бд). Schema 3 backup не містить
   нового prefix index; його побудова потребує окремого часу, місця та зупинки writers.
4. Після успішного restore оновити planner statistics через `ANALYZE` на
   відновленій БД до HTTP-приймання; це рекомендує
   [PostgreSQL 17](https://www.postgresql.org/docs/17/app-pgrestore.html#APP-PGRESTORE-NOTES).
   Відновити всі файли зі збереженням relative paths; звірити hashes/bytes.
   Запустити той самий release/keyring із restricted HTTP LOGIN і read-only
   artifact mount. Не переносити traffic до проходження readiness і control checks.
5. Звірити active version/generation, counts/digests, provenance та manifests;
   окремо звірити sequence state/config, які не входять до звичайних table digests.
   Перевірити exact/mask search, старі continuations, article, active/retained
   downloads і sitemap. Переконатися, що HTTP LOGIN не може змінити active pointer.
   Перевірити операторський retry на відновленому наборі.
6. Зберегти результати й повний elapsed time. Відсутні artifacts мають блокувати
   readiness. Ready саме по собі не доводить цілісність усіх retained файлів —
   їх повинен покрити backup inventory. Не видаляти вихідну ціль або backup.

## Автоматична локальна репетиція

```sh
make test-integration
```

`tests/serviceflow/lifecycle_test.go: TestServiceLifecycle` створює synthetic A/B
через фактичний CLI, публікує B і повертає A. Після зупинки всіх fixture writers
`restoreFixture` з `restore_test.go` знімає custom dump і копіює artifacts/CSV.
`pg_dump` та `pg_restore` виконуються у власному закріпленому PostgreSQL 17.11
контейнері runner; host PostgreSQL tools не потрібні.

Runner створює **другий порожній cluster** у своєму Compose project із окремим
volume і loopback port. Cluster system identifiers мають відрізнятися; source
owner і application groups на цілі спочатку відсутні. Перший restore без groups
завершується помилкою та не залишає application schema. Після provision нових
groups/LOGINs той самий dump відновлюється під новим NOLOGIN non-superuser owner.
Schema, relations, functions і types мають саме цього owner.

Counts і length-prefixed SHA-256 sorted JSON rows **кожної** таблиці збігаються;
усі artifact/source paths, sizes та hashes також. Цей table digest — test oracle,
не публічний версійний migration digest. Identity sequence `import_issues_id_seq`
перевіряється окремо: `last_value` та `is_called` усіх application sequences
мають збігатися. Synthetic fixture задає їй non-default state `37 / true`
до dump, щоб перевірка не залежала від наявності rejected records.

Оригінальна БД під час HTTP-приймання недоступна. Порожній artifact root дає
health 200/readiness 503; повна копія запускається, відтворює responses/cursors,
retained A/B files, sitemap і restricted-role behavior: SQLSTATE 42501 для
HTTP UPDATE/DDL/register та operator direct inventory DELETE/DDL. Оператор повторює sitemap
registration без зміни report; SECURITY DEFINER працює з новим owner. Fixture
keyring заданий тестом; secret escrow, off-host transfer і повний corpus тут
не перевірені. Обидва clusters залишаються на одному developer/CI host.
Cleanup видаляє лише власні процеси, БД/LOGINs, обидва containers/volumes та scratch backup.
Це developer/CI test, не операторська backup utility й не посіпака агента.

[Фактичний звіт](_assessments/2026-10-25-backup-restore/REPORT.md),
[Fresh-cluster доповнення](_assessments/2026-10-25-fresh-cluster-restore/REPORT.md),
[TRACEABILITY](TRACEABILITY.md). Production readiness, storage capacity та
погоджене RTO залишаються відкритими.
