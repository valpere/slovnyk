# Backup і restore нового сервісу

FR-13 / AT-13, без користувацького запису. Production та off-host відновлення
ще не виконано. Локальна synthetic репетиція описана нижче; вона не доводить
RTO <=24 h для повного корпусу.

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
4. Лише після повного комплекту й перевірки відновити operator jobs. Off-host
   місце, спосіб захисту конфігурації, розклад/RPO та retention ще потрібно визначити.

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
4. Відновити всі файли зі збереженням relative paths; звірити hashes/bytes.
   Запустити той самий release/keyring із restricted HTTP LOGIN і read-only
   artifact mount. Не переносити traffic до проходження readiness і control checks.
5. Звірити active version/generation, counts/digests, provenance та manifests;
   перевірити exact/mask search, старі continuations, article, active/retained
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
не публічний версійний migration digest. Послідовностей у поточній schema немає;
якщо з'являться, rehearsal має перевіряти також їхні значення.

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
