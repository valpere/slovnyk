# UI owner-review preparation — 2026-11-09

## Scope and review

Base: `c26a2cace0abc2954ee5a2d61bdd829498365cfa` (merged PR32).
Sequential self-review using code-review/requesting-code-review and
verification-before-completion; no independent reviewer. Exact reviewed inputs
are in `review-inputs.json` (excludes this report and its own receipt).
No verified unresolved defects in the reviewed diff.

Promotes the repeated scratch preview into agent-only Go `minions/ui-preview`.
It serves actual HTTP templates/catalogs/preferences with three fixed synthetic
pairs and real synthetic CSV.gz downloads; loopback only, no DB/credentials.
Search callbacks are fixed visual scenarios, not SQL/mask/pagination verification.
Only four Ukrainian messages changed: intro capitalization, “Слово або маска”,
“Мова інтерфейсу”, “Головна”; origins updated to `ai`. Source PO hash retained.
No language status, layout, production backend, schema or approved requirements changed.

Review passes: FR-05/06 and localization provenance; loopback binding and own-only
scratch lifecycle; unknown pair/dataset/entry handling; download metadata versus
bytes and visible fixture content; docs versus actual preview limits; owner
acceptance distinguished from structural/browser checks. All seven screenshots
visually inspected. UI_REVIEW is the concrete packet for owner feedback.

## Actual validation

- `go test ./minions/ui-preview ./internal/i18n ./internal/httpui` — PASS.
- `make check test test-race` — PASS, exit 0.
- `make test-smoke` — PASS, exit 0, including rebuilt embedded catalogs.
- `git diff --check` — PASS before commit.
- Chromium `Chrome/157.0.8103.38`, existing CDP 9222, no Playwright or new browser
  process. Final browser driver exit 0; [results](browser-results.json).
- 31 catalogs × four views at 320 CSS px: home, language pairs, downloads,
  no-match, correct HTML language and no horizontal overflow. This is 124
  synthetic navigations, not linguistic certification or a load test.
- Clean uk-UA first visit, uk→en pair proposal, saved manual en overriding de-DE,
  keyboard skip→main→query→native GET submit, RTL article/long/multiline text PASS.
- Seven full-page screenshots: en/uk desktop home and mobile search, uk no-match,
  uk downloads, RTL article. Widths: 1280 desktop, 390 mobile, 320 RTL article.

Initial browser attempt inherited an earlier loopback pair cookie (ports share
cookies); clean curl confirmed correct default proposal. Driver now creates and
disposes its own temporary browser context, preserving user cookies/tabs. A later
keyboard expectation also needed an explicit pair after the test's own previous
searches. These were driver-state errors, not application fixes; final full run
passed. Scratch driver/logs/binary remain in ignored `tmp/ui-review-2026-11-09/`.

Preview stopped by SIGTERM, exited 0; its generated `tmp/ui-preview-*` directory
removed by its own cleanup. No own browser context/tab remains. No retained data,
Oracle, legacy, unrelated runtime, DNS or production operations touched.

## Remaining acceptance

All 31 catalogs remain `ai-draft`. Owner design/language review (all 29 phrases
for each approved language), full-corpus catalog usability, screen-reader speech,
other browsers and target-server/off-host acceptance remain open. PR merge is not
language/design acceptance. PostgreSQL integration not repeated locally: backend
unchanged; normal exact-head CI remains required and its outcome belongs in PR.
