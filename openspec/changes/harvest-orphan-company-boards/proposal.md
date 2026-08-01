## Why

8,571 companies reach the catalogue only through a remote-jobs aggregator: they have open
postings from himalayas/remoteok/weworkremotely/jobspresso/workingnomads/4dayweek/jobicy/
remotive and not one row from a first-party ATS. An aggregator mirror is a worse copy of the
same vacancy — it lags the employer's own revision, carries the aggregator's branding, and
applies through the aggregator's form. Where the employer runs its own board we should be
crawling that board.

An earlier attempt (July 2026) tried to reach those boards by resolving each company's
*website* and detecting the ATS linked from its careers page, and died on website
resolution. Probing the ATS platforms directly with a slug derived from the company name
needs no website at all. Measured on 80 randomly sampled uncovered himalayas companies
against 12 platform APIs: **12 live boards found (15%)**, and comparing the employer name
the platform reports against the name the aggregator gave confirmed **11 of the 12** as the
right company. Seven of the twelve were Workable — whose board id simply *is* the company
slug, and whose board file holds 761 entries against Greenhouse's 7,109.

That name comparison is also a fix the harvest tool has needed since July, when an
unrelated harvest onboarded iCIMS `prequel` under the wrong tenant and had to be reverted.

## What Changes

- **New `cmd/harvest-orphans`**: reads the catalogue, finds companies whose open postings
  come only from aggregator sources, derives name-slug candidates for each, and writes one
  provider-agnostic seed file in the `[{board, company}]` shape `cmd/harvest-boards`
  already reads.
- **`cmd/harvest-boards` gains a corroboration gate**: today a seed-supplied `company` is
  only a fallback label for platforms that report no name of their own, and a platform that
  *does* report a name wins silently even when it names a different employer. When the seed
  states the expected employer and the platform reports a name, the two SHALL agree
  (compared normalized) or the candidate is dropped and counted. A seed that names no
  expected employer keeps today's behaviour exactly.

Not in scope, deliberately: resolving company websites, web search, and any LLM step. The
previous attempt failed on exactly that path, and the slug probe does not need it.

## Capabilities

### New Capabilities

- `orphan-company-seed`: deriving a candidate-board worklist from companies the catalogue
  holds only through aggregators — which companies qualify, what candidate slugs are
  proposed for each, and the seed shape handed to the harvest tool.

### Modified Capabilities

- `board-harvest`: a kept board must belong to the employer the seed named. Adds the
  corroboration gate to the existing live-validation requirement.

## Impact

- **New**: `cmd/harvest-orphans` (run-once host tool; needs `DATABASE_URL`, like every
  other worker).
- **Changed**: `cmd/harvest-boards` — `probeAll` and the seed-company handling in
  `seed.go`. Every existing seed (Common Crawl dumps, the universities directory, the
  wantapply list) names no expected employer and is unaffected.
- **Data**: a harvest run appends to roughly twenty `sources/*.yml` files — the providers
  whose board id is a name slug. Workday, gupy, iCIMS, Oracle, Taleo, Cornerstone, PageUp
  and NeoGov are excluded: their board ids are tenant tokens or numeric ids that a company
  name cannot produce.
- **Downstream**: ~1,200–1,500 new boards is a proportional rise in ingest crawl time and
  enrichment queue depth. Ingesting them needs no image rebuild — board files reach prod as
  bind-mounted data.
