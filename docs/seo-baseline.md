# SEO baseline, August 2026

Measured 2026-08-31 against the `sc-domain:freehire.me` Search Console property
(period 2026-08-01 → 2026-08-28), the live public API, and DataForSEO.

This document exists because five plausible optimisation hypotheses were tested
against this data and **all five failed**. Without a record, each is the kind of idea
that gets re-proposed every few months, sounds obviously right, and costs a week.

## The numbers

| | Clicks | Impressions | CTR | Avg position |
|---|---:|---:|---:|---:|
| All organic | 358 | 37,844 | 0.95% | 10.9 |
| Google Jobs (`JOB_DETAILS`) | 282 | 29,696 | 0.95% | 5.2 |
| Classic results (`JOB_LISTING`) | 39 | 924 | 4.22% | 9.4 |

**78% of impressions and 79% of clicks come through Google Jobs**, not classic
organic. That single fact governs how every other measurement here should be read.

Coverage — pages earning at least one impression in 28 days:

| Section | URLs offered | Earned an impression | Rate |
|---|---:|---:|---:|
| `/jobs/*` | 1,351,207 | 7,987 | **0.59%** |
| `/companies/*` | 160,512 | 3,486 | **2.18%** |
| `/collections/*` | 103 | 11 | 10.7% |
| `/insights/*` | 106 | 11 | 10.4% |

Read the last two rows with care: a 10% *rate* over 103 pages is 11 pages holding 28
impressions between them and no clicks at all. See hypothesis 5 — the rate counts
pages with at least one impression, which flatters a small section to the point of
being misleading.

Authority (DataForSEO, same date): 30 referring domains (28 root, 8 nofollow), 44
backlinks, domain first seen 2025-02-07.

Demand, from the top queries: `world vision vacancy in gambella`, `jobs in somalia`,
`part time jobs singapore`, `engineering jobs in uganda today 2026`,
`usina coruripe vagas`, `princess cruises careers`. The countries behind productive
postings are US 25%, Singapore 10%, Brazil 7%, Denmark 6%, India 4%. **The traffic
this site earns is non-tech and international**, against a homepage that promises
"the open-source search engine for tech jobs".

## Catalogue shape

Of the 1,351,207 documents the jobs index holds (and the sitemap therefore offers):

| Enrichment field | Documents carrying it | Share |
|---|---:|---:|
| `category` | 1,291,759 | 96% |
| `skills` (≥1) | ~920,000 | ~68% |
| `seniority` | 417,402 | 31% |
| `salary_min` | 144,595 | 11% |
| `salary_min` ∩ `seniority` | 57,566 | — |
| `salary_min` ∪ `seniority` | 504,431 | 37% |

## Demand does not match the catalogue

The catalogue is an even split — `is_tech` reports 648,706 tech against 643,289
non-tech. The demand it captures is not:

| | Clicks | Share | Impressions | Share |
|---|---:|---:|---:|---:|
| Queries naming a tech role or technology | 18 | **9.7%** | 1,262 | 6.5% |
| Everything else | 168 | 90.3% | 18,125 | 93.5% |

Half the catalogue is technical; under a tenth of the demand is. The queries earning
clicks are `world vision vacancy in gambella`, `jobs in somalia`, `part time jobs
singapore`, `production assistant jobs vancouver`, `princess cruises careers`.
Counting geographic markers across all 12,000 queries: Uganda 11 clicks, `vagas`
(Portuguese) 10, Singapore 5, Somalia 3, Ethiopia 3.

Measured over 12,000 query rows covering 186 of the period's 358 clicks — Search
Console withholds long-tail queries, and the tech/non-tech split is a keyword-list
classification, so treat the exact percentages as approximate. The gap is an order
of magnitude, which no plausible correction closes.

This is the one measurement here that implies work, and the work is a product
decision rather than a code change: the site's title, H1 and `og:description` promise
"the open-source search engine for tech jobs".

**This measurement deliberately avoids joining Search Console pages to the jobs API.**
Two attempts to do so failed the same way: the single-job API returns `is_tech` for
23% of sampled postings and `enrichment.category` for 23%, where the search index
reports 96% coverage for the latter. Whatever the cause, the populated subset is not
a random sample of the catalogue, so any rate measured across that join is
unreliable. Classifying the queries needs no join.

## Five hypotheses that failed

### 1. "Only company pages rank; job pages cannot compete on duplicated text"

**Source of the idea:** DataForSEO's ranked-keyword export. Of the top 100 URLs by
estimated traffic, 100 of 100 were `/companies/*`. Not one `/jobs/*` page appeared.

**What the data says:** `/jobs/*` earns 331 of 358 clicks — 92%.

