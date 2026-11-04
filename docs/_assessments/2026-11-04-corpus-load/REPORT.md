# Локальна проба змішаного навантаження

## Результат і протокол

2026-11-04 завершено дослідницький HTTP workload на вже опублікованому локальному
корпусі: 56 455 068 translations, 992 pairs, active generation 1.
**2 112/2 112 requests — HTTP 200 і content checks pass**, client timeouts та
HTTP errors — 0. Нового import/verify/publication не було.
[Summary](summary.json), [assertions](assertions.json), [protocol](protocol.json).

Три послідовні фази по 60 s, без пауз і зміни ресурсів:

1. `search-before`: 10 search/s, 600 requests.
2. `mixed`: ті самі 10 search/s + 5 word pages/s + 0.2 full sitemap shard/s,
   912 requests (600 searches, 300 word pages, 12 shard transfers).
3. `search-after`: знову ті самі 10 search/s, 600 requests.

Search mix — 3 exact : 1 prefix mask; mask = повне source + `*`, не короткий
чи leading-wildcard pattern. Default limit 50, plain UI `en`, без cursor.
Вибрано 128 entries: перша URL кожного з 128 рівновіддалених shards.
Усі пройшли заданий source-shape filter, без latency-based відбору:
3..40 code points, без крайніх пробілів, `*`/`?` та C0/DEL characters.
Це **128 cases / 119 directed pairs**, не вичерпне тестування 992 пар.
На кожну фазу припадають 450 exact і 150 prefix requests у тому самому порядку;
через періодичний розклад exact використовує 96 cases, prefix — інші 32.
[Workload metadata](workload-summary.json); actual terms/URLs пошуку — лише в ignored private file.

## Затримки

Нижче **client-observed** p95 у ms, включно з читанням body; це не окремий
server-side timer. Всі requests включені, failures не відфільтровувалися.

| Phase | Searches | Combined p95 ms | Exact p95 ms | Prefix p95 ms |
|---|---|---|---|---|
| search-before | 600 | 85.949 | 18.489 | 290.207 |
| mixed | 600 | 76.166 | 16.72 | 83.239 |
| search-after | 600 | 86.47 | 16.692 | 95.498 |

Word pages у mixed: p95 **8.721 ms**;
full shard transfers: p95 **66.615 ms**,
67,201,320 response bytes за фазу.
[Aggregates](search-aggregate.json), [metadata кожного request](observations.jsonl).
Nearest-rank percentile = sorted value at `ceil(p*N)`.

Open-loop scheduler: 32 worker threads, urllib HTTP client на host поза cgroups;
незалежні запити без явно налаштованого persistent connection pool.
Всі заплановані arrivals обліковані. Dispatch lag і scheduled-to-completion
recorded окремо, щоб queueing не зникало з вимірів; response hashes/content
перевіряються всередині timed request. Для search перевірено наявність plain
translation group і noindex; для word — очікуваний canonical; для shard —
повні SHA-256/bytes. Це bounded positive-result checks, не semantic audit перекладів.

## Ресурси, стан і lifecycle

DB: PostgreSQL 17.11, schema 3, **1 CPU / 8 GiB**, durability on,
`temp_file_limit=20GB` на process. HTTP: окремий non-root container,
**1 CPU / 512 MiB**, read-only rootfs/mounts, restricted LOGIN, loopback port.
[DB](startup.json), [HTTP](http-startup.json), [privileges](http-privileges.json).
Application revision `dd2039a3926bc1ed1494f72fbeec47b4d7a5b9a2`, binary SHA у protocol.
App source незмінний; host filesystem caches не скидалися, readiness завершилася
до запуску workload. Це не cold-cache experiment.

[Docker samples](resources.jsonl) приблизно кожні 10 s;
[summary](resource-summary.json) містить CPU/working-set observations.
[Final cgroup snapshot](runtime-resources-final.json) містить memory current/peak,
file/anon memory та CPU/throttling counters за **container lifetime**, включно
зі startup/readiness, без pre-workload delta baseline. Docker working set і
cgroup memory usage різні: inactive file cache не є Go heap.
OOM kills — 0; disk floor 30 GiB і runtime guard 1 200 s не спрацювали.

[Baseline](baseline.json) і [final state](final-state.json) тотожні:
ready/report/exports/source counters/import runs та active generation 1 не змінилися.
Власні HTTP/DB containers, network і тимчасовий LOGIN прибрані; DB bind,
snapshot та referenced artifact root збережені. [Lifecycle](lifecycle.json).
App logs перевірено приватно: лише відомі lifecycle fields, без credentials;
до Git лише [log metadata](http-log-check.json). Oracle і legacy не задіяні.

## Межі та наступні gates

Оцінка користувача до 10 search/s не визначає погоджений workload або crawler
budget. Три короткі фази на цьому developer host не закривають REQ-NFR-03/AT-16
або production capacity gate. Зміни між фазами не доводять причинність чи
статистичну значущість. Не перевірені broad/leading-wildcard masks,
negative/cursor/long-field queries, slow clients, mixed downloads, sustained soak,
cold-cache, target-server SLO та роботами crawl усіх 36.8M pages.
HTTP 503 був би непрозорим: без додаткової діагностики його не прирівнюємо до DB timeout.

Наступні задачі: розширити query classes і тривалість за узгодженим budget,
окремо перевірити складні маски та server-side duration; вибрати придатний
storage/server, виконати full-corpus backup/restore. Поточний DB layout
64.315 GiB плюс WAL/artifacts усе ще не поміщається на виміряний Oracle root.

`make check test`, metadata validation та послідовний self-review наведено у
[validation](validation.json); незалежного review не було. CI перевіряється
окремо для фінального PR head; merge виконує користувач.
Одноразові scripts — ignored `tmp/corpus-load/`, не входять у clean checkout.
