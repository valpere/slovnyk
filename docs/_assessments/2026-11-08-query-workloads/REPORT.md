# Schema 4: локальні HTTP/cache/contention probes — 2026-11-08

## Результат

Після merge [PR29](https://github.com/valpere/slovnyk/pull/29), на application
revision `f752e36ae984729ab63fea6db70a484d6327ab51`, виконано два обмежені
локальні запуски: **1858/1858 HTTP 200**, без client errors, SQLSTATE 57014
у виміряних фазах або cgroup OOM. **330/330** controls збігаються з
pre-restore sizes/SHA-256/Content-Type/Cache-Control; **508/508** negative
responses мають точне тіло англійської підказки no-matches.

Це діагностика, **не AT-16/capacity acceptance**: погодженого traffic/crawler
профілю немає, server-side timer не додано, цільовий Oracle не задіяно.
Application code, schema, SQL, індекси та двосекундний deadline не змінювалися.
[Повні агрегати](summary.json), [перевірка accounting/privacy та hashes](validation-receipt.json);
per-request observations і resources — у
`spaced/` та `largest/`, без URL, термінів, тіл і credentials.

## Методика й вибірка

Використано тільки успішно відновлений schema 4 cluster із
[попередньої репетиції](../2026-11-08-schema4-restore/REPORT.md):
56 455 068 translations, 992 pairs, ready/active generation 1.
Початкову dev-БД, legacy, Oracle і сторонні containers не запускали/не змінювали.
Перед кожним запуском стартував власний PostgreSQL process: **shared buffers
нові; OS cache неконтрольований**. Host caches не скидали. Readiness повністю
перевіряє artifacts і прогріває metadata; це не cold-host experiment.

Образ PostgreSQL 17.11 закріплений digest у protocol receipts. DB — 2 CPU/8 GiB;
HTTP — 1 CPU/512 MiB, non-root, readonly rootfs/artifact mounts, loopback ports.
Новий restricted HTTP LOGIN має `default_transaction_read_only=on`.
Вісім application slots спільні між backends; кожна операція відкриває новий
DB connection. Admission/connect/SQL входять у server deadline 2 s;
HTTP search та catalog також ділять outer budget. Pool не додавався.

- **Spaced:** 32 cases (indices 0,4,…,124 із попередньої 128-row shard-вибірки),
  32 directed pairs. Без відбору за latency.
- **Largest:** 8 pairs за `unique_count DESC,source_locale,target_locale`;
  перший eligible term у prefix64/id order, 3–40 codepoints без masks/controls.
  Ці selection reads перед expanded фазою прогрівають частину БД.
- Для кожної пари: positive exact, SQL-підтверджений absent exact, перші три
  codepoints + `*`, leading mask, infix і synthetic infix-negative.
  Spaced infix має 1–38 literal codepoints; largest — щонайменше 3.
- 55 старих controls (16 exact,16 prefix,16 word,7 continuations) проходять
  before/warm/recovery у кожному запуску з тим самим приватним keyring.
  Positive extra cases не мають незалежного semantic oracle.
  Negative body oracle побудовано окремо після run із `en.json` + plain renderer;
  точна SQL-відсутність marker не є доказом відсутності довільного substring.

Серіальні фази мають одного worker. Open-loop ordinary/mask phases — 300/160
arrivals з інтервалом 100 ms, 16 workers; largest mixed — 240, alternating
ordinary/masks 1:1. Ordinary тут означає рівні частки exact-positive,
exact-negative, short-prefix, а не погоджений production mix. Burst — 64
simultaneous scheduled arrivals, 16 workers, три masks однієї першої пари.
Client timeout 5 s починається при dispatch; maximum phase 600 s, body cap 8 MiB.
Усі outcomes, включно з помилками, лишаються в nearest-rank p50/p95.

Go-посіпака [http-probe](../../../minions/http-probe/main.go) зберігає окремо
client HTTP/body elapsed, dispatch lag і scheduled-to-complete. Keep-alive,
redirects і proxies вимкнені. Exit 0 означає complete accounting, а не HTTP success.
Клієнт працює поза container caps; host не ексклюзивний. Короткий повтор
minion race test 14:29:13–15 UTC частково перекрив кінець largest ordinary
і початок mask phase; результат не називається ізольованим benchmark.

## Затримки

У таблиці client p95, ms; усі HTTP outcomes включені. Serial/burst arrivals
із interval 0 навмисно стоять у клієнтській черзі; це не server queue timer.

| Фаза | Spaced: N / HTTP p95 | Largest: N / HTTP p95 |
|---|---:|---:|
| First-touch controls | 55 / 15.257 | 55 / 14.878 |
| Warm controls | 55 / 15.785 | 55 / 15.076 |
| Expanded serial | 192 / 61.533 | 48 / 76.021 |
| Ordinary 10/s | 300 / 15.676 | 300 / 15.693 |
| Masks 10/s | 160 / 59.551 | 160 / 75.203 |
| One-pair burst | 64 / 492.977 | 64 / 1513.180 |
| Mixed 10/s | — | 240 / 71.786 |
| Recovery controls | 55 / 14.977 | 55 / 15.115 |

Burst dispatch-lag p95 — 1329.004/3562.228 ms; scheduled-to-complete p95 —
1740.834/4370.988 ms. HTTP elapsed виключає цю клієнтську чергу.
Жодних 503/cancellations у цьому профілі не відтворено; це **не пояснює
історичні 503** і не доводить deadline/cancellation behavior для інших масок.
Порівняння first/warm одиничне й нерандомізоване; causal cache speedup не заявляється.
PostgreSQL block reads можуть обслуговуватись OS cache, а не фізичним диском.

## Ресурси й безпека

Phase-boundary cgroup snapshots: DB lifetime peak до кінця workload —
2.297/2.467 GiB; HTTP — 13.613/~512 MiB. **Це lifetime peaks від startup,
не phase peaks, Go heap або RSS**; final inventory виконується пізніше.
Largest HTTP `memory.events.max` уже був 13031 перед first controls,
13034 після них, далі без приросту; `oom/oom_kill` — 0.
[Пізніший memory.stat](largest/http-memory-after-workload.json) показує
переважання file cache над anon; він не відновлює склад startup peak.
Readiness hashing і різне cgroup cache charging — можливе пояснення,
а не виміряна причинність. Startup/low-memory behavior залишається окремою перевіркою.
CPU quota throttling counters збережено без приховування startup cost.

SQLSTATE 57014 рахувався з окремих приватних PG logs із безпечним prefix;
statement text logging вимкнений. HTTP logs містять лише перевірені lifecycle
fields/events. Raw PG logs, HTTP configs/paths і ключі лишилися ignored.
Disk guard 30 GiB контролювався між фазами та під час probes; phase-boundary
мінімум — 84.528/84.574 GiB. Глобальний pressure/caches flush не створювався.

## Цілісність і перевірки

Після кожного запуску виконано повний streaming multiset inventory:
17 таблиць/sequences тотожні попередньому restore; application state незмінний.
Count + sum/XOR SHA-256 — probabilistic oracle, не canonical migration digest.
Новий LOGIN і власні HTTP/DB containers/network прибрано, bind data/artifacts
збережено. Нічого з retained data не видаляли. Перша preflight-спроба зупинилась
до runtime через host permissions PGDATA; перевірку версії переведено на readonly
container без зміни прав. Усі спроби й scratch збережені локально.

`make check test test-race test-smoke` — PASS на фінальному code diff.
Self-review відтворило oversized-config failure: decoder limit приховував
trailing bytes після 2 MiB. Додано file-size guard до output creation; regression
test пройшов red→green. Для повільного CI також збільшено timeout запас у тесті.
Measured probe binary передував лише config-size hardening; request engine той
самий, усі реальні configs менші за limit. Hashes обох source states збережені.
[Локальна квитанція](local-checks.json) містить hashes коду/логів; CI перевіряється
на точному PR head. Посіпака не є service/operator utility чи CI dependency;
його unit tests входять у загальний `go test ./...`.

Послідовне self-review за `requesting-code-review`, `code-review` і
`verification-before-completion`: accounting/cancellation/body limits,
loopback/privacy/output overwrite, oracle boundaries та docs перевірені.
Окремого незалежного reviewer не було. Перевірки описані для цього diff;
CI та merge не оголошуються успішними до фактичного результату.

## Наступний gate

Потрібні безпечні server-side timings/класи результатів без query text,
погоджені traffic/crawler/resource profiles, довші й різноманітні маски,
справжні cache/low-memory та target-server measurements. AT-16, linguistic/UI
приймання, off-host RTO і escrow/RPO/retention відкриті. Ці спостереження
не є дозволом підвищувати deadline чи розгортати сервіс на зайнятому Oracle.
