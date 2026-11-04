# Погодження прогалин metadata перед verify

## Обсяг

База — `ced60959a3b7132e6668a2944bbf4c08a6ee601b` (PR #17, ще відкритий на
початку роботи). Окремий залежний PR додає CLI/publication adapter для known gaps,
конкретний [звіт власнику](../../LOSS_REVIEW.md) та synthetic тести.
Реальний корпус, Oracle, legacy і persistent DB не змінювалися.

`--loss-approval FILE` читає bounded JSON receipt від довіреного оператора.
Verify повторює source/inventory checks; dataset/fingerprint і SHA-256 точного
reconciliation JSON із LF мають збігатися з рішенням. Receipt та known gaps
зберігаються в immutable report. Readiness/export/publish перевіряють їх знову;
нових DB privileges або schema migrations немає. Receipt не є цифровим підписом.

Clean path/старі reports сумісні; ready retry повертає той самий report і відхиляє
інший receipt. Без рішення gaps блокують ready. Exceptions, rejected/issues,
незавершені/невідповідні дані та source failures і далі блокують verify.

## Перевірки

- `make check test test-race test-smoke` — exit 0.
- `make test-integration` — exit 0; disposable PostgreSQL і restore clusters прибрані.
- Corpus report JSON round-trip поточним Go type дав ті самі bytes/SHA-256:
  `6cdcb790cce656f21e714d52c27213d085b1959de14b9983829001fa176abc2c`.
  Це offline перевірка metadata, без нового corpus run.
- Synthetic перевірки включають unknown/duplicate/case-alias keys, неправильні
  audit refs/IDs/hashes, trailing/nested/oversized JSON, змінені counts/gaps,
  інший dataset/fingerprint/report, source/metadata/incomplete/exception guards,
  failure до ready commit, повтор ready, publish та HTTP readonly readiness.
- CLI інтеграція передає SHA-256 фактичного reconcile stdout через receipt
  у verify/export/sitemap/publish; помилки файлів не розкривають arguments/DSN.

Перший integration run знайшов nil/empty slice різницю при clean ready retry;
виправлено зі збереженням старого JSON shape, повторний run пройшов.
Тест, що намагався змінити ready DB reference, відхилено чинним immutable trigger;
цей тестовий сценарій прибрано без послаблення захисту.

## Self-review та межі

Послідовний self-review охопив contracts, JSON identity, source/inventory guards,
retained report/readiness, retry/failure isolation і документацію. Під час review
додано відмову для case-alias keys, які Go Decoder інакше сприймає як відомі поля.
Незалежного review не було. Inputs та локальні logs перевірок ідентифіковані
в `reviewed-inputs.sha256` та `validation.json`; CI результат — у залежному PR.

Файл погодження реального корпусу не створено. Власник ще має погодити конкретні
прогалини з LOSS_REVIEW; лише після цього можна виконати full verify/exports.
Цей adapter не реалізує схвалення rejected records/excluded artifacts, не
публікує dataset автоматично та не закриває production capacity/RTO acceptance.
