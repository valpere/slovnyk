# План міграції даних

Це план production-операції з окремими локальними synthetic перевірками.
Production міграцію не виконано; старі файли залишаються незмінними.
Підстава — [LEGACY](LEGACY.md); технічні компоненти — [HLD](HLD.md).
Користувач підтвердив 2026-10-23: новий сервіс має окрему нову БД з даними
старого сервісу; структура може відрізнятися зі збереженням змісту даних.
2026-10-23 користувач підтвердив доступні джерела: DDL + CSV.
CSV наповнює нову БД; DDL використовується для розуміння старої структури.
Наявні CSV підтверджено як остаточне джерело для першого запуску
(2026-10-23). Старий сервіс більше не працює; live cutover, freeze
та delta sync не потрібні. Аудит джерел і погодження втрат залишаються.

## 1. Зафіксувати доступні джерела

- Інвентаризувати DDL і CSV; PO обліковувати окремо як джерела локалізації UI.
- Використовувати CSV-колекцію як джерело перекладних зв’язків. DDL пояснює
  стару схему, але не відновлює вміст таблиць; старий DDL не запускати.
- Live БД, дамп даних і DB snapshot adapter не є передумовами міграції.
  Якщо згодом з’явиться інше джерело, окремо визначити його provenance
  та правила поєднання; не змішувати різні дати мовчки.
- Гарантія — збереження доступних CSV4-зв’язків, **не** всього історичного
  вмісту БД. `count/status/queries` у CSV4 відсутні й не вигадуються.

Результат: source manifest з hashes, форматами, датою snapshot (або unknown),
правами на дані, відомими прогалинами й погодженим scope. Старі файли readonly.

