# Launch preparation review — 2026-11-09

## Scope and outcome

Documentation-only slice based on main `68655c445527851dc5aa630731014eadb124362b`
(merged PR31). Adds `docs/LAUNCH_PLAN.md`, storage arithmetic and current decisions;
updates README, INDEX, MIGRATION and SESSION_HANDOFF. Reviewed input hashes are
in `review-inputs.json`; this report and that receipt are excluded from their own hashes.

Sequential self-review using code-review and verification-before-completion skills;
no independent reviewer. No verified defects remain in this scope. Checked the
final owner-approved traffic/crawler/download profile against the replies, and
kept duration, resources, server choice and production acceptance distinct.

## Evidence checked

- Recomputed all storage formulas and totals from the three existing schema 4
  evidence files; verified their SHA-256 hashes against `storage-budget.json`.
  Serving payload: 76,852,658,154 bytes; restore planning scenario:
  144,293,344,144 bytes. WAL/temp/reserve are assumptions, not enforced limits
  or a peak bound. OS/other workloads and retained growth are excluded explicitly.
- Compared restore/bootstrap instructions with BACKUP_RESTORE and MIGRATION:
  fresh DB, non-superuser roles, dump's existing migration ledger, ANALYZE,
  restricted reader, files/keyring and source-independent control checks.
- Compared domain/proxy/cookie and health guidance with `internal/httpui/entries.go`,
  `internal/httpui/preferences.go` and `internal/ops/http.go`.
- `make check` PASS: go vet, patch whitespace, repository checks.
- `git diff --check` PASS. Exact-head CI is required after publishing the PR;
  this pre-PR report does not assert its outcome.

## Limits and next step

No new app test suite, import, restore, workload, SSH, DNS or deployment ran.
Application and CI configuration are unchanged. Prior local receipts remain
historical evidence; AT-16, off-host RTO, target capacity, resource/backup budgets
and owner UI/translation acceptance remain open. Owner confirmed Oracle is still
occupied: prepare requirements only. Retained data and unrelated services untouched.
