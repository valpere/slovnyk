# Трасування та перевірка

Критерії AT визначені в [REQUIREMENTS](REQUIREMENTS.md); сценарії — у
[USE_CASES](USE_CASES.md); компоненти — у [HLD](HLD.md).
**Усі перевірки нижче заплановані, ще не реалізовані й не пройдені.**
При реалізації додати точний шлях тесту та фактичний результат, не оголошувати
всю вимогу виконаною після часткової реалізації.

| Вимога | Use case | Компонент HLD | Перевірка | Етап |
|---|---|---|---|---|
| FR-01 | UC-01, UC-03 | FUN-CATALOG | AT-01 | M1, M2 |
| FR-02 | UC-01 | FUN-LOOKUP | AT-02 | M1 |
| FR-03 | UC-02 | FUN-LOOKUP | AT-03 | M1 |
| FR-04 | UC-01, UC-02 | FUN-LOOKUP | AT-04 | M1, M2 |
| FR-05 | UC-01, UC-03 | FUN-UI | AT-05 | M1, M2 |
| FR-06 | UC-04 | FUN-EXPORT | AT-06 | M2 |
| FR-07 | UC-05 | FUN-IMPORT | AT-07 | M1, M2 |
| FR-08 | UC-06 | FUN-IMPORT | AT-08 | M1, M2 |
| FR-09 | UC-05, UC-06 | FUN-IMPORT | AT-09 | M2 |
| FR-10 | UC-06 | FUN-PUBLISH | AT-10 | M1, M2 |
| FR-11 | UC-09 | FUN-IMPORT | AT-11 | M2 |
| FR-12 | UC-07 | FUN-UI | AT-12 | M2, M3 |
| FR-13 | UC-08 | FUN-OPS | AT-13 | M3 |
| NFR-01 | UC-01, UC-06 | FUN-LOOKUP, FUN-IMPORT | AT-14 | M1 |
| NFR-02 | UC-01, UC-02, UC-04 | FUN-LOOKUP, FUN-UI, FUN-EXPORT | AT-15 | M1, M2 |
| NFR-03 | UC-02, UC-06 | FUN-LOOKUP, FUN-IMPORT | AT-16 | M2, M3 |
| NFR-04 | UC-01, UC-06, UC-08 | FUN-OPS | AT-17 | M1, M3 |
| NFR-05 | UC-05, UC-06 | FUN-CATALOG, FUN-IMPORT | AT-18 | M2 |

## Як перевіряти при реалізації

- Unit tests: нормалізація, маски/escaping, cursor, fallback, CSV parsers і mappings.
- PostgreSQL integration tests: реальна collation, LIKE Unicode, довгі ключі,
  транзакційність, повтор batch, atomic publish та rollback.
- Browser/HTTP smoke: no-JS форма, всі стани результату, locale, legacy URLs,
  text/plain та download; fixtures з HTML/SQL-метасимволами.
- Migration rehearsal: повний source manifest, reconciliation digests, перерваний
  імпорт, exports, backup/restore та контрольні запити на реальному corpus.

Команди збірки й тестів додати разом із Go-каркасом; сьогодні `go test ./...`
не є доступною перевіркою, бо `go.mod` і застосунку ще немає.
Не додавати модельних викликів або статистичних метрик тексту замість цих тестів.
