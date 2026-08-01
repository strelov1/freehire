## Context

`internal/job` is the aggregate with a single guarded construction door: a `Job` can
only be built through `job.New` or loaded through a repository, which makes "a Job
carries facets consistent with its source fields" a type-enforced invariant. The write
mapping `Fields.UpsertParams` extends that idea to persistence — one column list, shared
by every write path.

Two columns were left outside it. `UpsertParams`' doc states the contract in prose:
*"columns a caller derives separately (ContentHash, RoleFingerprint, or a PostedAt
supplied outside the aggregate) are set on the returned struct after this call."* The
result is what a prose contract usually produces:

| Write path | `content_hash` | `role_fingerprint` |
|---|---|---|
| `cmd/ingest/store.go` | set | set |
| `cmd/tg-extract/store.go` | **missing** | set |
| `internal/linkimport` | **missing** | set |
| moderator create (`UpsertManualJob`) | **not even a column** | **not even a column** |
| moderator edit (`UpdateManualJob`) | **not even a column** | **not even a column** |

The moderator create/edit queries are a fourth and fifth hand-maintained column list;
`internal/moderation/repository.go` builds `db.UpdateManualJobParams` as a literal
rather than going through the aggregate the way its sibling `Create` does.

Constraint that shapes the whole change: `jobhash.Of` hashes `posted_at`, and both
`cmd/tg-extract` and `internal/linkimport` assign `params.PostedAt` *after*
`UpsertParams()` returns. Computing the hash inside the mapping without touching those
two callers first would fingerprint a `NULL` posted time while writing a real one — a
hash that no re-ingest of the same posting could ever reproduce.

## Goals / Non-Goals

**Goals:**

- Make "a persisted posting carries both derived columns" true by construction, not by
  each caller remembering a second step.
- Make the derived columns identical across write paths for identical content, so a
  moderator-authored copy of a role is comparable with the crawled one.
- Fix the posted-time ordering so the fingerprint covers the value actually written.
- Fold the fifth column list (`UpdateManualJobParams`) into the aggregate, since the
  change has to edit it anyway.

**Non-Goals:**

- Backfilling `content_hash` on manual rows already in production. Handled
  operationally (see Migration Plan), not by a domain change.
- The duplicated `hashParams` remap in `cmd/backfill-descriptions` and
  `cmd/backfill-justjoin` — a separate review finding with its own trade-offs.
- `jobhash.Of`'s known omission of `cities`, `english_level` and `is_tech` from the
  fingerprint. Real, documented, and orthogonal: changing the hash input would
  invalidate every stored `content_hash` at once.
- Marking manual jobs as duplicates at write time. `jobdedup.CanonicalForRole` is
  called by ingest and link-import only; the moderator path continues to leave that to
  the batch reconciler.

## Decisions

### D1: The derived columns are computed inside the mapping, not returned beside it

`Fields.UpsertParams()` returns params with `ContentHash` and `RoleFingerprint` already
populated.

*Alternative considered:* a `Fields.Derived() (contentHash, roleFingerprint string)`
helper that callers apply. Rejected — it is the same "remember the second step" contract
with a shorter spelling, and the failure mode this change exists to remove is precisely
a forgotten second step. A helper would still let a write path compile while omitting a
column.

*Alternative considered:* enforcing the columns in the SQL (a generated column or a
trigger). Rejected — `role_fingerprint` normalizes titles through `internal/jobhash`'s
Go dictionary logic, which has no SQL equivalent, and the project keeps derivation in Go
so it is testable and dictionary-driven.

### D2: The manual mapping hashes the shared params, not a second hash function

`jobhash.Of` and `jobhash.RoleFingerprint` take `db.UpsertJobParams`, while the manual
path produces `db.UpsertManualJobParams`. `UpsertManualParams` therefore calls
`f.UpsertParams()` internally and fingerprints that.

*Alternative considered:* a `jobhash.OfManual(db.UpsertManualJobParams)` overload.
Rejected outright — two hash functions over two structs that must agree field-for-field
forever is a stronger version of the exact drift this change removes. Routing both
through one params shape makes cross-path equality structural rather than a convention,
which is what the spec's "identical content fingerprints identically" scenario asserts.

