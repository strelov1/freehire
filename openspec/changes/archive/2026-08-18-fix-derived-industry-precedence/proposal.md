## Why

#2074 made the `industries` facet match through the curated column **or** the
job-derived `domains` one, as two equal sources. Measured against real companies
rather than aggregates, they are not equal, and the change put well-known companies
under industries they are not in.

`companies.domains` is the union of the enrichment domain over **every** open job
the company has. For a company with hundreds of postings, that union drifts far from
what the company is. Uber carries `{adtech, cybersecurity, ecommerce, edtech,
fintech, gamedev, govtech, hrtech, logistics, media, other, saas, travel}` — so on
production today `GET /api/v1/companies?industries=gaming` returns Uber, as do
`edtech`, `government` and `adtech`.

The union was honest under the old label. "Domain" meant *this company posts jobs in
these areas*, which is true. #2074 reinterpreted the same array as *this company is
in these industries*, which is not.

## What Changes

- The derived arm applies **only to companies with no curated industry**. A company
  an importer has already classified is answered from that classification alone; its
  domains are not consulted. Uber is `{ai, data-analytics, logistics}` and stops
  matching `gaming`.
- Both backends change together, as before: Postgres gains a `cardinality(industries) = 0`
  guard, Meilisearch an `industries IS EMPTY` conjunct on the derived fragment.
- `media` now maps to `entertainment` and `mobility` to `transportation`, with the
  aliases both were missing. Left unmapped in #2074 for want of an honest target;
  both now have one (see design.md).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `companies`: the industry facet's two sources become ordered rather than equal —
  the derived one answers only where the curated one is silent.

## Impact

- `internal/db/queries/companies.sql` — the `industries` predicate; `make sqlc`.
- `internal/search/company.go` — the derived fragment becomes a conjunct.
- `internal/industrytag` — two mappings and their aliases.
- No schema change, no migration, no reindex.

**Known limitation, deliberately not fixed here.** Precedence removes the noise from
companies that have a curated industry. It does not remove it from the 9,989
companies that have none and carry three or more domains (mean 34 open jobs, 1,467 of
them above 50) — a focused company carries one or two. The honest fix is a domain-count
threshold, which cannot be expressed in a Meilisearch filter over the attributes that
exist: it needs a materialized column and a companies reindex. Forcing such requests
onto Postgres instead was measured and rejected — the filtered count there takes 1.2s
today and 2.6s with the threshold, against a Meilisearch count that is effectively free.
Tracked separately; this change is the half that needs no reindex, on a defect that is
live.
