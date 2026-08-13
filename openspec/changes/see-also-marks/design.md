## Context

`web/src/lib/collections.ts` defines `FILTER_COLLECTIONS`, a frontend-only registry of ~90 entries (languages, frameworks, cloud/infra tools, seniority levels, roles/categories, remote/geo combinations), each carrying a `params: Record<string, string | string[]>` that maps to a `/jobs` facet filter. `web/src/routes/jobs/[slug]/+page.server.ts`'s `buildSeeAlso()` resolves a job's related collection slugs (via `relatedCollectionSlugs()`, falling back to `POPULAR_COLLECTION_FALLBACK`), fetches live counts per slug from the facet API, and returns `SeeAlsoCard[]` to `JobSeeAlso.svelte`, which renders a horizontal-scroll row.

Today `SeeAlsoCard.mark` is `string | null`, populated only via `backerBadges([slug])[0]?.mark` — a lookup into `backers.ts`'s hand-maintained `MARKS: Record<string, string>` of committed PNG paths for backer collections (YC, Techstars, a16z). Every non-backer card (the vast majority) renders with no mark at all.

The user brainstormed and approved the target visual design interactively (mockups in `.superpowers/brainstorm/`, written up in `docs/superpowers/specs/2026-08-13-see-also-marks-design.md`). This design.md carries the technical decisions needed to implement it.

## Goals / Non-Goals

**Goals:**
- Every "See also" card gets a circular visual mark, sourced from the most specific thing available: a real brand logo for technology, the country's flag for a country collection, or a color-coded family icon otherwise.
- Reuse existing building blocks (`CountryFlag.svelte`, `@lucide/svelte`, the `backers.ts` pattern) rather than inventing new infrastructure.
- Keep mark resolution server-side, in the same place (`buildSeeAlso()`) it already happens — no client-side fetch, no new API surface.

