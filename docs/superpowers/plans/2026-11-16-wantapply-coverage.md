# WantApply catalogue recovery

**Goal:** Preserve WantApply's original inventory and crawl the global and Cyprus catalogues independently.

**Evidence:** The 2026-11-16 production run enumerated 2,385 candidates, dropped 272 unreadable pages, and discarded 1,514 of 2,113 readable postings because their company had another source. The global and Cyprus sitemaps currently enumerate approximately 2,355 and 462 vacancy pages respectively, plus three static pages each. They are regional catalogues, not interchangeable mirrors.

**Approach:** Keep the existing aggregator classification and posting-level deduplication. Add an original-postings capability that exempts WantApply from company-wide ingest suppression. Make `wantapply` always crawl `.com`; register `wantapply-cy` for `.cy`. Preserve existing global posting identities. Keep Scrapling browser egress for `.com` and proxy egress for `.cy`.

- [x] Add and run failing regression tests for original postings surviving company coverage and independent domain construction.
- [x] Implement the source capability, regional registration, and static-page exclusions; correct active documentation and regenerate contracts.
- [ ] Run focused tests, Go vet and unit tests, integration-tag compilation, and review the diff.
- [ ] Build the corrected worker on the crawl host from the reviewed patch, add the Cyprus board, run bounded recovery, and verify database/search counts. Preserve the normal release binary and scheduler state.

**Scope:** No global change to reseller coverage policy, no full search rebuild, and no applied migration edits. Source-level counts remain lower than raw site counts where posting-level deduplication or catalogue eligibility applies.
