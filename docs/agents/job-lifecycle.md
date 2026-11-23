# Job lifecycle conventions

## Scope

The open/closed state of a job row, the seven mechanisms that write `closed_at`, and the filtering semantics that depend on it.

## Always true

- A job is open while `closed_at IS NULL`. Closing is a soft state; nothing hard-deletes jobs. Catalogue withdrawals also use this state, with a distinct reason so they are not counted as employer closures.
- LinkedIn discoveries without a verified on-site application control are withdrawn with `native_source_pending` while native ATS ownership is resolved. Reconciliation can restore only this pending state after confirming on-site application, or transfer the same identity to its verified ATS. Genuine source closures are never reopened by this repair.
- A closed row keeps its `public_slug`, enrichment and `user_jobs` references; reopening is free.
- List, search and company surfaces filter `closed_at IS NULL`. Detail still serves a closed job (with `closed_at`) so links and history don't break.
- Catalogue sweeps exclude private postings, even when their source and age match a sweep.
- The ingest write stamps `last_seen_at` on every crawl (`RefreshUnchangedJob` when unchanged, `UpsertJob` otherwise). The post-run sweep closes a provider's jobs unseen for 48h. A reappearing posting reopens via the upsert.
- A board the run SKIPPED (failure cooldown, or the up-to-72h quiet pause of a board whose listing keeps coming back unchanged, see [internal/ingest/pipeline/AGENTS.md](../../internal/ingest/pipeline/AGENTS.md)) is excluded from the company- and source-scoped sweeps (`Stats.SkippedBoards`): its rows were not looked at, so their age proves nothing. The board-scoped close acts only on boards the run finished, so it never reaches one.
- Self-closing sources (`jobtech`, etc.) are excluded from the unseen sweep; the feed's `removed` events are the authoritative close signal.
- The liveness worker closes only on positive evidence (two consecutive `expired` reads) and never reopens.
- A source with no close signal at all (no re-crawl, no feed, a URL that outlives the vacancy) is closed by age: `telegram` today, at 45 days. It is the only close that rests on a guess.
- Every close records which mechanism wrote it in `closed_reason`; a reopen clears it. Rows closed before the column existed carry `''` (unknown) and are never relabelled.
- `cmd/close-chronic-boards`, separate from the ingest sweep, closes a board (or boardless provider) unreachable for a long time, not merely cooling down (`closed_reason = 'board_unreachable'`), and in a second pass a board whose crawls all SUCCEED but whose feed has carried nothing for a long time (`'feed_empty'`).
- Every flip of "listed" (open, canonical, public: `closed_at`, `duplicate_of` via its owner columns, `is_private`; and every insert) is appended to `job_changes` by a trigger (`jobs_log_change`, migration 0264), whatever wrote it. It is the source of `GET /jobs/changes`; the feed pages by (xid, id) and serves only entries below the oldest in-flight transaction, so a closer that commits late never lands behind a client's cursor. A new closer needs nothing; a new column that decides visibility must join the trigger's condition (migration 0264). The log is never pruned.
- An open posting its source has not shown for more than 14 days (`last_seen_at`) is **hidden from search, not closed** (see "The unseen safety net"). It stays open, its page serves, and it returns when its source lists it again.

## How it works

Observation state (`last_seen_at`, `hydrated_at`, `liveness_strikes`) is read from
`job_activity`, keyed by `jobs.id`. `jobs_read` combines it with posting content for
the existing `db.Job` shape. A migration that adds a `jobs` column and replaces
`jobs_read` must first `LOCK TABLE public.jobs_read IN ACCESS EXCLUSIVE MODE`, or it
deadlocks live readers of the view (guarded by `internal/platform/migrate`). An insert trigger creates activity even for raw SQL
inserts. Ordinary unchanged refreshes, open touches and covered refreshes update
only activity. Full posting writes use `job.Upsert`: write content first, then
observe activity with a second command in the same transaction. This preserves
the insert trigger's visibility and the lock order. Body hydration advances only
when a body was read; a substituted catalogue body preserves its previous stamp.
Older compatible writers still update `jobs`; a one-way bridge mirrors them into
activity without regressing timestamps until the contract release removes it.

