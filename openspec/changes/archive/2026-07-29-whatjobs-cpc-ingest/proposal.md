## Why

WhatJobs is an approved CPC publisher account (`publisher=7065`) whose FeedAPI carries
US tech postings the catalogue does not have — a spot check of 14 fresh tech postings
found only 3 already present. It is the first monetization-network feed whose tracked
click-through URL is **not IP-bound**, so unlike CareerJet it can be ingested and stored
on the ordinary crawl path instead of needing a live per-click resolver.

The feed also has two properties that make a naive adapter actively harmful: its postings
are old (56% older than 90 days, up to 413) and its `url` points at a billing landing page
rather than the employer's posting, so neither the liveness probe nor the origin can ever
verify them. The adapter has to compensate for both, or it pours unverifiable ghost jobs
into the catalogue.

## What Changes

- New `whatjobs` source adapter reading the WhatJobs FeedAPI as a multi-company aggregator,
  enumerated by search keyword — the same shape as `hh`, whose board is a `professional_role`.
- `sources/whatjobs.yml` lists one entry per keyword slice; `company` is a display label only,
  the real employer comes from each posting.
- The publisher id lives **only** in the environment (`WHATJOBS_PUBLISHER_ID`), never in the
  board file. `sources.All` registers the adapter only when it is set, matching `usajobs`/`reed`.
- Normalization compensates for the feed's defects: the reseller signature `#J-…-Ljbffr` is
  stripped from descriptions, and the always-garbage `salary` (`"0.000000 - 0.000000"`),
  always-empty `job_type` and always-null `logo` are ignored rather than stored as facts.
- `age_days` is **not** trusted as a posting date. It reports the age of the record in the
  reseller's index (15 postings from different companies share `age_days: 109`), so it never
  feeds `posted_at`.
- The post-run unseen sweep gets a **provider-declared grace window**. The sweep's caller
  already owns the cutoff; a provider whose crawl reaches only a slice of its catalogue
  declares a window wider than the 48-hour default, so a posting that merely drifted past
  the crawl's page depth is not falsely closed and then reopened.

## Capabilities

### New Capabilities

- `whatjobs-source`: the WhatJobs FeedAPI adapter — request shape and its documented-but-broken
  parameters, keyword-sliced pagination and its ceiling, posting identity derived from the
  tracked URL, and the normalization that discards the feed's junk fields.

### Modified Capabilities

- `job-lifecycle`: the unseen-job sweep's grace window becomes provider-declared rather than a
  fixed 48 hours, so a provider with deliberately partial catalogue coverage closes on a wider
  window. The default is unchanged for every existing provider.

## Impact

- **New code**: `internal/sources/whatjobs.go` (+ test), `sources/whatjobs.yml`.
- **Touched**: `internal/sources/registry.go` (conditional registration), `internal/sources/source.go`
  (grace-window marker), `cmd/ingest/main.go` (honour the declared window when computing the cutoff).
- **Environment**: `WHATJOBS_PUBLISHER_ID` — a new secret for the ingest worker; unset leaves the
  provider unregistered, so tests, CI and local dev are unaffected.
- **Not touched**: the liveness probe needs no change — it only probes jobs whose source is *not*
  a registered board provider, and `whatjobs` is one, so its jobs are excluded by the existing rule.
- **Deliberately out of scope**: filtering staffing intermediaries out of the feed (~35% of its
  companies), a click-proxy endpoint that would hide the publisher id from the public API, and any
  non-US inventory — the publisher id is per-country and this one is US-only.
