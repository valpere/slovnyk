# High-Level Design

Це цільова будова нового застосунку; перелічених пакетів і команд ще немає.
Нормативна поведінка — у [REQUIREMENTS](REQUIREMENTS.md).

## Компоненти та майбутня структура

```text
cmd/slovnyk/               # serve та операторські subcommands
internal/catalog/         # локалі, напрямки, dataset metadata
internal/lookup/          # запит, нормалізація, результати, cursor
internal/importer/        # inventory, parsers, staging, reconciliation
internal/publication/     # verify, publish, rollback, export
internal/storage/postgres/
internal/httpui/          # handlers, locale fallback, legacy adapter
internal/ops/             # config, health, diagnostics
migrations/               # лише нова схема, версійні SQL
web/templates/            # html/template
web/static/
web/locales/              # перевірені ресурси UI
testdata/                 # синтетичні fixtures, без повного корпусу
docs/
```

Go HTTP і CLI використовують ті самі application services. В HTTP немає
операторських routes. Команди нижче — запропонований інтерфейс, не готові інструкції запуску:

```text
slovnyk serve
slovnyk import inspect --source <directory> --report <path>
slovnyk import run --manifest <path>
slovnyk dataset verify <id>
slovnyk dataset export <id>
slovnyk dataset publish <id>
slovnyk dataset rollback <previous-id>
```

<a id="fun-catalog"></a>

## FUN-CATALOG — довідник і версії

Модель зберігання (логічна; DDL створюється при реалізації):

| Сутність | Ключові поля та правила |
|---|---|
| `datasets` | ID, manifest hash, parser/mapping/normalization versions, state, timestamps; незмінна після ready |
| `active_dataset` | Один рядок з dataset ID; зміна транзакційна, publish серіалізується блокуванням |
| `dataset_locales` | `(dataset_id, code)`, назва; сирі legacy-коди зберігаються, aliases окремо |
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

HTTP-контракт пропозиції: `GET /search?from=uk-ua&to=en-us&q=...&mode=exact&ui=uk&cursor=...`.
`mode` — `exact` або `mask`. `GET /` показує форму; `GET /languages` — каталог.
HTML — основний інтерфейс; публічний JSON API поки не обіцяється.

1. Перевірити розмір, UTF-8, локалі, mode і cursor. Початкові конфігуровані
   defaults: 1024 Unicode code points запиту, 8 KiB query text, 50 зв'язків
   на сторінку, максимум 100; DB timeout 2 s. Це захисні defaults, не SLO.
2. Зафіксувати dataset ID на перший запит. Обрізати крайні пробіли запиту,
   застосувати NFC → Unicode case folding → NFC; так само будувати ключ з
   source text, але без обрізання/очищення raw values. Версію бібліотеки та
   Unicode-даних фіксувати у normalization version.
3. Exact використовує рівність ключа. У mask `*` компілюється в `%`, `?` у `_`,
   буквальні `%`, `_` та escape-символ екрануються для SQL LIKE з явним ESCAPE.
   Довільні regular expressions і SQL від клієнта не приймаються.
4. Сортування за `(source_key, source_text, target_text, id)` з визначеною
   bytewise/C collation. LIMIT+1 встановлює `has_more`. Групування source text
   виконується лише в межах сторінки; велика група може продовжуватися на наступній.
5. Cursor містить dataset, fingerprint параметрів і останній sort tuple;
   перевіряється цілісність і межі. Неправильний cursor → 400, недоступна стара
   версія → 410 з пропозицією повторити пошук. Dataset не змінюється між сторінками.

Індексний початковий план: dataset + pair + source key; перевірити довгі
значення й обмеження індексних ключів на реальному corpus. Для exact можливий
digest нормалізованого ключа + residual equality. Для mask — виміряти prefix
та infix плани; не обіцяти швидкий `%...%` без вимірювань. Усі режими під timeout.
Timeout → явна помилка обмеження (503), а не «нічого не знайдено».
Новий збір `queries` у цей потік не входить.

<a id="fun-ui"></a>

## FUN-UI — інтерфейс і сумісність

Каталоги локалей словника відокремлені від UI-кодів. Явний підтримуваний `ui`
має пріоритет над `Accept-Language`; потім configured default. Якщо alias
мовного коду неоднозначний, не вибирати варіант мовчки. Пропозиція стартового
UI — українська й англійська; повний перелік після OQ-06.

Legacy adapter знає лише явний allowlist handlers і параметрів. `sel`/`pre`
автоматично обирають mask, якщо `iw` містить `*`/`?`, як старий пошук; нова форма
має явний mode. Для `pre` — UTF-8 `text/plain` з тим самим обмеженим результатом
і позначенням продовження. `hom`/`dwn`/`loc` можна зіставити новим сторінкам;
`top` показує повідомлення про недоступність статистики, доки її не реалізовано.
Точний старий path, redirects і sitemap — після інвентаризації OQ-04.

<a id="fun-import"></a>

## FUN-IMPORT — інвентаризація та завантаження

Два окремі adapters: legacy CSV4.gz і описаний DB snapshot export. П'ятипольний
CSV отримує власний format ID лише після аудиту: його не вгадувати по одному рядку.
CSV читається `encoding/csv`, gzip перевіряється до EOF; рядок текстового файла
не дорівнює CSV-запису з quoted newline. Правила порожніх полів, BOM і додаткових
колонок явні: порожні мови/тексти, хибний UTF-8 та неочікувані колонки → issue;
BOM допускається лише на початку файла й враховується у parser version.

Source manifest перелічує очікувані файли з SHA-256, parser/mapping versions,
обраним джерелом істини, очікуваними локалями, відомими пропусками й exceptions.
Шлях не визначає ідентичність запису сам по собі. Fingerprint імпорту складається
з канонічного маніфесту вмісту й versions; зміна байтів або правил — новий dataset.
Aliases застосовуються лише явним mapping зі збереженням оригінальних полів.

Import state: `created → loading → validating → ready`; помилка → `failed`.
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
hash → atomic rename на одному filesystem. Публікація перевіряє наявність
артефактів і validation report, бере DB lock та змінює active pointer одним commit.
Артефакт без commit може бути orphan і прибирається лише окремим maintenance;
активний набір ніколи не посилається на незавершений файл.
Старі версії зберігаються для cursor і rollback за майбутньою retention policy.

<a id="fun-export"></a>

## FUN-EXPORT — завантаження

`GET /downloads` показує manifest активного набору; файл видається за непрозорим
ID, не за довільним path. `Content-Disposition: attachment`, тип gzip; SHA-256
та logical record count доступні поруч. Export CSV4 є сумісним обміном перекладів,
а не повним backup метаданих. Відсутня пара → 404. Публічний доступ до artifacts
включається після OQ-05; до цього тести — на синтетичних fixtures.

<a id="fun-ops"></a>

## FUN-OPS — експлуатація і відновлення

`GET /healthz` — живий процес; `/readyz` — підключення до БД, сумісна schema
і готовий активний dataset, 503 за неготовності. DSN і secrets передаються
через runtime config, не через старі Perl-конфіги. CLI відрізняє exit 0 успіх,
ненульовий результат перевірки й execution error; звіт містить їх явно.

Backup складається з консистентного snapshot нової БД і всіх artifacts,
на які він посилається, плюс manifests/config versions. До завершення backup
GC заборонений; restore перевіряє всі references і hashes. Старий сайт не
вимикається і старі snapshots не видаляються автоматично. Деталі репетиції,
перемикання та відкату — у [MIGRATION](MIGRATION.md).
