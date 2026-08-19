## Context

`jobs.duplicate_of` has one meaning and five writers. Three are the batch passes driven by
`cmd/reindex`'s `refreshDuplicateMarkers` — `RecomputeRoleDuplicatesForCompanies`,
`SuppressAggregatorDuplicatesForCompanies`, `MarkFuzzyDuplicatesForCompany`. Two are
ingest-time: `cmd/ingest/store.go` and `internal/linkimport/linkimport.go`, both resolving a
canon through `jobdedup.CanonicalForRole` and therefore acting as an early write of the same
verdict the role pass would reach.

The role pass recomputes the column from scratch over `role_fingerprint` clusters. Its
`target` CTE selects every open row of the batch's companies, so a row suppressed by the
aggregator pass or collapsed by the fuzzy pass is in scope, and if that row is a canon or a
singleton in its own role cluster the pass writes NULL over the other pass's verdict. The
other two passes restore their markers later in the same run. `proposal.md` carries the prod
measurements: ~950k rewritten rows per cycle, six cycles a day, and a window of roughly 1.5
hours per cycle in which those duplicates are canonical as far as the database is concerned.

Two constraints shape everything below:

- **`duplicate_of` must stay a real, materialized column.** It appears in the predicates of
  partial indexes — `jobs_duplicate_of_idx` (migration 0012), the normalized-URL index
  (0042), the sitemap index (0107, `WHERE closed_at IS NULL AND duplicate_of IS NULL`). A
  view or a bare expression cannot carry those.
- **Postgres cannot convert an existing column into a generated one.** `ALTER TABLE ... ALTER
  COLUMN ... SET GENERATED` covers identity columns only. Adding a `GENERATED ... STORED`
  column is possible but rewrites the table — 7.4M rows under `ACCESS EXCLUSIVE`, which this
  change will not do on prod.

So the derived column of `proposal.md` has to be derived by something other than the
generated-column feature.

## Goals / Non-Goals

**Goals:**

- No pass can clear a marker it did not set.
- A refresh over an unchanged catalogue writes zero rows.
- The end state does not depend on pass order, so an interrupted or failed run cannot leave
  duplicates unmarked.
- Every current reader of `duplicate_of` keeps working untouched, including the partial
  indexes.
- The migration takes no long lock on `jobs`.

**Non-Goals:**

- Enqueuing marker changes onto `search_outbox` / `search_delete_outbox`. That is the change
  this one unblocks, not part of it.
- Changing what any pass decides. Matching rules, thresholds, and over-merge guards stay
  exactly as they are; only where the verdict is stored changes.
- Retiring the scheduled full rebuild, or touching any timer.
- Dropping pass ordering. It stays as a cost and merge-quality rule.

## Decisions

### Decision 1: A trigger derives `duplicate_of`, not a generated column and not each writer

`duplicate_of` stays a plain nullable `bigint`, maintained by a `BEFORE INSERT OR UPDATE`
trigger on `jobs` that sets it to `COALESCE(duplicate_of_role, duplicate_of_aggregator,
duplicate_of_fuzzy)`.

*Alternative — generated column:* the honest expression of the intent, and unavailable
without a full table rewrite (see Context). Rejected on lock cost, not on design.

*Alternative — every writer computes the COALESCE itself:* no trigger overhead and nothing
hidden, but the guarantee then rests on five current call sites and every future one
remembering. The whole defect being fixed is one writer reaching outside its lane; a rule
enforced by discipline is the same class of thing that produced it. Rejected.

The trigger is pure PL/pgSQL over `NEW`, issues no query, and returns early when none of the
three owned columns changed, so the cost on the ingest path is a few microseconds per
modified row. Against that, the change removes ~5.7M pointless row writes a day.

### Decision 2: Precedence is aggregator, then role, then fuzzy

`COALESCE(duplicate_of_aggregator, duplicate_of_role, duplicate_of_fuzzy)`.

This decision originally read role-first, on the reasoning that the passes are disjoint and
precedence is a formality. Task 1.1 measured prod and both halves of that reasoning were
wrong.

