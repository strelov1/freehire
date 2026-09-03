# Board catalog moves from YAML into Postgres

Date: 2026-09-03

## Problem

The board catalog — which company crawls on which ATS, under what board id — lives in
`sources/*.yml`, one file per provider plus `custom.yml`/`telegram.yml`. Adding a board
today means a human or an agent edits a YAML file and opens a PR; `cmd/validate-sources`
lints it in CI before merge. Separately, `link_contributions` already tracks the
lifecycle of a crowdsourced submission (`pending` → `onboarded`/`rejected`, plus a
`review` bucket for a raw URL nobody has classified yet) — but the row it produces is
inert: turning a `pending` contribution into a live board still means generating a PR
against the YAML file (the `onboard-contributions` skill). Two systems track
overlapping state — "is this board known" — and only one of them (git) is the one that
actually feeds `cmd/ingest`.

This change makes Postgres the catalog. `sources/*.yml` stops existing; `cmd/ingest`
reads boards for a provider from a table instead of parsing a file; the contribution
lifecycle folds into that same table instead of ending in a generated commit.

## Goals

1. **`cmd/ingest` no longer reads any file to know what to crawl.** The board catalog —
   `company`, `provider`, `board`, `region`, `hub`, `tenants` — lives in one Postgres
   table.
2. **A crowdsourced contribution with a recognized `(provider, board)` becomes a catalog
   row directly**, at `pending` status, with no PR and no `onboard-contributions` skill
   run in between.
3. **The catalog carries a lifecycle** (`pending` → `active`, or `rejected`) so a newly
   added board is visibly unproven until it has actually produced a successful crawl.
4. **Manual, curator-authored additions** (today: hand-editing YAML) go through a new
   `cmd/add-board` worker, not a raw `INSERT`.

## Non-goals

- **A smarter ingest queue.** Scheduling stays "one cron timer per provider, one
  `cmd/ingest <provider>` run crawls every eligible board for that provider" — exactly
  today's granularity, just keyed by provider name instead of file path. Reordering work
  by staleness, per-board scheduling, or any cross-provider work-stealing is a distinct
  follow-up that this change makes easier (the catalog is queryable) but does not
  attempt.
- **Changing `board_health`.** Its schema, its queries (`ProviderHealthRollup` behind
  `/status`), and everything that reads it (`cmd/queue-metrics`, the public status page)
  are untouched. It keeps tracking runtime crawl health, keyed by the same
  `(provider, board, region)` the new catalog table uses.
- **Changing provider/adapter code.** `sources.All`, `sources.Taxonomy`, the `Source`
  interface, and every adapter in `internal/ingest/sources/*.go` are unaffected — they
  already consume a `[]CompanyEntry` regardless of where it came from.
- **Rethinking the `review` (unclassified URL) submission UX.** It keeps behaving as it
  does today; only its storage table is renamed/narrowed (see below).

## Design

### New table: `boards`

Replaces `sources/*.yml` as the thing `cmd/ingest` reads, and absorbs
`link_contributions`' non-`review` rows (the ones with a known `(source, board)`).

```sql
CREATE TABLE boards (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider        text NOT NULL,
    board           text NOT NULL,
    region          text NOT NULL DEFAULT '',
    company         text NOT NULL,
    hub             boolean NOT NULL DEFAULT false,
    tenants         jsonb NOT NULL DEFAULT '{}'::jsonb,
    url             text,
    status          text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'active', 'rejected', 'retired')),
    submitted_by    bigint REFERENCES users(id) ON DELETE SET NULL,
    surface         text NOT NULL DEFAULT 'curator'
                        CHECK (surface IN ('web', 'telegram', 'discord', 'extension', 'cli', 'unknown', 'curator')),
    rejected_reason text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    activated_at    timestamptz
);

CREATE UNIQUE INDEX boards_identity_key
    ON boards (provider, lower(board), region)
    WHERE status IN ('pending', 'active');
```

`(provider, board, region)` is exactly `board_health`'s key — no new join concept, just
a second table keyed the same way. `tenants` moves from `map[string]string` YAML to
`jsonb` (same shape, `internal/ingest/sources`'s `CompanyEntry.Tenants` decodes it the
same way it decodes YAML today, just from a `[]byte` instead of a YAML node). `url` is
the submitted link for a crowdsourced row (what "My contributions" shows back to the
submitter) and is `NULL` for a curator-added row (`cmd/add-board` has no submitted URL to
record).

### New table: `board_submissions`

The part of `link_contributions` that is NOT a board yet: a raw URL nobody has
classified into `(provider, board)`. This is what `Repository.RecordReview` writes today
under `status='review'`.

```sql
CREATE TABLE board_submissions (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url          text NOT NULL,
    submitted_by bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    surface      text NOT NULL DEFAULT 'unknown'
                     CHECK (surface IN ('web', 'telegram', 'discord', 'extension', 'cli', 'unknown')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX board_submissions_url_key ON board_submissions (url);
```

Triage (human or the successor to `onboard-contributions`) resolves a submission's
`(provider, board)` and then does exactly what a recognized crowdsourced contribution
does: `INSERT INTO boards ... status='pending'`, then deletes the `board_submissions`
row. `board_submissions` never holds a `provider`/`board` column — if triage knows them,
the row's job is already done.

