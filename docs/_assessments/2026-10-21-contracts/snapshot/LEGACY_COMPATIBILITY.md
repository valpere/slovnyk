# Сумісність старих URL V1

Observed baseline: старий HEAD `94640dc6fe2fde643af3ee746254e0d60990a0ce`.
Цільові зміни нижче — пропозиція, не поведінка старого production.
Контракт нового пошуку — [LOOKUP_CONTRACT](LOOKUP_CONTRACT.md).

## Перевірені факти

- Sitemap містить `/fcgi-bin/dic.fcgi?iw=Harvest&hn=sel&il=en_us&ol=uk_ua&ul=en-us`.
- `cgi/dic.fcgi:190–225` читає `hn,il,ol,iw,ul`; параметра `hl` немає.
- Missing hn → hom; unknown hn у старому теж hom. Missing il → ul (:318),
  missing ol → en-us (`cfg/dic.conf:58`); UI fallback через Accept-Language.
- `hn=pre` викликає той самий lookup (:396–399), але старий результат — HTML
  з PRE (`pre.html:1–3`, config MIME text/html), не text/plain.
- Sanitizer (:169–176) видаляє `<>&%"'`, міняє underscores/whitespace; мовні
  underscores у sitemap суперечать цьому parser. Same-locale пошук блокується (:352).
- `dwn.html:176` посилається на `/csv/{FILE}`; `loc.html:37,49` —
  `/loc/slovnyk.{code}.po`. Додаткові aliases/domains/rewrite rules невідомі.

## Allowlist routes та handlers

| Вхід | Нова поведінка |
|---|---|
| `/fcgi-bin/dic.fcgi`, missing hn або hn=hom | 308 на `/`, з resolved ui |
| hn=sel | 200 HTML або помилка LOOKUP; без redirect, щоб уникнути повторного decode |
| hn=pre | 200 UTF-8 text/plain з версією, entries і continuation URL; свідома зміна HTML PRE |
| hn=dwn | 308 `/downloads` |
| hn=loc | 308 `/languages` |
| hn=top | 410 «статистика поки недоступна» |
| unknown/empty hn | 404; більше немає прихованого hom fallback |
| `/csv/{filename}` | Тільки точний legacy name з manifest active dataset → 308 на version-pinned export URL; unknown → 404 |
| `/loc/slovnyk.{code}.po` | Тільки allowlisted перевірений PO artifact; unavailable → 404, distribution blocked → 403 |

Redirect Location — лише application-generated relative URL. Не брати hostname,
path чи filename для filesystem доступу з користувацького вводу. Downloads,
включно з legacy redirects, не обходять distribution gate OQ-05: blocked → 403.
Інші scripts (`/cgi-bin/...` тощо) не обіцяються до перевірки server config OQ-04.

## Mapping параметрів

Один strict URL decode (`+` → space, `%2B` → plus), bounded UTF-8 parsing;
duplicate відомого key → 400, malformed escapes/NUL → 400. Загальні limits LOOKUP
діють до adapter. Unknown keys (включно з hl) ігноруються, не стають aliases hn.
`iw` зберігає punctuation і whitespace до нормалізації LOOKUP; старе руйнівне
очищення **не відтворюється**. Є `*`/`?` → mode mask, інакше exact. `%252A`
після одного decode — буквальне `%2A`, не wildcard.

Мовні коди il/ol/ul: ASCII lowercase, underscore → hyphen **лише тут**.
Повні dictionary codes шукаються буквально у pinned dataset. Fixed short aliases:
`uk → uk-ua`, `en → en-us`; інші короткі коди → 400 до явного розширення таблиці.
Якщо exact повний код існує, він має пріоритет; en-gb ніколи не зливається з en-us.
UI map для стартової пропозиції: uk-ua/uk → uk, en-us/en-gb/en → en; інше → UI fallback.

Для sel/pre missing il → resolved UI dictionary default (uk→uk-ua, en→en-us);
missing ol → en-us; явно порожня/невідома il/ol → 400. Missing/empty iw → форма 200
(для pre — коротка plain-text інструкція). Same-locale дозволено за наявності пари:
це виправлення старого обмеження, потрібне для імпортованого sr-rs→sr-rs.

Continuation URL веде на новий `/search` із явними from/to/q/mode/limit/cursor/ui;
для pre також `format=plain`. На новому endpoint format відсутній/html → HTML,
plain → UTF-8 text/plain, інший/empty/duplicate → 400. Format не входить у cursor
fingerprint: ті самі entries/порядок, але byte budget визначається actual encoding.

## Приймальна матриця

AT-12: snapshot sitemap URL з underscores і еквівалент з hyphens; missing defaults;
unknown handler; `iw=C%2B%2B` → C++; `iw=a+b` → a b; `%252A` без другого decode;
Unicode/апостроф/ampersand/буквальні `%` і `_`; malformed UTF-8/escape; duplicate keys;
same-locale; absent pair; pre plain text; cursor continuation after publish;
allowlisted/missing/blocked exports, path traversal та unknown script 404.
Це fixtures для майбутніх тестів, не завершена перевірка production compatibility.