**Non-Goals:**
- No change to which collections are selected for a job's "See also" row.
- No change to the row's layout (still a horizontal-scroll strip of fixed-width cards) — this is a mark/detail change only.
- No change to `backers.ts`, the `/collections` hub page, or the Go-side `job-collections` capability (company-level collection membership is a separate, unrelated concept from the frontend's `FilterCollection` registry).
- No attempt at 100% brand-logo coverage of every `skills`-param collection on day one — missing entries degrade gracefully to the family icon.

## Decisions

**1. `SeeAlsoCard.mark` becomes a discriminated union, not a richer string.**

```ts
export type SeeAlsoMark =
  | { kind: 'image'; src: string }
  | { kind: 'logo'; path: string; hex: string }
  | { kind: 'flag'; countryCode: string }
  | { kind: 'family'; icon: FamilyIconName; color: string };
```

Alternative considered: keep `mark` as a single image URL and pre-render every kind (flags, family icons) to a static SVG file server-side, so the component stays a dumb `<img>`. Rejected — it would mean generating and committing dozens of throwaway SVG files for family icons/flag combinations that already exist as components (`CountryFlag.svelte`) or are trivially rendered by Lucide, adding a build step for no benefit. A typed union keeps each kind's real data (hex, path, icon name) available to the component, which already renders conditionally (`{#if card.mark}`).

**2. Family is derived from which `params` key is present — no new metadata field on `FilterCollection`.**

`skills` → tech; `category` → role; `seniority` → seniority; `work_mode` + (`regions` or a `countries` value that isn't a single real country, e.g. absent) → remote; `collections` → company. A `countries` param with a concrete ISO code takes precedence and yields `flag`, not `family`.

`collections` was missed in the first pass of this design and caught in code review: `collectionBySlug()` resolves not just `FILTER_COLLECTIONS` but also `COMPANY_COLLECTIONS` (the Go-generated registry — backer, editorial, *and* credential kinds, e.g. "Unicorns", "Fortune 500", "H-1B sponsor history"), all to `{ params: { collections: slug } }`, and `relatedCollectionSlugs()` pulls every kind a job's company carries into "See also", not just backer ones. Only the 4 backer slugs have a real mark (via `backers.ts`); the other ~11 editorial/credential slugs were silently falling through to the generic `tech` family icon — a semantic mismatch this design's own goal (distinguishable card kinds) explicitly rules out. Fixed by adding a fifth family, `company` (amber, Lucide `Building2`), rather than reusing `tech`.

Alternative considered: add an explicit `family: 'tech' | 'role' | ...` field to every one of the ~90 `FILTER_COLLECTIONS` entries. Rejected — the `params` key already encodes this unambiguously for every existing entry (confirmed by inspection: no entry mixes e.g. `skills` and `seniority`), so a derived function is less to keep in sync by hand.

**3. Brand logos come from the `simple-icons` npm package, imported by name, not hotlinked from a CDN.**

The brainstorm mockups used `cdn.simpleicons.org` for visualization only. Production renders inline SVG from the `simple-icons` package's per-icon named exports (`siPython`, `siAmazonaws`, ...), each carrying `{ path, hex }`. This avoids a third-party network dependency at request time, matches the project's general avoidance of runtime external asset fetches, and each icon is tree-shaken individually so unused brands don't bloat the bundle.

**4. `web/src/lib/techmarks.ts` is a hand-maintained slug→icon map, mirroring `backers.ts`'s `MARKS`.**

Alternative considered: auto-derive the `simple-icons` slug from the collection slug (e.g. uppercase-and-strip). Rejected — several collection slugs don't match their `simple-icons` slug 1:1 (`aws` → `siAmazonaws`, `gcp` → `siGooglecloud`, `nodejs` → `siNodedotjs`, `dotnet` → `siDotnet`, `cpp` → `siCplusplus`), so a derivation function would need its own exception list anyway. A flat map is simpler and matches the existing `backers.ts` precedent the codebase already trusts.

**5. Logo glyph color (white vs near-black) is computed from the brand hex via YIQ luminance, not hardcoded per brand.**

Keeps the registry to just `{ path, hex }` per entry — no third field to keep correct as brands are added, and it's a well-known, cheap formula (`(r*299 + g*587 + b*114) / 1000`, threshold ~128).

**6. Resolution order in `buildSeeAlso()`: backer → tech logo (if `skills` param resolves in `techmarks.ts`) → country flag (if `countries` param present) → seniority/role/remote/company family (by `params` key) → tech family (default).**

This is a strict fallback chain per card, not a merge — a card gets exactly one mark.

## Risks / Trade-offs

- **[Risk]** `simple-icons` is a large package (thousands of icons); importing it carelessly (default/namespace import) could bloat the client bundle. → **Mitigation**: import only named icons actually used in `techmarks.ts` (`import { siPython } from 'simple-icons'`), confirmed tree-shakeable by the package's per-icon ESM exports; verify with a bundle-size check during implementation.
- **[Risk]** `techmarks.ts` coverage is incomplete on day one (not every `skills`-param collection has a curated import) → renders the family icon instead, which is an intentional degrade, not a bug — but a future skill added to `FILTER_COLLECTIONS` without a corresponding `techmarks.ts` entry will silently look generic rather than erroring. → **Mitigation**: none needed structurally; acceptable per the design (matches `backers.ts`'s existing "missing mark degrades silently" precedent, not treated as a coverage-invariant to enforce in code or tests, only spot-checked in review).
- **[Trade-off]** Backer PNG marks switch from `rounded-sm` to `rounded-full` alongside every other kind, for visual consistency across the row. Existing backer marks (YC, Techstars) are roughly square/centered so `object-contain` inside a circle is expected to read fine, but this is a visual judgment call, not verified pixel-by-pixel for every backer mark in this design phase — worth a visual glance during implementation.

## Migration Plan

No data migration — this is a pure frontend rendering change over data already computed at request time. Deploy is a normal frontend release: merge, build, ship. No feature flag — the row degrades gracefully (family icon fallback) even with partial `techmarks.ts` coverage, so there is no unsafe intermediate state. Rollback is a normal revert.

## Open Questions

None outstanding — scope and approach were confirmed interactively with the user during brainstorming (see `docs/superpowers/specs/2026-08-13-see-also-marks-design.md`).