**Why the two disagree, and the general lesson:** third-party SERP trackers scrape
classic organic results. They do not see the Google Jobs box. For a job board whose
traffic is 78% Google Jobs, a DataForSEO or Ahrefs visibility estimate is
systematically blind to the main channel. **Do not size this site's organic
performance with a third-party tracker.** Search Console is the only source that
sees it.

### 2. "Postings with richer structured data perform better, so the sitemap should prioritise them"

**The idea:** `salary_min` and `seniority` are what populate `JobPosting`'s
`baseSalary` and `experienceRequirements`. Split the job sitemap into a priority tier
carrying either field (37% of the catalogue) and an archive tier holding the rest,
so crawl budget favours the postings with something to win in Google Jobs.

**The test:** take the job URLs that actually earned impressions, and measure how
often they carry the predicate. Compare against a random catalogue sample measured by
the identical method.

| Sample | n | Carries salary or seniority |
|---|---:|---:|
| Random catalogue (control) | 100 | 43.0% |
| Earned ≥1 impression | 381 | **25.2%** |

**Result: lift ×0.68.** Enriched postings surface *less* often than the catalogue
average, not more. The predicate is not merely useless — it points the wrong way.

The plan built on it (`openspec/changes/tier-job-sitemap-by-enrichment`, complete
with proposal, design, tasks and a spec delta) was deleted rather than implemented.

### 3. "There is a subgroup of job cards that converts well; find it and copy it"

**The idea:** average CTR is 0.95%, but 184 job pages showed 4,357 impressions and
282 clicks — 6.47%. Find what separates them from the 6,407 pages with zero clicks.

**Why that was circular:** those 184 pages were *selected by having clicks*, and then
their CTR was computed. The control is whether zero-click pages are more common than
a uniform CTR would produce:

| Impressions per posting | Postings | Impressions | Clicks | CTR | Zero-click, observed | Zero-click, expected at 0.95% |
|---|---:|---:|---:|---:|---:|---:|
| 1–4 | 5,063 | 8,814 | 53 | 0.60% | 99% | 98% |
| 5–9 | 900 | 5,820 | 43 | 0.74% | 96% | 94% |
| 10–29 | 511 | 7,889 | 81 | 1.03% | 90% | 86% |
| 30–99 | 105 | 4,865 | 75 | 1.54% | 68% | 65% |
| 100+ | 12 | 2,308 | 30 | 1.30% | 42% | 24% |

Observed tracks expected. **There is no high-performing segment** — CTR is roughly
uniform at ~1%, drifting up mildly with impression count. Salary presence (14.0% vs
13.2%) and employment type (35.7% vs 34.0%) do not separate the groups either.

### 4. "Most company pages are unreachable by crawl, so internal linking is the constraint"

**The idea:** `/companies` is sorted by `job_count` descending and its pagination
stops at `page=500`. At `offset=9,980` the listing's `job_count` is 20, so a crawler
following internal links can reach only companies with **≥20 open jobs** — about
10,000 of 160,512. The other 94% are sitemap-only.

**The test:** do the company pages that actually earn impressions sit inside the
crawlable set?

| Productive company pages | Share | Impressions |
|---|---:|---:|
| Crawl-reachable (`job_count ≥ 20`) | 14.1% | 145 |
| Sitemap-only (`job_count < 20`) | **85.9%** | 657 |

Median `job_count` of a productive company page: **1**.

**Result:** 86% of the company pages that work are ones internal linking never
reaches. Sitemap discovery is doing the job. Raising the pagination cap would change
nothing. (Large companies are over-represented per page — 14.1% against a 6.2%
base — but the long tail still carries the volume.)

### 5. "Programmatic landing pages are the way to capture this demand"

**The idea:** the demand is geographic (`jobs in somalia`, `part time jobs
singapore`, `engineering jobs in uganda today 2026`), the `FilterCollection`
machinery maps a slug to arbitrary `/jobs` facet params in one frontend entry, and
every existing geographic collection pins `work_mode: 'remote'` — so there are no
country landings for on-site work, which is what the demand asks for. Adding them
looked like a one-line-per-country change.

**What the shipped ones do:** all 103 collection pages together earned **28
impressions and 0 clicks** in 28 days. Ten pages saw any impression at all, the best
of them ten impressions.

| Collection | Clicks | Impressions |
|---|---:|---:|
| `frontend` | 0 | 10 |
| `junior` | 0 | 4 |
| `network-engineering` | 0 | 4 |
| the other 7 | 0 | 1–2 each |

The `programmatic-seo-collections` change is complete and deployed. Its output does
not rank. Adding country variants would be adding more of a page type with a
measured yield of zero.

(An earlier reading of this data — "collections have the best impression rate on the
site, 11 of 103" — was true and meaningless: the rate counts pages with ≥1
impression, and the impressions are one to ten each.)

