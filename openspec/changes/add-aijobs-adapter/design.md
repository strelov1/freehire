## Context

aijobs.net has no public JSON API for its listing — it is server-rendered HTML behind a
Django CSRF-protected `POST /?page=N` endpoint (confirmed live via manual `curl`, effectively an
inline feasibility spike, see proposal.md). A listing card carries no company name and no
description; both live only on the per-job detail page, and the detail page itself masks the
real company name behind a "PRO" paywall (rendered as `@ M...`), recoverable only from the
`/company/<slug>-<id>/` href beside it. The catalogue is ~47k postings and grows continuously.

This shape — boardless, multi-company, cheap listing / expensive per-posting detail, first-run
backlog far larger than any single run should fetch — already has two precedents in
`internal/sources`:
- `justjoin.go` implements `HydratingSource.FetchNew(ctx, e, seen)`, where `seen(externalID)`
  is a pipeline-supplied predicate ("is this posting already in the catalogue") so detail
  fetches happen only for postings the catalogue does not already have.
- `4dayweek.go` fetches every posting's detail page just to read whether the description is
  paywall-locked, and is scheduled on a sparse (daily) cadence specifically because of that
  per-posting fetch cost.
- `bayt.go` hand-rolls its own "stop when a page adds no new link" pagination loop (rather than
  the shared `crawlPagedLinks` helper in `html.go`, which none of the HTML-based adapters
  actually call) and fans detail requests out through the shared `fetchDetails` bounded-worker
  helper (`internal/sources/helpers.go`).

aijobs.net combines all three pressures at once, at a larger scale than any existing adapter
(47k vs. justjoin's ~20k), so the design borrows each precedent rather than inventing a new one.

## Goals / Non-Goals

**Goals:**
- Crawl aijobs.net's listing and yield normalized `Job` records for postings not already in
  the catalogue, fetching each new posting's detail page exactly once.
- Bound both dimensions of cost a large, continuously-growing aggregator creates: how many
  listing pages one run walks, and how many detail pages one run fetches.
- Reuse the CSRF/cookie session for the whole run rather than re-authenticating per page.

**Non-Goals:**
- Solving cross-source company-identity matching. The adapter derives a display name from
  a URL slug; whether that string collides with an existing company row is entirely the
  existing `role_fingerprint` dedup's job (see proposal.md's Impact section) — this change
  does not touch `internal/jobdedup`.
- Recovering the paywalled original employer name or apply URL. Out of reach without a PRO
  account; not attempted.
- Parsing or storing salary. The site's figures are labelled "(estimate)" — aggregator-computed,
  not employer-provided — and storing them would misrepresent the salary facet's provenance.
- Wiring the production cron cadence for `sources/aijobs.yml`. That is an ops-repo deploy step,
  outside this repo's change.

## Decisions

**1. `HydratingSource.FetchNew`, not a plain `Fetch`.**
Like `justjoin`, the adapter implements both: `FetchNew(ctx, e, seen)` is the real path
`cmd/ingest` drives — the listing walk always runs, but the per-job detail `GET` is skipped
for any external ID `seen` already reports as ingested. `Fetch` is the fallback for a caller
that cannot supply a `seen` predicate, but unlike `justjoin`'s (genuinely list-only, since its
list carries company inline) it has nothing list-only to fall back to: aijobs's listing carries
no company, and a company-less posting is dropped (Decision 5), so `Fetch` delegates to
`FetchNew` with a predicate reporting everything unseen — hydrating everything the listing
yields, bounded by the same per-run budget as a real crawl (Decision 3). `HydratingSource` is
still the right mechanism for the "only fetch what's new" half of the problem — `Fetch` existing
at all is just interface plumbing this adapter has little use for.

