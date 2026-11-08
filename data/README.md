# Локальні дані

`data/experiments/` — постійне локальне сховище БД та експортів технічних
експериментів. Воно виключене з Git і не входить у чистий checkout.
Не очищати його разом із тимчасовими файлами. Це експериментальні дані,
не production-БД і не резервна копія поза машиною.

| Директорія | Вміст |
|---|---|
| `experiments/sqlite-probe/` | Початкова SQLite-проба чотирьох мовних пар |
| `experiments/sqlite-full/` | Повна serving-БД `corpus.sqlite3` та результати; restore-копію прибрано за погодженням 2026-11-03 |
| `experiments/provenance/` | Повна модель `model.sqlite3` із ledger/origins та 992 CSV.gz у `exports/` |
| `experiments/provenance-indexed/` | Похідну БД 29.66 GiB прибрано за погодженням 2026-11-03; відтворюється з `provenance/model.sqlite3` |
| `experiments/provenance-compact/` | Логічно звірена повна модель із compact dataset/artifact keys і тими самими indexes; БД 20.85 GiB |

2026-10-24 `sqlite-probe/`, `sqlite-full/` та `provenance/` перенесені
з однойменних директорій у `tmp/`. Наступні експерименти створюються одразу тут.
Перенесення відбулося в межах файлової системи; для кожного файлу перевірено
незмінність device/inode та розміру. Вміст БД та експортів не перезаписувався.
Старі шляхи в історичних звітах позначають місце первинного експерименту;
поточне розташування — тут.

Оригінальні CSV та старий код залишаються у `../slovnyk1.0/` без змін.
Інструменти та звіти зберігаються в `docs/_assessments/`.
Для нових запусків створювати окрему директорію в `data/experiments/`,
не перезаписувати наявні результати. `tmp/` використовувати для scratch-файлів.

## Runtime artifacts

`data/artifacts/` — окремий ignored root нової реалізації для immutable exports
та verification reports, не експериментальні SQLite exports. Provisioning:
`go run ./utilities/prepare-artifacts data/artifacts`. Ready/publication зберігають
посилання сюди; не очищати разом із `tmp/`, не змінювати final bytes, GC вимкнений.
HTTP отримує read-only mount. Backup має включати БД і всі referenced artifacts.

## Manifest репетиції CSV snapshot

