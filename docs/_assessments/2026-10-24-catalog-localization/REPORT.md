# M1-07: каталог і локалізація — 2026-10-24

## Scope і review

Base: `ea507d7b5a424d41f263d2de91e7667e53304fa2` (main після PR #2).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
[Reviewed input hashes](reviewed-inputs.sha256) фіксують усі змінені source,
ресурси, tests та активні docs; report/inventory не включені у власний scope.

Переглянуто catalog SQL/privileges/publication pin, negotiation, JSON/PO
provenance, template escaping/response budget, cookies, explicit endpoint
параметри, shared deadline/connection slots і integration/process tests.
Documentation pass звірено з FR-05, HLD та LOOKUP_CONTRACT. Після виправлень
підтверджених невиправлених findings у цьому зрізі немає.

Виправлено під час перевірки:
- x/text із generic `no` міг вибрати Danish для `nb-NO`; matcher використовує
  Bokmål tag, лишаючи UI identifier `no`. Regression test пройшов.
- Browser `nb` тепер може запропонувати наявний legacy `no-*` напрямок;
  endpoint aliases не додано, raw pair зберігається.
- Без налаштованого storage форма також явно повідомляє про недоступність
  каталогу. Успішний lookup не втрачає результати при catalog failure.
- Toolbar дозволяє перенос довгих локалізованих підписів на вузькому екрані.

## Реалізовано

`/languages` HTML/plain, published-ready inventory/counts, no-JS selectors;
після lookup каталог використовує dataset pin результату. До 256 locales і
4096 pairs, fail closed без truncation, остаточний body guard 1 MiB.
Search/catalog ділять 8 connection slots та outer 2 s deadline.

31 embedded JSON catalog × 22 messages: 84 тексти з придатних PO і 598 AI.
Кожна фраза має origin, кожен ресурс — legacy filename/hash; статус ai-draft.
Порядок UI: explicit supported → saved manual → browser High/Exact → en.
Непідтримуваний script може дати English fallback (наприклад sr-Latn).

Річні host-only HttpOnly/SameSite=Lax cookies окремо для ручного UI й пари;
Secure для TLS/production hosts. Автоматичні preferences не записуються.
Явна/часткова пара не доповнюється; unavailable saved pair без тихої заміни.
Browser proposal використовує тільки наявний однозначний напрямок.

## Фактичні перевірки

`make check test test-race test-smoke` та `make test-integration` — PASS, exit 0.
Локальний disposable PostgreSQL 17.11 під HTTP role перевірив catalog,
continuation зі зміною format/UI, manual cookie, browser proposal та pin під
блокуванням active pointer. Initial inventory дотримався timeout; unknown
published pin недоступний. Docker runner видалив власний volume/container.
Точні test paths — [TRACEABILITY](../../TRACEABILITY.md).

Screenshots через наявний Chromium/CDP 9222, без Playwright або нового browser
process; page scripts disabled. Використано transient loopback fixture із
реальним HTTP handler і синтетичними inventory/results, без PostgreSQL у
browser preview; окремі SQL integration checks описано вище.

- [Desktop, uk](desktop.png), 1280×900.
- [Mobile, uk](mobile.png), 390×844.
- [Language inventory](languages.png), 1280×900.
- [German, narrow mobile](german-mobile.png), 320×844.

## Залишковий scope

M1-07 не завершено: SEO/downloads ще попереду. AI-draft не є лінгвістичним
прийманням або закриттям OQ-06. Макети/повний accessibility і no-JS browser
acceptance ще потрібні. Ліміти каталогу — захисні guards, не capacity proof.
Повний corpus, Oracle й legacy не змінювалися. Production migration/p95
та остаточний deployment profile не перевірено. PR/CI results — у GitHub;
локальний звіт не передбачає їхнього результату.
