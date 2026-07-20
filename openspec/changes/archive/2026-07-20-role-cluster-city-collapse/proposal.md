## Why

The role-cluster collapse folds "one role, many cities" into a single canonical
card — but only when the city lives in the `location` field. Many ATS platforms
(Personio, Workday-style boards) bake the city into the **title** itself
(`"Senior Fullstack Engineer (m/w/d), Krakau"`), so five identical postings get
five distinct `role_fingerprint`s and show as five separate cards. A production
spike over 3.98M open jobs confirmed the title suffix is the sole differentiator
for these clusters, and that the description (already part of the fingerprint)
reliably keeps genuinely-different roles apart — so stripping the trailing
city/qualifier clause is safe for the tech catalogue.

## What Changes

- `role_fingerprint` becomes **city/qualifier-suffix agnostic**: its title
  normalization strips a trailing separator clause (after the last ` , `, ` | `,
  ` @ `, or spaced ` - ` / ` — `), keeping the prefix. Description stays in the
  key as the over-merge guard. Per-city variants of one role now share a
  fingerprint and collapse to one canonical card.
- The canonical job **unions its cluster's geography** — `countries`, `regions`,
  `cities` (and `work_mode`) across all open rows of the cluster — onto its
  search document, so a collapsed multi-city/multi-country role stays findable by
  every city and country it is open in (not just the canon's own).
- Existing rows are **backfilled**: `role_fingerprint` is recomputed for the live
  catalogue so already-ingested clusters (Towa, speechify, …) collapse without
  waiting for a re-crawl.
- **Behavior change (accepted):** because the fingerprint is shared with the
  reality signal, per-city variants now count together — `repost_count` /
  `mass_posting_count` reflect the true number of concurrent city postings of a
  role (e.g. 5 instead of 1). This is more correct; the likely-evergreen class
  still requires convergence, so counts alone do not reclassify a fresh role.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `ingest-content-dedup`: the role fingerprint that keys the collapse ignores a
  location-bearing title suffix; the canonical job unions its cluster's geography
  for search/filtering.
- `job-reality-signal`: repost/mass-posting counts share the city-agnostic
  fingerprint, so per-city variants of one role count as one clustered role.

## Impact

- **Code:** `internal/jobhash/rolefingerprint.go` (strip logic + shared separator
  helper), `internal/db/queries/jobs.sql` (new `RoleClusterGeoAll`),
  `internal/search` (union cluster geography onto the canonical document),
  `cmd/reindex` (wire the geo union), a `role_fingerprint` backfill path.
- **Data:** one-shot backfill of `role_fingerprint`; the next reindex recomputes
  `duplicate_of` and re-indexes canons with unioned geography.
- **Search index:** collapsed canons gain broader geography facets; reposts stay
  excluded. The incremental ingest push cannot compute the union (single-row
  view), so full geography is a full-reindex property — accepted, mirroring the
  existing reality-count behavior.
- **Non-tech caveat:** retail mass-posters whose title suffix is a shift /
  employment-type and whose description is templated may over-collapse those
  variants; these are outside the IT focus and the merge (same role, different
  hours) is acceptable.
