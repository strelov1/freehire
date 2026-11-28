# Tasks

- [x] Add consent, weekly delivery ledger and activity queries with integration checks.
- [x] Add weekly summary renderer and delivery runner with behavioral tests.
- [x] Expose consent in notification settings and test settings UI.
- [x] Wire existing notify worker and inbound notification guards.
- [x] Verify contracts, formatting, Go and relevant frontend checks; review full change.

Runtime database integration checks passed during release preparation, including the real-store weekly delivery scenario.

Validation: full Go unit suite, ordinary vet, ten settings UI tests, contract/sqlc generation, migration numbering and formatting passed during release preparation. PostgreSQL integration checks cover consent, ledger selection, weekly claims, API settings and the real-store delivery scenario. Integration-tag compilation is also checked before push.
