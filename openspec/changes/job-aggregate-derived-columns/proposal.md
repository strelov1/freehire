## Why

"What columns a persisted job must carry" is split between the aggregate and two
derived columns — `content_hash` and `role_fingerprint` — that every write path is
told, in a doc comment, to bolt on after the mapping returns. Three of the four write
paths forget at least one, and the moderator path forgets both: `UpsertManualJob` and
`UpdateManualJob` do not even list the columns.

The consequences are live, not hypothetical:

- A moderator- or submission-authored vacancy lands with `role_fingerprint` NULL.
  `RoleClusterCount` filters `role_fingerprint <> ''`, so that vacancy can never be
  deduped or clustered against the ATS copy of the same role — which is exactly the
  case a crowdsourced submission creates.
- It also lands with `content_hash` NULL. The re-embed trigger is
  `semantic_embedded_hash IS DISTINCT FROM content_hash`, and `NULL IS DISTINCT FROM
  NULL` is false, so after the first embed a moderator's description edit never
  refreshes the vector: the semantic index permanently describes the pre-edit text.
  The rule that justifies stamping NULL ("a non-board job with no content_hash is
  never re-crawled, so its text is stable") is falsified by `UpdateManualJob`, which
  exists to edit exactly those jobs.

A prose contract that three of four callers get wrong is not a contract. Moving the
two columns inside the mapping the aggregate already owns makes "a persisted job
carries a content hash and a role fingerprint" true by construction.

## What Changes

- `Fields.UpsertParams` computes `ContentHash` and `RoleFingerprint` itself instead of
  documenting that the caller must. The three automated write paths
  (`cmd/ingest/store.go`, `cmd/tg-extract/store.go`, `internal/linkimport`) drop their
  post-mapping assignments.
- **Ordering prerequisite:** `jobhash.Of` hashes `posted_at`, and `cmd/tg-extract` and
  `internal/linkimport` overwrite `params.PostedAt` *after* the mapping returns. Both
  move to the `job.Draft.PostedAt` field that already exists for this purpose, so the
  hash covers the posted time that is actually written.
- `Fields.UpsertManualParams` computes both columns too, and `content_hash` /
  `role_fingerprint` are added to the `UpsertManualJob` column list.
- The moderator edit path gets the same treatment: `content_hash` and
  `role_fingerprint` are added to `UpdateManualJob`'s SET list, and the hand-rolled
  `db.UpdateManualJobParams` literal in `internal/moderation/repository.go` — a fourth
  copy of the same column list — moves onto the aggregate as
  `Fields.UpdateManualParams`, mirroring `UpsertManualParams`.
- Existing rows are not backfilled by this change. `cmd/backfill-derive` already
  re-derives `role_fingerprint` for the whole catalogue; the remaining manual-job
  `content_hash` gap is called out in the design as a follow-up operational step
  rather than smuggled into a domain change.

No behavior visible on the public wire shape changes. No migration: both columns
already exist on `jobs`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `job-aggregate`: adds a requirement that the aggregate's write mapping — not the
  caller — owns every derived column a persisted job carries, so the derived columns
  cannot be omitted by a write path, and that the mapping hashes the posted time that
  is actually written.

## Impact

- `internal/job/job.go` — `UpsertParams`, `UpsertManualParams`, new `UpdateManualParams`;
  the package gains a dependency on `internal/jobhash`.
- `internal/db/queries/jobs.sql` — `UpsertManualJob` and `UpdateManualJob` column lists;
  `make sqlc` regenerates `internal/db`.
- `internal/moderation/repository.go` — `Update` stops hand-building params.
- `cmd/ingest/store.go`, `cmd/tg-extract/store.go`, `internal/linkimport/linkimport.go` —
  post-mapping assignments removed; the latter two pass `PostedAt` through the draft.
- Downstream, already-specified behavior that starts working for manual jobs:
  role clustering / repost dedup (`ingest-content-dedup`, `job-reality-signal`) and
  semantic re-embedding after a moderator edit (`semantic-embedding`). Those specs
  already require this behavior generally; none of their requirements change.
- Out of scope: the duplicated `hashParams` remap in `cmd/backfill-descriptions` and
  `cmd/backfill-justjoin` (a separate review finding), and `jobhash.Of`'s known
  omission of `cities` / `english_level` / `is_tech`.
