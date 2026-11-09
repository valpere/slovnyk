# Brand design review — 2026-11-09

## Scope and inputs

Base `5472976f0157298d101f6ce08e3e121e39ae9e2d` (merged PR33).
Owner requested a unified Slovnyk design from the supplied local Town brief,
personal PDF brand guidelines and cream screenshot. [Design rules](../../UI_DESIGN.md)
record the synthesis; these are implemented HTML/CSS views, not raster mockups.
No Town graphics, personal logo files or dictionary terms are published here.
Rubik comes from Google Fonts at the pinned revision and OFL license recorded
in [asset provenance](../../../internal/httpui/assets/README.md).

## Implementation

Warm paper background, green actions, yellow brand/focus accents, local Rubik,
large restrained heading, compact book symbol, and line-separated results.
Responsive search form retains native controls, original request fields,
keyboard order, labels, language annotations and no-JavaScript operation.
A bundled font route supports GET/HEAD/ranges and adds only same-origin fonts
to the shared search/entry CSP. No new translated strings; 31 catalogs stay draft.

## Actual verification

- `make check test test-race test-smoke`: PASS on final application diff.
- `TestBundledFontDelivery`: GET/HEAD lengths/type, no preference cookies,
  nosniff and byte ranges PASS.
- Existing Chromium via CDP 9222; no Playwright/browser launch. Isolated browser
  context, page JavaScript disabled. 31 locales × 4 views at 320px: correct
  HTML language and no horizontal overflow.
- Browser negotiation, saved UI override, keyboard skip/main/query, native
  GET form submission with q/from/to, RTL article and long/multiline text PASS.
- Local actual service on loopback8787: readiness, home, bundled font, owner search,
  full-corpus language inventory and downloads listing return HTTP200. Retained
  restored DB reused; no import/restore or workload run.
- Browser font inspection reports a custom Rubik font in use. Seven screenshots
  captured from synthetic fixtures only; desktop/mobile and RTL visually inspected.

[UI acceptance and screenshots](../../UI_REVIEW.md). Scratch scripts/results and
logs stay under ignored `tmp/ui-brand-2026-11-09/`; full imports, restores,
load probes and local PostgreSQL integration were not repeated for this UI change.
Normal CI includes isolated PostgreSQL tests. This report does not claim CI success.

## Review

Sequential self-review using code-review and verification-before-completion;
no independent reviewer or subagents. Reviewed template escaping, response bounds,
route specificity, font provenance/cache/versioning, CSP consistency, no external
resources, query fields, accessibility, mobile wrapping and document claims.
No verified defects remain in the reviewed application diff. Reviewed file hashes
are in `inputs.sha256`; later changes need review of their delta.

Owner design/language acceptance, assistive-technology speech, other browsers
and usability of long full-corpus lists remain open. Local HTTP evidence is
not target-server capacity acceptance. Nothing was deployed to Oracle.
