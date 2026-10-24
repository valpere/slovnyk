# Консиліум: уточнення контрактів Slovnyk

Дата: 2026-10-21. **Результат: документи готові як база M1 на синтетичних даних.**
Це не підтвердження готовності застосунку, реальної міграції або production launch.
Уточнено DATA_MODEL, IMPORT_CONTRACT, LOOKUP_CONTRACT, LEGACY_COMPATIBILITY;
синхронізовано вимоги, HLD, сценарії, migration plan, індекс і трасування.

## Метод і походження

Використано процедуру `~/wrk/common_codex/skills/fix-review/SKILL.md`.
З прикладів v2v council, showroomla consilium та lance-agent consilium взято
незмінний спільний input, незалежні відповіді, окремі журнали й арбітраж доказами.
Шлях `projects_fh/1654083-создание-сайта-для-транспортной/minions/council` не знайдено.
Чужі runner scripts не запускалися: їх HEAD-only snapshots пропустили б поточні
untracked docs; project-specific prompts і automatic merge не переносилися.

Цей прохід: 3 спеціалізовані native Codex агенти та 3 різні сім'ї cloud models
через локальний authenticated Ollama. Native агенти однієї сім'ї, точний model ID
не доступний через collaboration API. Cloud reviewers отримували всі 12 файлів
inline, з номерами рядків, без попередніх відповідей. Це model reviews без tool calls,
не запуск зовнішніх CLI-агентів. Native агенти читали frozen files самостійно.

| Cloud reviewer: фактичне поле model | Прохід 1 | Повторний прохід |
|---|---|---|
| gpt-oss:120b | complete, 6 findings | complete, 6 findings |
| mistral-large-3:675b | complete, 6 findings | complete, 6 findings |
| deepseek-v4-pro | complete, 6 findings | complete, 6 findings |

Усі шість відповідей: transport successful, done=true, done_reason=stop,
JSON structure/file/line validation pass. Обрані cloud tags і параметри —
run-config.json; actual model IDs, тривалість і token counts — *.status.json.
Tag не фіксує provider weights; вартість не визначена. Локальні генеративні моделі
й OpenRouter не використовувалися. Немає проваленого виклику, прихованого як clean.

## Підтверджено та виправлено

- Попередні прогалини: raw/mapped identity, повторне включення artifact,
  stale publish, failed/retry transitions, lossless CRLF, NUL, довгі B-tree keys,
  unbounded cursors, cursor dataset до validation, code points після fold,
  response-byte budget і явна trim asymmetry — закриті конкретними контрактами.
- Native migration reviewer: exports могли перезаписати попередню версію;
  atomic rename не гарантував durability. Додано immutable dataset/export/hash
  paths, no-clobber, fsync file/directories до DB commit, startup hash checks.
  Cloud DeepSeek незалежно звернув увагу на immutability.
- Native legacy reviewer: redirects губили UI-мову; permanent CSV redirect міг
  закешувати старий active dataset. Тепер resolved ui, 302 + no-store, Vary,
  cacheable immutable target; fixtures доповнено.
- Native lookup reviewer: пріоритет одночасних HTTP-помилок був неоднозначний.
  Визначено validation order; q=&mode=invalid повертає 400.
- Cloud GPT-OSS, повторний прохід: HLD CLI examples пропускали обов'язковий
  expected-generation. Виправлено publish і rollback examples. Це consistency
  defect документації, а не доведена runtime ABA-помилка.

Усі три native reviewers повторно перевірили виправлення на final-snapshot:
нових конкретних регресій у заявленій області не знайшли. Після цього змінено лише
два CLI examples; цей delta перевірено вручну і точним порівнянням рядків,
без заяви про третій model pass.

## Відхилені зауваження

Повний розбір **усіх 36 cloud findings**: adjudication.json. Більшість вимагала
вже прописаного контракту або припускала його порушення майбутньою реалізацією.
Наприклад, GPT-OSS заявив, що concatenated gzip members відхиляються, хоча текст
явно їх приймає. Інші вимагали per-file transaction, попри staging/failure gate,
або вважали, що cursor pin/CAS/foreign keys відсутні, цитуючи їхні ж визначення.
Незмінний page limit, fixed alias allowlist і 410 для недоступної старої версії —
свідомі правила, а не доведені дефекти. Не застосовано рекомендацію перевищити
response budget чи перевіряти fingerprint до завантаження pinned normalizer.
Кількість голосів не визначала рішення. Complete response не означає правильне рев'ю.

## Перевірки та точна ціль

- accepted-sha256.json та accepted-snapshot/: кінцеві 12 canonical documents.
- snapshot/: перший cloud/native input; final-snapshot/ і recheck/snapshot/:
  тотожний input повторної перевірки. Hash equality перевірено.
- final-structural-validation.json: 18 unique requirements, 18 trace rows,
  18 acceptance IDs, HLD anchors, локальні file links та whitespace — pass.
- accepted-*-check*.json: pinned tumanomir check, exit 0, 18 requirements,
  hanging=0, K_drift=0. D_const warn залишається (для всіх docs приблизно 0.0043);
  threshold не послаблювали і текст маркерами не роздували.
- Стохастичний measure повторно не запускався; попередній N=5 звіт залишається
  окремо в ../2026-10-21/. Не заявляємо зниження D_pair або причинний ефект правок.
- Застосунку/go.mod немає; app, PostgreSQL integration, crash, browser і migration
  tests **не виконані**. Перелік fixtures — завдання для реалізації.

## Що залишається відкритим

OQ-02: авторитетний snapshot/фактична legacy DB schema і точна JSON Schema DB adapter;
OQ-03: hosting/capacity/SLO; OQ-04: додаткові production URL aliases;
OQ-05: права на розповсюдження corpus; OQ-06: остаточні UI-мови.
Це не перешкоджає M1 на synthetic fixtures. Реальні import/cutover/distribution
потребують відповідних відповідей, rehearsal і приймальних перевірок.
Документаційні limits є пропозицією, їх відповідність corpus ще не виміряна.
Git commit/push, публікація, міграція та merge не виконувалися.
