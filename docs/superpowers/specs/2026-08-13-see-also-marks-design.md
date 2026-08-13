# "See also" cards: real marks instead of blank/PNG-only icons

## Problem

The "See also" row (`web/src/lib/components/JobSeeAlso.svelte`) on a job page links to related `/collections/:slug` pages (e.g. "Python jobs", "Senior-Level jobs", "AWS jobs"). Today every card is visually identical — a title and a job-count pill — except the rare backer-collection card (YC, Techstars) that gets a small square PNG mark. The row reads as a flat wall of text and is hard to scan.

## Goal

Give every card a recognizable circular mark: a real brand logo for technology/cloud/tool collections, the country's flag for country collections, and a color-coded family icon for everything else (seniority, work mode, role/category). Keep the row's current horizontal-scroll layout — this is a mark/detail change, not a layout redesign.

## Mark resolution: three kinds, one field

`SeeAlsoCard.mark` (`web/src/lib/collections.ts:611`) changes from `string | null` (an image URL) to a discriminated union, resolved server-side in `buildSeeAlso()` (`web/src/routes/jobs/[slug]/+page.server.ts:51`) exactly where it resolves today:

```ts
export type SeeAlsoMark =
  | { kind: 'image'; src: string }               // unchanged: existing backer PNG (backers.ts)
  | { kind: 'logo'; path: string; hex: string }   // brand SVG path data + brand color, from simple-icons
  | { kind: 'flag'; countryCode: string }         // renders <CountryFlag code={countryCode} />
  | { kind: 'family'; icon: FamilyIconName; color: string }; // Lucide icon + family color
```

Every card gets a mark now — there is no longer a "blank card" case. `JobSeeAlso.svelte`'s `{#if card.mark}` guard goes away.

Resolution order in `buildSeeAlso()`, per resolved collection slug:

1. **Backer collection** (`COLLECTIONS.find(c => c.kind === 'backer')`) → `{ kind: 'image', src: backerBadges([slug])[0].mark }`, unchanged from today.
2. **`FilterCollection` with a `skills` param** → look up the slug in the new tech-mark registry (below). Hit → `{ kind: 'logo', ... }`. Miss → falls through to family icon, same as any other skill (never a broken image).
3. **`FilterCollection` with a `countries` param** (single ISO code, e.g. `{ countries: 'de' }`) → `{ kind: 'flag', countryCode }`.
4. **Everything else** (`category`, `seniority`, `work_mode`+`regions`, `work_mode`+`countries: 'global'`-style entries without a specific country) → family icon, family derived from which param key is present.

This mirrors the existing `params` shape on `FilterCollection` — no new metadata field on the 90-entry registry, family is derived from the param key already there.

## Tech-mark registry

New file `web/src/lib/techmarks.ts`, same shape and spirit as `backers.ts`'s `MARKS`: a hand-maintained `Record<string, SimpleIcon>` from **our** collection slug to a named import from the `simple-icons` npm package (new dependency in `web/package.json`, alongside the already-present `@lucide/svelte`). Import icons individually (`import { siPython, siAmazonaws, siMicrosoftazure, ... } from 'simple-icons'`) so the bundler tree-shakes everything unused — do not import the package's default export.

```ts
import { siPython, siAmazonaws, /* … */ } from 'simple-icons';

export const TECH_MARKS: Record<string, { path: string; hex: string }> = {
  python: siPython,
  aws: siAmazonaws,
  azure: siMicrosoftazure,
  gcp: siGooglecloud,
  csharp: siCsharp,
  cpp: siCplusplus,
  nodejs: siNodedotjs,
  nextjs: siNextdotjs,
  dotnet: siDotnet,
  // … one entry per `skills` value in FILTER_COLLECTIONS (~44 today)
};
```

A collection slug with a `skills` param and no entry here silently falls back to the family icon — same "we don't have a verified mark, so show nothing brand-specific" posture as `backers.ts`, just with a non-empty fallback instead of a blank card. No test needs to enforce full coverage; missing entries degrade, they don't break.

**Icon fill contrast**: background is the brand's own `hex` (a filled circle); the SVG path is drawn white or near-black depending on the background's luminance (YIQ formula, threshold ~128), computed once per mark at render time — not hardcoded per brand. This is why `react`'s pale cyan background gets a dark glyph and `aws`'s orange gets a white one, without a manual exception list.

## Family icons

New `FamilyIconName` union and a fixed lookup table (component, not string-keyed dynamic import) in `JobSeeAlso.svelte` or a small co-located `familymarks.ts`:

| Param key present | Family | Lucide icon | Color |
|---|---|---|---|
| `skills` (no tech-mark hit) | tech (generic) | `Code2` | indigo |
| `category` | role | `Layers` | violet |
| `seniority` | seniority | `TrendingUp` | emerald |
| `work_mode` + `regions`/global `countries` | remote | `Globe2` | cyan |

(Exact hex values pulled from the design-system color tokens' existing semantic scale, not new colors invented for this feature.)

## Country flags

`{ kind: 'flag', countryCode }` renders the existing `CountryFlag.svelte` (`web/src/lib/components/CountryFlag.svelte`) unchanged — it already fills its circular container edge-to-edge via `flag-icons`' `fi fis` classes. No new component.

## Component changes — `JobSeeAlso.svelte`

The card layout is untouched (horizontal scroll strip, `w-40 shrink-0`, border, `hover:bg-muted`). Only the mark slot changes:

- Size bumps from `size-6` (24px) to `size-7` (28px) to keep a padded logo legible.
- Shape: `rounded-full` (was `rounded-sm`) for every kind, backer PNG included — the backer marks are already roughly square/centered, `object-contain` inside a circle reads fine.
- Render branches on `card.mark.kind`:
  - `image` → today's `<img>`, now circular.
  - `logo` → circular div, `background-color: {hex}`, inline `<svg>` with the path, fill computed for contrast.
  - `flag` → `<CountryFlag code={countryCode} />`.
  - `family` → circular div, `background-color: {color}`, the matching Lucide component centered inside, white glyph.

## Out of scope

- No change to which collections appear in "See also" (`relatedCollectionSlugs`, `POPULAR_COLLECTION_FALLBACK`) — this is a presentation-only change to the existing card set.
- No change to the `/collections` hub page's own cards — only the job-page "See also" row. (A follow-up could reuse `TECH_MARKS`/family icons there; not this change.)
- No new backer marks, no changes to `backers.ts`.
- Company page's own "See also" (if any reuses `JobSeeAlso`) inherits the change for free since it's the same component and same `SeeAlsoCard` type.

## Testing

- Existing coverage for `buildSeeAlso()` / `SeeAlsoCard` resolution (if any) extends to assert the right `mark.kind` per collection family — backer slug → `image`, a `skills`-registry hit → `logo`, a `countries` slug → `flag`, everything else → `family`.
- Visual check: load a job page whose facets hit all four kinds in one row (e.g. a senior Python AWS remote-Germany job) and confirm no broken images, readable contrast on both light and dark brand-color backgrounds.
