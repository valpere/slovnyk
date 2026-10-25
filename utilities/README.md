# Утиліти сервісу

Утиліти потрібні для роботи сервісу. Їх можуть використовувати оператори,
автоматизація та інші програми; це не особисті помічники агента.

| Команда | Призначення |
|---|---|
| `go run ./utilities/prepare-artifacts data/artifacts` | Створення artifact root і fsync його та всіх ancestors перед dataset verify |
| `go run ./utilities/prepare-csv-manifest --source SNAPSHOT --output manifest.json --authority NAME` | Канонічний V1 manifest із hashes/bytes і filename pairs для immutable CSV snapshot |

Потрібен Go з `.go-version`; Python runtime не потрібний. `make utilities`
збирає `bin/prepare-artifacts` і `bin/prepare-csv-manifest`. Binary можна запускати з будь-якої директорії;
відносний destination визначається від cwd. Existing files не змінюються.
Для `prepare-artifacts`: успіх → absolute path у stdout та exit 0;
помилка → ненульовий exit. Формат результату manifest utility наведено нижче.
Baseline filesystem — Linux із directory fsync, як для publication storage.

## Підготовка CSV manifest

```sh
make build utilities
bin/prepare-csv-manifest --source /path/to/frozen-snapshot \
  --output /path/to/new-manifest.json --authority source-owner \
  --snapshot-date unknown --rights-status owner_confirmed \
  --known-gap 'CSV4 does not contain legacy count/status/query metadata'
bin/slovnyk import inspect --manifest /path/to/new-manifest.json \
  --source /path/to/frozen-snapshot
```

Потрібна вже підготовлена незмінна **пласка** директорія тільки з regular files
`slovnyk_<from>_<to>.csv.gz`. Утиліта не копіює й не заморожує джерело. Symlinks,
піддиректорії, сторонні файли, неоднозначні назви, порожні файли й повторні hashes
відхиляються; нічого не пропускається автоматично. Не перейменовує locale codes
і не перевіряє їх за сучасним BCP 47 registry. Для іншого layout/multi-pair файлів
потрібен окремо підготовлений V1 manifest за [контрактом](../docs/IMPORT_CONTRACT.md).

Source authority задається явно; snapshot date і rights status за замовчуванням
`unknown`. Дату не виводити з mtime. `--known-gap` повторюється; exceptions не
створюються. Versions/limits беруться з чинного Go parser/normalizer. Output
directory має існувати поза snapshot; новий файл створюється приватно, з fsync
файла/директорії та без overwrite. Канонічні bytes без LF мають SHA-256,
рівний fingerprint; stdout містить JSON summary та `content_verified=false`.
Host paths і час запуску не входять до manifest. При помилці немає success summary;
після аварійного завершення перевірте новий output перед використанням.

Hash inventory **не читає gzip/CSV contents**. Назва задає очікувану пару,
яку перевіряє окремий `import inspect`; він також перевіряє stream до EOF,
admission/rejections і hashes. Manifest не доводить повноту історичних даних,
не схвалює втрат і не дозволяє ready/publication без verify. Утиліта не підключається
до БД. Exit 0 — manifest записано; 2 — usage; 1 — input/output failure.

Проєктні перевірки для людей і CI описані в [tests/](../tests/README.md).
[Посіпаки](../minions/README.md) — виключно особисті помічники агента.
Мова реалізації не визначає категорію. Тимчасові речі зберігаються у `tmp/`.
