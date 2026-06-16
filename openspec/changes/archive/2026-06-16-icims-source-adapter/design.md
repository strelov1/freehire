## Context

The full investigation and approved technical design live at
`docs/superpowers/specs/2026-06-16-icims-source-adapter-design.md`; this document
summarizes the decisions for OpenSpec tracking. iCIMS career sites at
`https://careers-{slug}.icims.com` expose a flat `sitemap.xml` of
`/jobs/{id}/.../job` URLs; the canonical job page is a SPA/WordPress wrapper, but
its embedded `<iframe src=".../job?in_iframe=1">` fragment is server-rendered and
carries a full schema.org `JobPosting` JSON-LD block. The established
`internal/sources/successfactors.go` adapter (sitemap → bounded detail fetch) and
the `ldJobPosting` helper in `internal/sources/jsonld.go` are the reuse seam.

## Goals / Non-Goals

**Goals:**
- An `icims` `Source` adapter yielding the normalized job shape (title, url,
  location, description, native id, posted_at, remote/work-mode) with rich data.
- A `cmd/harvest-boards` `icimsProber` that live-validates a slug (sitemap lists
  ≥1 job) so dead candidates never enter `sources/icims.yml`.

**Non-Goals:**
- Running the harvest over the ~9,937 seed slugs, ingesting, deploying.
- Parsing sitemap-index (nested) sitemaps — not observed in sampling.
- Resolving a "pretty" company display name (slug fallback, per lever/ashby).

## Decisions

- **Sitemap + JSON-LD detail over sitemap-only.** Feashliaa's reference parses
  the sitemap alone (title from URL slug, no description). We fetch
  `?in_iframe=1` and parse JSON-LD for the same request count, because the
  description feeds enrichment and the deterministic dict facets
  (skills/region/seniority). Rejected sitemap-only (breaks those facets) and a
  headless browser (unnecessary — the fragment is server-rendered).
- **Transport shape mirrors successfactors:** `icimsHTTP = XMLGetter + HTMLGetter`.
  `board` is the slug; the domain is `careers-{board}.icims.com`.
- **Detail mapping:** id from `/jobs/(\d+)/`; `Location` =
  `joinNonEmpty(locality, region, country)` dropping the `"UNAVAILABLE"`
  placeholder; `Description` = `sanitizeHTML(...)`; `PostedAt` =
  `parseRFC3339(datePosted)` (Go's parser accepts the fractional `.000Z`);
  `Company` = `firstNonEmpty(hiringOrganization.name, e.Company)`; `WorkMode` =
  `"remote"` when `jobLocationType == "TELECOMMUTE"`, else `isRemote(location)`
  sets only the `Remote` flag. `jobLocation` is an array — use the first entry.
- **Prober transport widening:** add `sources.XMLGetter` to the prober
  `httpClient` interface (the real `*sources.Client` already implements it). The
  `icimsProber` requires the sitemap to list ≥1 `/jobs/{id}/` URL, which rejects
  both HTTP 404 and HTTP 200-with-zero-jobs; company name falls back to the slug.

## Risks / Trade-offs

- [Large boards may use a nested sitemap-index] → Not observed across sampled
  live boards (counts 9–341, flat `urlset`). If one appears, those postings are
  silently missed until index parsing is added — acceptable for now, noted as a
  follow-up seam (same posture as successfactors).
- [iCIMS markup drift could break JSON-LD extraction] → Mitigated by parsing the
  standard schema.org `JobPosting` block via the shared `ldJobPosting` helper
  (already used by teamtailor/breezy/linksource), not bespoke selectors.
- [Detail fetch doubles request count vs sitemap-only] → Bounded by
  `defaultDetailWorkers` (8), identical to every other detail adapter.

## Open Questions

None — design approved.
