## Why

`sources/greenhouse.yml` and `sources/ashby.yml` are built from curated company datasets and
careers-page resolution (`cmd/harvest-ats`), which only find companies whose website is in one
of those datasets. Common Crawl's CDX index lets us query, for free, every URL its crawler has
ever seen under `boards.greenhouse.io` and `jobs.ashbyhq.com` — a source of company slugs that
doesn't depend on a company being in a curated dataset at all. A feasibility spike (see
`docs/superpowers/specs/` conventions — validated by hand before this change) confirmed the CDX
query returns real, extractable board slugs for both domains, with a meaningful new-candidate
yield (412 of 1789 Greenhouse slugs from a single snapshot were not already in
`sources/greenhouse.yml`). A third target, Lever, was ruled out: `jobs.lever.co/robots.txt`
disallows `CCBot` site-wide, so Common Crawl carries no board-path data for it at all.

## What Changes

- `cmd/harvest-boards` gains a shared Common Crawl CDX helper that, given a host prefix, fetches
  the latest 3 snapshot ids from `collinfo.json`, pages each snapshot's CDX index for that host,
  extracts the first path segment of each matched URL as a candidate board slug, and returns the
  deduplicated set.
- `greenhouseProber` and `ashbyProber` each gain a `discover` method (implementing the existing
  `discoverer` interface, the same one `gupyProber` and `opencatsProber` already implement) that
  calls the shared helper with their own board-URL host. Running `harvest-boards greenhouse` or
  `harvest-boards ashby` with no seed file now discovers candidates from Common Crawl instead of
  requiring one; a seed file, when supplied, still works exactly as before.
- No changes to `leverProber` — Lever discovery was ruled out by the spike (CCBot is disallowed
  by its own robots.txt).
- Discovered candidates go through the existing live-validation, dedup, and append pipeline
  unchanged — a candidate is appended to `sources/<provider>.yml` only when the provider's own
  API reports at least one open job for it.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `board-harvest`: adds a new requirement describing Common Crawl CDX discovery for Greenhouse
  and Ashby, alongside the existing Gupy and OpenCATS discovery requirements.

## Impact

- Code: `cmd/harvest-boards/` — one new file for the shared CDX helper, `discover` methods added
  to `greenhouseProber` and `ashbyProber`, unit tests for slug extraction and CDX response
  parsing.
- No schema, API, or runtime-worker changes — `harvest-boards` is a manually-run host tool, not
  a cron worker; this only changes what it does when invoked with no seed file for these two
  providers.
- External dependency: `index.commoncrawl.org` (public, no auth, no rate-limit key) becomes a
  data source for this tool only.