Closers that depend on freshness lock `jobs` then `job_activity` with
`FOR NO KEY UPDATE` and recheck the observation after waiting. A plain join in an
`UPDATE jobs` does not recheck a concurrently refreshed activity tuple. The
activity `state_version` changes on lifecycle or cheap-match-key changes; reopening
resets strikes. A cheap write rejected by that version fence rolls back before
retrying in a fresh transaction. Covered refresh commits its bulk `SKIP LOCKED`
pass before retrying unwritten rows individually; an exhausted retry or timeout
fails coverage instead of silently permitting a sweep. The writer release must
wait for every pre-reader-release process to exit, including crawler and liveness
workers, and for external SQL readers to use activity.

Seven independent mechanisms write `closed_at`, each covering a gap the others can't reach. Six close on evidence; the age rule (4) guesses, which is why every close records its mechanism. The safety nets (5) and (6) read a board's history, not a posting: (5) nothing could reach the board for weeks, (6) everything reached it and found nothing.

**(1) Ingest sweep, (1a) the board-scoped close and (1b)–(1e)** — what `cmd/ingest` closes after a crawl, the coverage each scope needs, the crawl deadline, emptied and region-repeated boards, and the full-catalogue source sweep — are in [job-lifecycle/ingest-sweep.md](job-lifecycle/ingest-sweep.md).

**(3) Stream-driven self-close** (`CloseJobBySourceExternalID`): a self-closing source (`jobtech`/Arbetsförmedlingen, consuming an incremental change feed) emits `Job{Removed: true}` for a posting its feed reports taken down. `pipeline.ingestStream` routes it to the Store's optional `closer` (the ingest `dbStore`), closing by `(source, external_id)`. Such a source implements `selfClosing` and is excluded from (1) (`sources.UnseenSweepExemptProviders`): it re-reports only changed ads, so the `last_seen_at` cutoff would close every untouched open ad. Trade-off: a removal is sent only while inside the change window, so one the crawl misses is never sent again. The window absorbs a skipped cron, a run that cannot read its whole window reports the board failed, and `jobtech`'s misses are closed by its `cmd/liveness` pass (2b).

