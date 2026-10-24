# Interface catalogs

`locales/*.json` contains 31 embedded UI catalogs, each with the same 22 message
keys. English (`en`) is the fallback. These base UI identifiers are separate
from the dictionary's raw regional locale codes. Build and runtime do not need
the legacy checkout or a translation API.

Each catalog records its legacy PO filename and SHA-256. `origins` marks each
message `legacy-po` (reused compatible singular text) or `ai` (new translation).
Only complete, non-fuzzy, plain-text legacy messages were reused; malformed PO
blocks were skipped. All catalogs remain `ai-draft`: structural tests do not
certify linguistic quality or close OQ-06. Public launch requires a separate
language review, including retained legacy wording.

Edit the JSON files directly. Keep message keys consistent with `en.json` and
update origins when replacing legacy text. Messages contain plain text, without
HTML or placeholders; templates escape them. `go test ./internal/i18n` checks
completeness, metadata, mask hints, fallback and browser negotiation. Adding a
language also requires updating the expected inventory and reviewing its tags.

Browser matching uses `golang.org/x/text/language` with at least High confidence;
unsupported languages/scripts fall back to English. The legacy identifier `no`
is matched as Bokmål (`nb`) to avoid Danish being selected for `nb-NO`; its UI
identifier and dictionary codes remain unchanged. Manual selection overrides
browser matching.
