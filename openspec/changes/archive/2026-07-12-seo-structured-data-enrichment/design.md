## Context

`web/src/lib/seo.ts` is a pure helper module: each function takes public wire
shapes (`Job`, `Company`, collection) and returns a plain JSON-LD object, which
route `+page.svelte` files compose via `jsonLdScript([...])` inside
`<svelte:head>`, emitted server-side. Two gaps: `organizationJsonLd` returns only
`{ name, url }` despite rich `company_info`, and `/collections/:slug` emits only a
`BreadcrumbList`. All changes stay inside this pure layer plus one route's head
composition — the established pattern, no new infrastructure.

## Goals / Non-Goals

**Goals:**
- Enrich company `Organization` JSON-LD from fields already on the `Company`
  wire shape, each field strictly conditional.
- Add `CollectionPage` + summary `ItemList` to collection landings from the
  already-SSR-loaded first page of jobs.
- Full unit coverage of both pure helpers via vitest.

**Non-Goals:**
- Per-collection FAQ, `ItemList` on company pages, meta-description rewrites,
  Core Web Vitals — all explicitly deferred.
- No backend / API / schema / dependency changes.

## Decisions

- **Conditional-field builder for Organization.** `organizationJsonLd` reads
  `company.company_info` (logo, description, website, linkedin) and top-level
  facts (`year_founded`, `employee_count`, `hq_country`). Each field is added
  only when present and non-empty; `sameAs` is the filtered non-empty
  `[website, linkedin]` array (omitted entirely if both absent); `foundingDate`
  is `String(year_founded)`; `numberOfEmployees` is a `QuantitativeValue`;
  `address` is a `PostalAddress` with `addressCountry` uppercased. This mirrors
  the existing conditional style in `jobPostingJsonLd` (never emit a made-up or
  empty value — a ranking liability).
- **Summary ItemList, not nested JobPostings.** New `collectionPageJsonLd(title,
  description, url, jobs, origin)` returns a `CollectionPage` whose `mainEntity`
  is an `ItemList` of `ListItem`s (`position`, `name` = job title, `url` =
  `${origin}/jobs/${public_slug}`). Google discourages many full `JobPosting`
  nodes on a list page; the summary/url shape is the recommended carousel form
  and keeps payload small.
- **Reuse loaded data.** The collection route already loads `data.initial.jobs`
  server-side; the helper consumes that array — no extra fetch. Composed into the
  existing `jsonLdScript([breadcrumbJsonLd(...), collectionPageJsonLd(...)])`.
- **TDD on the pure layer.** `seo.test.ts` (new) drives both helpers: enriched vs
  empty `company_info`, `sameAs` filtering, empty job list → empty `ItemList`.
  Route wiring is verified by `svelte-check` and a post-deploy live schema fetch.

## Risks / Trade-offs

- **`company_info` shape drift.** Fields are optional on the wire type; the
  builder guards every access, so a missing field degrades to omission, never a
  crash or empty node.
- **List size.** The `ItemList` uses the first SSR page (~20–25 items); no
  pagination of structured data — acceptable and within summary-page norms.
- **Escaping.** Both helpers flow through the existing `jsonLdScript`, which
  already escapes `<` — no new XSS surface.
