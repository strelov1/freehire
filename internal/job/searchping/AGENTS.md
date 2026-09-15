# internal/job/searchping

Announcing a posting's public URL to the external search engines that accept being
**told**, instead of waiting to be crawled. Drained by `cmd/search-ping`, hourly.

**Block: `job` (layer 5).** It reaches no further than `platform`. It is here and not in
`search` because `search` is OUR index — Meilisearch, the drain, saved searches — while
this is a fact about a posting's public address.

## Why it exists

Waiting does not work at this catalogue's size. Measured on prod, 2026-09-14:

| | |
|---|---|
| distinct `/jobs/<slug>` pages Googlebot fetched that day | **65** |
| job pages the sitemap declares | ~690,000 |
| new technical postings published per day | ~14,000 |
| Search Console URL Inspection on postings sampled from the sitemap | `URL is unknown to Google` |

`URL is unknown to Google` is not "rejected" or "low quality" — it is *never fetched*. A
posting is closed as soon as the employer's own listing disappears, so a page first
crawled months later is stale before it is reachable.

## The budget is the design, not the plumbing

Google's Indexing API grants **200 publish calls a day** by default against ~14k new
postings a day. Everything else follows from that.

**A ledger, not an outbox.** Every other queue in this schema (`search_outbox`,
`recent_feed_outbox`, `enrichment_outbox`) drains faster than it fills. This one cannot.
An outbox fed by `cmd/ingest` would grow without bound forever and its oldest row would
never be reached. `job_search_pings` (migration 0162) records what was **sent**, so each
run chooses the newest eligible postings afresh and the choice can change without a
backlog to unwind.

**The remaining allowance is read back from the ledger**, because the API offers no way
to ask: it answers `429` when the day is spent and nothing before that. The timer fires
hourly, so a run that assumed it had the whole allowance would spend it again every hour.

**The budget day ends at midnight Pacific**, where Google resets it — not UTC and not the
host's clock. Measured in UTC, a run just after 00:00 UTC would spend an allowance Google
still counts against yesterday, for most of the year.

**Newest first is the whole selection policy**, and it does two jobs. A posting is worth
announcing while it is still open, and the budget is far smaller than the catalogue. It
also stands in for a filter this package cannot apply: the sitemap additionally excludes
the `likely-evergreen` reality class, which lives only in the search index, but that
class is *earned by staying open a long time*, so the newest rows have not had the chance
to qualify. Google adjusts the quota by the quality of what is submitted, which is why
the divergence is named rather than left to be discovered.

## The two events, and which wins

A posting is announced twice: once when it appears (`created`), once after it closes
(`closed`); a company page once (`company`, below). **Both are `URL_UPDATED`.** A closed posting's page stays at HTTP 200 and
keeps its `JobPosting` markup with `validThrough` moved into the past, which is one of the
three ways Google documents for retiring a posting — so a closure needs a *re-crawl*, not
a deletion. Sending `URL_DELETED` for a page that is still online is a misuse of the API,
and the API's penalty is the quota.

**New postings take the budget first.** The two events share one engine's daily
allowance, and the order is the policy: a new posting brings a visitor, a closure only
tidies an index we do not own. While the allowance is 200/day against ~14k new postings,
the first pass consumes all of it and the second does nothing — which is correct.
Closures start flowing when the allowance grows, with no code change.

An **unbounded** engine gives each pass a full batch instead of the leftovers, or
IndexNow — which has no quota at all — would silently stop announcing closures the moment
new postings filled one batch.

The closure query only considers postings this engine was **already told about**:
announcing the closure of a page an engine never heard of teaches it a dead URL and spends
budget doing it. That also bounds the candidate set by construction.

## The company page is the asset, and only IndexNow may hear about it

Measured 2026-09-15 through the Bing Webmaster API — the first look this repository has
ever had at Bing's side:

