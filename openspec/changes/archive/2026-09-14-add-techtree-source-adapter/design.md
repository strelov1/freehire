## Context

See `proposal.md` for motivation, including the correction from this change's first draft:
the detail page is server-rendered, not a client-only SPA, so no browser or proxy tier is
needed.

Relevant existing machinery this design reuses rather than duplicates:

- `internal/ingest/sources.Source` / the `aggregator` + `boardless` marker interfaces
  (`registry.go`), the same combination `thehub` already uses for "one site-wide crawl, no
  per-tenant board, company read from each posting."
- `internal/ingest/sources/sitemap.go`'s `sitemapJobLocs(ctx, c, url, id)` — the shared
  flat-sitemap-plus-id-filter helper. TechTree's `sitemap.xml` is a flat `<urlset>` (not a
  `<sitemapindex>`, unlike `thehub`/`gulftalent`), so no `resolveSubSitemap` step is needed —
  this is actually simpler than either existing precedent.
- `internal/ingest/sources/jsonld.go`'s `ldJobPosting(root, v)` — decodes the page's first
  `schema.org/JobPosting` block into a caller-supplied struct. `thehub.go` is the closest
  worked example: same helper, same "sitemap → per-page ld+json → `Job`" shape.
- `internal/ingest/sources/html.go`'s `firstByClass` + `innerHTML` — for reading the full
  rich-text description out of the DOM (see Decisions).
- `internal/ingest/sources/schema.go`'s `schemaEmploymentType` for mapping
  `JobPosting.employmentType`. **Not** `schemaAddress`/`schemaPlace` — TechTree's
  `jobLocation.address` is emitted as a plain string (`"Latin America (remote)"`), not a
  `PostalAddress` object, so it needs its own field type (schema.org's `address` property
  does allow either shape; TechTree happens to use the string form).

Verified directly against a live page (2026-09-14, this change's own spike, both via a plain
`curl`/read and independently via `internal/platform/browser`'s in-page fetch — same bytes
either way, confirming no JS execution is involved in producing them): the response is
31,531 bytes of fully server-rendered HTML containing a `<script type="application/ld+json">`
block

```json
{"@context":"https://schema.org","@type":"JobPosting","title":"Mid/Senior AI Engineer, New Products (0→1)","description":"Mid/Senior AI Engineer, New Products at Telepatia - turn vague product bets into validated 0→1 products, fast","datePosted":"2026-09-11T07:46:06.765320","hiringOrganization":{"@type":"Organization","name":"Telepatia"},"jobLocation":{"@type":"Place","address":"Latin America (remote)"},"url":"https://jobs.techtree.dev/job/8773660e-c2c3-4b49-9d03-6f9f76859bc8","employmentType":"FULL_TIME"}
```

and, separately in the DOM, a `<div class="... prose prose-base ...">` container holding the
full rich-text body (`<h2>Why Telepatia</h2><p>...</p><h2>What you'll do</h2><ul>...`). The
`description` field above is confirmed to be the SAME short text as the page's
`<meta name="description">` — not the full body — on every field compared.

## Goals / Non-Goals

**Goals:**
- Ingest TechTree's ~100 postings into the catalogue via the plain HTTP transport every
  unmetered adapter already uses, with the full rich-text description, not the page's short
  summary.
- Keep the adapter itself the same shape as `thehub`/`gulftalent`/`compleo`, so a future
  reader recognizes the pattern immediately rather than learning a one-off structure.

**Non-Goals:**
- A TechTree-specific non-technical title vocabulary — use the generic
  `classify.ConfirmedNonTech` gate until measurement shows a gap.
- Cross-source dedup against a posting's first-party ATS copy — same deferral `wellfound`
  makes, for the same reason.
- Any TechTree apply-flow integration (`/job/<uuid>/apply` is an external redirect, not
  something this adapter touches).

## Decisions