Для відтворюваної підготовки V1 manifest існує Go utility
[prepare-csv-manifest](../utilities/README.md#підготовка-csv-manifest): hashes/bytes
і filename expectations для окремої замороженої пласкої копії CSV.gz.
Snapshot date лишається unknown, якщо точна дата невідома. Після inventory
потрібен окремий `import inspect`; manifest не є validation report.
[Фактична репетиція](_assessments/2026-10-25-csv-manifest/REPORT.md) звірила всі
992 compressed hashes/bytes, без імпорту у нову PostgreSQL БД.
Окремий [повний Go inspect](_assessments/2026-10-25-go-corpus-inspect/REPORT.md)
перевірив gzip/CSV та EOF/checksums усіх цих файлів: 56 455 068 parsed occurrences,
0 rejected, збіг кількостей у кожному файлі з аудитом. Це не staged import,
distinct tuple reconciliation або дозвіл на publication.

Перед повним PostgreSQL import виконано bounded [preflight](_assessments/2026-10-25-postgres-corpus-preflight/REPORT.md):
SIGINT/retry на тому самому manifest зберіг 55 000 committed prefix records і
додав наступні до 114 000, active null. Dataset лишився `loading`; жодного файла
ще не завершено. Враховуйте окремо БД/indexes, WAL, temp space, exports і backup
перед повним запуском. CLI `files=null` після втрати з'єднання означає недоступний
report, а не порожній checkpoint; відновлюйте тією самою командою без ручних UPDATE.

Наступний [повний staged import](_assessments/2026-10-27-postgres-corpus-import/REPORT.md)
завершив той самий dataset у стані `validating`: усі 992 files complete і
checksum-verified, 56 455 068 accepted unique, 0 duplicates/rejected/issues.
Actual counts ledger/origins/translations, per-file/pair totals та checkpoints
звірені; SHA-256 попередніх 55 000 рядків незмінні. Це не повний source replay,
normalization reconciliation або verify/exports/ready; active лишився null.
Поточний strict verifier блокує known_gaps manifest. Не обходьте це вилученням
gap: потрібні approval-loss adapter і погодження власником конкретного звіту.

До цього погодження використовуйте окремий `dataset reconcile ID --source DIR`
для source/normalization та inventory checks без ready/exports/publication.
Він повертає diagnostic JSON з fingerprint і незакритими gaps/exceptions;
не змінює manifest або persistent dataset. Наявність такого звіту не є
погодженням втрат. [Synthetic evidence](_assessments/2026-10-27-dataset-reconciliation/REPORT.md).
Повний PostgreSQL корпус цією командою ще не звірено.

DB/indexes одного dataset — 64.315 GiB, WAL окремо 1 GiB. Такий layout не
поміщається на root ~45 GiB за профілем Oracle 2026-10-23; нового probe не було.
Плануйте додатково другий dataset, WAL/temp, exports і backup/restore copies.
Локальний змішаний memory cap 512 MiB → 8 → 16 GiB при 1 CPU не доводить
Micro capacity, пошуковий SLO або RTO; ці gates залишаються окремими.

## 2. Повний аудит

Структурну частину виконано 2026-10-24: 992 файли, 56 455 068 різних
кортежів, 0 точних дублікатів і 0 структурних відхилень за перевіреними
правилами. SHA-256 повторно звірені, strict gzip boundaries перевірені.
Особливості raw-написань і межі доказу — у
[звіті](_assessments/2026-10-24-csv-audit/REPORT.md). Перевірки нижче
залишаються вимогами до production import/verify. Окремий
[локальний прототип](_assessments/2026-10-24-provenance/REPORT.md) звірив усі
translations/origins за CSV ordinal і всі 992 source/DB/export digests.
Семантична правильність перекладів і production migration ще не підтверджені.

Перевірити кожен gzip до EOF, декодування, кожен логічний CSV-запис і кількість
полів. Порахувати напрямки, розміри/довжини, дублікати, Unicode-колізії,
невідомі коди, порожні значення, відмінності вмісту від назв файлів.
Результат повинен пояснити всі 992 наявні архіви, але не припускати, що це
назавжди правильна кількість чи повна матриця мов.

`sr-rs → sr-rs` не відкидати. Відсутній `en-gb → fr-fr` не відновлювати
штучно. Старий alias `sr-sp → sr-rs` не застосовувати до всіх джерел без
перевірки та версійного mapping. Зберігати вихідні локалі й тексти.

## 3. Репетиція у новій БД

Створити нову схему; виконати dry-run та import у staging. Звірити повні
per-pair digests і totals, exact/mask queries, punctuation/Unicode fixtures,
CSV export round-trip. Перевірити fault injection: обрив gzip, DB disconnect,
зупинка після batch, повтор імпорту й конкурентний publish.

Для файлів, які коректно розібрані повністю:

```text
parsed logical records = accepted unique + duplicate occurrences + rejected
manifest files = completed files + explicitly failed/excluded files
```

Для failed CSV без визначених меж записів повний parsed count невідомий;
такий файл не може виглядати успішно врахованим. Дублікати не збільшують
число унікальних зв'язків, але зберігають усі origins і legacy metadata.
Conflicting metadata не усереднювати й не обирати випадково.

Будь-яка втрата має reason, locator і явне рішення. Власник погодив
можливість запуску без записів, які неможливо коректно імпортувати,
**після його схвалення конкретного звіту про втрати** (2026-10-23).
До схвалення звіт блокує публікацію; загальна згода не є дозволом на
довільні пропуски. Без unresolved errors або з документованими
погодженими exceptions dataset може пройти verify. Fatal file failure
потребує виправлення або явного виключення artifact у новому manifest
за [контрактом імпорту](IMPORT_CONTRACT.md).
Зберегти результати, витрати часу/RAM/диска та query latencies; числовий
capacity gate погодити до production, коли відомі ресурси й трафік.

## 4. Перемикання

Наведені production-дії — майбутній план, не дозвіл виконувати їх зараз.
Oracle Micro досі обслуговує інший проєкт (уточнено 2026-10-24);
його роботу не порушувати. Поточний доступ — лише легкі read-only перевірки.
Репетиції імпорту, навантаження й відновлення продовжувати локально.

1. Перевірити погоджений scope, UI-мови й приймальні тести.
   Права на словникові дані підтверджені власником (OQ-05 закрито).
2. Підготувати backup нової БД + artifacts і **перевірити restore** окремо.
   [Локальна SQLite-проба](_assessments/2026-10-24-sqlite-full/REPORT.md)
   перевірила копіювання закритої експериментальної БД (127.065 s,
   hashes збігаються, quick_check=ok); production backup з off-host
   відновленням сервісу й artifacts за <=24 h ще потрібний.
   [PostgreSQL synthetic rehearsal](BACKUP_RESTORE.md) відновлює нову БД й
   окремий artifact root, звіряє всі таблиці та контрольні HTTP-сценарії при
   недоступній вихідній БД. Наступне доповнення перевіряє окремий чистий cluster
   на тому самому host: нові roles, інший schema owner, restore ACL і SECURITY
   DEFINER. Це ще не off-host proof.
3. Зафіксувати hashes остаточного CSV snapshot; перевірити відповідність
   погодженому manifest. Freeze, dual-write та delta sync не застосовуються.
4. Імпортувати фінальний snapshot, verify, exports, atomic publish.
   Publish/rollback використовує expected-generation CAS за [IMPORT_CONTRACT](IMPORT_CONTRACT.md).
5. Запустити новий сервіс на `slovnyk.org` (основний домен). Налаштувати
   автоматичне перенаправлення `slovnyk.org.ua` на основний домен зі
   збереженням шляху й параметрів нових URL (погоджено 2026-10-23).
   Власник підтвердив доступ до керування DNS обох доменів (2026-10-23).
   До запуску перевірити DNS і підготувати TLS для обох доменів.
   Зміни proxy/DNS виконати окремою операторською дією. Підтримка старих
   URL не потрібна (OQ-04). Виконати smoke tests: пошук, маска, UI locale,
   download, readiness.
6. Зберегти release artifacts, конфігурацію й snapshots нового сервісу
   для відновлення. Пороги помилок/затримок — до запуску. Старого
   робочого endpoint для повернення трафіку немає.

## 5. Відкат

За проблем із новим dataset перемкнути active pointer на попередній перевірений;
за проблем із застосунком повернути попередній перевірений реліз нового
сервісу, якщо він є. На першому запуску попереднього релізу може не бути:
тоді зупинити проблемний реліз і відновити працездатність з перевірених
artifacts/backup. Повернення до старого сервісу неможливе. Після повної
втрати сервера відновлення має вкладатися в 24 години; потрібна репетиція.
Поточний план покриває запуск без редагування словника й нової статистики.
Редагування, реєстрація й консиліум ШІ погоджені як наступний етап
після запуску пошуку та завантажень (OQ-07). Перший реліз без користувацького запису.
**До запуску запису** переглянути план: відкат dataset або релізу
не повинен втрачати нові внески. Потрібні їх збереження, backup/restore журналу
редакцій і перевірений сценарій відновлення без втрати схвалених змін.

Відкат не видаляє failed/new dataset або звіти автоматично. Після restore
перевіряються active version, hashes, counts, provenance й контрольні запити.
Факт завершення міграції засвідчує звіт репетиції та фінальної операції,
а не лише успішний exit code імпортера.
