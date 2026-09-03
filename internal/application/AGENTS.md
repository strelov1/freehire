# internal/application

The candidate's relationship with a posting over time — tracking and stages, the event ledger, and the mail and calendar stack that feeds them. Mail and tracking are one block because a classified email advances a stage and the tracker reads the classifier.

**Layer 6 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job` — and itself.

Must NOT import: `search`, `engage`, `ingest`, `api`.

`search` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`appevent` `apptimeline` `autoapply` `calmatch` `calsync` `deliverywindow` `followup` `gmailsync` `ical` `inbox` `jobtracking` `mailbox` `mailclassify` `mailingest` `maillink` `mailmatch` `mailrecall` `mailtpl` `userjob` `viewlog`
