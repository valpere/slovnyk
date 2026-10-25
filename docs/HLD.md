# High-Level Design

Це цільова будова нового застосунку. M1-01 реалізує `cmd/slovnyk`,
`internal/ops` і `slovnyk serve`; M1-02 — `internal/normalization` та
`internal/importdata` як спільні format/key primitives.
M1-03 додає `internal/storage/postgres`, embedded SQL і `slovnyk migrate`;
M1-04 додає `internal/importer` і `import inspect/run` до стану `validating`;
M1-05 додає `internal/artifacts`, `internal/publication`, dataset CLI та HTTP readiness.
M1-06 додає `internal/lookup` і `postgres.ReadPage`: exact/mask, signed cursor,
C seek, pinned metadata й спільний SQL deadline; HTTP adapter — M1-07.
Нормативна поведінка — у [REQUIREMENTS](REQUIREMENTS.md).

## Компоненти та майбутня структура

```text
cmd/slovnyk/               # serve та операторські subcommands
internal/catalog/         # локалі, напрямки, dataset metadata
internal/lookup/          # запит, нормалізація, результати, cursor
internal/normalization/   # реалізовані versioned keys/query/mask primitives
internal/importdata/      # реалізовані CSV4, manifest і tuple digests
internal/importer/        # inventory, parsers, staging, reconciliation
internal/artifacts/       # immutable local files, no-clobber install/fsync
internal/publication/     # verify, publish, rollback, export
internal/storage/postgres/
internal/httpui/          # handlers, locale fallback
internal/ops/             # config, health, diagnostics
migrations/               # лише нова схема, версійні SQL
web/templates/            # html/template
web/static/
web/locales/              # перевірені ресурси UI
testdata/                 # синтетичні fixtures, без повного корпусу
docs/
```

Go HTTP і CLI використовують ті самі application services. В HTTP немає
операторських routes. `serve` доступна за [README](../README.md);
Наведені CLI commands реалізовані; paths і credentials — за README:

```text
slovnyk serve
slovnyk migrate
slovnyk import inspect --manifest <path> --source <directory>
slovnyk import run --manifest <path> --source <directory> [--batch-size 100]
slovnyk dataset verify <id> --source <directory> --artifacts <directory>
slovnyk dataset export <id> --artifacts <directory>
slovnyk dataset publish <id> --artifacts <directory> --expected-generation <N> --operator-ref <ref>
slovnyk dataset rollback <previous-id> --artifacts <directory> --expected-generation <N> --operator-ref <ref>
```

<a id="fun-catalog"></a>

## FUN-CATALOG — довідник і версії

Модель зберігання (DDL створюється при реалізації); точні ключі, типи та
інваріанти — у [DATA_MODEL](DATA_MODEL.md):

| Сутність | Ключові поля та правила |
|---|---|
| `datasets` | ID, manifest hash, parser/mapping/normalization versions, state, timestamps; незмінна після ready |
| `active_dataset` | Один рядок з dataset ID і generation; publish/rollback — lock + expected-generation CAS |
| `dataset_locales` | `(dataset_id, code)`, назва; сирі legacy-коди зберігаються без злиття |
| `dataset_pairs` | `(dataset_id, source_locale, target_locale)`, число унікальних зв'язків, export reference |
| `translations` | ID, dataset ID, source/target locale, source/target text, source search key |
| `source_files` | Dataset, artifact hash, relative name, format, bytes, logical record counts, validation state |
| `translation_origins` | Translation ID, source file/record locator, чотири сирі поля до mapping, nullable legacy count/status; усі входження дублікатів |
| `import_runs` | Dataset, run identity, state, batch checkpoints, totals, failure reason |
| `import_issues` | File, locator, issue code, безпечний опис; raw record в окремому контрольованому quarantine за потреби |
| `legacy_query_snapshots` | Доступні поля `queries`, raw timestamp без timezone, походження; не нова live-аналітика |
| `exports` | Dataset/pair, artifact path, content hash, compressed bytes, logical record count |

