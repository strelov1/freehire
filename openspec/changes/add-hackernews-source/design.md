## Context

See proposal.md - Why. Facts this design leans on, confirmed against the current tree:

- `internal/ingest/telegram` is crawl (`cmd/tg-ingest`) → durable queue (`telegram_posts`,
  claimed/retried) → extraction (`cmd/tg-extract`, LLM, `Extraction.Validate()` then
  `job.New(job.Draft{...})`, written via a direct `qtx.UpsertJob(...)` call in one
  transaction with enrichment/search-outbox enqueue — NOT `internal/ingest/pipeline`, which
  is the board-crawl pipeline, not used here).
- `internal/ingest/atsboard.Recognize(url) (source, board, canonical string, ok bool)`
  already exists and is exactly "URL → (provider, board)" in Go — the same job
  `scripts/ats_boards.py`'s `SLUG_PATTERNS` does in Python for the one-off harvest tooling.
  `internal/ingest/contribution` already calls it for the site's own contribution flow.
- The only way anything writes a `boards` row is `boardcatalog.NewInserter(repo,
  registry).Insert(ctx, InsertInput{...}, StatusPending)` — `contribution` and
  `cmd/harvest-boards` both call it directly; there is no other wrapper to go through.
  `InsertInput.Surface` is a plain string on `boardcatalog` (not validated there), and a
  separate DB `CHECK (surface IN ('web','telegram','discord','extension','cli','unknown'))`
  constrains what actually persists (migration 0123). `cmd/harvest-boards` already uses
  `Surface: "cli"` for its own non-interactive writes.
- `jobs_source_external_id_key UNIQUE (source, external_id)` is the whole-catalog dedup key.
  Telegram's shape: `source = "telegram"`, `external_id = "<channel>/<msg_id>/<job_index>"`
  (channel+msg_id because a Telegram message id is only unique *within* a channel).
- `cmd/liveness`'s `unsignalledSources = []string{"telegram"}` (main.go:75) does double duty:
  it's both the exclude-from-probe list and the source list fed to
  `CloseStaleUnsignalledJobs` (45-day `expiryWindow`, `main.go:82`) — one list, both halves
  of "no close signal, so close by age instead."