### D3: A caller-supplied posted time goes through `job.Draft`, which already has the field

`job.Draft` already declares `PostedAt *time.Time`, documented as "the source posted
time"; `cmd/tg-extract` and `internal/linkimport` simply do not use it and patch the
mapped params instead. Both move to the draft field.

This is why the ordering fix is cheap: it is not a new mechanism, it is starting to use
an existing one. `jobderive.Derive` reads only the embedded `jobderive.Input`, and
`PostedAt` sits outside it, so routing the value through the draft cannot disturb facet
derivation.

`cmd/tg-extract` holds its value as a `pgtype.Timestamptz` (the Telegram post's
timestamp); it converts to `*time.Time` at the draft boundary rather than the aggregate
learning a persistence type.

### D4: `internal/job` takes a dependency on `internal/jobhash`

`internal/jobhash` imports `internal/db` only, so there is no cycle. The direction is
right: the aggregate owns what a persisted posting carries, and fingerprinting is part
of that; `jobhash` stays a leaf that knows nothing about the aggregate.

### D5: The moderator edit path moves onto the aggregate

`Fields.UpdateManualParams(slug, actorID)` joins `UpsertParams` and
`UpsertManualParams`, and `internal/moderation/repository.go`'s `Update` stops building
the literal. Without this, the change would add the two columns to a hand-rolled list
and leave the sixth-caller-forgets problem in place one line below the fix.

`content_hash` and `role_fingerprint` are added to `UpdateManualJob`'s SET list — the
edit path is the one that *must* rewrite them, since re-deriving facets from edited
content is exactly when the fingerprints move.

## Risks / Trade-offs

**A manual job that starts carrying `role_fingerprint` becomes visible to role
clustering, and a non-canonical repost is hidden from catalogue and search — a curated
vacancy could disappear behind a crawled canon.** → Verified as pre-existing, not
introduced here: `cmd/backfill-derive` pages the whole `jobs` table with no source or
`created_by` filter and writes `role_fingerprint` for every row, so manual rows already
acquire fingerprints on each run. This change makes new manual rows consistent with what
the backfill already produces. Duplicate *marking* still happens only in the batch
reconciler for this path, unchanged.

**A previously-`NULL` `content_hash` becoming non-NULL enqueues manual jobs for
re-embedding.** → Intended, and the desired outcome of the finding: the vector currently
freezes on the pre-edit text forever. The volume is bounded — manual and submission jobs
are a small slice of the catalogue — and the semantic outbox is leased and rate-bounded,
so the enqueue drains rather than spikes.

**Computing two SHA-256 hashes inside the mapping makes `UpsertParams` non-trivial, and
it is called once per ingested row.** → Both hashes were already computed once per row
on the ingest path; this relocates the work rather than adding it. The two paths that
previously computed only one hash now compute two, which is a per-row cost measured in
microseconds against a query round-trip.

**`make sqlc` regenerates `internal/db` from the edited queries; a stale checkout could
produce a params struct without the new fields.** → The generated code is committed, and
`go build ./...` fails loudly on a missing field, so the failure is compile-time and
local, not a silent production divergence.

## Migration Plan

No schema migration: `content_hash` and `role_fingerprint` already exist on `jobs`.

Deploy order is unconstrained — the change only starts writing two columns that readers
already tolerate as NULL.

Existing rows, after deploy:

1. `role_fingerprint` on legacy manual rows — already handled by the next scheduled
   `cmd/backfill-derive` run; no new step.
2. `content_hash` on legacy manual rows stays NULL until that row is edited or
   re-created, since nothing re-crawls a manual posting. This is the pre-existing gap,
   now bounded to rows written before the deploy rather than growing. Closing it is a
   one-line addition to `cmd/backfill-derive`'s update set and is left as a follow-up so
   this change stays a domain change with no worker behavior in it.

Rollback is a plain revert: the columns become unwritten again, and no reader breaks on
values already present.

## Open Questions

None blocking. The follow-up question — whether `cmd/backfill-derive` should also
re-derive `content_hash`, closing the legacy manual-row gap — is deliberately deferred
rather than unresolved.
