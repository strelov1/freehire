## 1. Feed evidence

- [x] 1.1 Add failing tests and implement validated Scalis and paginated Teamtailor feed observations.
- [x] 1.2 Add failing tests and implement canonical-ID, body and field-availability comparison with request-failure accounting.

## 2. CLI and verification

- [x] 2.1 Add failing tests and wire the read-only bounded audit mode, seed/catalog selection and JSON output.
- [x] 2.2 Document usage, run focused and repository checks, simplify and review the complete change.

## Verification

Repository unit tests and vet passed. The harvest package passed race tests and the lint ratchet; integration-tagged repository vet and documentation links also passed. CodeRabbit's one review raised a minor fixed-budget concern; the board deadline is now configurable and tested.

Live Tibber evidence matched all eight jobs and normalized bodies: one feed request versus eleven adapter requests. Scalis returned 91 location rows for 40 native jobs; its existing adapter repeated an ID, so the audit correctly reported an unverified baseline.

The full integration run was attempted and stopped after unrelated backfill packages repeatedly failed in testdb startup: PostgreSQL produced only the first of the two readiness messages before the container startup deadline. This run did not establish a passing full integration suite.
