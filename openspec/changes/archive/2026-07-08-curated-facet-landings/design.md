## Context

freehire already ships curated filter-collection landing pages at
`/collections/[slug]` (`web/src/lib/collections.ts` → `FILTER_COLLECTIONS`,
rendered by `routes/collections/[slug]/+page.svelte`). The route, the
`excludeFacets = Object.keys(params)` facet-hiding, the self-canonical + breadcrumb
JSON-LD, and the `collectionPaths()` sitemap inclusion all exist. Only two axes are
covered (remote regions, languages/frameworks). The `job-collections` spec is stale
about landing pages (see the spec delta).

## Goals / Non-Goals

**Goals:**

- Add ~33 hand-verified filter collections across four new axes (tech categories,
  seniority, infra skills, named roles), each with a live open-job count ≥ 300.
- Reconcile the `job-collections` spec with the shipped landing-page reality.
- Guard the registry with a slug-uniqueness + non-empty-params unit test.

**Non-Goals:**

- No programmatic generation, no facet-combination pages.
- No new URL namespace — stays `/collections/[slug]`.
- No backend, routing, sitemap, or page-template changes.

## Decisions

- **Curated, not programmatic** — each value is added by hand only after its live
  count is confirmed ≥ 300. Alternative (auto-generate from facet counts) rejected:
  thin-content risk and Google penalties for mass low-value pages.
- **Reuse `/collections/[slug]`** — not `/skills/[skill]` / `/categories/[cat]`.
  Alternative (dedicated namespaces) rejected: orphans the already-ranking
  `/collections/python`, needs redirects, inconsistent with the 42 existing pages.
- **Singular facet params** — `category=`, `seniority=`, `role=`, `skills=` (the
  `/jobs` feed ignores plural forms). Verified live before listing each value.
- **Slugs are bare canonical tokens** (`backend`, `senior`, `software-engineer`) —
  no collision with existing skill slugs; each drives the `"<title> jobs"` heading.

## Risks / Trade-offs

- [Seniority pages have generic intent ("senior jobs")] → count-gated and honestly
  worded; droppable later if they underperform.
- [The `role` facet mixes non-tech / seniority-only values] → only clearly-technical
  named roles with an individually-verified count are taken.
- [A collection's live count could fall below the floor over time] → counts are
  decorative and the feed still renders; periodic curation review, not automated.

## Migration Plan

Frontend-only data addition. Ships with the normal web release (blue/green on
host-2). No migration, no rollback complexity — reverting the commit removes the
pages; existing pages are untouched.

## Open Questions

None — axes, floor, and URL structure are decided.
