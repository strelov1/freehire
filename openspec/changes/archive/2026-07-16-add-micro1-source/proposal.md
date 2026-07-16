## Why

micro1 runs a public job board (`jobs.micro1.ai`) of remote, output-based
engineering and specialist roles that is not covered by any existing adapter.
Its full posting set is enumerable from a sitemap and each posting server-renders
a complete, structured payload, so it is a low-cost, high-signal source to ingest.

## What Changes

- Add a `micro1` source adapter: a boardless single-company source that
  enumerates postings from `jobs.micro1.ai/sitemap.xml` and maps each
  `/post/<uuid>` detail page to a normalized `Job`.
- Register `micro1` in the source registry and add `sources/micro1.yml` with a
  single boardless entry (`company: micro1`).
- Parse the detail page's Next.js RSC flight payload (there is no standalone
  `<script type="application/ld+json">` tag): extract the embedded job `data`
  object and resolve its referenced description chunk.
- Emit structured `Skills` from micro1's `required_skills` list into the pipeline
  facet seam.

## Capabilities

### New Capabilities
- `micro1-source`: crawl the micro1 job board (sitemap enumeration + per-posting
  RSC-flight detail parse) and normalize its postings into the job catalogue.

### Modified Capabilities
<!-- No existing spec-level behavior changes; the adapter plugs into the existing
     source registry and pipeline contracts without altering their requirements. -->

## Impact

- **New code**: `internal/sources/micro1.go`, `internal/sources/micro1_test.go`.
- **Registry**: `internal/sources/source.go` (register the `micro1` provider).
- **Config**: `sources/micro1.yml` (new board file, one boardless entry).
- **Runtime**: adds one `cmd/ingest sources/micro1.yml` crawl target; no schema,
  API, or migration changes. Close-detection is handled by the existing pipeline
  sweep (postings dropping out of the sitemap are closed).
- **Dependencies**: none new; reuses `XMLGetter`/`HTMLGetter`, `fetchDetails`,
  `sanitizeHTML`, and the structured-facets seam.
