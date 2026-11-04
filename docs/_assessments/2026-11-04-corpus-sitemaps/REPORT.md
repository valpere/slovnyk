# Повний локальний sitemap та HTTP rehearsal

## Результат

2026-11-04 actual `dataset sitemap` створив **36,763,066 унікальних URL,
736 shards та один index** для retained ready dataset
`96541c0b-ac71-4e38-b57c-b593e82ac9d5`. [CLI inventory](generation.json).
Усі shards — 4,117,544,352 bytes (3.835 GiB), index — 120,826 bytes;
JSON report окремо. Це додатковий disk cost до БД, WAL та CSV exports.

Перший запуск: **324.345 s**, повторний: **343.579 s**,
обидва exit 0; включено завершальне polling до 10 s.
Повторний stdout побайтово тотожний першому; inventory у БД один.
[Determinism](determinism.json), [SQL inventory](sql-inventory.json).
Після генерації active null/generation 0, source counters, report та import runs
незмінні; публікація виконана окремою наступною командою.

## XML та HTTP

Окремий потоковий XML validator прочитав **кожен файл** за 61.323 s:
bytes/SHA-256, namespace, index links/order, canonical origin і URL shape,
strict global entry-ID order без дублікатів, URL totals і protocol limits.
[Validation](xml-validation.json) містить digest усіх IDs та три sample IDs.
Це hash identities, без словникових текстів у Git.

Actual `dataset publish --expected-generation 0` завершився exit 0 за
**33.624 s**. [Receipt](publication.json), [execution](publication-execution.json).
Локальна БД тепер active на цьому dataset, generation 1, один publication event;
ready/report/exports і source counters не змінені. [Final state](final-state.json).
Це публікація лише в ізольованому локальному середовищі, без зовнішнього трафіку.

Через HTTP перевірено hashes/bytes **усіх 736 shards і 992 CSV.gz downloads**,
index, download listing, robots, canonical трьох sample word pages,
один exact/mask sample, основний/додатковий Host redirect, HEAD/ETag/304/Range
для index і одного download. Immutable cache/ETag та відсутність cookies
звірені для всіх shards/downloads. [HTTP observations](http-check.json).
Raw queries, HTML, CSV і XML не збережені в evidence.

До publish: health 200, readiness/sitemap 503. Після publish readiness стала
200 через **37.4 s** від початку polling після CLI;
після restart HTTP — через **37.909 s**.
Це час очікування readiness, не чистий hash benchmark: включає фоновий цикл
і poll scheduling. Word/index hashes після restart збереглися.
Першу restart-перевірку перервано: Docker змінив автоматично обраний
loopback port, а harness звертався до старої адреси. Після явного повторного
читання порту поточна readiness була 200; окремий наступний restart перевірено
з rediscovery порту та виміром вище. Попередні completed checks збережені,
application code не змінено. [Harness correction](http-restart-harness-correction.json).
Приватно перевірено відсутність sampled source terms і credentials у process logs;
до Git потрапили тільки їхній розмір і hash.

## Профіль, lifecycle та межі

DB: локальний PostgreSQL 17.11/schema 3, **1 CPU / 8 GiB**, durability on,
`temp_file_limit=20GB` на process; actual CLI поза cgroup.
[Startup](startup.json), [progress summary](progress-summary.json).
30 GiB disk floor, 85 GiB DB ceiling та 24 h/attempt guard не спрацювали.
Cumulative temp bytes не є peak live temp; зміни host free space не
приписуємо виключно цьому запуску.

HTTP: окремий non-root container, **1 CPU / 512 MiB**, loopback-only port,
read-only rootfs і всі mounts, restricted LOGIN лише в `slovnyk_http`.
[Profile](http-startup.json), [effective privileges](http-privileges.json).
Власні HTTP/DB containers і network прибрані, тимчасовий LOGIN видалено.
Bind DB лишається з generation 1; referenced artifact root —
`data/experiments/corpus-verification-2026-11-04-retry/artifacts/`.
Snapshot, DB та інші retained inputs збережені. [Lifecycle](lifecycle.json).

Application source не змінювався. Sitemap/publish binary — application revision
`5b4011f9e6187839330e6c8f96a50ec645c9e64e`; HTTP binary зібрано статично на
`c7944cddfffaef316149305601a62fd0c18b1054`. Hashes в invocation/startup.
Використано одноразові локальні checks у `tmp/corpus-sitemaps/`; вони не
входять у clean checkout. Алгоритм і межі перевірок наведені вище.

Oracle/legacy не задіяні. Це не crawl 36.8M HTML pages, concurrency/SLO gate,
мовне/дизайн приймання, production deployment або off-host restore/RTO.
Known gaps і owner receipt з попереднього verify залишаються чинними;
нових втрат чи approval scope не вводили. Наступні gates: змішане
crawler/search навантаження, придатний storage/server та full-corpus restore.

`make check test` і metadata checks наведені у [validation](validation.json).
Послідовний self-review; незалежного review не було. CI перевіряється
окремо для фінального PR head; merge виконує користувач.