**The passes overlap.** Of 1,162,487 open marked rows, 8,279 are both aggregator-shaped and
share a `role_fingerprint` with their canon — 0.7%, small but not zero. Precedence decides
what those rows point at.

**Today the aggregator pass wins that overlap**, and the ordering is not incidental. Its
candidate set is documented as "canonical OR already pointing at a non-aggregator row", so a
row the role pass pointed at a non-aggregator canon is deliberately re-decided by the
aggregator pass, while a row the role pass pointed at another aggregator is deliberately left
alone. Role-first `COALESCE` would have inverted the first case for up to 8,279 rows —
a silent behavior change smuggled in under a refactor.

Aggregator-first reproduces both cases exactly. The row the role pass pointed at another
aggregator never enters the aggregator pass's candidate set, so its aggregator column stays
NULL and `COALESCE` falls through to role — the same answer either order would give. The row
pointed at a non-aggregator canon does enter it, and aggregator-first keeps the aggregator's
verdict, which is what happens today.

The "deterministic first" rationale survives the flip: aggregator suppression matches on
exact normalized title plus compatible country, which is no less deterministic than role
clustering. Fuzzy is the only heuristic pass, and it is last in either ordering — it is
scoped to rows the other two left canonical, so it can never contend.

### Decision 3: Ingest-time writes belong to `duplicate_of_role`

Both `cmd/ingest/store.go` and `internal/linkimport/linkimport.go` resolve their canon via
`jobdedup.CanonicalForRole`. They are the role verdict arriving early — the same clustering,
minutes after the posting lands instead of hours later in the batch. `MarkJobDuplicateOf`
becomes `MarkJobDuplicateOfRole` and writes the role column. No fourth owner is introduced.

### Decision 4: Seed the backfill by shape, and let the first refresh converge

Provenance cannot be recovered from a single stored value: a marked row does not say which
pass marked it. The backfill therefore seeds by shape — a marked row whose own source is an
aggregator and whose canon's source is not gets `duplicate_of_aggregator`; every other marked
row gets `duplicate_of_role`.

Fuzzy markers are indistinguishable from role markers this way and will land in the role
column. That is self-correcting: the first role recompute clears the ones that are not role
clusters, and the fuzzy pass re-sets them in its own column during the same run. The cost is
exactly one more cycle of the churn this change removes, once.

*Alternative — seed nothing and let the first refresh build all three columns:* leaves the
whole catalogue un-deduplicated between the migration and the first refresh, which is a
flood of duplicates into search. Rejected.

### Decision 5: Migration order is columns, backfill, trigger, reconcile, then code

The trigger cannot exist before the backfill: it would derive `duplicate_of` from three empty
columns and clear the marker on every row ingest touches.

1. `ALTER TABLE jobs ADD COLUMN ... bigint` three times — catalog-only, no rewrite, no
   meaningful lock.
2. Chunked keyset backfill over `id`, `IS DISTINCT FROM`-guarded and resumable, in the shape
   of `cmd/backfill-slug-folded`. Interrupting it is free; re-running it writes nothing.
3. Create the trigger.
4. One reconcile sweep for rows written between the backfill passing their chunk and the
   trigger existing: `duplicate_of IS NOT NULL` with all three owned columns NULL. Same
   seeding rule, idempotent.
5. Deploy the code that writes owned columns.

Steps 1-4 are safe with the current binary running: until step 5 nothing reads or writes the
new columns except the backfill, and the trigger's derivation reproduces exactly what the old
code already stores.

### Decision 6: No indexes on the owned columns yet

Each pass filters by company and by its own matching criteria, then guards per row with `IS
DISTINCT FROM` on rows it has already selected. Nothing scans by an owned column. The seam is
noted rather than built; if a future consumer wants "everything the aggregator pass
suppressed", it gets an index then.

### Decision 7: No foreign key on the owned columns

`duplicate_of` references `jobs(id)`; the three owned columns do not.

