## Why

The curated-collection datasets (yc-oss, Techstars, European startups, Fortune 500, unicorns) name thousands of companies we don't yet ingest — the import worker logged ~4.3k unmatched YC, ~3.2k Techstars, ~3k European companies. A slug-probe harvest of those names yielded almost nothing (15 greenhouse boards from ~14k candidates, ~0.1%): guessing a board slug from a company name is a weak signal. But the datasets also carry each company's **website**, and a feasibility spike showed that fetching a company's careers page and detecting the ATS link in its HTML resolves the real board ~19% of the time on static HTML alone — ~190× the slug-probe hit rate. This turns the collection datasets into a real coverage worklist: discover the boards of companies we're missing, by following their own sites.

## What Changes

- Add `internal/atsdetect`: a pure function that detects a Greenhouse/Lever/Ashby board (provider + slug) from a page's HTML, covering direct board links and embed variants.
- Add `cmd/harvest-ats`: a run-once host tool with two phases —
  - `extract`: read the collection datasets (reusing the `internal/collections` registry's dataset URLs), parse each company's `(name, website)`, drop companies whose normalized slug is already in our catalogue (a supplied prod company-slug set), and emit the unmatched companies with websites.
  - `resolve`: for each unmatched company, fetch its candidate career pages (homepage, `/careers`, `/jobs`, and a discovered careers link) via the shared `sources` HTTP client, run `atsdetect`, and write per-provider seed files of detected slugs.
- The per-provider seeds feed the **existing** `cmd/harvest-boards <provider> <seed>`, which validates each slug against the provider's official API and appends only live boards to `sources/<provider>.yml` — unchanged. The operator reviews the diff, then deploys sources (no image rebuild) and ingests.

## Capabilities

### New Capabilities
- `domain-ats-harvest`: discover the ATS board of a company by following its website to its careers page and detecting the embedded ATS link — the discovery half of a harvest pipeline whose validation half is the existing `harvest-boards`.

### Modified Capabilities
<!-- none — harvest-boards is reused unchanged; no existing spec's requirements change -->

## Impact

- **Code**: new `internal/atsdetect` + `cmd/harvest-ats`; reads `internal/collections` (dataset URLs) and uses the `internal/sources` HTTP client. No changes to `harvest-boards` or the ingest/serving paths.
- **Ops**: a manual discovery run (host tool) producing a reviewed `sources/*.yml` diff, then the normal sources deploy + ingest. Requires a one-off prod company-slug export (`SELECT slug FROM companies`) as the unmatched filter.
- **Coverage**: static-only fetch resolves ~19%+ of careers pages; JS-only pages are skipped (a documented limitation, headless is a future seam).
