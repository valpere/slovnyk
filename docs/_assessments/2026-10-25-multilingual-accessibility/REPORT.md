# M1-07: багатомовна доступність — 2026-10-25

## Scope і review

Base: `114098424dd44b53462f4727895efe8ba4817f68` (main після PR #8).
Послідовний self-review за code-review skill, не незалежний або multi-agent.
[Reviewed inputs](reviewed-inputs.sha256) фіксують змінені source, catalogs,
активні docs та screenshots; цей report та його hash inventory виключені.
Звірено FR-05/AT-05, no-JS preferences, SEO identity, escaping, body budget,
мовні tags, напрямок raw текстів, focus order та claims у документах.
Підтверджених невиправлених findings у цьому scope немає.

## Зміни

Header/navigation винесено за межі main; перший Tab відкриває локалізований
skip link, Enter переводить фокус до main, наступний Tab — у пошукове поле.
Прибрано autofocus; додано search landmark і scope для headers каталогу.
Новий `skip_content` перекладений ШІ у всіх 31 catalog, status лишається
`ai-draft`; решта повідомлень і provenance не змінені. Тепер 29 keys.

Search, як article, позначає мову source/target окремо від UI через наявний
contentLang; invalid tag має `lang=""`. `dir="auto"` на raw заголовках,
кожному перекладі й query input підтримує незалежні RTL/LTR rows. Немає
переписування raw текстів, locale codes, URL, cookies чи plain output.

## Фактичні перевірки

- `make build check test test-race test-smoke` — PASS, exit 0.
- Browser CDP checks — PASS, exit 0; log: `tmp/accessibility/browser-results.txt`.
- Existing HTTP checks включають escaping, continuation, error statuses,
  plain output та final 1 MiB response guard; catalog checks — 31 × 29 keys.

Локальний transient preview використовував actual HTTP handler з synthetic
search/catalog/article/download callbacks, без PostgreSQL. Наявний Chromium
156 через loopback CDP 9222; створено й закрито лише власну вкладку, page scripts
вимкнені. Playwright/новий browser process не використовувалися.
DOM, computed styles, accessibility tree, layout metrics та keyboard input
перевірялися через CDP. Keyboard Enter включав carriage-return text: початкова
перевірка без нього не активувала native button; виправлено test driver,
продуктова форма не потребувала змін для submit. Діагностика читала лише
synthetic form values. Після виправлення повний browser сценарій пройшов.

Перевірено: keyboard skip → main → query → Arabic typing → native GET submit
з точними q/from/to; Ukrainian UI + Arabic RTL source + English target;
German UI + Arabic та Latin target rows із незалежним direction; article з
довгим нерозривним і multiline текстом; нерозбірні теги без UI lang inheritance.
Accessibility tree має main/navigation/search. При 320 CSS px layout metrics
не мають горизонтального overflow у search/article/languages/downloads,
no-match і service-failure views; desktop перевірено при 1280px.

Візуально переглянуто screenshots:
- [Keyboard skip](keyboard-skip.png), 390×844.
- [Search desktop](search-desktop.png), 1280×900.
- [Mixed RTL/LTR rows](mixed-direction-mobile.png), 320×844, прокручено до результатів.
- [Article mobile](entry-mobile.png), 320×844.

Logs/одноразові preview/check scripts — ignored `tmp/accessibility/`.
PostgreSQL implementation не змінено; локальний integration suite не
повторювався. PR CI evidence додається до PR після фактичного виконання.

## Межі

Це technical synthetic baseline, не повний accessibility audit. Не перевірялися
screen-reader speech, інші browser engines, усі UI languages/комбінації та
остаточне дизайн-приймання власником. Переклади залишаються AI-draft; OQ-06 і
M1-07 відкриті. Oracle, legacy, AGENTS.md і real corpus не змінено. Production,
full corpus/crawl acceptance, capacity та restore — поза цим зрізом.