Логічна унікальність перекладу: dataset + чотири **оригінальні** значення.
Не покладатися на B-tree всього довгого текстового кортежу: при реалізації
використати bounded digest key з перевіркою рівності оригіналів; hash collision
має зупинити verify з явною помилкою, а не злити різні записи.
Нормалізований ключ не унікальний. Ідентичні ключі не об'єднують написання.
Усі посилання обмежені одним dataset; FK та verify перевіряють цілісність.

<a id="fun-lookup"></a>

## FUN-LOOKUP — пошук

HTTP-контракт пропозиції: `GET /search?from=uk-ua&to=en-us&q=...&ui=uk&cursor=...`.
`mode` — `auto` (за відсутності параметра), `exact` або `mask`. В `auto`
наявність `*`/`?` означає mask, інакше exact; основна форма використовує auto. `GET /` показує форму; `GET /languages` — каталог.
HTML — основний інтерфейс; публічний JSON API поки не обіцяється.

1. Перевірити розмір, UTF-8, mode і підпис cursor; locale/pair перевіряти
   лише після вибору pinned dataset у кроці 2. Початкові конфігуровані
   defaults: 1024 Unicode code points запиту, 8 KiB query text, 50 зв'язків
   на сторінку, максимум 100; DB timeout 2 s. Це захисні defaults, не SLO.
2. Зафіксувати active dataset ID на перший запит або published retained ID
   з cursor; вибрати його normalizer, перевірити locale/pair. Обрізати крайні пробіли запиту,
   застосувати NFC → Unicode case folding → NFC; так само будувати ключ з
   source text, але без обрізання/очищення raw values. Версію бібліотеки та
   Unicode-даних фіксувати у normalization version.
3. Для `auto` визначити ефективний режим за наявністю `*`/`?` у запиті;
   явний `exact`/`mask` має пріоритет. Exact використовує рівність ключа. У mask `*` компілюється в `%`, `?` у `_`,
   буквальні `%`, `_` та escape-символ екрануються для SQL LIKE з явним ESCAPE.
   Довільні regular expressions і SQL від клієнта не приймаються.
4. Сортування за `(source_key, source_text, target_text, id)` з визначеною
   bytewise/C collation. Page обмежена rows і serialized bytes; lookahead встановлює `has_more`. Групування source text
   виконується під заголовком оригінального source text лише в межах сторінки;
   різні raw-написання не зливаються. Велика група може продовжуватися на
   наступній сторінці з повтореним заголовком (погоджений напрям UI).
5. Cursor містить dataset, fingerprint параметрів і anchor translation ID,
   з якого сервер відновлює sort tuple; повних текстів у token немає;
   перевіряється цілісність і межі. Неправильний cursor → 400, недоступна стара
   версія → 410 з пропозицією повторити пошук. Dataset не змінюється між сторінками.

Детальний контракт HTTP, code points після fold, dataset pin до валідації
та byte budget — у [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md).
Індекс exact: dataset + pair + digest нормалізованого ключа + residual equality.
Не створювати B-tree з повними довгими текстами. Для mask — виміряти prefix
та infix плани; не обіцяти швидкий `%...%` без вимірювань. Усі режими під timeout.
Timeout → явна помилка обмеження (503), а не «нічого не знайдено».
Новий збір `queries` у цей потік не входить.

<a id="fun-ui"></a>

## FUN-UI — інтерфейс і локалізація