**Enumerate via the flat `sitemap.xml`, using `sitemapJobLocs` directly — no sub-sitemap
resolution.** Confirmed live: TechTree's sitemap is a single `<urlset>` listing all ~100 job
URLs plus 3 static pages, unlike `thehub`'s sitemap INDEX pointing at a `jobs` sub-sitemap.
`sitemapJobLocs(ctx, c, "https://jobs.techtree.dev/sitemap.xml", techtreeJobID)` is the whole
enumeration step; `techtreeJobID` is a regex requiring the `/job/<uuid>` shape, so `/`,
`/talent-scout`, and `/terms-of-service` are naturally excluded.

**Structured fields from `ldJobPosting`, full description from a DOM class match — not one
source for both.** The ld+json block's `description` is confirmed short (the meta-description
text, not the body). This is the same split `internal/ingest/sources/AGENTS.md` documents for
`broadpeak` ("the listing carries no description at all... the body lives only on the
posting's own page, in a server-rendered `<div class="description rich-text …">` block...a
plain exact class-token match"), adapted here to a DOM match on the description page itself
rather than a separate listing page. `firstByClass(root, "prose")` finds the container (a
Tailwind typography utility class applied to exactly one element per job page — the rich-text
body — confirmed on the sampled page); `innerHTML` + `sanitizeHTML` produce the stored
description.

*Alternative considered*: use `ldJobPosting`'s own `description` field, matching
`thehub`/`gulftalent`'s simpler shape. Rejected — verified live that it is truncated to the
same one-sentence summary as the meta description on every posting compared, which would
ship every TechTree job with a materially incomplete body (see the spec's "full posting
body, not the short summary" requirement).

**`jobLocation.address` decodes as a plain string field, not `schemaAddress`.** TechTree
emits `"jobLocation":{"@type":"Place","address":"Latin America (remote)"}` — a bare string,
not a `PostalAddress` object. A `techtreePosting` struct with `JobLocation struct{ Address
string }` decodes this directly; reusing the shared `schemaAddress`/`schemaPlace` types would
fail every posting's decode (a struct destination cannot unmarshal a JSON string). This also
means the location text itself already carries a "(remote)" qualifier when applicable, which
`isRemote(location)` (the same substring-based helper `thehub`/`gulftalent` already use)
picks up unchanged.

**`ExternalID` is the job UUID from the URL path, never the `tp` query parameter.** Unchanged
from the first draft — `tp` was observed on every page including the "All jobs" link, so it
reads as tracking, not identity. `techtreeJobID` is a regex over the path
(`/job/([0-9a-f-]{36})`, or similar), matching the shape `thehub`'s/`gulftalent`'s own id
extractors use.

**Registered as `aggregator` + `boardless`, exactly like `thehub`.** Each posting names its
own hiring company and the job URL carries no tenant slug — the same shape as `thehub`, not
a per-board pattern needing `cmd/add-board` at all.

**Every posting is hydrated on every crawl (not a `HydratingSource`).** At ~100 total
postings, the Factorial-shape precedent applies directly, same as the first draft's
reasoning — unaffected by the transport correction.

## Risks / Trade-offs

**[The "prose" class match is a Tailwind utility class, not a semantic one]** A page-wide
Tailwind rebuild could change or remove it with no notice, unlike a semantic class name a
redesign is less likely to touch → Mitigation: the "full body required or the posting is
dropped" requirement (see spec) turns a broken match into a visible per-posting drop rather
than a silently truncated description; `firstByClass` matching an unintended element earlier
in the page (a false positive) is checked for during fixture capture — if the sampled page
carries "prose" only once, this risk is theoretical rather than observed.

**[Vendor-controlled, unversioned markup]** TechTree could change its frontend build or DOM
structure with no notice → Mitigation: same as above — a loud per-posting failure, and the
crawl-wide "read nothing of what was listed" failure if it affects every posting at once.

**[Small catalogue size limits the value of the investment]** ~100 postings is modest →
Mitigation: none needed; this is a call for the curator/maintainer to make when prioritizing
the change, not a technical risk.

**[`tp` turns out to carry tenant meaning on some posting not yet sampled]** → Mitigation:
low likelihood given it appears on the site's own non-job "All jobs" link; flagged as a spec
scenario so a contradicting observation would be caught by a fixture-derived test, not missed
silently.
