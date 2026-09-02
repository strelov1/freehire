# Job lifecycle conventions

## Scope
The open/closed state of a job row, the four mechanisms that write `closed_at`, and the filtering semantics that depend on it.

## Always true
- A job is open while `closed_at IS NULL`. Closing is a soft state, and the lifecycle never deletes.
- The one exception is `cmd/prune` (catalogue pruning), which hard-deletes jobs that do not belong on an IT job board. It is a deliberate, operator-driven campaign, not part of the lifecycle: `closed_at` keeps meaning "the employer took this down", and "not our profile" is expressed by the row being gone and archived in `pruned_jobs`. Overloading `closed_at` with the second meaning would corrupt a signal three mechanisms already write.
- A closed row keeps its `public_slug`, enrichment, and `user_jobs` references, and reopens for free.
- List, search, and company surfaces filter `closed_at IS NULL`. Detail still serves a closed job (with `closed_at`) so links and history don't break.
- The ingest write stamps `last_seen_at` on every crawl — `RefreshUnchangedJob` when the posting is unchanged, `UpsertJob` otherwise; the post-run sweep closes a provider's jobs unseen for 48h, scoped both to the companies the run wrote and to the boards it proved it covered.
- A reappearing posting reopens via the upsert.
- Self-closing sources (`jobtech`, etc.) are excluded from the unseen sweep — the feed's `removed` events are the authoritative close signal.
- The liveness worker closes only on positive evidence (two consecutive `expired` reads) and never reopens.
- A source carrying no close signal at all — no re-crawl, no feed, and a URL that outlives the vacancy — is closed by age instead: `telegram` today, at 45 days. It is the only close that rests on a guess.
- Every close records WHICH mechanism wrote it in `closed_reason`; a reopen clears it. Rows closed before that column existed carry `''`, meaning unknown, and are never relabelled.

## How it works
Closing is a soft state on one column (`closed_at`) written by four independent mechanisms, each covering a gap the others can't reach. Three of them close on evidence; the fourth, the age rule, closes on a guess, which is why every close now records which one wrote it.

One interaction with catalogue pruning is worth knowing. Once ingest starts rejecting a board's non-technical postings, the ones already stored stop being seen and the unseen sweep closes them after 48h — so `closed_at` fills with rows the campaign is about to delete. That is why `cmd/prune`'s scan covers closed rows: a scan over open jobs only would leave exactly the rows nothing will ever replace.

