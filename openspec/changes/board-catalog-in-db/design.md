## Context

See `proposal.md` - Why. Full exploratory design (including approaches considered and
rejected before this one) lives at
`docs/superpowers/specs/2026-09-03-board-catalog-in-db-design.md`; this document is the
implementation-facing distillation of it.

Today: `sources/*.yml` (one file per provider, plus a mixed `custom.yml`) is parsed by
`internal/ingest/sources/config.go` into `[]CompanyEntry`, validated against the adapter
registry, and fed to `pipeline.Runner` by `cmd/ingest <path-or-SOURCES_FILE>`.
`board_health (provider, board, region)` sits alongside it as pure runtime state.
`link_contributions (submitted_by, url, source, board, status, surface)` tracks a
crowdsourced submission; turning a `pending` row into a crawled board is a human running
the `onboard-contributions` skill, which generates a PR against the YAML.

## Goals / Non-Goals

**Goals:**
- Every read/write path currently touching `sources/*.yml` or `link_contributions` moves
  to the tables in this design, with no interim dual-write period.
- `pipeline.Runner` and every `Source` adapter are touched by nothing — they already
  consume `[]CompanyEntry`, agnostic to where it came from.

**Non-Goals:**
- Changing `board_health`'s schema, queries, or consumers (`/status`,
  `cmd/queue-metrics`). See proposal.md's Non-Goals for the ingest-scheduling exclusion.

## Decisions

### `boards` absorbs the catalog and the recognized half of `link_contributions`; `board_health` is untouched

Rejected alternative: one wide table merging `boards` and `board_health`. `board_health`
is mature (its own AGENTS.md, `ProviderHealthRollup` behind the public `/status` page,
direct consumers like `cmd/queue-metrics`); folding it in multiplies blast radius for no
requirement in this change. Both tables share the `(provider, board, region)` key
already, so a join costs nothing new.

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

CREATE UNIQUE INDEX boards_identity_key ON boards (provider, lower(board), region)
    WHERE status IN ('pending', 'active');
```

The unique index covers only `pending`/`active` — not `rejected` or `retired` — so
neither a retired board nor a previously-rejected one permanently occupies its identity.
Found during implementation: a first draft filtered only `status <> 'retired'`, which
meant a validation failure (say, a typo'd provider) stored at `status='rejected'` would
then block every future correct resubmission of that same `(provider, board, region)`
forever, since a `rejected` row would collide with itself under that narrower filter. The
"board-catalog" spec's duplicate scenarios reflect the corrected `pending`/`active`-only
filter.

`url` (added during spec-writing, absent from the exploratory doc): the submitted link,
`NULL` for a `cmd/add-board` row. Needed because "My contributions view" shows the
canonical URL back to the submitter.

### `board_submissions` is a narrow triage inbox, not a `boards` variant

Rejected alternative: make `provider`/`board` nullable on `boards` itself. A submission
with no resolved `(provider, board)` isn't a catalog row with missing fields — it's not a
board at all yet — so giving it a home in the identity-keyed catalog table would mean
`boards`' unique index and every reader of it has to account for rows that aren't really
boards. A dedicated table with no such columns makes that impossible by construction:

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

Triage deletes the row and inserts into `boards` — the same code path a recognized
contribution uses.

### Insert-time validation is one function, called from three places

`cmd/validate-sources`'s checks (provider registered, board non-empty unless
`boardless`, no case-insensitive duplicate) become a Go function in
`internal/ingest/sources` taking a candidate row and the adapter registry, returning
either nil or a typed reason. It is called by: the crowdsourced-contribution insert path,
`cmd/add-board`, and `cmd/backfill-board-catalog` (defensively — the YAML it reads should
already be valid, but the backfill is the last code to trust that assumption). There is
no separate CI-time validation pass, because there is no file for CI to see.

### `cmd/ingest` takes a provider name, not a path

`SOURCES_FILE`/positional-path parsing is removed. The binary's argument becomes a
provider name; it runs `SELECT ... FROM boards WHERE provider = $1 AND status IN
('pending','active')` in place of `sources.LoadConfig(path)`. The `--shard=i/n` selector
is otherwise unchanged (see the modified `source-ingest` requirement).

**Corrected during implementation** (the exploratory doc and an earlier draft of this
section both claimed "same count" — wrong): a query filtered on one `provider` can only
ever return that one provider's rows, so one `cmd/ingest <provider>` invocation now
crawls exactly one provider, always. That is a no-op for every file that already held one
provider — but `sources/custom.yml` is a single file whose ~27 entries span **25 distinct
providers** (each row already names its own, but today's ONE cron timer crawls all of
them together in one process). Splitting that into one invocation per provider means
splitting it into **~25 cron timers**, not one. Task 8.1 (updating deployment cron units)
needs to account for this — it is a real increase for `custom.yml`'s providers
specifically, not a rename of an existing unit.

### `pending → active` transition lives in `pipeline.Runner`

The Runner already learns per-board success/failure to write `board_health`; the same
signal (a board-level crawl completing without error) flips a `pending` `boards` row to
`active` and stamps `activated_at`. This keeps the transition co-located with the only
other runtime-outcome write the Runner already makes, rather than introducing a second
place that reacts to crawl results.

## Migration Plan

1. Ship migrations creating `boards` and `board_submissions` (additive; nothing reads
   them yet).
2. Ship `cmd/backfill-board-catalog`; run it once in prod. It reads every
   `sources/*.yml` entry and inserts it into `boards` at `status='active',
   activated_at=now()`, guarded by the same unique index (idempotent — a second run
   inserts nothing new).
3. Ship the DB-backed loader in `internal/ingest/sources`, `cmd/ingest`'s provider-name
   argument, the `pending → active` transition in `pipeline.Runner`, `cmd/add-board`, and
   the `internal/ingest/contribution` rewrite against `boards`/`board_submissions` —
   together, since `cmd/ingest` cannot read from two sources at once and the
   contribution path must not write to a table nothing reads yet.
4. Switch deployment cron/systemd units from per-file to per-provider-name invocation.
5. Delete `sources/*.yml`, `cmd/validate-sources`, the "Validate sources" CI step, and
   the `onboard-contributions` skill.
6. Once step 3 has run clean in prod for a full crawl cycle of every provider, drop
   `link_contributions` in a migration.

**Rollback:** steps 1-2 are additive and reversible by simply not proceeding. Step 3 is
the only step that changes `cmd/ingest`'s contract; rolling it back means reverting to
the prior `cmd/ingest` binary and cron units (both still work against the untouched
`sources/*.yml`, which step 5 has not yet deleted) while `boards` sits unused. Once step 5
has run, rollback requires restoring the deleted YAML files from git history — the
reason step 5 is ordered after a full prod crawl cycle has validated step 3, not
alongside it.

## Risks / Trade-offs

- **[Risk]** The backfill (step 2) and the cutover (step 3) are separate deploys; a board
  added to YAML by hand in between them is silently dropped once `cmd/ingest` stops
  reading YAML. → **Mitigation**: freeze YAML edits between steps 2 and 3 (both are
  one-off, low-latency deploys in the same change window).
- **[Risk]** `cmd/add-board`'s validation and the crowdsourced-contribution validation
  drifting apart over time (two call sites of the same function, edited independently).
  → **Mitigation**: one shared function (see Decisions), not two implementations; a test
  exercises both call sites against the same fixture table.
- **[Trade-off]** Losing the git-diff review step for new boards. Accepted per the
  hybrid answer this change is built on: insert-time validation plus a visible `pending`
  state until the first successful crawl is the replacement review signal, not a
  human-reviewed diff.
