# Утиліти сервісу

Утиліти потрібні для роботи сервісу. Їх можуть використовувати оператори,
автоматизація та інші програми; це не особисті помічники агента.

| Команда | Призначення |
|---|---|
| `go run ./utilities/prepare-artifacts data/artifacts` | Створення artifact root і fsync його та всіх ancestors перед dataset verify |

Потрібен Go з `.go-version`; Python runtime не потрібний. `make utilities`
збирає `bin/prepare-artifacts`. Binary можна запускати з будь-якої директорії;
відносний destination визначається від cwd. Existing files не змінюються.
Успіх → absolute path у stdout та exit 0; помилка → ненульовий exit.
Baseline filesystem — Linux із directory fsync, як для publication storage.

Проєктні перевірки для людей і CI описані в [tests/](../tests/README.md).
[Посіпаки](../minions/README.md) — виключно особисті помічники агента.
Мова реалізації не визначає категорію. Тимчасові речі зберігаються у `tmp/`.
