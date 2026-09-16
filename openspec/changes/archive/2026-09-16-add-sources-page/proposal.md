## Why

The catalogue is assembled from 259 source adapters, and a visitor has no way to see
that. `/open` reports one number ("sources"), `/status` reports fleet health in
operational language, and neither answers the question a candidate, a white-label
prospect or an investor actually asks: *which* sources, how many jobs does each carry,
when did we last read it, and what do we find there that a first-party ATS crawl would
not have found anyway.

That last question is the one we have never answered at all. Roughly half the
catalogue's open postings come from aggregators, and the dedup pass already knows which
of those postings it matched to a first-party ATS posting (`jobs.duplicate_of_aggregator`)
— but nothing reads that as a *per-source* figure, so "what is this aggregator worth to
us" has only ever been a guess.

## What Changes

- **New `source_stats` rollup**, written by a pass inside `cmd/rollup-stats` (which runs every 3 hours):
  per source, the raw open-posting count, how many of those the dedup pass matched to a
  first-party ATS posting, one sample posting URL (for the logo host), and the
  de-duplicated count Meilisearch itself holds. Both counts are measured in the same run
  so they can never describe different moments.
- **New public endpoint `GET /api/v1/sources`** — the rollup joined with the per-provider
  health rollup `/status` already computes (kind, status, last run, last success, last
  ingested count, board counts). One request serves the whole page.
- **New public page `/sources`** — every source grouped by kind (ATS platforms /
  aggregators / company career pages), searchable client-side, each row linking to
  `/jobs?source=<key>`, with a lazily-loaded logo resolved from the source's own posting
  host through the existing `logo.freehire.me` proxy.
- **The ATS-overlap figure is labelled for what it measures** — "postings we could not
  match to a first-party ATS posting", never "exclusive". The dedup pass finding no pair
  is not proof that no pair exists.

No breaking changes: every existing endpoint, page and table keeps its current shape.

## Capabilities

### New Capabilities
- `source-catalog-page`: the public per-source catalogue — what the `/sources` page
  states, what `GET /api/v1/sources` serves, and the honesty rules that govern the
  figures on it (in particular the ATS-overlap figure's meaning and its labelling).
- `source-stats-rollup`: the scheduled measurement behind it — what `source_stats` holds,
  how the two job counts are measured together, and how a degraded measurement is
  reported rather than silently zeroed.

### Modified Capabilities
<!-- None: /status, /open and the dedup passes keep their current requirements. This
     change reads what they already produce. -->

## Impact

- `migrations/` — one new migration creating `source_stats`.
- `internal/platform/db/queries/` — the per-source aggregate query and the snapshot
  upsert/read; `make sqlc` regeneration.
- `cmd/rollup-stats/` — one new pass; already holds both `DATABASE_URL` and the
  Meilisearch credentials, so no new deployment surface.
- `internal/api/handler/` — one new public handler, reusing the provider-rollup
  derivation `status.go` already owns.
- `web/src/routes/sources/` — the new page; `web/src/lib/api.ts` a new client method;
  `web/src/lib/facets.ts` `sourceLabel` reused for display names.
- `web/src/routes/sitemap-pages.xml` and the footer — the page has to be reachable.
- `deploy/` — nothing new: the rollup rides an existing timer.