The asymmetry is deliberate and mirrors 0113's. A row can carry an owned marker the derivation
does not surface — an aggregator verdict outranking a role one leaves `duplicate_of_role`
pointing somewhere nothing reads. `cmd/prune` collects a duplicate cluster by walking
`duplicate_of`, so it would not collect that row, and deleting the job it points at would then
fail on a constraint protecting a value no reader consults. Without the key the same case
degrades to a stale pointer in a pass-owned column, which that pass overwrites on its next run.

The invariant that matters — a duplicate points at a real job — is enforced where it is read,
on `duplicate_of`, which keeps its key.

## Risks / Trade-offs

- **A trigger on the hottest table in the schema** → Pure `NEW`-local PL/pgSQL, no query, and
  an early return when no owned column changed. Benchmark the ingest upsert path before and
  after on the integration harness; the change also deletes far more write volume than the
  trigger adds.
- **The trigger silently overrides a direct write to `duplicate_of`** → Intended, and the
  reason a direct write cannot leave the schema inconsistent. The failure mode is a future
  writer setting the column and wondering why it does not stick, so all current writers are
  converted in this change and a test walks `internal/db/queries/*.sql` asserting no
  statement assigns `duplicate_of` outside the trigger.
- **Seeding puts fuzzy markers in the role column** → Self-correcting within one refresh
  cycle (Decision 4), and the correction is bounded and observable: the re-marked counts in
  the first cycle after deploy should look like today's, and the cycle after that should be
  near zero.
- **Precedence changes behavior for rows marked by two passes** → Expected population is
  zero; Task 1 measures it on prod before the migration is written. If it is not zero, the
  count and the shape of those rows are a finding in their own right and the precedence
  decision gets revisited with data.
- **`updated_at` stops moving for rows that used to churn** → That is the point, but anything
  that inadvertently relied on the churn to be re-examined — chiefly `reindex --since` — needs
  checking, since it will now see roughly a third fewer changed rows per cycle. This is a
  correctness improvement for `--since`, not a regression, but it changes its working set
  enough to be worth confirming rather than assuming.

## Migration Plan

Deploy is ordinary: migrations first, then code, per the repo's standing rule that
`cmd/migrate` runs before code that reads new schema.

The backfill runs as a one-off worker in the shape of `cmd/backfill-slug-folded` — chunked,
paced, idempotent, resumable, needing only `DATABASE_URL`. On prod it must be launched under
`systemd-run`, not a bare ssh command: a long worker run does not survive the session ending,
and an interrupted run here is safe only because the chunk statement is guarded.

**Rollback:** before step 5, dropping the trigger and the three columns restores the previous
behavior exactly — `duplicate_of` still holds the values it held. After step 5, rollback is a
code revert plus dropping the trigger; the owned columns can stay, since `duplicate_of` keeps
whatever the last derivation wrote and the old code writes it directly again.

**Acceptance:** the prod baseline is in `proposal.md` — 293k-495k / 120k-128k / 305k-348k rows
re-marked per pass per run, stable over three days. One full refresh cycle after deploy
absorbs the seeding correction; every cycle after that must report near-zero re-marked rows
for an unchanged catalogue. That number is the test, and it is already being logged.

## Open Questions

- ~~What is the current population of rows that would carry a marker in more than one owned
  column?~~ **Answered 2026-08-19: 8,279 of 1,162,487 open marked rows (0.7%).** Precedence
  is a real rule; Decision 2 was rewritten because of it. A further 28,201 marked rows are
  neither aggregator-shaped nor sharing a fingerprint with their canon — the backfill seeds
  those into the role column, and the first refresh re-decides them. What they are is worth
  a look, but not a blocker: whatever pass owns them will claim them on its next run.
- Does anything outside the three passes and the two ingest sites write `duplicate_of` today
  through a path the query grep would miss — a manual admin action, a moderation tool, a
  one-off script kept outside the repo?
- Should the reconcile sweep of step 4 stay in the tree as a permanent consistency check
  (rows where `duplicate_of` disagrees with the derivation), or is it one-off scaffolding to
  be deleted with the change?
