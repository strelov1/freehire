## Context

The full measured background — prod table sizes, the TOAST bloat arithmetic, the
`pg_stat_user_tables` counters and the ruled-out I/O hypothesis — lives in
`docs/superpowers/specs/2026-08-02-ingest-write-amplification-design.md`. This document
covers only the technical decisions.

Two facts constrain every option below:

- The write path already knows whether a re-ingest changed anything. `UpsertJob` returns
  `changed` by comparing the incoming `content_hash` against the pre-update snapshot, and
  `cmd/ingest/store.go` already reads it to skip the Meili push. The signal exists; only
  Postgres does not act on it.
- The "refresh liveness without rewriting content" operation already exists as `TouchJob`,
  built for hydrating sources so a re-listed offer keeps its hydrated description. The seam
  this change needs is not new, it is unreached from the main path.

## Goals / Non-Goals

**Goals:**

- An ingest pass over a board of unchanged postings writes one narrow, HOT-eligible `UPDATE`
  per row and nothing to `companies`.
- `updated_at` comes to mean "content last changed" rather than "last crawled", which the
  jobs sitemap's `<lastmod>` and the public wire field both serve, and which is the
  precondition for any incremental reindex scoped by that column.
- Every observable behaviour is preserved: the 48h unseen sweep, the reopen, the incremental
  index push, the dedup marking, the enrichment enqueue.

**Non-Goals:**

- Reclaiming the ~43 GB already accreted (`pg_repack`). A repack before the write path is
  fixed just re-bloats; it is the follow-on change, gated on this one's measured effect.
- Dropping the two ~1 GB indexes with near-zero scans, purging `semantic_embedding`, tuning
  autovacuum workers, anything about Meilisearch's index size.

## Decisions

### The match key is `content_hash` AND `cities`, not the hash alone

The obvious key is the content fingerprint, and it is not sufficient. `UpsertJobParams`
carries one column the upsert writes that `jobhash.Of` does not read: `cities`. A caller's
structured city list overrides the location-derived one (`jobderive.Derive`), so it can move
while every fingerprinted field stands still — and the cheap path would then skip a row whose
`cities` really did change.

Folding `cities` into `Of` was the alternative and is disqualified by its one-time cost:
every stored `content_hash` would change at once, so the first crawl after deploy would
report `changed` for all 5.6M rows, rewriting each and re-pushing the whole catalogue to
Meilisearch — the write storm this change exists to remove, plus it would drown the
before/after measurement the rollout depends on. Carrying `cities` in the predicate costs one
array comparison on a row already fetched by index, no migration, and no invalidation.

`TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites` (`internal/job`) is the
authority here: it walks `jobderive.Input` through the real composition and fails if any
mutation moves a written column without moving the key. A derived column added later outside
the fingerprint fails there, and the fix is the same fork — hash it, or widen the key.

The ingest path does not currently supply structured cities (`normalizeJob` leaves
`Input.Cities` empty), so the hole is unreachable today. That is an accident of one
unpopulated field, not a design property, which is exactly why the key carries `cities`
rather than a comment saying it need not.

### The unchanged test lives in SQL, not in Go

`RefreshUnchangedJob` matches on `(source, external_id, content_hash, cities)` and returns
four narrow columns, or nothing. The caller branches on `pgx.ErrNoRows` — which is also what
a brand-new posting yields, and correctly so: both cases want the same statement.

*Alternative considered:* read the stored hash into Go first, then choose a statement. That
costs a guaranteed extra round trip on the hot path and opens a window between the read and
the write. *Alternative considered:* extend the per-board seen-set (`ExistingExternalIDs`,
already one query per board returning `external_id → is_tech`) to carry `content_hash`, so
the pipeline could decide before writing. Attractive — it is a bulk read rather than a
per-row one — but the seen-set is consulted only by hydrating sources, so it would leave
most providers unchanged while adding a second decision site. Rejected as the wrong shape
for the first cut; the seam stays available.

The chosen form costs **one** statement on the common path, the same as today. Only the
rarer changed path pays two.

### The cheap branch returns four columns, not the row

