## A. Expand schema and move all readers

- [x] A1. Add activity schema, bounded restartable backfill, insert invariant and one-way legacy bridge; prove insertion, concurrent backfill and timestamp/strike semantics on PostgreSQL 18.
- [x] A2. Add explicit `jobs_read` projection and sqlc model naming; migrate all complete-job readers and writer return adapters without public API changes.
- [x] A3. Migrate observation predicates, candidate scans and closers to activity; prove locked freshness rechecks and safe lock ordering.
- [x] A4. Add reader/view completeness guards, update fixtures and documentation, and validate representative read plans.
- [ ] A5. Run unit/full integration/build/vet/lint/sqlc checks, simplify and independent review; publish and deploy additive release A; verify backfill and drain pre-A readers.

- [ ] A6. Ship the compatible observation lock-order follow-up and drain unsafe initial-A ingest writers before the activity-only cutover.

## B. Switch ordinary writers to compact activity

- [ ] B1. Add state-change fencing covering lifecycle and the complete cheap match key, with concurrent lifecycle/content regression tests.
- [ ] B2. Move unchanged refresh, live touch and covered refresh to activity-only writes; prove main-row xmin/xmax/ctid stability.
- [ ] B3. Release fenced cheap transactions before full fallback; preserve form/enrichment/search side effects and full upsert/reopen semantics.
- [ ] B4. Move strike and hydration writers; remove every legacy-column reference from runtime SQL, including generated full-row returns.
- [ ] B5. Run required checks and independent review; publish/deploy B only after the reader gate; verify running binaries and observation costs.

## C. Contract and measure

- [ ] C1. Verify repository and operations consumers use activity; drain every legacy writer and verify current SQL does not name the old columns.
- [ ] C2. Add a new cleanup migration removing the bridge and old columns while preserving the read projection and insertion invariant.
- [ ] C3. Run required checks and review; deploy cleanup under the documented lock/worker constraints.
- [ ] C4. Measure tuple/WAL/read performance, record limitations, finish runtime verification and archive/sync this OpenSpec change.

The writer implementation and its verification are preserved in #4545. It is temporarily deferred from the reader-repair release: pre-reader processes still consume the legacy columns. Reintroduce that patch only after A5/A6 and the external-reader, binary-drain and read-cost gates pass.
