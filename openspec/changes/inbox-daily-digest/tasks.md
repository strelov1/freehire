## 1. Delivery

- [x] 1.1 Test and implement consent, due-recipient selection, thread grouping and claim-first delivery with SQL-backed integration coverage.

## 2. Product integration

- [x] 2.1 Extend notification settings contract/API/UI and wire the digest into the existing notify worker.

## 3. Verification

- [x] 3.1 Document behavior, run focused and full unit checks, simplify and review the change, and fix material findings.

## Validation

Full unit tests and the complete integration-tagged suite passed. The tagged run used
`-p=2 -timeout=30m` to avoid the cumulative handler timeout seen with the earlier run.
Both vet modes, changed-code Go lint, Svelte checks, web build, browser settings tests,
frontend lint, migration lint, file-size checks and documentation checks passed.
Read-only review findings (self-ingestion and delayed-classification chronology) were
reproduced with failing tests and fixed. The email preview was inspected in a browser.