Каталоги локалей словника відокремлені від UI-кодів. Пріоритет: явний
підтримуваний `ui` → збережений підтримуваний ручний вибір → підтримувана мова
з `Accept-Language` → англійська (`en`). Ручний вибір зберігається між
відвідуваннями без JavaScript; невідомий збережений код пропускається.
Реалізовано host-only HttpOnly/SameSite=Lax cookies на рік; Secure для TLS
та production hosts. Деталі — [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md#каталог-і-налаштування-форми). Автоматичне
підлаштування під мову браузера — підтверджена функція. Наявні PO — джерело
перекладів, не обов'язковий runtime-формат; сучасний i18n-механізм дозволений.
Новий UI має власний каталог фраз: використовувати придатні старі переклади,
доперекладати пропущені й нові за допомогою ШІ. Перевіряти placeholders,
безпечне відображення й відповідність макетам. [Аудит PO](_assessments/2026-10-23-localization/REPORT.md)
не є прийманням локалей нового UI. Резервна мова — `en`; перелік готових локалей
уточнюється за OQ-06; лише uk/en не є погодженим обмеженням.

Остання обрана мовна пара зберігається між відвідуваннями й підставляється
у форму без JavaScript, незалежно від UI-мови. Явні `from`/`to` мають пріоритет.
Недоступну збережену пару показувати з поясненням і вибором іншої, без
мовчазної заміни. Форма передає обидві мови в пошуковому URL явно;
збережені налаштування не змінюють параметрів пошуку або cursor.
На першому візиті без збереженої пари запропонувати мову браузера →
англійську; для англомовного браузера — англійську → українську. Пропозицію
можна змінити. Перевіряти наявність напрямку в каталозі активного dataset;
за його відсутності показувати вибір доступних напрямків. Точний source browser tag має пріоритет; інакше primary language має
визначити єдиний напрямок із потрібною target language. Неоднозначність
залишає вибір порожнім; raw codes не змінюються.

Обраний користувачем напрям — лаконічний сучасний пошуковий інструмент:
виразний пошук, багато вільного простору, мінімум зайвих елементів.
Конкретні макети й критерії приймання дизайну ще потрібно погодити;
наявність HTML-форми сама по собі не засвідчує їх виконання.
Основний сценарій залишається доступним без JavaScript. Якщо збігів немає,
показати локалізоване повідомлення й підказку використати маску `*` / `?`.
Автоматичні пропозиції схожих слів не потрібні; запит самостійно не розширювати.

Підтримка старих пошукових і download-URL не потрібна: legacy adapter та
redirects не реалізовуються (OQ-04 закрито). Новий пошук і `format=plain`
визначені в [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md).

<a id="fun-import"></a>

## FUN-IMPORT — інвентаризація та завантаження

Для погоджених джерел DDL + CSV потрібен legacy CSV4.gz adapter. DDL
використовується як довідка про стару структуру, а не виконується.
Описаний DB snapshot adapter — лише можливе розширення за появи іншого
джерела; він не є передумовою першого релізу. П'ятипольний
CSV отримує власний format ID лише після аудиту: його не вгадувати по одному рядку.
CSV читається lossless parser за [IMPORT_CONTRACT](IMPORT_CONTRACT.md),
не `encoding/csv.Reader`, який змінює embedded CRLF. Gzip перевіряється до EOF; рядок текстового файла
не дорівнює CSV-запису з quoted newline. Правила порожніх полів, BOM і додаткових
колонок явні: порожні мови/тексти, хибний UTF-8 та неочікувані колонки → issue;
BOM допускається лише на початку файла й враховується у parser version.

Source manifest перелічує очікувані файли з SHA-256, parser/mapping versions,
обраним джерелом істини, очікуваними локалями, відомими пропусками й exceptions.
Шлях не визначає ідентичність запису сам по собі. Fingerprint імпорту складається
з канонічного маніфесту вмісту й versions; зміна байтів або правил — новий dataset.
Mapping V1 — identity. Непомітного
злиття raw locale codes немає.

Import state: `created → loading → validating → ready`; terminal validation/syntax
failure → `failed`. Втрата процесу або DB connection — retryable interruption:
новий attempt з тим самим manifest продовжує loading/validating після
звільнення writer lock. Fatal failed не resume-иться; деталі в IMPORT_CONTRACT.
Активність — окремий pointer, не додатковий mutable стан рядків перекладів.
Batch і checkpoint комітяться разом, імпорт одного fingerprint захищений lock.
Для gzip без надійного seek повтор може читати від початку; committed records
ідентифікуються за source artifact + logical record index і не дублюються.
Файли поза маніфестом не потрапляють у імпорт мовчки.

<a id="fun-publish"></a>

## FUN-PUBLISH — звіряння, експорт і публікація

Verify перевіряє облік записів/файлів, сирі значення, provenance, локалі, дублікати,
детерміновані per-pair digests від відсортованих length-prefixed UTF-8 кортежів,
а також контрольні exact/mask queries. Самі counts не доводять рівність даних.
Якщо CSV recovery ненадійний або не завершено gzip, файл failed; його totals
не видаються за повний облік записів. Рішення щодо кожної втрати зафіксоване.

До ready формуються та перевіряються експорти через temp file → завершення →
hash → durable atomic no-clobber installation на одному filesystem за
IMPORT_CONTRACT; готові versioned artifacts не перезаписуються. Публікація перевіряє наявність
артефактів і validation report, бере DB lock, перевіряє expected generation та змінює active pointer і generation
одним commit; stale команда отримує conflict. Деталі retry/verify/CAS —
[IMPORT_CONTRACT](IMPORT_CONTRACT.md).
Артефакт без commit може бути orphan і прибирається лише окремим maintenance;
активний набір ніколи не посилається на незавершений файл.
Старі версії зберігаються для cursor і rollback за майбутньою retention policy.

<a id="fun-export"></a>

## FUN-EXPORT — завантаження

Для кожної наявної напрямленої пари доступний окремий CSV.gz; загальний
архів корпусу не потрібен. Це не обмежує внутрішню організацію backups.
`GET /downloads` показує manifest активного набору; файл видається за непрозорим
ID, не за довільним path. Реалізовано `/downloads/<dataset-id>/<export-id>`
для published ready/retained наборів; [DOWNLOAD_CONTRACT](DOWNLOAD_CONTRACT.md)
визначає pin, readiness, ranges/cache та streaming limits. `Content-Disposition: attachment`, тип gzip; SHA-256
та logical record count доступні поруч. Export CSV4 є сумісним обміном перекладів,
а не повним backup метаданих. Відсутня пара → 404. Права на словникові дані підтверджені власником (OQ-05 закрито); окремий
rights gate для публічного доступу не потрібен. M1 використовує синтетичні fixtures
відповідно до scope етапу; реальний корпус проходить аудит і verify у M2.

<a id="fun-seo"></a>

## FUN-SEO — словникові сторінки та індексація

У першому релізі забезпечити придатність сторінок окремих слів/фраз
із перекладами до індексації (FR-14 / AT-19). План, джерела та відкриті
питання — у [SEO_PLAN](SEO_PLAN.md). Сторінка показує вихідне написання
й переклади однією обраною мовою; напрямлена пара визначається URL
та має пріоритет над збереженою парою. Конкретна схема URL ще не обрана. Контракт `/search`, ліміти й pinned cursor залишаються чинними.
Перевірити discovery, canonical, взаємодію з автолокалізацією та навантаження
роботів; індексація не може залежати від cookie або JavaScript.

<a id="fun-ops"></a>

## FUN-OPS — експлуатація і відновлення

`GET /healthz` — живий процес; `/readyz` — підключення до БД, сумісна schema
і готовий активний dataset, 503 за неготовності. DSN і secrets передаються
через runtime config, не через старі Perl-конфіги. Startup/recovery readiness
очікує перевірки active artifact references/hashes за IMPORT_CONTRACT. CLI відрізняє exit 0 успіх,
ненульовий результат перевірки й execution error; звіт містить їх явно.

M1-05 підключає `SLOVNYK_HTTP_DSN` та `SLOVNYK_ARTIFACT_ROOT` разом.
Без них readiness=503. Фоновий monitor перевіряє pointer щосекунди, а schema,
report/hash та exports round-trip — на startup, reconnect і зміні generation.
Результат кешується лише для immutable retained artifacts; mount read-only,
GC вимкнений. Помилки БД скидають readiness; recovery виконує повну перевірку.
Операторських HTTP routes немає. SIGINT/SIGTERM скасовує monitor і запускає
bounded graceful shutdown. Search/cursor service реалізовано в M1-06;
HTTP lookup/rendering ще не підключені (M1-07).

Backup складається з консистентного snapshot нової БД і всіх artifacts,
на які він посилається, плюс manifests/config versions. До завершення backup
GC заборонений; restore перевіряє всі references і hashes. Старий сервіс
більше не працює; відкат використовує перевірені datasets/релізи нового
сервісу або restore. Після повної втрати сервера відновлення має тривати
не більше 24 годин; це потребує перевірки репетицією. Старі snapshots
не видаляються автоматично. Деталі репетиції,
перемикання та відкату — у [MIGRATION](MIGRATION.md).