**(4) Age rule** (`cmd/liveness`, `CloseStaleUnsignalledJobs`): a `telegram` job's URL is the post, which stays live after the vacancy is filled, and no crawl re-visits it. It is closed once `COALESCE(posted_at, created_at)` is strictly older than 45 days, with `closed_reason = 'expired'`, and never reopens. The source list is passed in by the caller, not derived from "whatever the sweep misses", so a new adapter never drifts into age closes; it is the same list the probe excludes, keeping "what cannot be probed is expired instead" one decision. The exception is a telegram posting whose stored URL is itself a LinkedIn job view or a `*.myworkdayjobs.com` posting (a channel that relays the link): `unsignalledPass` routes it into the `linkedin` paced pass (guest closed marker) or the `workday` paced pass (CXS 403 `S22`), sharing that host's pace and per-run budget; the age rule stays its backstop. The plain GET probe is never used for these, since Workday and other SPA hosts answer it with a JS shell `Classify` reads as expired (freehire#4855).

**(2) Liveness probe, (2b) the scope-leak backstop and (2c) the age fallback** — what `cmd/liveness` closes for sources no sweep can prove, per source — are in [job-lifecycle/liveness.md](job-lifecycle/liveness.md).

**(5) Chronic-board safety net** (`cmd/close-chronic-boards`, issue #2017): (1) never closes what a run did not prove it covered, with no upper bound, so a permanently gone board (retired, blocking us, moved) would stay open forever. `board_health.ListChronicBoards` measures from `last_success_at`, or `first_seen_at` if it never succeeded (`last_error_at` is overwritten every failed run and cannot date the FIRST failure). Two windows: 30 days (`CHRONIC_BOARD_WINDOW_DAYS`) is what `cmd/ingest`'s per-run health summary surfaces for curation; 60 days (`CHRONIC_BOARD_CLOSE_WINDOW_DAYS`) is what `cmd/close-chronic-boards` closes. The gap is curator lead time. The close is scoped to the qualifying `(provider, board)`; a boardless provider's single health record closes the whole provider. A board id repeated across regions (`board_health`'s key is `(provider, board, region)`, e.g. Adzuna's `it-jobs` per country) is refused, not board-scoped: `jobs.external_id` carries no region, so a LIKE close could take a healthy region down with a dead one (the same refusal as (1a)'s `ambiguousRegionBoards`). It is reported as skipped. Ships dry-run-only on its daily timer until a human has read at least one full closure window's report.

The worker also has a separately armed `board_gone` pass for adapters that recognize the platform's explicit no-such-board answer (`sources.ErrBoardGone`). It requires at least three consecutive such failures spanning seven days (`BOARD_GONE_CLOSE_WINDOW_DAYS`), plus another board from the same provider succeeding in the last 48 hours. The three observations are stricter than the two expired reads used for a single posting by the liveness worker because a board can retire many postings at once. The seven-day span allows several crawl/recovery cycles (longer than board health's 24-hour maximum cooldown) while staying shorter than the 30-day successful-empty-feed and 60-day generic-unreachable waits: this path has typed platform-level disappearance evidence, unlike a failed request or a feed that merely returned no jobs. A recent successful sibling is an independent provider-health check; the separate dry-run and apply flag add an operator review before any close. Generic failures reset the gone streak; success resets it as well. This excludes 403/429, transport/adapter failures, provider-wide outages without a healthy sibling, and successful empty listings. The close statement repeats the checks and refuses region-ambiguous identities. It remains dry-run by default; `--apply-board-gone` arms only this pass and jobs receive `closed_reason = 'board_gone'`. The shared lifecycle path covers any provider adapter that returns `sources.ErrBoardGone`; this change adds that classification for Teamtailor's confirmed double-404 response. Providers without a typed board-gone signal, and providers without a separate healthy sibling board, do not qualify.

**(6) Empty-feed safety net** (`cmd/close-chronic-boards`' second pass, migration 0158): the twin of (5) for a board that answers perfectly and has nothing in it, which (5)'s `last_success_at` cannot see. On 2026-09-10 an emptied WhatJobs publisher account returned 200 with zero results every crawl, health clean, and 107 postings stayed open for a month. (1) could not reach them: `shouldSweep` requires at least one ingested job (right for a broken crawl, wrong for an honest zero), and nothing written means no company scope.

So `board_health.last_yield_at` records when a board last YIELDED a posting. **Being listed is not being written**: `pipeline.boardListedAnyPosting` counts ingested, rejected, already-covered, unreadable and failed-to-save postings alike. Stamping on the ingested count would call an all-rejected board (a non-tech-heavy national feed) or an aggregator board whose employers we crawl directly empty, and close live jobs. It is deliberately NOT `boardReachedPostings` (they differ by `Skipped` and `Unreadable`): there false costs one crawl cycle of proof, here false accumulates into a close, so this caller needs the generous predicate. Example: a link-only adapter whose detail requests all fail makes every posting `Unreadable` (not `Failed`); the board keeps a fresh `last_success_at` and would age into this net with every posting live.

`ListEmptyFeedBoards` selects boards demonstrably REACHABLE (succeeded inside the window, `consecutive_failures = 0`) that have not yielded for `EMPTY_FEED_CLOSE_WINDOW_DAYS` (default 30, shorter than (5)'s 60: (5) waits out a problem on our side, while an empty feed has already answered successfully every run). `'feed_empty'` is its own label because `'unseen'` claims proven coverage, `'board_unreachable'` would send an operator to check our access instead of the source's inventory, and `'feed_removed'` is a stream's per-posting notice. **A NULL `last_yield_at` never qualifies**: it means no yield was observed, not an empty feed. When the column landed that was true of all 155k rows, so migration 0158 seeds the fleet from `last_success_at`; the seed is load-bearing. Same region-ambiguity refusal and dry-run default as (5).

**(7) Board retirement** (`boardcatalog.Retire`, `CloseRetiredBoardJobs`): retiring a board (`cmd/add-board --retire`) closes its open postings in the same call, `closed_reason = 'board_retired'` (migration 0212). Nothing else would: no crawl runs for a retired board, so (1) never sweeps it; `Retire` deletes its `board_health` row, so (5) and (6) have no evidence; and its source is still registered, so (2) and (4) leave it to the sweep. aijobs (retired 2026-09-07) kept 2,569 postings open for eight weeks that way (freehire#3796). Re-retiring closes what an earlier retirement left open. Same region-ambiguity refusal as (5): while the board name is live in another region, nothing closes.

**The unseen safety net** (`search.Withheld`): every `last_seen_at` close waits for a run that PROVED coverage, so a board whose crawl keeps failing never sweeps. On 2026-10-26, 96,479 of 848,549 open canonical technical postings were unseen for over 14 days. Search withholds a posting unseen for longer than `search.UnseenWindow` (14 days), or the adapter's `sweepGrace` when wider. A filter, not a close: nothing is written, the page serves it, and a crawl that lists it again (any write stamps `last_seen_at`) brings it back. The rule is applied where a document is WRITTEN (`cmd/reindex` deletes it from the rebuild; `cmd/search-drain` and `linkimport` skip it) and mirrored by the facet recount in `company_facets.sql`' count and the similar-jobs list. Not a query-time filter on an indexed `last_seen_at`: `RefreshUnchangedJob` deliberately pushes nothing to `search_outbox`, so an indexed value would freeze at the last content write and hide live, unchanged postings. The cost is a few hours' latency either way (next full rebuild or content write). Per-source windows come from `sources.UnseenWindows`, which omits every source whose stale `last_seen_at` is not evidence: `selfClosing` adapters (re-report only changes), `livenessProbed` ones (re-list only the newest slice; a probe closes the dead: `hh`, `himalayas`, `echojobs`, `jobicy`, `remoteok`, `empregos`, `adzuna`), and sources with no adapter (`telegram`, employer vacancies, manual and pasted-link imports), which nothing re-crawls.

## Limitations

- A missed liveness cron run leaves orphans open until the next run.
- The probe is pure heuristics, so a 200 with a "position filled" message in an uncurated language or phrasing stays open.
- The age rule is a guess: a Telegram vacancy still open at 46 days is closed and nothing reopens it. `closed_reason = 'expired'` makes the set findable and restorable.
- Every mechanism queues the closed row's removal from the facet index in its own statement (`search_delete_outbox`), so close and de-indexing commit together. `TestEveryClosingQueryQueuesTheRemoval` enumerates them by a hand-written list; both `cmd/liveness` closes were missing from it, and from the queue, until 2026-09-06.
- The unseen safety net reads `last_seen_at` by SOURCE, so a row stored under a crawled provider's name that no crawl of it will list (a Telegram post linking to an uncrawled ATS board, a moderator import filed under its origin) is hidden 14 days after it was written, live or not. Its page still serves it.

### Boardless catalogue status backstops

The November 2026 audit found dead postings from `instaffo`, `remotedotcom` and
`thehub` outside the company-scoped sweep. Reed's bounded keyword union has the
same scope gap and is covered for its own stored Reed posting URLs; employer
redirect URLs are deliberately excluded. None claims `fullCatalog`, so when an
employer leaves the listing its last postings never re-enter that sweep's scope.
`cmd/liveness` includes their open postings unseen for more than 48 hours. Each
source is paced at two seconds per request, capped at 500 postings per run, and
stops on 403/429. Only a 404/410 on the same host and posting path adds a strike;
a redirect to a listing or another host is uncertain. The ordinary two-run close
threshold, crawl-race cutoff and transactional search-delete enqueue apply.
Search's unseen safety net stays enabled while the backlog is worked off.

For a scoped cleanup after deploying the worker, run `liveness -source=instaffo`,
`liveness -source=remotedotcom`, `liveness -source=thehub` or
`liveness -source=reed`. Repeat in a later
run rather than stacking invocations: two independent expired reads close a
posting with `closed_reason = 'probe_expired'`. Never treat an audit sample or
its canary alone as closure evidence.