The first shape tried was `RETURNING sqlc.embed(jobs)`, so the helper could hand `save` a
`db.UpsertJobRow` on either branch and leave the rest of the function untouched. That is the
tidier Go and the wrong database call: embedding the row detoasts the ~2.5 KB `description`
and ships `semantic_embedding` back for every re-seen posting, which is read amplification
added to the path built to remove write amplification — on the host whose disk is already the
constraint.

What `save` actually reads on this branch is four columns: `id` (the enrichment enqueue and
the apply-form capture queue), `source` and `company_slug` (the crawled-set that scopes the
post-run sweep), and `duplicate_of` (the index-push gate). The fuller row is only ever needed
to BUILD a search document, and this branch never does — `needsIndex` is false by
construction. `TouchJob`, the hydrating-source sibling, returns `company_slug` alone for
exactly this reason.

The cost is that `save` cannot treat both branches as one type. Synthesising a partial
`db.Job` was rejected outright: a later reader touching `saved.Job.Title` on the cheap branch
would silently get `""`. Task 3.1 therefore introduces an explicit return type whose zero
values cannot be mistaken for data.

What does stay uniform is the gating: `clustersByRole` and `needsIndex` are already keyed on
`Inserted`/`Changed`, both false on the cheap branch, so the dedup lookup and the index push
are skipped by logic that already exists rather than a new branch to keep in sync. The
apply-form write and the crawled-set record sit after the seam and run on both branches, as
they must.

### `closed_at IS NULL` is a correctness predicate, not an optimisation

Without it, a posting that had been closed and reappears on the board with identical content
would have its liveness refreshed and stay closed — the sweep would then never reopen it,
because it is being seen. Excluding closed rows sends them to `UpsertJob`, which reopens.

### The enrichment enqueue stays on the cheap path

`EnqueueJobEnrichment` is already gated (`enriched_at IS NULL OR enrichment_version <
target`) and idempotent on `UNIQUE (job_id, target_version)`. Keeping it preserves today's
behaviour that a job which never got enriched is re-offered on every crawl. Dropping it
would save a lookup per row but would strand such a job until a manual backfill — a worse
trade than the lookup costs.

### Skipping derived columns is safe by subset, and the subset is tested

`RoleFingerprint` reads `company_slug`, `title`, `description`; all three are inputs to
`jobhash.Of`. Equal `content_hash` therefore implies equal `role_fingerprint`. The same
argument covers the deterministic facets — with the dictionary caveat in the next section.
This is an invariant between two functions that can drift, so it becomes a test in
`internal/jobhash` rather than a comment — the package already has precedent for exactly
this (`TestOfRow_CarriesEveryFieldTheHashReads` exists because the same mapping had already
drifted once).

### A dictionary change no longer rides in on the next crawl

`UpsertJobParams` carries 26 fields; `jobhash.Of` reads 19. Three derived columns the upsert
writes are outside the **content hash**: `cities`, `is_tech`, `english_level`. Their own
inputs are all hashed — `cities` from `location`, `is_tech` from `category` and `title`,
`english_level` from `description` — so equal `content_hash` implies equal derived values
**given the same dictionary**. The dictionary is an implicit input the content hash cannot
see.

So a dictionary edit (a city added to the GeoNames set, a term added to the non-tech set)
stops propagating to unchanged rows as a side effect of re-crawling them. This is a real
change to an operational contract and is accepted rather than worked around: including a
dictionary version in the hash would invalidate the whole catalogue on every dictionary
edit, which is the opposite of what this change is for.

The same applies to a change in a derivation's own code, not only its dictionary — a new
normalization rule in `RoleFingerprint`, say. That is the sharper case: unlike adding a field
to `jobhash.Of`, it does not move any stored hash, so it would never propagate through a
crawl again.

The reconciler already exists and is the documented mechanism — `cmd/backfill-derive`
re-derives these three in one keyset pass, and already compares before writing, so it is
cheap on a catalogue where little moved. What changes is that running it after a dictionary
or derivation edit becomes required rather than merely faster than waiting.

Read its own doc comment before making that routine: the pass re-derives **every**
deterministic column — all thirteen facets, the role fingerprint and both slugs — and in
doing so overwrites structured-source facets an adapter supplied and blanks moderator-stated
`regions`/`cities` on hand-authored rows. That blast radius is unchanged by this change, but
promoting the command from occasional to routine makes it newly relevant.

