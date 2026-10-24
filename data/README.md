# Локальні дані

`data/experiments/` — постійне локальне сховище БД та експортів технічних
експериментів. Воно виключене з Git і не входить у чистий checkout.
Не очищати його разом із тимчасовими файлами. Це експериментальні дані,
не production-БД і не резервна копія поза машиною.

| Директорія | Вміст |
|---|---|
| `experiments/sqlite-probe/` | Початкова SQLite-проба чотирьох мовних пар |
| `experiments/sqlite-full/` | Повна serving-БД `corpus.sqlite3`, копія `restored.sqlite3` та локальні результати |
| `experiments/provenance/` | Повна модель `model.sqlite3` із ledger/origins та 992 CSV.gz у `exports/` |
| `experiments/provenance-indexed/` | Окрема копія повної моделі з ordered covering index; БД 29.66 GiB |
| `experiments/provenance-compact/` | Логічно звірена повна модель із compact dataset/artifact keys і тими самими indexes; БД 20.85 GiB |

2026-10-24 `sqlite-probe/`, `sqlite-full/` та `provenance/` перенесені
з однойменних директорій у `tmp/`. Наступні експерименти створюються одразу тут.
Перенесення відбулося в межах файлової системи; для кожного файлу перевірено
незмінність device/inode та розміру. Вміст БД та експортів не перезаписувався.
Старі шляхи в історичних звітах позначають місце первинного експерименту;
поточне розташування — тут.

Оригінальні CSV та старий код залишаються у `../slovnyk1.0/` без змін.
Інструменти та звіти зберігаються в `docs/_assessments/`.
Для нових запусків створювати окрему директорію в `data/experiments/`,
не перезаписувати наявні результати. `tmp/` використовувати для scratch-файлів.

## Runtime artifacts

`data/artifacts/` — окремий ignored root нової реалізації для immutable exports
та verification reports, не експериментальні SQLite exports. Provisioning:
`go run ./utilities/prepare-artifacts data/artifacts`. Ready/publication зберігають
посилання сюди; не очищати разом із `tmp/`, не змінювати final bytes, GC вимкнений.
HTTP отримує read-only mount. Backup має включати БД і всі referenced artifacts.
