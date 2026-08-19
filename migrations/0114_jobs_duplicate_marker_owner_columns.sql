-- One duplicate-marker column per dedup pass. Until now all three passes wrote
-- jobs.duplicate_of (0012), and the first of them — RecomputeRoleDuplicatesForCompanies —
-- recomputes it from scratch over role_fingerprint clusters, writing NULL to every row that is
-- a canon or a singleton in its own cluster. That includes rows the aggregator-suppression and
-- fuzzy passes had marked for entirely different reasons, which they then re-apply later in the
-- same run. Measured on prod over 2026-08-16..19, six cycles a day: the role pass re-marks
-- ~460-495k rows, of which ~470k is exactly the aggregator (125k) and fuzzy (345k) populations
-- it had just cleared. Roughly 5k per cycle is real work.
--
-- The cost is not only churn. Between the clearing pass and the passes that repair it — about
-- an hour, six times a day — the catalogue holds hundreds of thousands of duplicates as
-- canonical, and a facet rebuild that scans during that window indexes them. On 2026-08-19 the
-- role pass cleared at 02:13 and the fuzzy pass restored 344,919 markers at 03:09.
--
-- With one column per pass, a pass can only ever clear a marker it set itself.
-- jobs.duplicate_of becomes derived from these three (0115) and keeps its exact current
-- meaning, so every reader — job search, the facet index claim, the semantic outbox,
-- enrichment, pruning, cluster copies, and the partial indexes in 0012/0042/0107 — is
-- untouched.
--
-- NO FOREIGN KEY TO jobs, deliberately, unlike duplicate_of itself. These columns are
-- pass-internal state; the invariant that a duplicate points at a real job is enforced where it
-- is read, on duplicate_of. A foreign key here would instead give cmd/prune a new way to fail:
-- a row can carry an owned marker that the derivation does NOT surface (an aggregator verdict
-- outranking a role one), so prune's walk down duplicate_of would not collect it, and deleting
-- the row it points at would be blocked by a constraint on a value nothing reads. Without the
-- key that case degrades to a stale pointer in a pass-owned column, which the next run of that
-- pass overwrites. Same reasoning as 0113's deliberate lack of a key.
--
-- Adding three nullable columns is a catalog-only change: no table rewrite, no lock worth
-- planning around on the 7.4M-row table. The backfill that seeds them
-- (cmd/backfill-duplicate-marker-owner) runs BETWEEN this migration and 0115, because a
-- derivation over three empty columns would clear every marker in the catalogue.
ALTER TABLE public.jobs
    ADD COLUMN duplicate_of_aggregator bigint,
    ADD COLUMN duplicate_of_role       bigint,
    ADD COLUMN duplicate_of_fuzzy      bigint;