`link_contributions` is dropped once both new tables exist and the read/write paths in
`internal/ingest/contribution` move over.

### Lifecycle

1. **Crowdsourced, `(provider, board)` known at submission time** (today:
   `Repository.Record`) → validate against the adapter registry (the same checks
   `cmd/validate-sources`/`Config.Validate` run today: provider exists in
   `sources.Taxonomy()`, required fields present) → valid: `INSERT ... status='pending'`;
   invalid: `INSERT ... status='rejected', rejected_reason=...` (so the submitter sees
   why, instead of the row silently not existing). A duplicate of an existing `pending`
   or `active` row (the unique index) is refused outright — an error to the caller, the
   same way it is today — rather than stored as a second row of any status: a `rejected`
   or `retired` row does NOT occupy the identity, so a corrected resubmission of a
   previously-rejected `(provider, board, region)` is never permanently blocked by its
   own earlier typo.
2. **First successful crawl of a `pending` board** — `pending` boards ARE crawled by
   `cmd/ingest` (unproven, not untested), same as `active` ones. `pipeline.Runner`, right
   where it already resolves company identity per board, flips `pending → active` and
   sets `activated_at` the first time a board's crawl completes without a board-level
   error (mirrors `board_health`'s own success/failure signal, just written to a
   different table).
3. **Curator, manual** — `cmd/add-board` (report-by-default, `--apply` to write, same
   convention as `cmd/merge-companies`) runs the identical validation and inserts
   directly at `status='active'` — a curator adding a board by hand has already verified
   it, so there is no unproven period to model.
4. **Retiring a board** (today: delete the YAML line) — `status='retired'` via
   `cmd/add-board --retire provider/board`, never a hard delete. A `retired` row is
   excluded from `cmd/ingest`'s query, and `board_health`'s row for it goes stale/inert
   exactly as documented today ("a stale row for a removed board is inert") — nothing
   about that changes.

### `cmd/ingest`

Drops `SOURCES_FILE`/path-argument loading. Takes a provider name:

```
go run ./cmd/ingest greenhouse
```

which becomes `SELECT ... FROM boards WHERE provider = $1 AND status IN ('pending', 'active')`
in place of `sources.LoadConfig(path)`. Everything downstream — `Config.Validate`
against the registry, `pipeline.Runner` — is unchanged; both already consume
`[]CompanyEntry`. One cron timer per provider — for every provider-only file, the same
count as one file per provider today. **Not** for `custom.yml`: its ~27 rows already each
name their own provider, but today they all crawl in ONE cron-triggered process
(`cmd/ingest sources/custom.yml`), spanning ~25 distinct providers. Since a
provider-filtered query can only ever return one provider's rows, that becomes ~25
separate `cmd/ingest <provider>` invocations — ~25 cron timers where there is one today.
(Corrected during implementation; an earlier draft of this paragraph claimed "same
count" for `custom.yml` too, reasoning only about the per-row provider field and missing
that today's single cron timer crawls them all in one process.)

### What is retired

- `sources/*.yml`, `sources/custom.yml`, `internal/ingest/sources/config.go`'s YAML
  parsing (`LoadConfig`, `ParseConfig`, `ParseRawEntries`, `dedupeBoards`) — replaced by
  a DB query plus the unique index.
- `cmd/validate-sources` and the "Validate sources" CI step — validation moves to the
  insert-time function shared by the contribution path and `cmd/add-board`; there is no
  more file for CI to lint.
- The `onboard-contributions` skill — triage now ends in an `INSERT`, not a PR.
- `link_contributions` (table, and `internal/ingest/contribution`'s
  `Record`/`RecordReview`/`ListByUser` read/write paths get rewritten against `boards`
  and `board_submissions`).

### Migration

A one-off `cmd/backfill-board-catalog` worker, matching this repo's existing
`cmd/backfill-*` convention (`DATABASE_URL` only, chunked, idempotent insert guarded by
the unique index) rather than a data-carrying schema migration. It reads every
`sources/*.yml` entry and inserts it into `boards` at `status='active', activated_at=now()`
— every one of them is already crawling successfully in prod, so none of them should
re-enter the unproven `pending` window. Runs once, before the deploy that removes YAML
loading from `cmd/ingest`, and reads the YAML files from the working copy it's run
against — it is the last piece of code in the repository allowed to parse
`sources/*.yml`, and is deleted once the backfill has run in prod.

### Testing

- Unit: the insert-time validation function (provider-exists, required-fields,
  duplicate-of-existing-active-row) gets the same test coverage
  `internal/ingest/sources/config_test.go` has for `Config.Validate` today, adapted to
  DB-row input instead of parsed YAML.
- Integration (`-tags=integration`, `internal/db`-style testcontainers): the
  `pending → active` transition on first successful crawl, exercised through
  `pipeline.Runner` against a fake `Source`.
- `cmd/add-board`: report/apply behavior tested the same way `cmd/merge-companies` is —
  dry run changes nothing, `--apply` writes, re-running is a no-op (idempotent insert
  guarded by the unique index).
