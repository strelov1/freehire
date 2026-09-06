## Why

Adzuna is the catalogue's largest single source — **301,008 open postings, 4.34% of the
6,940,260 open catalogue** — and the crawl that fills it spends its entire request budget
re-reading inventory it already has.

Measured live on 2026-09-06, against `gb/it-jobs`:

| request | reported total | first result's `created` |
|---|---:|---|
| what the adapter sends today | 52,942 | **2026-02-10** |
| the same request with `sort_by=date` | 52,941 | 2026-09-06T12:34 |

The adapter has never sent a sort order, so Adzuna answers in *relevance* order, which is
stable between runs. The crawl walks 40 pages of it every hour and gets substantially the
same ~2,000 postings each time, the oldest of them seven months old.

What that costs, and what it returns:

- **~470 requests/day** — roughly 14,000/month observed, against Adzuna's stated ceiling of
  **250/day and 2,500/month**, so about **5.6× over the monthly limit**. (The hourly timer's
  theoretical 3,840/day — 4 boards × 40 pages × 24 runs — is never reached, because most
  firings find the ingest slots contended and do not run.) The only thing holding the source
  up is that nobody has looked.
- **~6,300 new postings/day** actually written (`jobs.created_at`, 7-day mean). The budget
  buys 23,500 posting-slots and fills 27% of them.
- Meanwhile Adzuna publishes **~17,300 new IT postings/day** across the four crawled
  countries. We capture roughly a third of it.

Sending `sort_by=date` and cutting the budget to **240 requests/day** is not a trade: it is
strictly better on every axis at once — half the requests, inside the platform's terms, and
up to 12,000 posting-slots aimed at postings we do not already hold.

The second half is the consequence. Today an Adzuna posting stays open because the
relevance-ordered crawl keeps stumbling over it; the 14-day unseen sweep then reads that as
evidence of life. Once the crawl is date-ordered, an ageing posting drifts past the page
budget and never returns, so the crawl stops being a liveness signal for it. A live probe
cannot take over: sampled 40 stored URLs on 2026-09-06 and Adzuna answered **403 Access
Denied on ~30 of them** — its own bot protection on the `…/land/ad/<id>` tracking URL we
store. A 403 is not a death verdict, so the probe can never reach one.

That is exactly the shape `whatjobs` is already in, and `cmd/liveness` already has the
mechanism for it (`expireDespiteRegisteredPrefixes`). Adzuna joins it. No new machinery.

## What Changes

- **Order the Adzuna crawl by date and bound it to a freshness window.** The adapter sends
  `sort_by=date` and `max_days_old=2`; the page budget drops from 40 to 15; the timer drops
  from hourly to every six hours. Four boards × 15 pages × 4 runs = **240 requests/day**.
- **Let the freshness window end a board's crawl.** The page loop already stops on an empty
  page (`adzuna.go:124`). Without a window the pages never run out — the feed is 53k deep —
  so the quiet boards (`de`, `au`) burn their whole budget on inventory the seen-set
  discards. `max_days_old` is what makes the existing early exit reachable.
- **Close ageing Adzuna postings by age instead of by absence.** Add `adzuna` to
  `cmd/liveness`'s `expireDespiteRegisteredPrefixes`, which closes an open posting whose
  effective posting date is past the existing 45-day `expiryWindow` with reason `expired`.
- **Specify the rule the code already follows.** `job-lifecycle`'s age-rule requirement
  currently forbids exactly what `expireDespiteRegistered` does — it says the age rule "SHALL
  NOT apply to a source the ingest sweep or the liveness probe already covers". `whatjobs`
  has been closed by age despite being swept since that mechanism landed, and no spec records
  it. The requirement is rewritten to describe the real rule, with `whatjobs` and `adzuna` as
  its members.
- **Add the missing `adzuna-source` capability spec.** Adzuna is the largest source in the
  catalogue and has no spec at all.
- **Refresh the duplicate markers once after deploy** (`REINDEX_DEDUP_ONLY=1`), so the
  142,229 open Adzuna postings whose company we already hold first-party are suppressed
  where a title twin exists.

### Non-goals

- **Per-board page budgets.** The United States carries 77% of the fresh flow
  (13,339/day of 17,300) and a flat 15-page budget reaches about a quarter of it, while
  `gb`/`de`/`au` are covered whole. A per-board budget would fix that, but the `boards` table
  has no column for one and nothing else needs it yet. The seam is named in design.md; it is
  not built here.
- **`cmd/hydrate-adzuna-description`.** It fetches the same `…/land/ad/<id>` URLs every 30
  minutes and meets the same 403, which matches the "majority failure mode of the first
  production runs" its own source comment records. It is very likely running empty. Real, and
  a separate change.
- **Deprecating Adzuna.** Answered and rejected on measurement — see design.md. 20,350
  companies and 81,730 open postings exist in the catalogue through Adzuna and nowhere else.

## Capabilities

### New Capabilities
- `adzuna-source`: the Adzuna Job Search API adapter — how a board addresses a country and
  category, what request order and freshness window bound a crawl, and what request budget
  the platform's terms allow.

### Modified Capabilities
- `job-lifecycle`: the age rule is restated to cover a source the sweep *does* otherwise
  close on evidence, for the tail its crawl budget structurally cannot re-reach — the rule
  `whatjobs` has followed unspecified, and which `adzuna` now joins.

## Impact

- **Go:** `internal/ingest/sources/adzuna.go` (request parameters and page budget);
  `cmd/liveness/main.go` (one entry in `expireDespiteRegisteredPrefixes`).
- **Deploy:** `deploy/systemd/freehire-ingest@adzuna.timer` — hourly to every six hours.
  **The unit must be copied to the host**; `release.sh` flips the app and never touches a
  unit, so this half of the change does not ship itself.
- **Schema:** none.
- **Search:** no reindex is required by the change itself. The one-off
  `REINDEX_DEDUP_ONLY=1` marker pass is an operational step, not a code dependency.
- **Expect the Adzuna slice of the catalogue to shrink**, from 301k toward roughly 150-180k
  over the following weeks, and faster than the 45-day window alone implies — Adzuna's own
  14-day `sweepGrace` closes what the date-ordered crawl stops re-seeing before the age rule
  reaches it. Intake roughly doubles over the same period, so the slice refills with postings
  that are actually current. This is the intended outcome, not a regression, and it is
  recorded here so the drop is not mistaken for a broken adapter.
