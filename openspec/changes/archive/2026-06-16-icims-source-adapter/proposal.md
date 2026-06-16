## Why

iCIMS is the single largest ATS gap in the catalogue: a seed list of ~9,937
company career sites that we ingest zero of, because no iCIMS `Source` adapter
exists. iCIMS career sites turn out to be cleanly parseable without a headless
browser (sitemap enumeration + a server-rendered JSON-LD `JobPosting` behind the
`?in_iframe=1` fragment), so the gap is closable with the established
detail-fetching adapter pattern.

## What Changes

- Add an `icims` `Source` adapter (`internal/sources/icims.go`) that enumerates a
  board's `sitemap.xml`, fetches each job's `?in_iframe=1` fragment, and maps its
  schema.org `JobPosting` JSON-LD to the normalized job shape (reusing the
  existing `ldJobPosting` helper).
- Register the adapter in `sources.All` (one line).
- Widen the `cmd/harvest-boards` prober transport to add `sources.XMLGetter`, and
  add an `icimsProber` that live-validates a candidate slug by requiring its
  sitemap to list ≥1 job — so dead boards (HTTP 404, or 200 with zero jobs) are
  filtered before they reach `sources/icims.yml`.
- Unit tests for the adapter and the prober.

Out of scope: running the harvest over the seed list, ingesting, deploying.

## Capabilities

### New Capabilities

(none — iCIMS is a new provider within the existing source-ingest capability,
not a new capability.)

### Modified Capabilities

- `source-ingest`: add a scenario establishing that an adapter MAY enumerate a
  board through a sitemap (rather than a list endpoint) and obtain each posting's
  fields from a server-rendered JSON-LD detail page. This generalizes the
  existing detail-fetch requirement to sitemap-enumerated providers; no existing
  behavior changes.

## Impact

- New: `internal/sources/icims.go`, `internal/sources/icims_test.go`.
- Modified: `internal/sources/source.go` (registry line);
  `cmd/harvest-boards/prober.go` (transport widening + `icimsProber`);
  `cmd/harvest-boards/prober_test.go`.
- Generated later (out of scope): `sources/icims.yml`.
- No schema, API, or runtime-config changes. The shared `*sources.Client`
  already implements `GetXML`, so the prober transport widening needs no new
  transport code.
