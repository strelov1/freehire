## 1. Residual-title miner

Whole-title clustering was built first and measured at 6.6% coverage — half the
unclassified mass has a title occurring exactly once. Tasks 1.1 and 1.2 are
reopened to cluster by word pair instead (15.2%, and a pair is directly usable as
a dictionary term). See the "Measured" section of `design.md`.

- [x] ~~1.1 Group by normalized whole title~~ — superseded by 1.4/1.5
- [x] ~~1.2 `cmd/mine-titles` over whole titles~~ — superseded by 1.6
- [x] 1.4 Add the stop-word list as a curated Go dictionary passed to the query as a parameter, guarded by a test that no stop word is a token of any `classify` non-tech term (a collision would silently hide that whole role family from mining). Tokenization stays in SQL — `[^[:alnum:]]+` is already Unicode-aware — and is covered by an integration case with accented Latin and Cyrillic titles
- [x] 1.5 Replace the query: expand each title into word pairs, drop pairs containing a stop word, a token under three characters, or a numeric token, and return each pair with its count of DISTINCT jobs and its sources
- [x] 1.6 Update `cmd/mine-titles` for the new row shape and re-verify the report renders and sorts
- [x] 1.7 Run it against prod read-only and record the top clusters in the change notes, confirming the 44/21/25/10 usable/dangerous/shrapnel/noise split the spike measured still holds

## 2. Ingest-time rejection

- [x] 2.1 Add the catalogue-fit predicate as a package-private helper in `internal/pipeline`, taking the constructed `job.Job` and reporting whether the posting is rejected by the non-tech title rule
- [x] 2.2 Add `Rejected` to the pipeline run stats, kept distinct from `Skipped`
- [x] 2.3 Wire the predicate between `normalizeJob` and `Store.Save` in the batch path (`internal/pipeline/pipeline.go:336`)
- [x] 2.4 Wire the same predicate in the stream path (`internal/pipeline/pipeline.go:476`)
- [x] 2.5 Log one line per board with a non-zero rejected count, including the rejected share of that board's postings
- [x] 2.6 Assert in tests that `cmd/tg-extract` and the other non-crawled write paths remain unfiltered

## 3. Deletion archive

- [x] 3.1 Add the migration creating `pruned_jobs(id, source, external_id, title, company_slug, rule, pruned_at)` with no description or enrichment column
- [x] 3.2 Add `PruneJobs`: archive and delete in ONE statement rather than an archive insert the delete could drift from. Absorbs 5.3 (duplicate-cluster extension) and 5.5 (archive rows), since separating them is what would let them diverge
- [x] 3.3 Note in the change that the migration must be applied to prod by hand before the worker first runs

## 4. Prune rule

- [x] 4.1 Add the company-evidence query: per `(source, company_slug)` over the entire history including closed jobs, whether any job ever had technical evidence and whether any ever had tagged skills
- [x] 4.2 Implement the pure rule predicate — title rule, non-tech category at a company without technical evidence, unknown at a company with no evidence at all — table-driven tested across bucket × `is_tech` × category
- [x] 4.3 Implement the non-crawled source exclusion as an ALLOW-list of providers built from the board files (`boards.crawled`), not a deny-list — a deny-list silently arms any non-crawled source added later, and that deletion is unrecoverable
- [x] 4.4 Build the guard's ingredients: `boards.retired(provider, slug)` keyed the way the catalogue is, and `companyScoped(rule)`
- [x] 4.5 WIRE the guard in the worker (group 5 must not skip this — 4.4 is ingredients only). The refusal is per candidate and runs BEFORE the delete, not as a post-hoc report; `loadBoards` must be called before `worker.Bootstrap` so an unreadable directory kills the run before anything is removed. Aggregator providers whose board entry is a region rather than an employer (`trudvsem` and the other multi-employer sources) must be excluded from the company-scoped rules outright — their employers are never "listed" and would read as retired while the board is still crawled
- [x] 4.6 Assert `len(ids) == len(rules)` in the `PruneJobs` wrapper: the query inner-joins on ordinality, so a caller off-by-one silently under-deletes with no error

## 5. Prune worker

- [x] 5.1 Add `cmd/prune` skeleton: keyset scan over candidate rows, `--dry-run` default, `--apply`, `--limit`, `--sample` flags. The scan MUST include closed rows: once ingest rejects a board's non-tech postings, the 48h unseen sweep closes the ones already in the catalogue, and a scan restricted to open jobs would leave them as permanent dead weight
- [x] 5.2 Compute company evidence once at start, before any deletion
- [x] ~~5.3 Batched delete extending to the duplicate cluster~~ — done by `PruneJobs` in 3.2
- [x] 5.4 Mirror each batch to the facet index via `search.Client.DeleteJobs`
- [x] ~~5.5 Write archive rows for every deleted job~~ — done by `PruneJobs` in 3.2
- [x] 5.6 Dry-run output: random sample of pending titles plus counts broken down by rule and by source
- [x] 5.7 Honour `--limit` as a hard cap and report how many rows matched versus were deleted

## 6. Board-retirement report

- [x] 6.1 Add `cmd/prune --boards`: read the `sources/*.yml` entries, slugify each `company` with the same normalization ingest uses, and list the entries whose company has no technical evidence
- [x] 6.2 Test the slug matching against a board file fixture, including an entry whose company name differs in case and punctuation

## 7. First dictionary iteration

- [x] 7.1 Add anchored terms for the behavior-technician cluster to `classify.nonTechTitleTerms`, each with a positive test and a negative test naming a real technical title
- [x] 7.2 Add anchored terms for maintenance technician, machine operator, crew member and car rental. `service technician` and `field service` are deliberately DEFERRED — both are ambiguous for hardware and IT field roles, and the first iteration should not spend its credibility on the two most arguable terms in the batch
- [x] 7.3 Add anchored terms for the medical-speciality cluster (behavioral health, speech-language pathologist, therapy assistant, care aide/assistant, social worker), same test discipline
- [x] 7.4 Verify no added term matches any title in a fixture of real technical titles sampled from prod

## 8. Documentation

- [x] 8.1 Update `docs/agents/job-lifecycle.md` and `CLAUDE.md`: closing remains soft, with the catalogue-pruning hard delete as the stated exception
- [x] 8.2 Add `internal/pipeline/AGENTS.md` notes for the rejection path and the `Rejected` counter, the hydrating-source asymmetry (a `SeenRefresh` posting is touched and reopened without meeting the filter), and the per-crawl re-hydration cost a pruned posting incurs on hydrating adapters
- [x] 8.4 Note in `openspec/specs/ingest-status-page` terms that `ingested_total` now counts post-rejection saves, so the published number steps down when the filter deploys
- [x] 8.3 Document the iteration loop and the end-of-campaign `backfill-derive` plus `reindex` step