- `cmd/gen-contracts/main.go:388` hardcodes the non-ATS-adapter `source` values that feed
  the frontend's `SOURCE_VALUES`: `[]string{"telegram", "workatastartup", "remoteok",
  "arc"}` unioned with the ATS registry.
- Layering: `internal/platform/arch/layering/blocks.go` places `telegram`, `atsboard`,
  `boardcatalog`, `contribution`, `sources`, `pipeline` all in one `ingest` block; packages
  within one block are not order-checked against each other, so a new package in that same
  block can import any of them.

## Goals / Non-Goals

**Goals:**
- Land the two-branch pipeline described in proposal.md, reusing every existing mechanism
  identified above rather than inventing a parallel one.
- Keep the new package self-contained: its own extraction types, its own crawl/extract
  workers, no changes to `internal/ingest/telegram` (that coupling is explicitly a later,
  separate proposal).

**Non-Goals:**
- No `hn_channels`-equivalent table. There is exactly one implicit source — the current
  "Who is hiring?" thread(s), discovered fresh each run via Algolia — not a configured list.
- No sub-comment splitting. A comment is graded once: at least one recognized ATS link →
  board contribution; otherwise → LLM extraction. A comment that names one company with a
  link and separately describes an unrelated second opportunity in prose is not split into
  two outcomes — this mirrors the proposal's explicit branch-per-comment framing and keeps
  the extraction stage's contract simple (one comment, one decision).
- No new `boards.surface` value. Reusing `"cli"` (see Decisions) avoids a schema change for
  a distinction nothing currently reads.
- No fix to the `board-harvest` spec's own pre-existing YAML-destination staleness — out of
  scope, unrelated to this change (already flagged once, in harvest-githublists-boards).

## Decisions

**No shared "vacancy extraction" type with `internal/ingest/telegram`.** Telegram's
`Extraction`/`ExtractedJob`/`Validate()` (`internal/ingest/telegram/extraction.go`) are
structurally close to what this package needs, and the layering guard would permit
importing them (same `ingest` block). Decided against it: every other pair of ingest
subpackages here is independent even where structurally similar (`atsboard` and
`boardcatalog` do not share a type despite both being "about a board"), and reaching into
telegram's types would couple two sources that otherwise evolve for unrelated reasons — a
telegram-specific field added later has no reason to ripple into hackernews's extraction.
`internal/ingest/hackernews` defines its own small `Extraction`/`ExtractedJob` and
`Validate()`, following the same shape (drop a job missing `Title`/`Description`) but as its
own copy.

**Board-contribution branch reuses `"cli"` for `Surface`, not a new enum value.** The
`boards.surface` CHECK constraint is a closed, small set
(`web/telegram/discord/extension/cli/unknown`) meant to distinguish *where a human or
automated submission came from*. `cmd/harvest-boards` already established the precedent
that a non-interactive automated writer uses `"cli"`. Adding `"hackernews"` would need a
migration extending that CHECK (mirroring how migration 0074 added `"discord"`) for a
distinction nothing currently reads — the row's own audit trail (created_at, which worker
ran) already answers "where did this come from" for an operator who needs to know. Revisit
if a real need to filter/report boards by harvest source shows up later.

**Job identity: `source = "hackernews"`, `external_id = "<comment_id>"` (or
`"<comment_id>/<job_index>"` when a single comment yields more than one job).** Unlike a
Telegram message id (only unique within its channel, hence `<channel>/<msg_id>`), an HN
item id (a comment's own id) is globally unique across the whole site — no thread-id prefix
is needed for uniqueness. The `hn_posts` row is still keyed by the comment id directly
(see below), and carries `thread_id` only for traceability/debugging, not identity.

**`hn_posts` is a single table, PK'd by the comment's own HN item id — no
`hn_threads`/channel-list table.** Telegram's `(channel, msg_id)` composite key exists
because a message id repeats across channels; HN comment ids don't repeat, so a bare PK is
enough. The thread(s) to crawl are discovered fresh every run from Algolia's
`search_by_date?tags=story,author_whoishiring` search, not read from a config table —
there is nothing to configure. Shape (new migration `0165_hn_posts.sql`, modeled on
`telegram_posts` at `migrations/0001_init.sql:377-389` minus the parts that don't apply):

```sql
CREATE TABLE hn_posts (
    id           bigint PRIMARY KEY,          -- the comment's own HN item id
    thread_id    bigint NOT NULL,             -- the "who is hiring" story id (traceability only)
    text         text NOT NULL,
    posted_at    timestamptz NOT NULL,
    fetched_at   timestamptz NOT NULL DEFAULT now(),
    attempts     integer NOT NULL DEFAULT 0,
    claimed_at   timestamptz,
    failed_at    timestamptz,
    last_error   text NOT NULL DEFAULT '',
    extracted_at timestamptz
);
```
No `links jsonb` column: Telegram's crawl stage parses the web-preview HTML's own `<a>`
tags as a distinct step from the post text; here, the comment's `text` already contains
whatever URLs it has, and the extraction stage regexes them out of `text` directly (via
`atsboard.Recognize` over each URL found) — no reason to pre-extract and store them
separately.

**Closed by age: add `"hackernews"` to `cmd/liveness`'s existing `unsignalledSources`
slice.** `main.go:75`'s `[]string{"telegram"}` becomes `[]string{"telegram", "hackernews"}`
— this single change gets both halves (excluded from the liveness probe, closed at the same
45-day `expiryWindow`) for free, no new query, no new migration. The rationale is identical
to telegram's: the stored reference (the comment/thread's permalink, or whatever `URL` the
extracted job carries) outlives the vacancy, so a probe can never reach a death verdict.

**Package layout, mirroring `internal/ingest/telegram`'s file split:**
- `internal/ingest/hackernews/fetch.go` — Algolia HTTP calls (`hiringThreadIDs`,
  `commentsFor`), the untested-by-design IO boundary.
- `internal/ingest/hackernews/crawl.go` — `CrawlRunner`, mirroring `telegram.CrawlRunner`'s
  shape (fetch → prefilter → `Store.Insert` into `hn_posts`).
- `internal/ingest/hackernews/prefilter.go` — reuse the same non-vacancy filter idea
  `telegram/prefilter.go` uses (a comment that plainly isn't a job listing, e.g. a reply
  asking a question, shouldn't reach the LLM stage) — HN "who is hiring" comments are
  English-only by convention, so this is simpler than telegram's per-language marker set.
- `internal/ingest/hackernews/atslink.go` — `recognizedBoard(text string) (provider, board,
  url string, ok bool)`: regexes URLs out of a comment, calls `atsboard.Recognize` on each,
  returns the first hit.
- `internal/ingest/hackernews/contribute.go` — wraps
  `boardcatalog.NewInserter(...).Insert(...)` for the "has a link" branch, deriving the
  company name from the comment the same way `scripts/harvest_boards.py`'s
  `hn_company_name()` already does (leading `Company | Role | ...` token, rejecting a
  prose/role-looking leader) — ported to Go, not imported from Python.
- `internal/ingest/hackernews/extraction.go` — `Extraction`/`ExtractedJob`/`Validate()`
  (this package's own copy, per the Decision above).
- `internal/ingest/hackernews/llm.go` — `LangChainExtractor`, one system prompt (no
  `kind` split — every HN "who is hiring" comment is the same shape, unlike Telegram's
  authored-vs-board channels).
- `internal/ingest/hackernews/extract.go` — `ExtractRunner`: claim → branch (link found →
  `contribute.go`; else → `llm.go`) → write → mark `extracted_at`.
- `cmd/hn-ingest/main.go`, `cmd/hn-extract/main.go` — `worker.Main`/`worker.Bootstrap`,
  mirroring `cmd/tg-ingest`/`cmd/tg-extract` exactly.

**`internal/ingest/hackernews` is added to `internal/platform/arch/layering/blocks.go`'s
`ingest` block table**, alongside `telegram`/`atsboard`/`boardcatalog`/`contribution`.

**`cmd/gen-contracts/main.go`'s hardcoded source list gains `"hackernews"`**
(`[]string{"telegram", "workatastartup", "remoteok", "arc"}` →  + `"hackernews"`), then
`make gen-contracts` regenerates `web/src/lib/generated/contracts.ts`'s `SOURCE_VALUES` —
never hand-edited (per the repo's own generated-contracts convention).

## Risks / Trade-offs

- **A board contributed from a one-line HN comment has no live-validation richer than what
  `boardcatalog.Insert` already does** (it probes the provider's own API before persisting,
  same as any other contribution) — accepted, this is the same trust level
  `cmd/harvest-boards`/the site's own contribution flow already operate at.
- **A comment naming a company with a *misrecognized* or dead ATS link** (e.g. a link to a
  page `atsboard.Recognize` doesn't match) falls to the LLM-extraction branch instead of
  being flagged — indistinguishable from a comment with no link at all. Acceptable: the
  free-text branch still captures the vacancy, just without the standing-board upside.
- **English-only prefilter** — HN's "who is hiring" convention is English by nature (unlike
  Telegram, which spans RU/EN/UA channels), so no per-language marker set is needed; if that
  assumption ever breaks it fails the same way telegram/AGENTS.md documents (a
  non-English post silently looks non-vacancy).
- **One thread can be large** (the contributor's own numbers: 417 posts/comments in a
  month) — `hn_posts` claim/attempts columns give the same retry safety telegram_posts has,
  so a partial run is never lost work.

## Migration Plan

1. `migrations/0165_hn_posts.sql`.
2. `internal/ingest/hackernews` package (crawl half first, independently testable, then
   extract half).
3. `cmd/hn-ingest`, `cmd/hn-extract`.
4. `internal/platform/arch/layering/blocks.go` registration.
5. `cmd/gen-contracts` source-list update + `make gen-contracts`.
6. `cmd/liveness/main.go`'s `unsignalledSources` gains `"hackernews"`.
7. No backfill needed — this is a new source with no prior data.