`experiments/csv-manifest-2026-10-25/` містить окрему regular-file копію 992
CSV.gz у `snapshot/` (535 766 648 compressed bytes) і приватний `manifest.json`
нової Go utility. Hashes/bytes усіх файлів збігаються з аудитом 2026-10-24.
Fingerprint `ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
Snapshot date — `unknown`; mtime не використано як дату даних. Копію та manifest
не перезаписувати; нова версія snapshot/метаданих потребує нового місця.

У legacy directory також є `slovnyk_en-us_pl-pl.csv.zip`: його єдиний CSV має
той самий SHA-256 decompressed bytes, що відповідний CSV.gz. ZIP залишився
у legacy; новий snapshot має явний формат CSV4.gz. Жодних унікальних даних цим
вибором не вилучено. [Доказ і межі](../docs/_assessments/2026-10-25-csv-manifest/REPORT.md).
Ця репетиція підготувала лише inventory; наступну перевірку описано нижче.
CSV contents у Git не входять.

## Повний Go inspect

`experiments/go-inspect-2026-10-25/` зберігає `inspect.json`, invocation/execution
metadata, `time.txt` і `comparison.json`. Перевірено саме snapshot/manifest вище:
992 complete/checksum-verified files, 56 455 068 parsed occurrences, 0 rejected;
per-file counts збігаються з аудитом. Metadata-only копії результатів у
[звіті](../docs/_assessments/2026-10-25-go-corpus-inspect/REPORT.md).
Результати не перезаписувати; повторний запуск потребує нової директорії.
Це не distinct tuple count, PostgreSQL import, verify або publication.

## PostgreSQL corpus preflight

`experiments/postgres-corpus-preflight-2026-10-25/` містить окремий `.pgdata/`,
private `credentials.json`, Compose `override.yaml` та metadata-only результати
двох контрольовано перерваних CLI запусків. 114 000 committed records того самого
manifest, стан `loading`, active null, 0 complete files. Prefix 55 000 records
зберіг counts/digests після retry. [Evidence](../docs/_assessments/2026-10-25-postgres-corpus-preflight/REPORT.md).

Ці числа описують historical preflight checkpoint; після повного import
та сама `.pgdata/` містить 56 455 068 records; після verify стан `ready` (див. нижче).
Власні container/network прибрані; bind data і credentials залишені для
продовження. Не видаляйте/не копіюйте live pgdata й не перезаписуйте результати.
Для повторного запуску container потрібні той самий image, project/config та
локальні credentials. Перерваний import відновлюють тією самою command/environment;
завершений import не потрібно повторювати для наступних verification кроків.
Credentials, pgdata та raw dictionary contents не входять у Git.
Preflight сам не створив ready dataset або backup; estimate з однієї пари не є capacity gate.

## Повний PostgreSQL corpus import

`experiments/postgres-corpus-import-2026-10-25/` містить metadata-only results
повного resume attempt і фінальні SQL counts/digests, перевірені 2026-10-27.
DB не дублювалася: storage лишається `.pgdata/` у preflight root вище.
Усі 992 files complete/checksum-verified, 56 455 068 unique records,
0 duplicates/rejected/issues; dataset `validating`, active null.
[Evidence і межі](../docs/_assessments/2026-10-27-postgres-corpus-import/REPORT.md).

DB/indexes 64.315 GiB плюс WAL; exports/ready/publication і full source
reconciliation ще не виконано. Known gaps не видалено й не схвалено цим run.
Після SQL checks власні container/network прибрані, input і storage збережені.
Для продовження відновіть той самий bind/image/private credentials та отримайте
новий loopback port. Memory caps import 512 MiB → 8 → 16 GiB були локальними
live updates; базовий Compose знову задає 512 MiB. Ресурсний профіль нового
експерименту задавайте й фіксуйте явно; не вважайте його Oracle acceptance.

## Повна source/DB звірка

`experiments/corpus-reconciliation-2026-11-03/` зберігає cleanup receipt і
незавершену спробу; `experiments/corpus-reconciliation-2026-11-04/` — успішний
повторний full replay, report та metadata. [Evidence](../docs/_assessments/2026-11-04-corpus-reconciliation/REPORT.md).
БД та snapshot ті самі, dataset validating/active null; known gaps незакриті.
Власні container/network прибрані, bind DB збережено.

## Повний verify та exports

`experiments/corpus-verification-2026-11-04/` зберігає перервану спробу;
`experiments/corpus-verification-2026-11-04-retry/` — успішний full verify,
owner receipt, progress/report metadata та immutable `artifacts/`.
992 CSV4.gz exports містять 56 455 068 записів; окремий `dataset export`
повторно перевірив усі файли й retained report. Та сама bind DB тепер `ready`,
active null/generation 0. Gaps збережені з явним погодженням власника.
[Evidence](../docs/_assessments/2026-11-04-corpus-verification/REPORT.md).
Власні container/network прибрані; DB, snapshot і referenced artifacts збережені.
Не видаляйте artifacts разом із `tmp/`; це локальний результат, не off-host backup.

## Повний sitemap та локальний HTTP

`experiments/corpus-sitemaps-2026-11-04/` зберігає metadata generation/retry,
XML validation, publication та HTTP checks. Sitemap artifacts додані в
**той самий** `experiments/corpus-verification-2026-11-04-retry/artifacts/`;
новий порожній root не замінює retained exports/report. 736 XML shards
містять 36 763 066 URL, додатково 3.835 GiB плюс index/JSON report.
[Evidence](../docs/_assessments/2026-11-04-corpus-sitemaps/REPORT.md).

Та сама `.pgdata/` тепер active на ready dataset, **generation 1** після
локальної publication. Усі shards/992 downloads перевірені через HTTP;
власні containers/network і тимчасовий HTTP LOGIN прибрані.
Private HTTP env/credentials лишаються поза Git; для продовження створіть
окремий restricted LOGIN, не використовуйте видалений. DB/snapshot/artifacts
збережені, це не off-host backup або production deployment.

## Локальна змішана проба

`experiments/corpus-load-2026-11-04/` зберігає протокол, per-request metadata,
resource samples і фінальні SQL/lifecycle checks трьох коротких HTTP фаз.
[Evidence](../docs/_assessments/2026-11-04-corpus-load/REPORT.md): 2 112 успішних
запитів; active generation 1 та retained DB/snapshot/artifacts незмінні.
Private workload містить реальні терміни; `http.env` і `http-private.json`
містять credentials. Вони ignored, не публікувати. Власні containers/network
та LOGIN прибрані; старі credentials не придатні для наступного запуску.

## Повний локальний backup/restore

`experiments/corpus-restore-2026-11-04/` зберігає `backup/database.dump`, незалежні
`backup/artifacts/`, `backup/sources/`, manifest і release binary; `restored/` —
другі звірені file copies. Окремий restored `.pgdata/` видалено 2026-11-08
з дозволу власника; решта цього комплекту збережена. Оригінальна
БД/snapshot/artifacts збережені. [Evidence](../docs/_assessments/2026-11-04-corpus-restore/REPORT.md).
Власні containers/networks і тимчасові reader/operator LOGINs прибрані;
Конфіг new owner/admin лишається локально. `*-private.json`, `*.env`, raw logs і workload
paths містять secrets або реальні дані: не публікувати. Bundle не містить публічного
keyring; rehearsal key/LOGIN configuration лишаються приватно поряд. Це той самий
host, не off-host backup; durability/escrow/RPO/retention ще не погоджені.
Не прибирайте backup/restored copies без окремого рішення власника.


## Prefix query-plan діагностика

`experiments/mask-plans-2026-11-04/` зберігає metadata HTTP/EXPLAIN, state
snapshots і lifecycle. [Evidence](../docs/_assessments/2026-11-04-mask-plans/REPORT.md).
`private.json`, `*-http.env` та `*-plan-private.json` містять secrets або
реальні filter literals; не публікувати. Вхідний workload лишився в попередньому
`corpus-load-2026-11-04/workload-private.json`. One-off harness — `tmp/mask-plans/`.
Тимчасовий diagnostic LOGIN і власні containers/network прибрані. Оригінальні
DB/sources/artifacts та всі backup/restore copies збережені; схема незмінна.
Для наступного запуску створіть свіжий restricted LOGIN і rediscover container ID.


## Bounded-prefix copy-only prototype

`experiments/prefix-index-2026-11-04/` містить окрему `.pgdata` із 16 повними
скопійованими мовними парами, synthetic/write-sample tables та candidate index.
[Evidence](../docs/_assessments/2026-11-04-prefix-index/REPORT.md). Оригінальний
корпус і всі backup/restored copies незмінні; runtime containers/networks/source
reader прибрані. Source admin та experiment admin config лишаються приватними.
`*-private.json` і raw plans не публікувати; workload містить реальні терміни.
`initial-default-collation/` — збережені замінені receipts, не acceptance evidence.
Повторні результати в root run directory мають production collation C.
One-off Go harness й orchestration — `tmp/prefix-index/`, не service utilities.
Це subset SQL prototype, не повна service DB або production migration.

## Повний корпус: schema 4 prefix index

`experiments/full-prefix-2026-11-04/` зберігає нові metadata/inventories та
приватні diagnostics повного upgrade. **Початкова** `.pgdata/` у
`postgres-corpus-preflight-2026-10-25/` тепер schema 4: 56 455 068 translations,
ready/active generation 1, application hashes і sequences незмінні.
Index 2.428 GiB, БД 66.744 GiB без WAL/artifacts. Temporary reader і власні
containers/network прибрані; bind збережено. Старі credentials з container/port
треба rediscover при наступному запуску; новий temporary reader уже нечинний.

Backup bundle і restored cluster у `corpus-restore-2026-11-04/` залишилися
schema 3, їх цей upgrade не змінював. **2026-11-08 власник окремо дозволив
видалити лише стару restored `.pgdata/`** для місця під нову репетицію: звільнено
58.56 GiB; backup bundle, restored files і початкова schema 4 БД збережені.
Для поточного binary БД, відновлену зі старого dump, треба
окремо мігрувати owner-командою. Новий schema 4 backup/restore описаний нижче.
[Evidence та наступні gates](../docs/_assessments/2026-11-04-full-prefix/REPORT.md).


## Повний backup/restore schema 4 — 2026-11-08

`experiments/schema4-restore-2026-11-08-retry/` зберігає новий `backup/`
із custom dump, release binary, manifest, незалежними artifacts/sources;
`restored/` — другі звірені файлові копії, `.pgdata/` — свіжий schema 4 cluster.
17 таблиць/sequences, prefix index/ledger та HTTP/cursors збігаються з source;
усі 736 shards, index і 992 downloads перевірені через target HTTP.
[Evidence](../docs/_assessments/2026-11-08-schema4-restore/REPORT.md).

Власний runtime й temporary reader/operator LOGINs прибрані; bind data й файли
збережені. Source admin і новий target owner/admin конфіг приватні; старі ports,
container IDs і видалені LOGINs не використовувати повторно.
`target-private.json`, `*-private.json`, `*.env`, raw logs і HTTP paths не публікувати.
One-off scratch — `tmp/schema4-restore-retry/`; повторний inventory — Go minion.

Попередня спроба `experiments/schema4-restore-2026-11-08/` перервана reboot під час
source inventory, до dump/target; її receipts і `tmp/schema4-restore/` збережено.
Повторний повний source inventory підтвердив збіг із PR27 після reboot.
Дозвіл видалення стосувався лише старого schema 3 restored cluster; інші копії
не видаляти без окремого рішення. Це один host; off-host/escrow/RPO/retention відкриті.


## HTTP/cache/contention probes — 2026-11-08

`experiments/query-workloads-2026-11-08-retry/` та
`experiments/query-workloads-largest-2026-11-08/` зберігають дві завершені
[діагностичні серії](../docs/_assessments/2026-11-08-query-workloads/REPORT.md):
private configs/paths, ресурси, observations, приватні PG logs і post-workload
inventories. Обидві використовували лише успішний schema 4 restored bind
`experiments/schema4-restore-2026-11-08-retry/.pgdata` та його artifacts;
нових DB copies не створювали. Inventories тотожні restore; data збережено.

Runtime/reader LOGINs прибрані; ephemeral ports/IDs у `runtime-private.json`
не використовувати повторно. Keyring/DSN/passwords, terms/paths і raw logs
не публікувати. Scratch — `tmp/query-workloads/`; повторюваний runner —
Go-посіпака `minions/http-probe`. Перший `query-workloads-2026-11-08/`
зберігає preflight failure receipt (PGDATA host permissions), runtime не запускався.
Додаткових видалень не погоджено й не виконано. Oracle/legacy не задіяні.