| | |
|---|---|
| pages Bing holds in its index | **255,038**, climbing ~6k/day |
| its highest-impression pages | `/companies/<slug>` — laserfocus, astra-tech-labs, truebiz, read-bean |
| the query shape behind them | **"<company name> careers"** |
| bingbot's crawl, two days | 6,275 company pages vs 3,679 job pages |

Google's own query data from early August says the same thing (`princess cruises
careers`, `techno brain careers`). Two engines, independently, agree.

The reason is structural: a job page's text belongs to the employer and exists in a dozen
other copies, while a company page is OUR assembly — every open role of one employer in
one place — which the employer often does not publish anywhere.

**But Google may not be told.** Its Indexing API admits only `JobPosting` and
`BroadcastEvent` pages, so a company page sent there is a terms violation whose penalty
is the quota. `Engine.Accepts` is where that fact lives: Google declines `KindCompany`,
IndexNow takes everything, and the runner skips a refused pass without reporting it — a
line reading `google/company: nothing to announce` would look like an empty catalogue
rather than a rule.

Eligibility is `companies.job_count > 0`, the same gate that puts a company in the
sitemap, so a page announced here is exactly a page the site already claims. Newest
first, and deliberately **not** by `job_count`: the company pages actually ranking are
the long tail, because for a small employer this page may be the only assembled list of
its roles while a large one's own careers site already owns that query.

Company pings have their own ledger (`company_search_pings`, migration 0164) and **no
kind column** — a company page has one event. It gains and loses postings continuously
and its URL never dies of it: a company whose postings all close keeps a 200 page.

## The two engines

| | Google Indexing API | IndexNow |
|---|---|---|
| Reaches | Google | Bing, Yandex, Seznam, Naver |
| Budget | 200/day (raisable on request) | none published |
| Credential | service account, **owner** in Search Console | a public key served from the site |
| Batching | one URL per call; batching saves HTTP, **not quota** | up to 10,000 per call |
| Partial failure | normal | impossible — it answers for the list at once |

`Announce` therefore takes a batch and returns the URLs the engine **accepted**. The
ledger records what an engine *took*, never what it was offered: a partial batch recorded
in full silently drops the postings that were not sent, and they are never selected
again. The accepted prefix is recorded even when the batch as a whole failed — a send
that happened and was not written down costs the budget twice.

**The send is at-least-once, and that direction is chosen.** An HTTP call cannot join
the transaction that records it, so one of the two orderings has to lose. Sending first
and failing to record costs a duplicate announcement — bounded, surfaced in the run's
error, harmless to the engine. Recording first and failing to send would cost the posting
its announcement permanently and silently, because a recorded row is never selected
again. The loud, bounded failure is the one to keep.

**The budget day needs the real zone, not a fixed offset.** During DST a fixed `-8`
standing in for Pacific puts the boundary an hour late, so pings sent in that hour go
uncounted while Google counts them, and the run reads more allowance left than it has. A
fixed `-7` fails the same way in winter. `time/tzdata` is imported so this cannot depend
on whether the host carries a zone database.

**Eligibility lives in the SQL**, beside the query, not in a caller. The API's terms admit
only `JobPosting` and `BroadcastEvent` pages, and the penalty for anything else is the
quota itself.

## Two traps worth naming

**`siteFullUser` is not `siteOwner`.** A service account added to Search Console with
"Full" permission can read every report and cannot publish: the Indexing API answers
`403 Permission denied. Failed to verify the URL ownership.` Nothing in that message says
"change the role".

**The IndexNow key lives in two places** — `web/static/<key>.txt`, which the site serves,
and `INDEXNOW_KEY`, which the worker sends. IndexNow's answer to a mismatch is a `403` on
every submission, which reads like a broken integration rather than two copies that
drifted apart, so `VerifyKey` fetches the file once at startup and refuses to run if they
disagree. The key is **public by design** — it is ownership proof, not a secret, which is
why the file is checked in.

## Rolling it back

Clear `GOOGLE_INDEXING_KEY_FILE` and `INDEXNOW_KEY`. An engine with no credential is not
configured and is skipped without error; with neither, the run is a no-op that never
opens the pool. The timer can stay enabled.
