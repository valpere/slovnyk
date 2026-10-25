# M1-07: постійні словникові сторінки

## Scope і review

Base: `87c230e65a161f804575b66c73e908495d9339df` (main після PR #4).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
[Reviewed input hashes](reviewed-inputs.sha256) фіксують змінені source,
tests, docs, screenshots та assistant helper; report і hash inventory
не включені у власний scope. AGENTS.md не змінено.

Переглянуто FR-14 / AT-19, raw identity та SQL/Go parity, publication/readiness,
retained cursors і purpose binding, index-compatible queries, role grants,
shared admission/deadline, body/metadata limits, escaping, cookie-independent
HTML, canonical/robots, redirect origin, HEAD/errors та активну документацію.
Підтверджених невиправлених findings у цьому зрізі немає. Перед завершенням
усунуто застарілі proposed SEO rules і уточнено shared-slots comment.

## Реалізовано

`/word/<entry-id>` — server-rendered стаття однієї raw мовної пари й написання.
SHA-256 identity не залежить від dataset, normalization або UI; case/NFC/пробіли
не об’єднуються. Bare URL слідує active dataset, відсутнє написання → 404.
До 50 цілих перекладів, signed retained continuation, final body limit 1 MiB.
Continuation noindex і canonical до bare URL. Search містить HTML links на
raw articles, залишається noindex. Canonical завжди на `https://slovnyk.org`;
UI source-primary/en не залежить від cookies або browser. Robots дозволяє
crawling; alias `.org.ua` дає fixed-origin 308 зі збереженням path/query.

Migration 000002 додає fixed-size expression index, schema version 2;
готові raw rows незмінні. Metadata ділить 8 slots і 2 s із search/catalog/
downloads; readiness обов’язковий. Деталі — [SEO_PLAN](../../SEO_PLAN.md).

## Фактичні перевірки

`make check test test-race test-smoke` і `make test-integration` — **PASS**,
exit 0. Disposable local PostgreSQL 17.11, synthetic fixtures, container/volume
видалено runner. Logs: `tmp/seo/checks.log`, `tmp/seo/final-integration.log`
(ignored); test paths і межі — [TRACEABILITY](../../TRACEABILITY.md).
Початковий integration fixture виправлено відповідно до існуючого ownership
contract: новий operator connection на операцію та створений artifact root.
Подальший повний suite пройшов. Forced index plan доводить придатність виразу
до індексу, не production planner choice або throughput.

Visual check через наявний Chromium CDP 9222, без Playwright/нового browser
process, page scripts disabled. Loopback synthetic fixture використовує
реальний HTTP handler; PostgreSQL перевіряється окремим integration suite.
Обидва screenshots переглянуто. Це не повний browser/accessibility acceptance.

- [Desktop, uk](desktop.png), 1280×900.
- [Mobile, uk](mobile.png), 390×844.

Повторюваний capture винесено в assistant-only `minions/capture-cdp.mjs`;
обидва screenshots отримано ним. Сервіс, оператори й CI від нього не залежать.

## Залишковий scope

Це частковий FR-14 / AT-19 synthetic зріз. Offline sitemap/index discovery,
повний corpus/index build cost, crawler capacity, production p95, TLS/edge,
мовне/дизайн/accessibility приймання ще відкриті; M1-07 in progress.
Oracle та legacy не змінено. PR CI перевіряється окремо у GitHub;
локальний звіт не передбачає його результату.
