## 1. The slug rule (pure, no schema)

`normalize.CompanySlug` and `normalize.CompanyKey` ALREADY EXIST
(`internal/normalize/company.go`) and are called from exactly one place,
`cmd/harvest-orphans/candidates.go:47`. This group does not add them — it makes them the
module's single legal-form rule and closes the hole each existing implementation has.

- [x] 1.1 Fix `normalize.CompanySlug`'s tokenization: match the trailing form on the name's own
      words reduced to ASCII letters (`register.go`'s `letters` helper) instead of on `Slug`'s
      hyphenated output. Tests first, covering what each current implementation gets wrong:
      `Booking B.V.` → `booking` (today `booking-b-v`); `Acme GmbH & Co. KG` → `acme`;
      `Tiffany & Co.` → `tiffany`; and the cases that must NOT change — `Limited Brands` →
      `limited-brands`, `Limited` → `limited`, `Acme Holdings Ltd` → `acme-holdings`.
      Review caught a regression the first attempt introduced: splitting on WHITESPACE loses
      `Sun Technologies,Inc.`, which the old slug-level strip handled and which 13,730
      companies / 55,962 open jobs are written as. The word break is therefore every rune
      `Slug` drops EXCEPT `.` and `/`, which live inside the forms themselves (`B.V.`, `A/S`).
- [x] 1.2 Retire the `" a s"` / `" s a"` entries from the token set: field-level `letters()`
      makes them redundant (`Trafalgar A/S` → `as`). Test `Trafalgar A/S` → `trafalgar` before
      removing them, so the removal is proven inert rather than assumed.
      (Subsumed by 1.1: a token map of whole words cannot express a two-word entry, and
      `TestSameCompany`'s existing "Trafalgar A/S" case plus a new `CompanySlug` row prove it.)
- [x] 1.3 Make `collections.RegisterSlug` delegate to `normalize.CompanySlug`, deleting
      `collections.legalSuffixes`, `significantFields` and `letters`. `RequireCountry`'s
      token-counting must keep agreeing with the strip — it currently shares
      `significantFields`, so give it the shared helper rather than re-deriving. The existing
      `register_test.go` and `nlsponsor`/`uksponsor`/`ush1bsponsor` corpora must stay green.
      Done: `significantFields` SURVIVES, because RequireCountry's whitespace token count is a
      different concern from the strip's word breaks ("T-Mobile Inc" is one significant token,
      and CompanySlug's breaks would split it). Only the LIST was duplicated, so only the list
      is shared, via the new `normalize.IsLegalForm`. The union added `lp`, `cic`, `cio`,
      `incorporated`; each verified against prod — Bloomberg LP, Texas Instruments
      Incorporated and friends all land on the right employer, none on a different one.
      Two tests changed rather than deleted: `DoesNotStripCo` inverts to `StripsCo` on the
      catalogue evidence, and `Community Co CIC` now yields `community` because the strip
      repeats.
- [x] 1.4 Make `cmd/harvest-ats`'s `trimLegalForm` delegate too, deleting
      `legalFormSuffixes`. Note `-se` and `-group` are in that list and in no other; decide
      each on evidence and record the decision in the token set's comment.
      Decided on prod data: `se` and `aps` JOIN the shared list (Capgemini SE, SAP SE, Allianz
      SE are plain Societas Europaea). `spa` and `group` do NOT — "Hilton Luxor Resort & Spa"
      outweighs every real S.p.A. in the catalogue, and `group` is a brand component, not a
      form. Harvest keeps those two as `boardNameTails`, which is sound because it emits board
      GUESSES: an extra candidate costs a lookup, a merged employer costs a company.
- [x] 1.5 Add a guard test that the module defines exactly one legal-form token set — a grep-
      style test over the package sources, in the shape of
      `internal/db/folded_slug_rule_test.go`, because three lists is how this started.
      It was FOUR: the guard immediately found `internal/mailmatch`, whose six-token list meant
      mail from "Acme Limited" resolved to a name no company matched — and an unmatched mail
      links to no application silently. Unified too. Guard proven by planting a decoy list and
      watching it fail before removing it.
- [x] 1.6 Update the doc comments that are now wrong: `normalize.Slug`'s "deliberately does not
      strip legal suffixes … a noted future refinement" should point at `CompanySlug`, and
      `register.go`'s "Co is a deliberate omission" is contradicted by the catalogue evidence
      (297 companies on `-co`, all `& Co.`, zero bad collisions) and must not be carried over.

## 2. Schema and queries

- [x] 2.1 Add the `company_slug_aliases` migration (`alias_slug` PK, `canonical_slug`,
      `folded_key`, `reason`, `created_at`) with an index on `folded_key`. Number it **0110** —
      `0109` is already taken twice, so confirm against `migrations/` and the prod ledger before
      naming the file. Carry the design's rationale in the file comment, following 0109's
      precedent: why the table is not derived from `jobs`/`companies`, and why `reason` exists.
      It is **0112**. First numbered 0110 (taken), then 0111 — and by the time the branch was
      ready `origin/main` had landed `0111_onboarding_advanced_search_step.sql` too. The number
      has to be re-checked against a FRESHLY fetched `origin/main` immediately before the PR,
      not only when the file is created; checking once is exactly how the two existing 0109s
      happened. Verified against `git ls-tree origin/main`, every remote branch, and prod's
      `schema_migrations`. Also carries CHECKs against a self-alias and an unknown `reason`, and
      deliberately NO foreign key to `companies(slug)`, which would delete the rows the table
      exists to keep.
- [x] 2.2 Add the sqlc queries: batch lookup by `folded_key = ANY($1)`, single lookup by
      `alias_slug`, and the upsert the merge worker writes with. Run `make sqlc`.
- [x] 2.3 Add the chunked, `IS DISTINCT FROM`-guarded `jobs` re-key query, writing
      `company_slug` and `company_slug_folded` in one statement. Confirm
      `internal/db/folded_slug_rule_test.go` still passes — it counts the write paths, so the
      new one must raise the count, not slip past the detector.
      It DID slip past, and not for the expected reason: the guard tested the raw statement
      text, so the doc comment's own mention of `company_slug_folded` satisfied it while the
      column went unwritten. Fixed by stripping `--` comments before the check, proven by
      removing the column and watching it fail. Population minimum raised 4 -> 5.
      `jobs.company` is deliberately NOT rewritten: the source keeps sending "DollarTree", so
      the next crawl would put it straight back, and the display name comes from
      `companies.name` anyway.

## 3. Write path

- [x] 3.1 Switch `internal/jobderive/jobderive.go:183` to `normalize.CompanySlug`. Assert in a
      test that `jobderive.Derive` still takes no context and touches no database — purity is a
      requirement, not an accident. Purity is asserted on the SIGNATURE via reflection, so
      adding a ctx parameter fails the build's tests rather than a reviewer's attention.
- [x] 3.2 Resolve the alias registry once per board run in `pipeline.Runner`, over the distinct
      slugs `distinctCompanySlugs` already computes, and feed the resulting map to BOTH the
      coverage lookup and the upsert. Test that a posting whose folded slug matches a registered
      `folded_key` is stored under the `canonical_slug`. The canon rides on `job.Draft` rather
      than being set on a built Job: the aggregate stays constructed-once-never-mutated, and a
      setter would let any caller re-key a company. Wired in `cmd/ingest` on the existing
      `dbStore`, which already serves the read-side `SeenLookup`.
- [x] 3.3 Add the structural guard: the coverage gate and the upsert must read the slug from the
      same resolved map. A test that fails if either re-derives it independently — this is the
      failure mode the coverage-gate leak spike found, and a comment will not hold it.
      The guard earned itself immediately: 3.1 had ALREADY reopened the leak, and the first
      two tests missed it because their fixture company ("DollarTree") carries no corporate
      form, so Slug and CompanySlug agreed. `distinctSlugs` still derived with
      `normalize.Slug` while the posting keyed on `CompanySlug`, so every company with a
      form asked the lookup about "acme-inc" and then decided about "acme" — the gate
      suppressed nothing, silently. Fixed in both the buffered path and the CoverageGated
      probe, pinned by a test whose fixture DOES carry a form.
- [x] 3.4 Update `distinctCompanySlugs`'s doc comment: the invariant is now "one map, two
      consumers", not "both call the same pure function".

## 4. The merge worker

- [x] 4.1 Add `cmd/merge-companies` grouping by `normalize.CompanyKey(name)` over companies with at
      least one open job, electing the highest `job_count` variant. Unit-test the election
      against the real counterexamples: `dominos`(14396) beats `domino-s`(1);
      `alfa-bank`(1617) beats `al-fa-bank`(20).
- [x] 4.2 Make dry-run the default and `--apply` explicit, following `cmd/prune`'s shape. Test
      that a default run writes nothing to `jobs`, `companies` or `company_slug_aliases`.
- [x] 4.3 Add `--min-jobs N` wave bounding, and skip re-electing against a slug already present
      as a `canonical_slug` — the canon freezes at first merge.
- [x] 4.4 Wire the chunked re-key plus the alias insert, and test idempotence: a second run with
      the same bounds updates zero rows.
- [x] 4.5 Wire the worker through `internal/worker`'s `Main`/`Bootstrap` convention and its exit
      codes, like every other cron worker.
      `--limit` was dropped before it shipped: `--min-jobs` already bounds a wave, and a second
      overlapping cap would have taken an alphabetical prefix rather than the biggest groups.

## 5. Read path

- [x] 5.1 In `internal/handler/companies.go`, fall through a company miss to an `alias_slug`
      lookup and answer 301. An existing `companies` row wins over the registry, so a re-created
      company is never shadowed. Integration test: 301 on a merged slug, 404 on an unknown one,
      200 on a slug present in both.
- [x] 5.2 Propagate the redirect in `web/src/routes/companies/[slug]/+page.server.ts` instead of
      `error(404)`. The client must ask for `redirect: 'manual'`: left to the default, `fetch`
      FOLLOWS the 301 and returns 200, rendering the right company under the retired url —
      the one outcome a permanent redirect exists to prevent. `call` translates the 3xx into a
      typed `MovedError`, the same shape in which it already translates a non-ok into
      `ApiError`. Verified with `svelte-check` (0 errors) and eslint; web CI runs neither, so
      the worktree needed a real pnpm install to check it at all — which immediately caught a
      `noUncheckedIndexedAccess` violation in the new code.

## 6. Collections

- [x] 6.1 Re-key the hand lists (`AICompanySlugs`, `Mag7Slugs`, `BigTechSlugs`, `AINativeSlugs`)
      and `eastern_roots.txt` through `CompanySlug`.
- [x] 6.2 Add a guard test that every hand-list entry is already `CompanySlug`-stable, so a
      future entry written in the old spelling fails the build rather than silently matching
      nothing.
      The guard paid immediately. All three offending `eastern_roots.txt` entries were pointing
      at the WRONG company already: `epam-systems-pte-ltd` (19 jobs) instead of `epam-systems`
      (1,172), `headway-inc` (14) instead of `headway` (69), and `ex-corp` at a slug ingest will
      stop producing. It also exposed that `pte` was in NO list — without it EPAM's Singapore
      entity keys apart from EPAM itself.
      Scope grew by one thing the task did not name: `Collection.Members` slugged EDITORIAL
      records with `normalize.Slug` while the catalogue keys by `CompanySlug`, so an editorial
      dataset naming a company with a corporate form matched nothing. Both kinds now slug the
      same way; what still separates a credential is the ambiguity drop and the gates. Two tests
      encoding the old assumption were inverted rather than deleted.

## 7. Documentation

- [x] 7.1 Document the alias registry where it will be found: the company-slug rule, the frozen
      canon, and the fact that this is the one non-derived table in a derived neighbourhood.
      Update `internal/collections/AGENTS.md`'s note that editorial collections match on
      `normalize.Slug` — they now match on `CompanySlug`.
      Written as `docs/agents/company-identity.md`, following how mail-stack / notifications /
      job-lifecycle are documented, and linked from the module table plus a new Conventions
      bullet. Carries the gotchas that cost time in this change: electing by readability picks
      backwards, the word break is not whitespace, `significantFields` must stay a separate
      tokenization, and a fixture with no corporate form proves nothing.
- [x] 7.2 Add `merge-companies` to the worker list in `CLAUDE.md`, with the dry-run default, the
      wave flags, and the "no manual reindex" rule. Written to `AGENTS.md` — `CLAUDE.md` is a
      git-tracked symlink to it and the editor refuses to write through one.

## 8. Prod rollout (manual, after merge)

- [ ] 8.1 Confirm `reindex-companies` is actually rebuilding and not silently skipping — it has
      previously reported success while skipping for 14 days. If it is skipping, fix that first;
      otherwise `/companies` and the sitemap will not reflect any merge.
- [ ] 8.2 Deploy the migration alone, ahead of the code. `release.sh` carries a FIXED worker
      list — add `merge-companies` to it, or the binary never reaches the host.
- [ ] 8.3 Deploy the code. Verify behaviour is unchanged against the empty table, and that a
      freshly ingested `X, Inc.` posting now lands on `x`.
- [ ] 8.4 Wave 1: dry-run `--min-jobs 1000`, review the plan, then `--apply`.
- [ ] 8.5 Waves 2-4: `--min-jobs 100`, `10`, `1`, each dry-run first.
- [ ] 8.6 Let the scheduled `freehire-reindexw` refresh the facet index. Do NOT run a manual
      reindex and do NOT set `REINDEX_DEDUP`. Confirm afterwards that a merged company's job
      count matches Postgres.
