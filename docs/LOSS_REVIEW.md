# Звіт про прогалини даних: CSV snapshot першого запуску

## Рішення, яке потрібне від власника

**Статус: погоджено власником 2026-11-04.** [Рішення](DECISIONS.md#owner-approval-2026-11-04),
[receipt](_assessments/2026-11-04-corpus-verification/owner-approval.json).
Дозволено перший запуск без історичних
`count/status/queries`, відсутніх у доступному CSV4 snapshot. Це конкретне
погодження обмежень джерела; словникові записи не пропонується виключати.
Загальне рішення 2026-10-23 та успішна звірка не замінюють цього погодження.

## Набір і перевірені факти

- Dataset: `96541c0b-ac71-4e38-b57c-b593e82ac9d5`.
- Manifest fingerprint: `ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01`.
- [Технічний звіт](_assessments/2026-11-04-corpus-reconciliation/report.json),
  SHA-256 **точних bytes із завершальним LF**:
  `6cdcb790cce656f21e714d52c27213d085b1959de14b9983829001fa176abc2c`.
- 992 complete/checksum-verified CSV.gz; 56 455 068 parsed = unique,
  0 duplicates/rejected, exceptions порожні. Raw tuples, normalized keys,
  ledger/origins та inventory звірені з PostgreSQL.
- State `validating`, active null; exports/ready/publication не виконувалися.

Оригінальний known gap лишається в immutable manifest:

> CSV4 has no legacy count/status/query metadata; completeness is limited to the available snapshot

## Чого бракує та наслідки

| Прогалина | Поведінка нового сервісу |
|---|---|
| Історичні `count` перекладів | У provenance — NULL/unknown; значення не вигадуються й не відновлюються з кількості рядків CSV. |
| Історичні `status` перекладів | У provenance — NULL/unknown; імпорт не приписує `status=1`. Старі рішення про статус із CSV відтворити неможливо. |
| Історичні записи `queries` | `legacy_query_snapshots` порожня. Минулі запити, їхні лічильники й timestamps не відновлюються. Нова статистика — окремий етап. |
| Історична повнота | Звірено весь доступний snapshot. Невідомо, чи до нього потрапив кожен запис колишньої live БД; кількість можливих відсутніх записів невідома. |

Пошук і CSV.gz завантаження спираються на наявні переклади. Мовна правильність
та історичний статус кожного перекладу цією звіркою не засвідчені. DDL описує
структуру, але не містить відсутніх значень. Старий сервіс загинув; джерела
першого запуску — остаточні DDL + CSV. Вихідні файли зберігаються незмінними.

## Як зафіксувати погодження

Після явного рішення власника довірений оператор створює окремий JSON-файл
за схемою нижче. **Шаблон не є погодженням**; placeholders потрібно замінити
фактичними відомостями. `decision_ref` посилається на рішення власника щодо
саме цього звіту й technical SHA-256. Receipt створюється лише після цього
явного рішення; агент не надає погодження від свого імені.

```json
{
  "version": 1,
  "dataset": "96541c0b-ac71-4e38-b57c-b593e82ac9d5",
  "fingerprint": "ce4227513bfcc36afafaef1834c8cb76a10e03ab5dbd66085c2e633daa150e01",
  "reconciliation_sha256": "6cdcb790cce656f21e714d52c27213d085b1959de14b9983829001fa176abc2c",
  "owner_ref": "<ідентифікатор власника>",
  "decision_ref": "<посилання на явне погодження цього звіту>",
  "approved_at": "<фактичний час рішення RFC3339>"
}
```

Операторська команда після погодження:

```sh
./bin/slovnyk dataset verify <dataset-id> --source <snapshot> --artifacts <root> --loss-approval <approval.json>
```

Verify повторює source/inventory checks і звіряє SHA-256 відтвореного JSON
`dataset reconcile`, а потім створює exports/ready зі збереженням рішення
та known gaps у retained report. Source/manifest/gaps/counts мають збігатися.
Зміна версії diagnostic JSON також потребує нового звіту й погодження.
Форматування stdout слід зберігати: pretty-print змінює його SHA-256.

Receipt — audit evidence від довіреного CLI-оператора, не цифровий підпис
і не перевірка особи. Доступ до operator credentials/root залишається межею довіри.
Без receipt gaps блокують verify. Exceptions, rejected records/issues, fatal files
та невідповідності блокуються навіть із receipt. Погодження виключення записів
або artifacts потребуватиме окремого adapter; поточний цього не підтримує.
Publish залишається окремою CAS-командою; readiness повторно перевіряє retained
report і receipt. Цей документ не дозволяє production deployment чи зміни Oracle.
