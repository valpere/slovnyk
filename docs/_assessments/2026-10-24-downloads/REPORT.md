# M1-07: завантаження словників

## Scope і review

Base: `f3e521652cbb5858c5562a5b69a9f519024cb6b5` (main після PR #3).
Роботу продовжено 2026-10-25. Послідовний **self-review**, не незалежне або
multi-agent рев’ю. [Reviewed input hashes](reviewed-inputs.sha256) фіксують
змінені source, resources, tests, screenshots та активні docs; цей report
і hash inventory не включені у власний scope.

Переглянуто published/retained SQL identity, HTTP role, confined artifact
references, file ownership, metadata/stream admission, write deadlines,
HEAD/ranges/ETag, HTML escaping, body limit і документацію FR-06 / AT-06.
Підтверджених невиправлених findings у цьому зрізі немає.

Виправлено під час review: plain manifest екранує raw locale codes, щоб
TAB/CR/LF та лапки не змінювали структуру записів; regression перевіряє
точне відновлення значень. Range errors не отримують attachment/immutable
cache headers. Великий stream перевірено після звичайного write timeout.

## Реалізовано

Локалізований `/downloads` HTML/plain із version pin, мовними парами,
logical rows, compressed bytes, SHA-256 і посиланнями на CSV4.gz.
Файловий URL містить dataset/export UUID, зберігає версію після publication;
ready-but-unpublished datasets закриті. Файли не генеруються на HTTP запит.

Metadata ділить 8 DB slots і 2 s budget із search/catalog, connection
звільняється до streaming. До 8 передач одночасно, renewable 30 s write
deadline; attachment не обмежений page body budget 1 MiB. HEAD, ranges,
ETag і immutable caching. Missing/symlink files дають 503. Production
потребує readiness та read-only immutable artifacts; per-request full hash
не виконується. Деталі — [контракт](../../DOWNLOAD_CONTRACT.md).

31 catalog тепер має 28 keys: 84 legacy PO texts і 784 AI texts. Додано
186 AI texts для downloads; статус ai-draft, мовне приймання ще відкрите.

## Фактичні перевірки

`make check test test-race test-smoke` та `make test-integration` — **PASS**,
exit 0. Лише disposable local PostgreSQL 17.11 і synthetic fixtures; runner
видалив власні container/volume. Logs: `tmp/downloads/final-checks.log` і
`tmp/downloads/final-integration.log` (ignored). Перша перевірка links виявила
відсутній report; після його додавання повторний `make check` пройшов.
Test paths і межі покриття — [TRACEABILITY](../../TRACEABILITY.md).

Screenshots через наявний Chromium/CDP 9222, без Playwright або нового
browser process, page scripts disabled. Loopback fixture використовує
реальний HTTP handler і synthetic manifest; PostgreSQL окремо перевіряється
integration runner. Це visual check, не повний browser/accessibility acceptance.

- [Desktop, uk](desktop.png), 1280×900.
- [Mobile, uk](mobile.png), 390×844.
- [German, narrow mobile](german-mobile.png), 320×844.

## Залишковий scope

M1-07 залишається in progress: SEO, фінальний дизайн/accessibility і OQ-06
ще відкриті. Production corpus, HTTP capacity, deployment profile,
retention/GC та off-host restore не перевірено. Oracle і legacy не змінено.
PR/CI results фіксуються у GitHub; локальний звіт не передбачає їх результату.