### Two pre-existing gaps this change leans on without widening

Neither is introduced here and neither is fixed here; both are recorded because the design
now depends on the paths they sit on.

- **A reopen with unchanged content reaches no live index push.** `UpsertJob` reports
  `changed = false` (the stored hash equals the incoming one) and `inserted = false`, so the
  reopened row stays absent from the live index until the next full rebuild. The behaviour is
  identical before and after this change, but `closed_at IS NULL` is now described as a
  correctness predicate, which makes the reopen path more load-bearing than it was.
- **A `cities`-only drift reaches no live index push either**, for the same reason — the hash
  did not move — even though `cities` is part of the search document. Also identical before
  and after: the old code ran `UpsertJob`, wrote the new `cities`, and reported
  `changed = false`. Unreachable from ingest today in any case, since `normalizeJob` supplies
  no structured cities.

### Only `UpsertJob`'s company CTE is guarded

Three queries carry a `company_upsert` CTE. `UpsertManualJob` and `UpdateManualJob` fire
once per moderator action; guarding them would be churn without measurable value. The guard
goes on `UpsertJob`'s alone, which is the one running millions of times per day.

## Risks / Trade-offs

**The cheap-path hit rate is unmeasured and may be low for some providers** → `jobhash.Of`
covers 19 fields, and a provider only has to vary one per crawl — a session token in the
URL, a re-serialized `posted_at`, unstable whitespace in `location` — for its rows to keep
taking the full path forever. The failure is silent, the same shape as `ingested=0 failed=0`
on a dead board. Mitigation: per-provider hit-rate logging is in scope (not deferred), and
the first post-release check is that distribution rather than the global counters. A
provider at ~0% is a finding worth acting on at the adapter, since the same churn has also
been forcing a pointless index push every crawl.

**The `companies` guard saves regardless, which could mask a failed `jobs` fix** → the two
effects must be read separately in `pg_stat_user_tables`, not as one aggregate number. The
verification step names both tables for this reason.

**`fillfactor = 90` does nothing to existing pages** → they stay packed at 100% until the
deferred repack, so the HOT share improves gradually rather than at once. This is expected,
not a defect; it means the 24h measurement will understate the eventual steady state.

**The storage-parameter migration takes an ACCESS EXCLUSIVE lock** → no table rewrite, but
it will queue behind a long-running read and block every reader behind it, the same
mechanism that has bitten `ADD CONSTRAINT` here before. Mitigation: its own migration file,
deployed separately with a `lock_timeout`, retried rather than waited out.

**Falling back on `ErrNoRows` conflates "changed" with "absent"** → a brand-new posting also
returns no rows, and correctly falls through to `UpsertJob`, which inserts. The conflation is
harmless because both cases want the same statement; noted so a future reader does not
"fix" it into a separate existence check.

## Migration Plan

1. Snapshot `pg_stat_user_tables` for `jobs` and `companies` on prod before the release.
2. Release the code change. It is backward-compatible: no schema dependency, and a rollback
   is a plain revert with no data migration.
3. ~~Deploy the storage-parameter migration on its own, with `lock_timeout`.~~ **Corrected by
   the actual release, 2026-08-02:** `release.sh` runs the migrations itself as one of its
   steps, so 0073 went out with the code rather than separately. It applied cleanly
   (`migrate: applied 0073_jobs_write_storage_params.sql`, prod `reloptions` now
   `fillfactor=90, autovacuum_vacuum_scale_factor=0.02`) because `internal/migrate` sets
   `lock_timeout` on its own connection — which is the whole protection this step was asking
   for. Separating it was never available and was not needed.
4. After 24 h, re-snapshot. Expected: `companies.n_tup_upd` down by orders of magnitude,
   `jobs.n_tup_upd` down toward the rate of genuine content change, HOT share up, dead
   tuples falling. Read the per-provider hit-rate logs first.

If the counters do not move, the diagnosis was wrong and the follow-on repack must not be
run on this change's strength.

## Open Questions

None blocking. Deferred by decision, not by uncertainty: whether the per-board seen-set
should later carry `content_hash` so the decision moves out of the per-row statement
entirely.