**2. Listing pagination stops on the first page that is entirely already-seen, not just
on a page with no within-run-duplicate links.**
aijobs.net's listing is sorted newest-first ("8h ago" at the top). `bayt.go`'s inline
loop stops when a page adds nothing *new to this walk* (a within-run dedup set) — that
guards against an infinite loop but still re-walks all ~956 pages every run forever, since
every page's links are "new to this walk" even if every one of them is already in our
catalogue. Because the feed is chronologically sorted, once a whole page is composed of
IDs `seen` already reports as known, every following page is older still — so the walk can
stop there. This turns steady-state runs (after the first backlog fill) into a handful of
pages instead of ~956, at the cost of one extra `seen` check per listing-page link (cheap,
in-memory once the pipeline has loaded the provider's seen-set — see
`internal/pipeline/AGENTS.md`'s seen-set note).
Alternative considered: cap the listing walk to a fixed `aijobsMaxPages` every run
(mirroring `baytMaxPages`/`justJoinMaxPages`). Rejected as the *sole* stop condition — sorted
aggregators like `bayt` and `justjoin` don't get this shortcut because their feeds aren't
reliably date-sorted end to end; aijobs.net's is, so not using it would mean either an
unbounded per-run page count or an arbitrary cap that eventually falls behind the real
posting rate. Kept as a secondary safety cap (`aijobsMaxPages`) in case the sort order or
markup ever silently changes.

**3. `AIJOBS_MAX_NEW_PER_RUN` caps detail-page fetches (default 500), separate from the
page cap.**
The "stop at the first fully-seen page" rule (Decision 2) only helps once the catalogue has
caught up. On the very first run — and after any outage long enough for the backlog to grow
past one run's budget — most or all of the ~47k postings are unseen, and each needs one
detail `GET`. Without a separate cap, `FetchNew` would try to fetch all of them in one run.
The cap is applied inside `FetchNew`: once `AIJOBS_MAX_NEW_PER_RUN` unseen postings have been
queued for detail fetch, the walk stops issuing new detail requests (and, since a "stop"
here is not a listing failure, the page walk itself can also stop at that point — no reason
to keep discovering links this run won't fetch). The next run resumes from the front of the
listing; because still-unfetched postings from this run remain unseen, they are naturally
rediscovered (Decision 2's early-stop does not trigger until the catalogue is caught up).
This mirrors `APPLY_FORM_MAX_PER_RUN` in `cmd/capture-apply-form` (same shape: a backlog
far bigger than one run should take, capped per run, self-resuming next run).

**4. CSRF/session flow: cookie-jar client + one new `PostFormWithHeaders` helper.**
Confirmed live: a plain `GET https://aijobs.net/` sets the `csrftoken` cookie; every
subsequent listing `POST` must echo that same value as both the `x-csrftoken` header and the
`csrfmiddlewaretoken` form field, and must carry a `Referer` header (confirmed required —
its absence gets a `403 CSRF verification failed` naming the missing Referer specifically,
not a generic auth failure). No separate token-scraping step is needed (unlike `taleo.go`'s
`portalNo`, which is embedded per-page and must be regexed out) — the cookie value itself
*is* the token, so this is simpler than taleo's flow despite looking superficially similar.
`internal/sources/http.go` already has `newCookieClient` (cookie-jar-backed `*Client`, built
for taleo) and `PostJSONWithHeaders`, but every POST helper today JSON-encodes the body; none
send `application/x-www-form-urlencoded`. Adding `PostFormWithHeaders(ctx, url, headers,
values url.Values) (*html.Node, error)` to `http.go` (parallel in shape to
`PostJSONWithHeaders`, but form-encoding the body and returning parsed HTML like `GetHTML`
rather than decoding JSON) is the minimal addition; the adapter is built with
`newCookieClient()` so the bootstrap `GET`'s `Set-Cookie` carries into every paginated `POST`.

**5. Company name: title-cased URL slug, not the raw slug.**
The `companyname` package's convention — leave a raw slug as `Job.Company` and let
`cmd/backfill-company-names` fix it later — assumes a resolver exists for that source
(`companyname`'s `Registry` only covers Pinpoint/BambooHR/Lever/Ashby title-scraping today).
aijobs has no such resolver and, given its own company name is paywalled, could never grow
one. Left raw, the slug would stay squished (`medison-pharma`) forever, degrading the UI
label and breaking `logo.dev`'s name lookup (per `companyname`'s own documented failure
mode) with no future fix path. Title-casing the hyphen-split slug at ingest
(`medison-pharma` → `Medison Pharma`) is a one-line improvement over a dead end, accepting
that it is a heuristic (won't recover legal suffixes, stylized capitalization like "iRobot",
or acronyms) — the same accepted imprecision `bayt`/`whatjobs` already carry for their
aggregated company names.

**6. Description is assembled from the page's own structured "Tasks" section, not free
prose — because there is no free prose.**
aijobs.net itself pre-processes every posting into structured sections (`<h5>Tasks</h5>` +
`<ul><li>`, `<h5>Skills/Tech-stack</h5>`, `<h5>Perks/Benefits</h5>`, etc.) before serving it;
there is no original-text description field anywhere on the page (confirmed: no
`application/ld+json` block, no prose container). `Job.Description` is built by rendering
the Tasks `<li>` items as a bullet list. `Job.Skills` is populated directly from the
Skills/Tech-stack section's anchor texts (the `Job` struct already has a dedicated
`[]string` field for this — no reason to also duplicate skills into the description text).
"Perks/Benefits" is skipped when its only item is the literal string `N/A` (observed on
every sampled posting with no real perks) so an empty section doesn't add "N/A" noise to
every job.

**7. `PostedAt` from the relative-time string ("Found Xh ago" / "Xd ago"), parsed
generically.**
Only `h` (hours) and `d` (days)-suffixed values were observed live, but the site's own
faceted URLs and copy suggest broader units exist for older postings. The parser handles a
generic `\d+[a-z]+ ago` shape rather than hardcoding just `h`/`d`, so a `w`/`mo`/`y` value
degrades to "parsed with an approximation" rather than "silently dropped." **The exact
month/year approximation (calendar-aware `time.AddDate` vs. a flat 30/365-day duration) is
called out as its own task in tasks.md**, left for the user to write.

## Risks / Trade-offs

- **[Risk] Some duplicates against directly-crawled ATS sources will not collapse**, because
  cross-source dedup only clusters postings sharing a company row, and aijobs' company name
  is a derived slug rather than a scraped-verbatim string. → **Mitigation**: none attempted in
  this change (explicit, discussed trade-off — see proposal.md); the existing
  `role_fingerprint` clustering still catches what it can, same as it does for `bayt`/`whatjobs`
  today, and a surviving duplicate is a harmless extra catalogue row, not a user-visible one
  (search still shows one canonical result once/if clustering matches it).
- **[Risk] `Job.URL` points at aijobs.net's own page, not the employer's original posting**,
  because the real apply link is paywalled and its own internal redirect loops back
  unauthenticated. → **Mitigation**: this is the same shape several existing aggregators
  already have; freehire's job page still resolves to a working apply flow (aijobs.net's own
  page), just not the originating ATS.
- **[Risk] The "stop at the first fully-seen page" pagination shortcut (Decision 2) assumes
  the listing stays strictly newest-first.** If aijobs.net ever changes its default sort, the
  walk could stop early and silently miss postings inserted out of order. → **Mitigation**:
  `aijobsMaxPages` remains as an independent safety cap either way, and `board_health`'s
  `last_ingested_count` makes a sudden drop to near-zero visible in the existing per-board
  health monitoring — no new observability needed.
- **[Trade-off] Title-cased company names are a heuristic, not authoritative** (Decision 5) —
  accepted, same class of imprecision already live for `bayt`/`whatjobs`.

## Migration Plan

Additive only: a new provider key, a new board file, a new HTTP helper. No schema change, no
existing adapter touched. Rollback is deleting `sources/aijobs.yml` (or removing `"aijobs"`
from the registry) — `cmd/ingest` simply stops being pointed at the board file; already-ingested
jobs are unaffected (the lifecycle sweep only closes a provider's jobs when that provider's
crawl runs and stops seeing them, so removing the board file freezes those rows rather than
closing them). Wiring the actual cron schedule in the ops repo is a separate, later step.

## Open Questions

- None blocking. The relative-time unit-conversion granularity is deliberately left as an
  explicit implementation task (tasks.md) rather than resolved here, per the user's request.