## The taxonomy is shaped for a catalogue we no longer have

`vocab.CategoryValues` holds 37 values and covers 95.6% of the catalogue, so
coverage is not the problem. The *shape* is: the vocabulary is finely divided where
the catalogue is thin and undivided where it is fat.

| Category | Postings | Share |
|---|---:|---:|
| `management` | 210,322 | 15.6% |
| `sales` | 146,935 | 10.9% |
| `support` | 101,177 | 7.5% |
| … | | |
| `developer_relations` | 435 | 0.03% |
| `blockchain` | 427 | 0.03% |

`management` and `support` hold 311,499 postings — 23% of the catalogue — and both
are **title-token buckets rather than domain buckets**. In a 300-title sample of
`management`, 279 titles contain the word "Manager": Tax Credit Property Manager, EHS
Manager, Warehouse Manager, Senior Analog Engineering Manager and Art Studio Manager
share one facet value. In `support`, 219 of 300 contain "support", placing Technical
Support Engineer beside Direct Support Professional (children's residential care) and
Customer Service Representative.

A jobseeker filtering on `management` learns nothing about the work. These are also
the categories the traffic comes through — `support` is 14.4% of the postings that
earn impressions and `management` 12.9%.

Note that "is this a people-management role?" is **already a separate facet**:
`role_type = people_manager`, derived from the title alone, 80,081 postings. The
`management` category answers the same question on the wrong axis — a category should
say what field the work is in, and the manager/IC distinction already has its own
attribute.

### Why the `management` bucket should stay as it is

The obvious follow-up — re-route those 210,322 postings by domain — was costed and
rejected. Three findings, in increasing order of importance.

**There is no head to attack.** Across 800 sampled `management` titles, the word
qualifying "Manager" takes **352 distinct values, 256 of which (73%) occur exactly
once**. The most common is `engineering` at 2.2%, and `{"engineering manager",
"management"}` is already an explicit, deliberate mapping. Everything below it is at
or under 1%: partner, business, restaurant, construction, production, property, EHS,
warehouse, logistics, guest experience.

**Most of what could be routed needs categories we do not have.** Of the top thirty
qualifiers, the ones with real volume — restaurant, construction, property, EHS,
warehouse, care, guest experience — have no home in `vocab.CategoryValues`. Adding
them is a decision about what catalogue this is, not a dictionary fix. What could go
to an existing category (partner → sales, communications → marketing, logistics →
operations, data → data_analytics) is roughly 5–6% of the bucket, about 12,000
postings, at the cost of 15–20 hand-curated aliases — and `dictionaries.go` already
argues why bare nouns like `content`, `growth` and `performance` cannot be trusted
alone, since they name technical roles too.

**The fall-through is load-bearing.** `{"manager", "management"}` is the terminal
entry in `categoryTable` (`internal/dict/classify/dictionaries.go:792`), and
`management` is a `NonTechCategories` member, so `is_tech` is false and the
enrichment enqueue gate (`is_tech IS TRUE`) never sends those postings to the LLM.
Narrow the fall-through and roughly 121,000 bare-"Manager" titles resolve to no
category from the dictionary and get no LLM answer either — which is exactly
`search.CategoryUnresolved` (`internal/search/search/document.go:156`), and
`cmd/reindex`'s `splitJobs` **deletes** those from the index. They would leave the
site's own search and the sitemap.

The asymmetry decides it: the upside is a better facet for ~12,000 postings, the
downside is ~121,000 postings falling out of the catalogue.

## Facet coverage, and which gaps are fixable

| Facet | Whole catalogue | Tech categories | Non-tech categories |
|---|---:|---:|---:|
| `category` | 95.6% | — | — |
| `countries` | 93.2% | — | — |
| `employment_type` | 38.6% | 34.1% | 43.2% |
| `seniority` | 30.9% | 40.5% | **21.2%** |
| `work_mode` | 27.6% | 29.2% | 25.6% |
| `salary` | 13.5% | 15.9% | **3.9%** |
| `education_level` | 6.9% | — | — |
| `role_type` | 5.9% | — | — |
| `english_level` | 5.7% | — | — |
| `ai_archetype` | 0.5% | — | — |

The gaps have two different causes, and only one is a decision we control:

- **`salary` and `seniority`** are 4× and 2× better inside the tech categories. That
  is the enrichment enqueue gate (`is_tech IS TRUE`) working as designed — non-tech
  postings never reach the LLM. Filling them means spending enrichment budget on
  644,000 postings the gate deliberately excludes, and the queries' comments argue
  why it was closed.
- **`work_mode` and `employment_type`** are flat across the split — `employment_type`
  is actually *higher* for non-tech — so the enrichment gate does not explain them.
  They are thin because postings do not state the fact and the dictionary sets them
  only on an explicit marker. Sizing what better detection could recover: 127,437
  postings contain the token `remote` and 90,262 of those (71%) already carry a
  `work_mode`; `hybrid` is 74,834 against 39,861. About 70,000 postings are
  recoverable from the obvious tokens, against a 978,000 gap — **7%**.

Do not size this with a multi-word `q=`: Meilisearch does not phrase-match here, so
`q=work from home` returns 404,277 documents containing those three words anywhere.
Single tokens only.

## The supply gap this uncovered

Not an SEO finding, recorded here because the data is the same query:

| Searcher country | Clicks | Impressions | CTR | Jobs in catalogue |
|---|---:|---:|---:|---:|
| Ethiopia | 20 | 211 | **9.5%** | 158 |
| Kenya | 28 | 420 | **6.7%** | 713 |
| Nigeria | 14 | 248 | 5.6% | 2,945 |
| Uganda | 14 | 291 | 4.8% | 119 |
| United States | 50 | 12,499 | 0.4% | 567,816 |

East African searchers convert an order of magnitude better than the site average
(0.95%) and hit a catalogue holding roughly a hundred postings for them. That is a
sourcing question for the ingest fleet, not a discoverability one.

Three cheap routes into that supply were probed on 2026-08-31 and ruled out. Curating
known employers is exhausted: 19 of 28 well-known African tech employers are already
in `sources/`, and the 15 that resolve contribute 412 open jobs between them. Regional
aggregators are closed: Fuzu returns 403, MyJobMag serves a Cloudflare challenge, and
BrighterMonday and Jobberman expose neither a sitemap nor `JobPosting` JSON-LD.
Guessing board slugs from employer names yields nothing: 25 names slugified from the
Ethiojobs directory and probed against the public Greenhouse and Workable board APIs
returned 0 hits.

A fourth route — using a board's employer directory as a *worklist* rather than as a
source — was proposed, gated on a measurement, and closed by it.

The idea: `ethiojobs.net/sitemap-companies.xml` lists 5,141 employers, and each company
page carries the employer's own website in its `__NEXT_DATA__` payload, which is the
`{name, website}` shape `cmd/harvest-ats resolve` already consumes. The proposal's first
task was to measure the detected-board rate before building anything.

| Step | Result |
|---|---:|
| Companies sampled uniformly across the directory | 280 |
| Carrying a website | 36 (12.9%) |
| Resolving to a detectable ATS board | 3 |
| **Of those, not already in `sources/`** | **1** |

The two rejects are the informative part: NRC's Oracle board and Mastercard
Foundation's Workday board were already committed. One new board per 280 companies
extrapolates to roughly **18** across the whole directory — for a 5,141-page crawl and
a parser reading a third party's internal payload.

Note how the website rate fell as the sample grew: 33% (n=12), 18% (n=60), 12.9%
(n=280). The first two samples were clustered rather than uniform.

The overlap repeats the 19-of-28 finding above from a different angle: the
internationally-operating half of an East African directory is largely the half
curation has already found. Employer-level board discovery for this region is closer
to complete than its job counts suggest, and the remaining Ethiopian supply sits with
local employers that run no ATS and publish no website — reachable only by crawling
Ethiojobs itself.

## What this leaves

Everything the site controls technically is working: sitemap coverage is complete
across both jobs and companies, `JobPosting` structured data is rich enough to win
Google Jobs placement at an average position of 5.2, pages are server-rendered,
canonicals and redirects are correct, and closed postings leave the sitemap.

The constraints are on the demand side, and neither is a code change:

- **Authority.** 30 referring domains is the ceiling every other lever runs into.
- **Positioning.** The measured audience is non-tech and international. That is now a
  fact, not a hypothesis.

At 358 clicks per month, micro-optimisation arithmetic does not work: moving CTR from
0.95% to 1.3% is worth ~130 clicks, assuming a lever exists — and hypothesis 3 found
none.

## Reproducing this

Search Console access needs the `webmasters.readonly` scope on ADC and a quota
project with the API enabled:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/webmasters.readonly
gcloud auth application-default set-quota-project <project>
gcloud services enable searchconsole.googleapis.com --project=<project>
```

Two traps that cost time here:

- Google Jobs attributes impressions to the URL **with** its
  `?utm_campaign=google_jobs_apply` parameters. A regex anchored on a clean
  `/jobs/<slug>$` matches 33 rows out of 6,596.
- `searchAnalytics` returns at most 1,000 rows per call and sorts by clicks
  descending. Paginate with `startRow`, or you are looking at the head only — which
  is exactly the selection bias that produced hypothesis 3.
