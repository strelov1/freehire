## Why

The "See also" row on a job page (`web/src/lib/components/JobSeeAlso.svelte`) links to related `/collections/:slug` pages, but every card looks identical — a title and a job-count pill, with no visual distinction between "Python jobs", "Senior-Level jobs", or "AWS jobs". Only the rare backer-collection card (YC, Techstars) carries a mark today. The row reads as a flat wall of text. Design brainstormed and approved by the user in `docs/superpowers/specs/2026-08-13-see-also-marks-design.md`.

## What Changes

- `SeeAlsoCard.mark` changes from `string | null` (an image URL) to a discriminated union with four kinds: `image` (existing backer PNG, unchanged), `logo` (a real brand SVG + brand-color background, for technology/cloud/tool collections), `flag` (the collection's country, rendered via the existing `CountryFlag` component), and `family` (a color-coded Lucide icon for collections with no real brand — seniority, role/category, remote/work-mode). **BREAKING** (internal type only, no external contract): every consumer of `SeeAlsoCard.mark` must switch on `kind`.
- New file `web/src/lib/techmarks.ts`: a hand-maintained registry mapping `FILTER_COLLECTIONS` slugs with a `skills` param to a named import from the new `simple-icons` npm dependency (~44 entries). A `skills`-param slug with no registry entry falls back to the `family` kind rather than rendering nothing.
- Mark resolution moves entirely server-side in `buildSeeAlso()` (`web/src/routes/jobs/[slug]/+page.server.ts`), preserving the "no client-side fetch" property the component already documents.
- `JobSeeAlso.svelte`'s mark slot becomes circular (`rounded-full`, `size-7` instead of `size-6`/`rounded-sm`) and renders per `mark.kind`. The `{#if card.mark}` blank-card branch is removed — every card now always has a mark.
- No change to which collections appear in "See also" (`relatedCollectionSlugs`, `POPULAR_COLLECTION_FALLBACK`), no change to the row's horizontal-scroll layout, no change to `backers.ts` or the `/collections` hub page.

## Capabilities

### New Capabilities
- `see-also-marks`: resolving and rendering a circular visual mark (brand logo, country flag, or family icon) for each card in a job page's "See also" row, given the card's underlying collection.

### Modified Capabilities
(none — `job-collections` covers company-level collection membership on the Go side and is unaffected; this change is presentation-only for the frontend's separate `FilterCollection` registry, which today carries no spec of its own)

## Impact

- **Affected code**: `web/src/lib/collections.ts` (`SeeAlsoCard` type), `web/src/routes/jobs/[slug]/+page.server.ts` (`buildSeeAlso`), `web/src/lib/components/JobSeeAlso.svelte`, new `web/src/lib/techmarks.ts`, new `web/src/lib/familymarks.ts` (or co-located in the component).
- **Dependencies**: adds `simple-icons` to `web/package.json`. `@lucide/svelte` and `CountryFlag.svelte` already exist and are reused.
- **No API/schema/migration impact** — purely a SvelteKit frontend presentation change on data already computed server-side.