**(1) Ingest sweep** (`cmd/ingest`, `CloseUnseenJobs`): for board sources, the ingest write stamps `last_seen_at` every crawl — the cheap `RefreshUnchangedJob` on the common path, `UpsertJob` for a new, edited or reopening posting. The post-run sweep closes a provider's jobs unseen for 48h — if a posting drops off a board we crawl, it closes. A reappearing posting reopens via the upsert (the `ON CONFLICT` path clears `closed_at`). The sweep runs per provider, and only for providers that ingested at least one job, so a total crawl outage can't mass-close a catalogue. It is scoped two ways, and a job closes when EITHER scope covers it. **By company_slug** — the slugs the run actually wrote — so a partial or targeted run closes only the companies it saw. **By board** (`CloseUnseenJobsForBoard`) — the boards the run PROVED it covered — because the company scope is derived from postings written and therefore cannot tell "company gone" from "company not reached this run": a company whose LAST posting leaves a board never re-enters the crawled set, so nothing closed its row, ever. Measured on prod 2026-09-02, 235,313 open rows sat on boards that had been crawled successfully after the posting was last seen (freehire#2328).

A run proves it covered a board when the crawl did not fail, the board yielded at least one posting (counting ones the catalogue filter rejected and the coverage gate skipped — the crawl reached them either way), and the entry names a board. The yield condition is the load-bearing one: a board that lists NOTHING cannot be told apart from a board whose crawl broke, which is how a Workday board reporting `total:0` on its second page once had its live tail retired (freehire#725). A boardless entry is refused outright — its postings carry an empty namespace, so a board-scoped match would select the provider's whole catalogue. A provider that declares a wider sweep grace is excluded entirely, since that marker means the crawl reaches only a SLICE of the catalogue.

What still leaks: boardless entries, boards that yielded nothing, and boards that have left `sources/` altogether (a board that never runs again never proves anything). A recency-budgeted aggregator (`himalayas` pages only its freshest slice per run — see `internal/ingest/sources/himalayas.go`) is boardless and leaks this way routinely: any company that ages out of that window stops appearing in *any* future crawl. Mechanism (2) backstops those.

`CloseUnseenJobs`/`CloseUnseenJobsBySource` are single-statement bulk `UPDATE`s, so one row Postgres can't write (a corrupted index entry — see the 2026-08-11 incident, one duplicated `jobs_pkey` value) aborts the whole statement, leaving *every* closeable row in the provider open, silently, run after run. `cmd/ingest`'s `sweepRowByRow` is the fallback: on a bulk failure it re-fetches the same candidate set (`UnseenJobIDs`/`UnseenJobIDsBySource`) and closes each row in its own statement (`CloseUnseenJobByID`), logging and skipping whichever id still can't be written instead of blocking the rest.

**(1b) Full-catalogue source sweep** (`CloseUnseenJobsBySource`): a *full-catalogue* aggregator (`sources.fullCatalog` marker, e.g. `habr_career`, `geekjob`) lists its whole catalogue every run, so an unseen job is genuinely gone — including the vanished-company case (1) leaks. For such a provider the sweep drops the `company_slug` scope and closes by source alone. Sound **only** because a full-catalogue adapter errors a *truncated* crawl instead of returning it as a partial success (`habr_career` fails any bad listing page; `geekjob` uses `crawlAllPagedLinks`): `cmd/ingest` gates the source-scoped close on a zero-`Failed` run (`sweepBySource`), so a mid-listing failure (`Failed>0`) falls back to the safe company-scoped close rather than mass-closing every posting past the failed page.

**(3) Stream-driven self-close** (`CloseJobBySourceExternalID`): a *self-closing* source (a streaming aggregator like `jobtech`/Arbetsförmedlingen that consumes an incremental change feed) emits a `Job{Removed: true}` for a posting its feed reports taken down. `pipeline.ingestStream` routes that to the Store's optional `closer` (the ingest `dbStore`), closing by `(source, external_id)`. Such a source implements the `selfClosing` marker and is excluded from the (1) unseen sweep (`sources.SelfClosingProviders`): it re-reports only changed ads, so the sweep's `last_seen_at` cutoff would wrongly close every still-open ad it did not touch. Trade-off: a missed run can leave an orphan open until a future reconcile; the change window is sized wide enough to absorb a skipped cron.

**(4) Age rule** (`cmd/liveness`, `CloseStaleUnsignalledJobs`): the probe cannot judge every orphan. A `telegram` job's stored URL is the post, which stays live after the vacancy is filled, so no fetch of it can ever read as dead — and no crawl re-visits it either. Such a source has no evidence to appeal to, so it is closed once its effective posting date (`COALESCE(posted_at, created_at)`) is older than 45 days, with `closed_reason = 'expired'`. The source list is passed in by the caller rather than derived from "whatever the sweep misses", so a new adapter can never drift into being closed by age; it is the same list the probe excludes, which keeps the two halves of one decision — what cannot be probed is expired instead — from separating. Strictly older than the cutoff, and it never reopens, because nothing re-crawls these rows.

**(2) Liveness probe** (`cmd/liveness`): board sources are not the whole catalogue — jobs from sources not in the `sources.All` registry (manual/`resolve-url` imports and the like) are never re-crawled, so the sweep can't reach them. (Aggregators like `habr_career`/`geekjob` *are* registered providers, swept by (1)/(1b) and excluded from the probe; `telegram` is excluded too, via `unsignalledSources`, because its URL outlives the vacancy — mechanism (4) closes it instead.) The liveness worker URL-probes those orphans, classifies the page via `internal/job/liveness` (pure heuristics — HTTP 404/410, error/listing redirect, curated EN/DE/FR closed-posting phrases, or near-empty content — no browser, no LLM), and closes a job after two consecutive `expired` reads (the `liveness_strikes` counter; any healthy probe resets it). It closes only on positive evidence and never reopens, biasing toward under-closing (an orphan has no re-ingest to reopen it). Run-once-and-exit, cron-scheduled.

**(2b) Backstop for a registered provider's scope leak** (`probeDespiteRegistered` in `cmd/liveness`): a source stays a normal registered provider — the sweep still owns its regular closes — but is *also* added back as a candidate, restricted via `SelectStaleRegisteredCandidates` to jobs already past the sweep's own 48h staleness window (mirroring `cmd/ingest`'s `staleAfter`), so it only ever picks up what (1) is structurally unable to reach rather than racing it. This is the shape shared by every boardless, recency-ordered aggregator whose crawl budget (a page cap, an offset ceiling, or a freshness window) is smaller than its live catalogue — `himalayas`, `echojobs`, `jobicy`, and `remoteok` are members as of this writing.

Each member needs its own evidence path — there is no generic per-source plugin here, by design, since a GET probe is not guaranteed to work for any given source (`probeDespiteRegisteredGET` in `cmd/liveness/main.go` is the subset that *is* verified this way):
- `jobicy`, `remoteok`: the plain-GET probe already used for (2) works as-is — jobicy's job page answers a definitive `410` for a removed posting; remoteok answers `200` with a detectable "this job post is closed" banner in the body (`internal/job/liveness`'s `hardExpired` patterns).
- `himalayas`: its job page is Cloudflare-blocked (403 on every plain GET, verified from both a dev machine and prod, even with a browser User-Agent). `cmd/liveness/himalayas.go` instead downloads `himalayas.app`'s own sitemap (unprotected, ~114k URLs across 3 gzipped shards discovered from a `sitemapindex`) once per run and checks each candidate's URL for membership — real evidence (the site's own record of what is still live), not a guess, and one run-wide fetch instead of one request per candidate.
- `echojobs`: its stored `jobs.url` is the *employer's own* ATS link (Workday, Greenhouse, …), not an echojobs.io page — probing it inherits the reliability of whichever of ~330k postings' employer happens to be up. `cmd/liveness/echojobs.go` instead GETs echojobs.io's own server-rendered job page, keyed on the posting's `external_id`, and reads the plain HTTP status: `200` means still listed, `404`/`410` means removed — the same pair `internal/job/liveness`'s shared classifier treats as "gone" — and anything else is unverifiable rather than a false death signal. (An earlier version of this probe hit echojobs.io's JSON detail API and matched a distinguishable error body at `HTTP 500`; that API was retired and the probe moved to the page GET.)

**(2c) Age fallback for a registered provider with no evidence at all** (`expireDespiteRegistered` in `cmd/liveness`): same company_slug/keyword-scope leak as (2b), but for a source (2b)'s "find *some* evidence" approach cannot serve at all. `whatjobs` is the member as of this writing: its `jobs.url` is the CPC ad network's own billing/tracking landing page, not the employer's posting, so it answers identically whether the underlying posting is live or long gone (see `internal/ingest/sources/whatjobs.go`). Rather than invent a signal that doesn't exist, `cmd/liveness` reuses mechanism (4)'s own query (`CloseStaleUnsignalledJobs`) and 45-day `expiryWindow` against this source too — the same "what cannot be probed is expired instead" guess, just applied on top of a source the sweep otherwise closes on real evidence via its own extended `sweepGrace`. Kept as a separate list from `unsignalledSources` because the two guards are opposite: an `unsignalledSources` member must NOT be a registered provider (the age guess is its ONLY closer), while an `expireDespiteRegistered` member MUST be one (the sweep still owns the common case; age only backstops the tail it structurally can't re-reach).

## Limitations
- A missed liveness cron run leaves orphans open longer; no reconciliation beyond the next run.
- The liveness probe uses pure heuristics (no browser, no LLM) — a posting that returns a 200 with a "position filled" message in a language or phrasing not in the curated set stays open.
- Self-closing sources trade missed-run safety for feed-accuracy: a skipped cron leaves orphans open until the next run's change window catches up.
- The age rule is a guess, not a verdict: a Telegram vacancy still genuinely open at 46 days is closed anyway, and nothing reopens it. `closed_reason = 'expired'` is what makes that reversible — the rows it closed can be found and restored as a set.

## Catalogue pruning (cmd/prune)

A separate, operator-driven campaign that permanently removes jobs which do not belong on an IT job board — roughly 1.5M of a 3.5M catalogue. It is the only hard delete in the system.

**The loop.** `cmd/mine-titles` reports the word groups still carrying no `is_tech` signal, an operator picks the next real cluster, its anchored terms go into `classify.nonTechTitleTerms` by PR, and `cmd/prune` removes what the dictionary now recognises. Ingest applies the same dictionary, so what is removed does not come back.

**Three rules, and the boolean that gates them.** A posting's board is either still in `sources/*.yml` or not, and the two rule families read that in opposite directions:

| rule | needs the board | why |
|---|---|---|
| `title` — the non-tech dictionary recognises it | **listed** | a listed board is re-crawled, so withdrawing an over-broad term brings the postings back; an unlisted board makes the same deletion permanent |
| `business_at_nontech_company` | **absent** | no counterpart at crawl time, so what it removes returns within the hour unless the board is gone |
| `unknown_at_empty_company` | **absent** | same |

The board is resolved from `external_id` (`"<board>:<native id>"`), never from the company slug: many adapters take the company name from the posting payload, so on `jazzhr` only 2453 of 3940 companies match their board file and on `careerplug` only 71 of 8014.

**Gates.** `--apply` is required to delete anything and demands an explicit `--limit`. A dry run prints a random sample of matched titles plus a breakdown by rule and by source — a batch dominated by one board is a broken board title, not a real cluster. Every removal is archived to `pruned_jobs` with the rule that matched.

**Retiring a board.** A board is retired by MOVING its entry from
`sources/<provider>.yml` to `sources/retired/<provider>.yml`, never by deleting the
line. Ingest takes one board file by path and `cmd/prune` globs `sources/*.y*ml` without
descending, so an entry there is neither crawled nor counted as live — the retirement is
expressed by where the line lives, and a mistake is undone by moving it back. See
`sources/retired/README.md`.

**Operational notes.**
- Migration `0041_pruned_jobs.sql` must be applied to prod by hand before the first run.
- `--apply` refuses to start without Meili configured: deleting rows the index keeps serving would 404 every result. Both the facet and the semantic index are mirrored.
- A run that fails partway leaves earlier batches committed. `SELECT rule, count(*) FROM pruned_jobs WHERE pruned_at > $since GROUP BY rule` is the durable record; re-running is safe, since deleted rows no longer match.
- Prune a provider's boards *before* moving its last entry to `sources/retired/`. Once a provider has no entries left in `sources/`, none of its jobs are re-crawlable and every rule refuses them — the dead weight becomes permanent.
- End of campaign: one `cmd/backfill-derive` to resynchronise `is_tech` on survivors, then one `make reindex`. `is_tech` is absent from `content_hash`, so a flip on a surviving row does not reach the index on its own.
