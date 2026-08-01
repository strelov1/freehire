## Why

"Y Combinator picked this company" is one of the strongest trust signals a job card
can carry, and today a reader gets it only by recognising the company name. A
company's collections already reach the frontend on every job, but only the
*credential* subset renders — the editorial tags, `yc` and `techstars` among them,
are invisible outside the `/collections` hub.

a16z, the third brand worth showing, is not modelled at all: it reaches us as
`source='speedrun'` on the 11 companies in `sources/speedrun.yml` that have no ATS
of their own, while the fund's public directory lists 317 relevant companies.

## What Changes

- A third collection `kind`, **backer**, for a tag that names the accelerator or
  fund that selected the company. `yc` and `techstars` move onto it.
- Two new collections, `a16z-portfolio` (the fund's portfolio) and `a16z-speedrun`
  (the accelerator's cohorts), sourced from the Speedrun talent-network directory.
  The directory's third tier, `market` — TikTok, Walmart, Amazon, P&G — carries no
  a16z relationship and is never imported.
- A fourth membership-payload form for datasets: a source that fetches and parses
  itself, for a directory that no single URL can express because it is paginated.
- A **backer badge** — the brand's own logo, committed to the repo as an SVG —
  rendered next to the company name in the job feed, on the job page, on the
  company page, in the company list, on the `/collections` hub and landing, and
  inside the collection filter chips.
- Filter chip options gain an optional icon, so a chip can carry a mark rather
  than text alone.

Not in scope: badges for the seven remaining editorial collections (`bigtech`,
`unicorn`, `fortune500`, `mag7`, `ai`, `ai-native`, `european`, `eastern-roots`).
They have no logo of their own and stay filters.

## Capabilities

### New Capabilities
- `backer-badges`: what a backer collection is, which brands qualify, how a badge
  is presented on each surface, and why it carries no monogram fallback.

### Modified Capabilities
- `job-collections`: the `kind` contract gains a third value; a dataset gains a
  fourth payload form (self-fetching, for paginated sources).

## Impact

- `internal/collections` — new `KindBacker`, two registry entries, a Speedrun
  directory parser, an extended `Dataset`.
- `cmd/import-collections` — resolves the new payload form. No generator change:
  `cmd/gen-contracts` already emits `kind` verbatim.
- `web/src/lib` — `backers.ts`, `BackerBadge.svelte`, three brand SVGs, an optional
  `icon` on `FacetOption`.
- Surfaces: `JobRow.svelte`, the job page, `CompanyHeader.svelte`, the `/companies`
  cards, `/collections` hub and landing, `PillGroup.svelte`.
- External dependency: the Speedrun talent-network public API, already crawled by
  `internal/sources/speedrun.go`.
- Operational: a search reindex after the import, as with any collection change.
