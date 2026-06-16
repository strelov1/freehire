## Why

`cmd/harvest-boards` expands a board file by live-validating a **seed list** of
candidate slugs. Gupy has no such seed: its boards are numeric `companyId`s that
appear nowhere as a public list — they are only discoverable by enumerating
Gupy's global jobs feed (`employability-portal.gupy.io/api/v1/jobs`), which
returns ~25 distinct companies per 40-job page across all employers. So Gupy
coverage cannot grow through the seed path; we currently hold 33 Gupy boards and
could reach hundreds.

## What Changes

- **Add a discovery mode to `cmd/harvest-boards`.** A new opt-in `discoverer`
  interface lets a provider's prober enumerate its own candidate boards from the
  platform API when no seed file is given, instead of reading a seed list. The
  rest of the pipeline (dedup against the existing board file, concurrent probe,
  append) is unchanged — discovery only replaces the seed as the candidate source.
- **Add a `gupy` prober** implementing both:
  - `discover` — pages the Gupy global jobs feed (no `jobName` filter, so **all**
    companies with open jobs, not only IT) collecting distinct `companyId`s until
    a page yields no postings (bounded by a max-offset safety).
  - `probe(companyId)` — queries `?companyId=<id>&limit=1` for the company's
    `careerPageName` (name; falls back to the id) and open-job `total`, the same
    contract every other prober honours. Discovered ids run through this probe, so
    the committed file stays our own live-validated fact set.
- **Invocation:** `harvest-boards gupy` (no seed argument) discovers; the existing
  `harvest-boards <provider> <seed.json>` path is untouched.

## Capabilities

### New Capabilities

- `board-harvest`: the host dev tool that expands a board file with
  live-validated boards, by seed-list validation or by platform discovery.

### Modified Capabilities

<!-- none -->

## Impact

- `cmd/harvest-boards/prober.go`: new `discoverer` interface + `gupyProber`
  (`discover` + `probe`); register `probers["gupy"]`.
- `cmd/harvest-boards/main.go`: `run()` selects discovery when the prober is a
  discoverer and no seed path is given; the probe/dedup/append flow is shared.
- `sources/gupy.yml`: grows by the discovered boards on each run (committed after
  diff review; sources-only, deploys via rsync, no image rebuild).
- No runtime/server change; this is a run-once host tool.
