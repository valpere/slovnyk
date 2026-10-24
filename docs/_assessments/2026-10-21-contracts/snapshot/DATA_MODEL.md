# Модель даних V1

Статус: конкретизація запропонованих технічних рішень HLD, ще не SQL-міграція.
Цей документ визначає ключі й інваріанти; реалізація має забезпечити їх DDL,
транзакціями та verify. Поведінкові вимоги залишаються у REQUIREMENTS.

## Типи та ідентичність

Dataset ID — UUID. Translation ID — додатний bigint, унікальний у dataset.
Artifact/tuple/key/fingerprint digest — 32 байти SHA-256, не текст довільної довжини.
Locale code — непорожній UTF-8 рядок до 64 bytes, без NUL; дозволений лише код
з маніфесту. Не вимагати відповідності сучасному BCP-47 від legacy-кодів.
Source/target text — непорожній UTF-8 без NUL, до 64 KiB **кожне поле** у V1.
Це захисний admission limit, не висновок про довжини реального корпусу.
Більші значення потрапляють у quarantine та потребують зміни версійного контракту
або погодженого винятку; ніколи не обрізаються для успішного імпорту.

Mapping V1 — **identity**: чотири декодовані поля зберігаються буквально,
включно з кодами локалей і крайніми пробілами. Aliases працюють лише у HTTP.
`sr-sp` не зливається з `sr-rs`. Майбутній неідентичний mapping потребує окремого
рішення щодо raw identity та експорту; його не можна увімкнути непомітно.

Tuple digest рахується з чотирьох полів у порядку `source_locale`,
`target_locale`, `source_text`, `target_text`: для кожного — uint64 big-endian
довжина UTF-8 bytes та самі bytes. Duplicate — рівність усіх чотирьох полів,
а не лише hash. Різні tuples з одним hash зупиняють імпорт/verify як collision.
Нормалізований ключ — NFC → full Unicode case folding → NFC; жодного trim.
Його hash — SHA-256 UTF-8 ключа. Exact завжди перевіряє ще й рівність ключа.

## Таблиці та обов'язкові обмеження

| Таблиця | Ключ, поля та інваріант |
|---|---|
| `datasets` | PK `id`; UNIQUE fingerprint; manifest bytes/hash, schema/parser/mapping/normalization versions, state, timestamps. Готовий набір незмінний. |
| `active_dataset` | Singleton PK з CHECK id=1; nullable dataset ID, bigint generation >=0. FK лише на готовий набір через publication service. |
| `dataset_locales` | PK `(dataset_id, code)`; raw code, nullable display name. |
| `dataset_pairs` | PK `(dataset_id, source_locale, target_locale)`; composite FK обох локалей; unique_count bigint >=0. |
| `translations` | PK `(dataset_id,id)`; composite FK на pair; чотири raw поля, source_key, key_hash, tuple_hash; UNIQUE `(dataset_id,tuple_hash)` з обов'язковою collision check до merge. |
| `source_files` | PK `(dataset_id,artifact_hash)`; original relative name, format, byte size, completion/checksum state; один hash не можна двічі включити у manifest. |
| `import_records` | PK `(dataset_id,artifact_hash,ordinal)`; ordinal від 1; outcome unique/duplicate/rejected, nullable translation ID, nullable issue code; composite FK до file/translation того самого dataset. |
| `translation_origins` | PK і FK на accepted import_record; translation ID, чотири raw поля, nullable legacy count/status, source locator і provenance. Є для unique **та** duplicate occurrences. |
| `import_runs` | UUID run ID, dataset FK, attempt, checkpoint per artifact, state/error class; лише один writer dataset. |
| `import_issues` | Dataset/artifact/ordinal або byte offset, reason code, контрольований quarantine reference; жодних секретів. |
| `legacy_query_snapshots` | Dataset, artifact/ordinal, raw input/output locale, word, nullable count int64, nullable raw modified timestamp, timezone known/unknown; не live-лічильник. |
| `publication_events` | Generation PK; previous/new dataset IDs, operator/run reference, timestamp; append-only. Підтверджує право cursor читати retained version. |
| `exports` | PK `(dataset_id,export_id)`; FK pair, relative artifact reference, SHA-256, bytes, logical rows, verified flag. |

Nullable `count/status` — int64 зі збереженням 0 і від'ємних значень, якщо вони
є в snapshot; невідомі значення — NULL, не default 1. Source timestamps зберігаються
як raw text і опис типу, без вигаданого UTC. Точний DB adapter затверджується після
аудиту живої схеми OQ-02. Записи з несумісними типами блокують verify або мають
окреме погоджене рішення; conversion не допускає silent overflow.

## Індекси й незмінність

Початковий exact index: `(dataset_id,source_locale,target_locale,key_hash)`;
residual `source_key = query_key` обов'язковий. Не індексувати необмежений text
кортеж B-tree. Mask спочатку використовує pair filtering, параметризований LIKE,
sort і timeout; оптимізація після EXPLAIN на корпусі. Порядок — C collation за
`(source_key,source_text,target_text,id)`, не locale-dependent collation.

Після ready заборонені зміни translations, origins, каталогу й manifest;
окремий export verification/publish event не змінює вміст набору. FK та verify
забороняють cross-dataset references. HTTP DB role не пише у ці таблиці;
вибір опублікованого dataset виконується application service, не довільним ID клієнта.
