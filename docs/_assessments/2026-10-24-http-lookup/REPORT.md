# M1-07: перший HTTP зріз — 2026-10-24

## Scope і review

Base: `00517458fefee0b71b76feb0041e2e67fe43deae` (`main`).
Послідовний **self-review**, не незалежне або multi-agent рев’ю.
Reviewed input hashes наведено в [reviewed-inputs.sha256](reviewed-inputs.sha256);
report та inventory не включені до власного hash scope.

Переглянуто новий `internal/httpui/`, config/keyring, backend і wiring
`internal/ops/`, HTTP-role integration, process smoke, активні docs та env example.
Порівняно з LOOKUP_CONTRACT і M1-07: transport/parameter precedence, єдиний
URL decode, cursor до acquisition, explicit pair, statuses, HTML escaping,
whole-entry continuation, final body guard, bounded connections і deadline.
Окремий documentation pass перевірив розрізнення реалізації та цільового scope.
Підтверджених невиправлених findings у цьому зрізі немає.

## Реалізовано

- `/` і `/search`, HTML без JavaScript та `format=plain`, grouped results,
  no-match mask hint і continuation; errors не містять driver details.
- Raw target 32 KiB, query bounds, duplicates, explicit empty parameters,
  malformed percent/UTF-8/NUL, controls; empty form не викликає lookup.
- Буфер до 1 MiB перед headers, коротка 503 на overflow, без часткового 200.
- До 8 dedicated search connections; admission/connect/SQL ділять deadline 2 s.
- Required HTTP keyring з rotation і safe config errors. Probes лишаються JSON.
- `no-store`, `noindex, follow`, CSP, nosniff; жодних operator routes.

## Фактична перевірка

`make check`, `make test`, `make test-race`, `make test-smoke` та
`make test-integration` — PASS (exit 0). PostgreSQL runner створив і видалив
власні Docker network/volume/container; version 17.11. HTTP-role test пройшов
lookup/continuation поверх фактично імпортованого, verified і published
синтетичного dataset. TCP handshake timeout перевірено unit/race тестами.
Process smoke перевірив actual binary, routes, statuses, query privacy і signals.
Точні test paths наведено в [TRACEABILITY](../../TRACEABILITY.md).

[Desktop](desktop.png) і [mobile](mobile.png): existing Chromium через CDP 9222,
скрипти сторінки вимкнено. Новий browser process і Playwright не використовувалися.
Перший screenshot виклик із compositor defaults завершився timeout; capture
із `fromSurface=false` дав обидва знімки. Це visual check форми, не всіх станів.

## Залишковий scope

M1-07 **не завершено**. UI поки англійський, вводить literal locale codes;
мовні каталоги/PO, browser/manual localization, cookies і пропозиція пари,
`/languages`, downloads, stable dictionary URLs/discovery/sitemap ще попереду.
Це не затверджене обмеження набору локалей, не production UI/release і не повний AT.
Повний corpus, Oracle, production capacity/p95 та legacy не змінювалися.
PR/CI результати фіксуються окремо в GitHub; цей локальний звіт їх не передбачає.
