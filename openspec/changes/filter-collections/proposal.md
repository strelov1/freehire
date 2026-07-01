## Why

Today a "collection" on `/collections` is strictly a curated group of **companies**
(company-level membership, denormalized to a `jobs.collections` search facet). Some
natural groupings a user wants to browse are properties of a **job**, not a company
— "Remote Worldwide", "Senior roles", "Python jobs" — where within one company only
some jobs qualify. The company-membership mechanism cannot express these. We want
the hub to also offer curated groupings that map to an arbitrary `/jobs` facet
filter, so any browsable slice of the catalogue can become a first-class collection
card with a one-line definition.

## What Changes

- Introduce a **second kind of collection — a "filter collection"** — that maps a
  curated `slug` / `title` / `description` to an arbitrary set of `/jobs` facet
  filter params (e.g. Remote Worldwide → `work_mode=remote` + `regions=global`).
- The `/collections` discovery hub lists filter collections **alongside** the
  existing company collections, as visually identical cards. A filter-collection
  card links directly to `/jobs?<query>` (mirroring how a company-collection card
  links to `/jobs?collections=<slug>`); its open-job count comes from a job-search
  total for that filter. **No dedicated per-collection page** — the `/jobs` feed
  remains the single rendering of a collection's jobs.
- Seed one filter collection: `remote-worldwide` (`work_mode=remote`,
  `regions=global`). Adding another is a single data entry.
- Frontend-only: filter collections live as data in `web/src/lib`. No Go registry
  entry, no company/job membership, no DB migration, no reindex, no API change.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `job-collections`: the discovery hub gains a second kind of collection whose
  membership is an arbitrary job-search filter (not company membership); its card
  links to `/jobs?<query>` and its count is a job-search total. Company-collection
  behavior (membership fact, propagation, import worker, the `collections` facet)
  is unchanged.

## Impact

- `web/src/lib/collections.ts` — add a `FilterCollection` type, a
  `FILTER_COLLECTIONS` data array (seeded with `remote-worldwide`), and a `toQuery`
  helper that expands params into a query string.
- `web/src/routes/collections/+page.server.ts` — additionally fetch each filter
  collection's count via a job-search total; return one normalized card view-model
  combining both kinds.
- `web/src/routes/collections/+page.svelte` — render from the unified card list
  (`href` + `count` per card); adjust the hub subtitle copy.
- No backend, DB, or API changes. Verification is `svelte-check` + `npm run build`
  + manual load of `/collections` (the `web/` package has no test runner).
