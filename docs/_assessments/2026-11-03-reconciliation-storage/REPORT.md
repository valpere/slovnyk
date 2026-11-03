# Місце для повної PostgreSQL звірки

## Результат preflight

2026-11-03, runtime base `63a7ce2634e53a272c1b4681950faab1ac81027b`.
Перед `dataset reconcile` виявлено 22.09 GiB вільного місця, менше
за попередній локальний захисний поріг 30 GiB. Точні bytes — у
[preflight.json](preflight.json). PostgreSQL та reconciliation не запускалися;
стан dataset відомий лише з [попереднього звіту](../2026-10-27-postgres-corpus-import/REPORT.md).
Нових SQL-підтверджень немає. Сортування та joins можуть потребувати temp disk;
30 GiB — початковий захисний поріг, не доказ достатнього peak budget.

## Кандидати на погодження

Усі шляхи нижче відносні до `data/experiments/`. Нічого не видалено.

| Файл | Розмір GiB | Відтворення та наслідки |
|---|---:|---|
| `sqlite-full/restored.sqlite3` | 12.43 | Копія `corpus.sqlite3` після історичної restore-проби. Обидва файли сьогодні прочитані `sha256sum`; hashes збігаються з historical restore evidence. Повторне копіювання через `restore.py` відтворює копію; попередні results зберегти. |
| `provenance-indexed/model.sqlite3` | 29.66 | Похідна базової `provenance/model.sqlite3` з covering index. Відтворюється з неї через `prepare.py`; до відтворення цей layout недоступний для повторних benchmarks. Поточний повний hash цього файла не перевірявся. |
| `provenance-compact/model.sqlite3` | 20.85 | Також відтворюється з базової моделі через compact `probe.py`. Залишити як найменший перевірений SQLite layout; видалення зараз не потрібне. |

Рекомендація: погодити лише перші два файли — близько **42.09 GiB**
allocated space, очікувано близько 64.19 GiB free за незмінного іншого використання.
Фактичне звільнення перевірити через `df` після погодженого прибирання.
Папки й JSON-результати не видаляти. Відтворення потребує часу й нового
вільного місця; історичні timings не є новим виміром.

## Підстави та захищені дані

- [Restore procedure і hashes](../2026-10-24-sqlite-full/REPORT.md#відновлення),
  [restore.py](../2026-10-24-sqlite-full/restore.py).
- [Indexed procedure](../2026-10-24-sqlite-indexed/REPORT.md#відтворення),
  [prepare.py](../2026-10-24-sqlite-indexed/prepare.py).
- [Compact procedure](../2026-10-24-sqlite-compact/REPORT.md#відтворення).

Зберегти seed `sqlite-full/corpus.sqlite3`, базову provenance-модель, compact
модель, manifests/snapshot CSV, усі metadata/results, private config та `.pgdata`.
Це локальні експерименти, не off-host backups. Oracle та legacy не задіяні.
`tmp/` і `bin/` разом займають лише близько 102 MiB, проблему не вирішують.
Docker cache/images/volumes можуть належати іншим проєктам; global prune
не пропонується. `lsof` бачив лише власний hash reader, але мав permission
warnings щодо Docker namespaces; перед видаленням повторно перевірити users.

## Перевірка та продовження

Послідовний self-review: розміри зіставлені зі `stat`/`du`, процедури прочитані,
приватні дані й dictionary tuples до evidence не включені. Це не незалежний review.
`make check` пройшов: go vet, patch whitespace і repository checks.
Application code не змінено; full integration локально не запускали.
Після погодженого прибирання: повторити disk gate, запустити лише власний
Compose project, явно зафіксувати RAM/CPU та контролювати free disk під час replay.
Known gaps, exports/ready/publication та production acceptance залишаються відкритими.
