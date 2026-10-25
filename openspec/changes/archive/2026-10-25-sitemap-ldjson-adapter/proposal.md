## Why

Sites that publish jobs as schema.org `JobPosting` blocks but do not run a supported ATS have no way into the catalogue. Issue #3565 asks for one adapter that can take many such sites — company career pages and public job boards — without a new env file or a per-site parser.

## What Changes

- Add a `sitemapldjson` ingest adapter. One board is one sitemap URL (`sitemap.xml` or `sitemap.xml.gz`, including a sitemap index). Each listed page is read for a `JobPosting` ld+json block.
- The optional URL filter for a board is `CompanyEntry.Region` (substring, or a `*` glob). Pages with no `JobPosting` block are skipped.
- Detail fetches run only for postings the catalogue does not already have, and only up to a fixed cap per board per run. Postings that leave the sitemap are closed by the ordinary unseen sweep. There is no retention setting and no robots.txt reader.
- Register the provider in `sources.All`, give it a display label, and regenerate the source facet.

## Capabilities

### New Capabilities

- `sitemapldjson-source`: crawl a configured sitemap, keep only URLs matching the board's pattern, and hydrate new pages that carry a schema.org `JobPosting`.

### Modified Capabilities

## Impact

- New `internal/ingest/sources/sitemapldjson.go` and its test. Shared sitemap decode gains gzip-body support (`decodeSitemapBody`).
- One registration in `sources.All`, one label in `web/src/lib/facets.ts`, regenerated `SOURCE_VALUES`.
- A live board is added later with `cmd/add-board`, not by this change. Scheduling the provider on the host is the existing timer generator, after the first board exists.
- Out of scope: env-based sitemap lists, a JSON config file, a crawl-delay reader, and `fullBoardListing`.
