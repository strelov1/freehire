# Design

Every figure below was measured on production or against Adzuna's live API on 2026-09-06.
They are recorded here because most of them argue against an option that otherwise looks
obvious, and re-deriving them costs a request budget we are trying to shrink.

## Why Adzuna is not simply dropped

The question this change answers began as issue #1759's step 4: does what Adzuna carries
have an ATS equivalent anywhere, or is it exclusive?

Classifying every open Adzuna posting by whether its company is reachable another way:

| the company is… | companies | postings | of which `is_tech` |
|---|---:|---:|---:|
| already held through its own ATS | 8,915 | 142,229 | 87,464 |
| carried by another aggregator | 6,517 | 72,931 | 45,471 |
| **reachable only through Adzuna** | **20,350** | **81,730** | 38,603 |

(2,118 postings carry an empty `company_slug` and fall outside the classification, so the
three rows total 296,890 against that run's 299,008 open Adzuna postings. Counts drift by a
few thousand between runs hours apart on the same day — the crawl and the sweep are both
running — which is why each figure here names the query that produced it rather than being
reconciled to a single total.)

So **72% of Adzuna's output is redundant at the company level** — and dropping the source
would still cost 20,350 employers the catalogue holds nowhere else. That is more companies
than most whole sources carry. Adzuna stays.

The second row is company-level, not posting-level: "another aggregator carries this
employer" does not establish that it carries these same postings. It is a ceiling on
redundancy, not a measurement of it.

Reader attention agrees Adzuna is worth keeping, narrowly. Over the 55 days
`job_daily_views` retains, Adzuna postings drew **16,308 bot-filtered `page_uniques`**
against **334,908** catalogue-wide — **4.87% of attention for 4.34% of the catalogue.**
Slightly above its weight. (The raw `view_count` for the same postings is 244k; the 15×
gap is crawler traffic, and reading it instead would have made Adzuna look like a runaway
success. `page_uniques` is the only honest column here, the same trap
`internal/engage/socialdigest` documents.)

`applied_count` was measured and **discarded**: 4 for Adzuna against 496 for the entire open
catalogue. A number that small describes how applications are recorded, not how a source
performs.

## Why the request budget and catalogue coverage are not in tension

The first draft of this change presented a choice — obey the 250/day ceiling and lose
coverage, or keep the volume and stay outside Adzuna's terms. That framing was wrong, and
the reason it was wrong is worth keeping.

It assumed each request returns something new. Measuring what the budget actually delivers
dissolved it:

| | today | after |
|---|---:|---:|
| requests/day | ~470 | **240** |
| posting-slots bought (×50) | 23,500 | 12,000 |
| new postings actually written/day | ~6,300 | up to ~12,000 |
| within Adzuna's stated terms | no | **yes** |

`jobs.created_at` over the seven days to 2026-09-06: 4,838 / 4,264 / 1,773 / 9,456 / 8,225 /
8,473 / 7,365 — a mean near 6,300. The budget's yield is **27%**. Once the yield is that
low, halving the budget and doubling the intake stop being opposites.

## Choosing the freshness window and the page budget

