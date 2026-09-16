## Why

Issue #2869 proposed `hackernews` as a third aggregator source, crawling the monthly "Ask
HN: Who is hiring?" thread as a plain link-harvest (417 posts listed, 244 ingested by the
contributor's adapter). A pure link-harvest under-serves this source: a "who is hiring"
comment is free text, and a large share of comments name a company and a role with no ATS
link at all ("Acme Corp | Senior Backend | Remote | apply: jobs@acme.com") — a link-only
adapter simply drops those. The comments that DO carry a recognizable ATS link are better
served by a permanent board contribution (the company then joins the regular first-party
crawl and every future posting of theirs is surfaced, not just the one HN mentioned) than by
a one-off job record, which is exactly the shape the `githublists` fix (PR #2879) already
established for structured aggregator lists — but HN's comments are not structured, so that
fix's approach doesn't reach the free-text majority.

This proposes a two-branch pipeline, architecturally close to `internal/ingest/telegram`
(cheap crawl → queue → extraction stage): a comment with a recognizable ATS link becomes a
board contribution; a comment with no such link is free text and goes through LLM
extraction into the job catalogue directly, the same shape `cmd/tg-extract` already uses for
Telegram. This was the shape committed to in the issue comment already posted
(https://github.com/strelov1/freehire/issues/2869#issuecomment-5689029816).

## What Changes

- New `internal/ingest/hackernews` package: fetches the current "Who is hiring?" thread(s)
  via the Algolia HN Search API, stores each top-level comment as a durable post (new
  `hn_posts` table, no channel-list table needed — unlike Telegram, there is exactly one
  source and it is discovered fresh each run, not configured).
- New `cmd/hn-ingest` (crawl → enqueue) and `cmd/hn-extract` (dequeue → branch → write),
  both `worker.Main`/`worker.Bootstrap` run-once-and-exit cron workers, following
  `cmd/tg-ingest`/`cmd/tg-extract`'s exact shape.
- The extraction branch: every URL in a comment is checked against
  `internal/ingest/atsboard.Recognize` (the existing Go-native ATS URL recognizer, already
  used by the site's own contribution flow — **not** a reimplementation of
  `scripts/ats_boards.py`'s Python regex table, which is one-off maintainer tooling, not
  part of the deployed pipeline). A comment with at least one recognized link is submitted
  as a board contribution via `boardcatalog.Inserter.Insert(..., StatusPending)` — the same
  entry point `internal/ingest/contribution` already uses, so a hackernews-sourced board is
  indistinguishable in the catalog from a site-submitted or harvested one. A comment with no
  recognized link goes through LLM extraction (provider-agnostic `internal/platform/llm`
  client, the same `Extraction.Validate()` + `job.New(job.Draft{...})` gate `tg-extract`
  uses) directly into the job catalogue (`source = "hackernews"`).
- Closed by age, the same accepted limitation `internal/ingest/telegram/AGENTS.md` documents
  for Telegram jobs (no de-list signal from a static thread).
- Adds `internal/ingest/hackernews` to the layering table
  (`internal/platform/arch/layering/blocks.go`), same block (`ingest`) as `telegram`,
  `atsboard`, `boardcatalog`, `contribution`.

## Capabilities

### New Capabilities
- `hackernews-ingest`: crawls HN's monthly "Who is hiring?" thread, branches each comment
  into a board contribution (has a recognized ATS link) or an LLM-extracted job posting (does
  not), closed by age.

### Modified Capabilities
<!-- none -->

## Impact

- New migration(s): `hn_posts` table (queue/audit of crawled comments — see design.md for
  exact shape) plus whatever the `jobs` row needs to distinguish a hackernews-sourced posting
  for the age-based close sweep (mirrors how Telegram jobs are excluded from `cmd/liveness`
  and closed on `COALESCE(posted_at, created_at)`).
- New packages/binaries: `internal/ingest/hackernews`, `cmd/hn-ingest`, `cmd/hn-extract`.
- `internal/platform/arch/layering/blocks.go`: register the new package.
- No change to `scripts/harvest_boards.py`'s existing `--hn` flag (`harvest_hn()` and
  friends) — that stays as ad-hoc maintainer tooling; this proposal does not remove or
  repurpose it, since it answers a different need (exploratory harvesting, hand-run) than a
  deployed cron pipeline.
- No change to `cmd/tg-extract`/`internal/ingest/telegram` — extending Telegram's own
  extraction with the same ATS-link-first branch is a separate, later proposal (mentioned in
  the issue comment as a possible follow-up, explicitly out of scope here).
