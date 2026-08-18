## 1. The slug rule (pure, no schema)

- [ ] 1.1 Add `normalize.CompanySlug(name)` and `normalize.Fold(slug)` with the legal-form token
      set, moving `legalSuffixes`, `significantFields` and `letters` out of
      `internal/collections/register.go`. Tests first, covering: `RingCentral, Inc.` →
      `ringcentral`; `Booking B.V.` → `booking` (the strip is field-level, so it beats
      `normalize.Slug`'s `booking-b-v`); `Limited Brands` → `limited-brands`; `Limited` →
      `limited`; `Acme Holdings Ltd` → `acme-holdings` (one strip, no recursion).
- [ ] 1.2 Make `collections.RegisterSlug` delegate to `normalize.CompanySlug`, and add a test
      that the two agree on the same input. The existing `register_test.go` corpus must stay
      green — it is the validation this rule already earned.
- [ ] 1.3 Update `normalize.Slug`'s doc comment: the "noted future refinement" about legal
      suffixes now points at `CompanySlug` instead of describing an absence.

## 2. Schema and queries

- [ ] 2.1 Add the `company_slug_aliases` migration (`alias_slug` PK, `canonical_slug`,
      `folded_key`, `reason`, `created_at`) with an index on `folded_key`. Number it **0110** —
      `0109` is already taken twice, so confirm against `migrations/` and the prod ledger before
      naming the file. Carry the design's rationale in the file comment, following 0109's
      precedent: why the table is not derived from `jobs`/`companies`, and why `reason` exists.
- [ ] 2.2 Add the sqlc queries: batch lookup by `folded_key = ANY($1)`, single lookup by
      `alias_slug`, and the upsert the merge worker writes with. Run `make sqlc`.
- [ ] 2.3 Add the chunked, `IS DISTINCT FROM`-guarded `jobs` re-key query, writing
      `company_slug` and `company_slug_folded` in one statement. Confirm
      `internal/db/folded_slug_rule_test.go` still passes — it counts the write paths, so the
      new one must raise the count, not slip past the detector.

## 3. Write path

- [ ] 3.1 Switch `internal/jobderive/jobderive.go:183` to `normalize.CompanySlug`. Assert in a
      test that `jobderive.Derive` still takes no context and touches no database — purity is a
      requirement, not an accident.
- [ ] 3.2 Resolve the alias registry once per board run in `pipeline.Runner`, over the distinct
      slugs `distinctCompanySlugs` already computes, and feed the resulting map to BOTH the
      coverage lookup and the upsert. Test that a posting whose folded slug matches a registered
      `folded_key` is stored under the `canonical_slug`.
- [ ] 3.3 Add the structural guard: the coverage gate and the upsert must read the slug from the
      same resolved map. A test that fails if either re-derives it independently — this is the
      failure mode the coverage-gate leak spike found, and a comment will not hold it.
- [ ] 3.4 Update `distinctCompanySlugs`'s doc comment: the invariant is now "one map, two
      consumers", not "both call the same pure function".

## 4. The merge worker

- [ ] 4.1 Add `cmd/merge-companies` grouping by `Fold(CompanySlug(name))` over companies with at
      least one open job, electing the highest `job_count` variant. Unit-test the election
      against the real counterexamples: `dominos`(14396) beats `domino-s`(1);
      `alfa-bank`(1617) beats `al-fa-bank`(20).
- [ ] 4.2 Make dry-run the default and `--apply` explicit, following `cmd/prune`'s shape. Test
      that a default run writes nothing to `jobs`, `companies` or `company_slug_aliases`.
- [ ] 4.3 Add `--min-jobs N` wave bounding, and skip re-electing against a slug already present
      as a `canonical_slug` — the canon freezes at first merge.
- [ ] 4.4 Wire the chunked re-key plus the alias insert, and test idempotence: a second run with
      the same bounds updates zero rows.
- [ ] 4.5 Wire the worker through `internal/worker`'s `Main`/`Bootstrap` convention and its exit
      codes, like every other cron worker.

## 5. Read path

- [ ] 5.1 In `internal/handler/companies.go`, fall through a company miss to an `alias_slug`
      lookup and answer 301. An existing `companies` row wins over the registry, so a re-created
      company is never shadowed. Integration test: 301 on a merged slug, 404 on an unknown one,
      200 on a slug present in both.
- [ ] 5.2 Propagate the redirect in `web/src/routes/companies/[slug]/+page.server.ts` instead of
      `error(404)`.

## 6. Collections

- [ ] 6.1 Re-key the hand lists (`AICompanySlugs`, `Mag7Slugs`, `BigTechSlugs`, `AINativeSlugs`)
      and `eastern_roots.txt` through `CompanySlug`.
- [ ] 6.2 Add a guard test that every hand-list entry is already `CompanySlug`-stable, so a
      future entry written in the old spelling fails the build rather than silently matching
      nothing.

## 7. Documentation

- [ ] 7.1 Document the alias registry where it will be found: the company-slug rule, the frozen
      canon, and the fact that this is the one non-derived table in a derived neighbourhood.
      Update `internal/collections/AGENTS.md`'s note that editorial collections match on
      `normalize.Slug` — they now match on `CompanySlug`.
- [ ] 7.2 Add `merge-companies` to the worker list in `CLAUDE.md`, with the dry-run default, the
      wave flags, and the "no manual reindex" rule.

## 8. Prod rollout (manual, after merge)

- [ ] 8.1 Confirm `reindex-companies` is actually rebuilding and not silently skipping — it has
      previously reported success while skipping for 14 days. If it is skipping, fix that first;
      otherwise `/companies` and the sitemap will not reflect any merge.
- [ ] 8.2 Deploy the migration alone, ahead of the code.
- [ ] 8.3 Deploy the code. Verify behaviour is unchanged against the empty table, and that a
      freshly ingested `X, Inc.` posting now lands on `x`.
- [ ] 8.4 Wave 1: dry-run `--min-jobs 1000`, review the plan, then `--apply`.
- [ ] 8.5 Waves 2-4: `--min-jobs 100`, `10`, `1`, each dry-run first.
- [ ] 8.6 Let the scheduled `freehire-reindexw` refresh the facet index. Do NOT run a manual
      reindex and do NOT set `REINDEX_DEDUP`. Confirm afterwards that a merged company's job
      count matches Postgres.