Adzuna's own catalogue turnover is derivable from two numbers it already reports — total
listings and listings under seven days old — by Little's Law (`items = arrival rate × time
in system`):

| country | listed | new/day | implied mean lifetime | share of fresh flow |
|---|---:|---:|---:|---:|
| us | 462,866 | 13,339 | 35 days | 77% |
| gb | 53,026 | 2,186 | 24 days | 13% |
| de | 23,616 | 1,055 | 22 days | 6% |
| au | 15,358 | 705 | 22 days | 4% |

Adzuna itself holds a posting for 22-35 days. The existing 45-day `expiryWindow` sits
deliberately wider than that, which is the under-closing bias the mechanism already declares.
**No new knob is introduced**; a second window for one source would be a second answer to a
question that already has one.

Page budget: 4 boards × 15 pages × 4 runs/day = 240 requests, under the 250/day ceiling with
room for a retry. At 50 results a page, one run reaches the newest 750 postings per board.
Against per-run flow (daily ÷ 4): `gb` 547, `de` 264, `au` 176 all fit whole; `us` 3,335 does
not, and is covered to about a quarter.

**Seam, deliberately not built:** a per-board page budget would let `us` take the pages
`de`/`au` cannot use — roughly 48/8/4/3 rather than a flat 15 — and capture most of the flow
inside the same 240 requests. It needs a column on `boards` that nothing else wants, and
whether US coverage matters to this audience is unmeasured. If it turns out to, that is where
it goes.

## Why `max_days_old` is not redundant with `sort_by=date`

With a date ordering and a fixed page budget the crawl is already bounded, so a freshness
window looks like belt and braces. It is not: it is what makes the adapter's existing exit
reachable.

`adzuna.go`'s page loop breaks when a page returns zero results. Against a 53k-deep feed that
never happens inside 15 pages, so `de` (264 per run) and `au` (176) would spend their full
budget on pages the pipeline's seen-set then discards. With `max_days_old=2` the feed runs
out and the loop stops, and those requests are simply not made.

A malformed value is safe to get wrong loudly: `max_days_old=zzz` returns **HTTP 500**, not a
silently unfiltered result set. `results_per_page=100` returns **HTTP 400**, confirming 50 is
the ceiling the constant already assumes.

## Why age, and not a probe

The catalogue's preferred close is evidence: the sweep sees a posting vanish from a board, or
the liveness probe reads a death verdict off its URL. Adzuna will offer neither once the
crawl is date-ordered.

Sampling 40 stored Adzuna URLs and following redirects:

| response | count | what it means |
|---|---:|---|
| `403 Access Denied` | ~30 | Adzuna's bot protection on `…/land/ad/<id>` — **not a verdict** |
| `404` / `410` | ~7 | genuinely gone |
| `200` | ~3 | alive |

The stored URL is Adzuna's own tracking redirect (it carries our `utm_source`, which is how
click attribution reaches our account — see the attribution work in #2308). It answers 403
whether or not the posting behind it lives. A probe built on it would read "blocked" as
"dead" and close the catalogue's largest source.

That is precisely the condition `cmd/liveness` classifies with
`expireDespiteRegisteredPrefixes`, whose own comment describes it as a source

> the sweep DOES otherwise close on evidence … for the tail its crawl budget structurally can
> never re-reach

with no evidence a probe could read. `whatjobs` is already there for the same reason. Adzuna
joins it, and the change is one string in a list.

### The three close mechanisms, and why Adzuna belongs to the third

`cmd/liveness` holds three lists, and they are not a hierarchy — they are a classification by
what evidence a source can offer:

| list | the source's situation | how it closes |
|---|---|---|
| `unsignalledSources` | no re-crawl, no feed, URL outlives the vacancy (`telegram`) | age |
| `probeDespiteRegistered` | swept, but the sweep leaks; URL answers honestly | probe |
| `expireDespiteRegistered` | swept, but the sweep leaks; URL cannot answer | age |

Adzuna's URL cannot answer, so the third row is the only one that fits. Putting it in the
first would be wrong twice over — it *is* re-crawled — and the worker refuses to start on
exactly that mistake (`cmd/liveness/main.go:191`), which is why misclassifying here fails
loudly rather than mass-closing the catalogue.

## The spec drift this change closes

`job-lifecycle`'s age-rule requirement says the rule

> SHALL NOT apply to a source the ingest sweep or the liveness probe already covers, since for
> those a close rests on evidence and age would override it with a guess.

`expireDespiteRegistered` has done exactly that for every `whatjobs` market since it landed,
and nothing in `openspec/specs/` records it — the string does not appear anywhere under
`openspec/`. The code is right and the spec is stale: the concern the sentence protects
against is real, but the answer is not "never", it is "only for the tail the sweep
structurally cannot reach, and only for a source whose URL cannot be probed."

This change rewrites that requirement to say so, and names its two members. It is not a
change in behaviour for `whatjobs`; it is the first time that behaviour is written down.

## Expected effect on the catalogue, and why it is not a regression

Two closes now reach an ageing Adzuna posting, and the faster one is not the new rule:

1. Adzuna's existing 14-day `sweepGrace`. The sweep is scoped to the company slugs a run
   actually crawled, so an old posting at a company that is *still posting* gets crawled-past
   and closed at 14 days. Under relevance ordering that posting was often re-seen by luck;
   under date ordering it will not be.
2. The 45-day age rule, for everything the sweep's company scope never reaches.

Both are correct — the first closes on the sweep's own evidence, the second on the declared
guess — but together they will pull the Adzuna slice from 301k toward roughly 150-180k over
several weeks. Its age profile today shows why that is the point: **only 19.7% of open Adzuna
postings were published in the last 14 days**, while 15.5% are older than 90 days and 5.2%
older than 180 — well past anything Adzuna itself still lists.

Intake roughly doubles over the same period. The slice shrinks and then refills, with
postings that are current rather than with the February inventory the relevance ordering kept
handing us.
