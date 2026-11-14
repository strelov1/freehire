## 1. Synchronization

- [x] 1.1 Test and implement SENT parsing, known-thread backfill and independent incremental discovery with retry-safe cursors.

## 2. Persistence and readers

- [x] 2.1 Test and implement additive schema, outgoing classification/event guards, inbox API fields and sent-message rendering.

## 3. Verification

- [x] 3.1 Update mail documentation, run focused and repository checks, simplify and review the complete change.

## Validation

Full unit tests, both vet modes, changed-code Go lint, targeted mail integration tests,
Svelte checks, frontend build, frontend lint, formatting, migration lint, file-size and
Markdown checks passed. The full integration run reached the HTTP handler package's
cumulative ten-minute timeout during creation of an OAuth test database. Its Gmail query
assertion also ran an earlier binary; the corrected query and final mail regressions were
rerun independently. The complete tagged suite is not claimed green.
