# Swedish PO refresh — 2026-11-09

Base: merged PR34, `9c145572addad133f577854cc9215665d7a6c859`.
Owner reported updating `slovnyk.sv-se.po`; legacy checkout was read only.
The previously assessed file had no translations. The current source compiles
with `msgfmt --check --statistics`: 70 translated messages; nonfatal warning
for missing `Language-Team`. Header dates are retained as source metadata and
are not treated as evidence of the date the owner updated the file.

## Mapping

[Source hash and exact mappings](mapping.json) record seven compatible singular
plain-text entries: intro, from, to, translate, ui, home and downloads.
They are copied verbatim, with origins `legacy-po`. The remaining 22 strings
have no direct equivalent for the new UI and stay unchanged with origins `ai`.
This includes combined mask hints and error messages. Old HTML, links, menu
items and regional language labels are not inserted into the new templates.
No translator contact details or full legacy file are published.

Swedish remains `ai-draft`: this refresh does not approve all new messages or
certify linguistic quality. Historical localization reports remain unchanged.
Builds need only the embedded JSON, without access to the legacy checkout.

## Validation and review

- `msgfmt --check --statistics`: PASS with the header warning above.
- Exact mapping equality against the compiled PO and source hash verified.
- `make check test test-race test-smoke`: PASS.
- Existing Chromium/CDP9222, isolated context, scripts disabled: fresh browser
  `sv-SE` selects `sv`; home/languages/downloads/search at 320/390/1280px
  have correct HTML lang and no horizontal overflow. PASS; desktop/mobile
  synthetic screenshots captured and visually inspected. No Playwright.
- Sequential self-review of source mapping, plain-text safety, metadata and
  retained draft state; no verified defects found in this scope. No independent
  reviewer/subagents. Reviewed inputs are recorded in `inputs.sha256`.
- No application code, DB data or Oracle changes. Large integration/data probes
  are not repeated locally; normal CI includes isolated PostgreSQL.
