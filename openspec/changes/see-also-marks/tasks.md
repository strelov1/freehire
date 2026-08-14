## 1. Dependency & data registries

- [ ] 1.1 Add `simple-icons` to `web/package.json` and install it
- [ ] 1.2 Create `web/src/lib/techmarks.ts`: `Record<string, { path: string; hex: string }>` mapping every `FILTER_COLLECTIONS` slug with a `skills` param to its named `simple-icons` import (e.g. `aws` → `siAmazonaws`, `gcp` → `siGooglecloud`, `nodejs` → `siNodedotjs`, `cpp` → `siCplusplus`, `dotnet` → `siDotnet`, ...); slugs with no reasonable `simple-icons` match are simply omitted (they fall back to the family icon)
- [ ] 1.3 Create `web/src/lib/familymarks.ts`: a `FamilyIconName` union (`'tech' | 'role' | 'seniority' | 'remote'`) and a fixed table of `{ icon: <Lucide component>, color: <hex> }` per family, colors drawn from the design-system's existing semantic scale

## 2. Pure mark-resolution logic (TDD)

- [ ] 2.1 Write a failing `vitest` test for a glyph-contrast helper (`web/src/lib/markColor.test.ts`, relative imports only, no `$lib`/`$app`): given a brand hex, returns a near-black or white glyph color via YIQ luminance threshold (~128). Implement `web/src/lib/markColor.ts` to pass it.
- [ ] 2.2 Write failing `vitest` tests for a pure mark-resolution function (`web/src/lib/seeAlsoMark.ts` / `.test.ts`, no `$app` imports) that, given a resolved collection's `params` plus an optional backer-image lookup result, returns the `SeeAlsoMark` per the precedence in design.md: backer image → tech logo (via `techmarks.ts`) → country flag (single concrete `countries` value) → family icon (via `familymarks.ts`). Cover one test per precedence branch (mirrors the spec's five scenarios). Implement to pass.
- [ ] 2.3 Update `SeeAlsoCard.mark`'s type in `web/src/lib/collections.ts` from `string | null` to the `SeeAlsoMark` discriminated union (`image` / `logo` / `flag` / `family`)

## 3. Wire into server-side resolution

- [ ] 3.1 Update `buildSeeAlso()` in `web/src/routes/jobs/[slug]/+page.server.ts` to call `resolveSeeAlsoMark()` (from 2.2) per card instead of the current `backerBadges([link.slug])[0]?.mark ?? null` one-liner
- [ ] 3.2 Confirm (via existing route/test coverage or a targeted addition) that `buildSeeAlso()` still returns one card per resolved collection slug with the count logic untouched — only `mark` construction changed

## 4. Component rendering

- [ ] 4.1 Update `web/src/lib/components/JobSeeAlso.svelte`: mark slot becomes `size-7 rounded-full` (was `size-6 rounded-sm`); remove the `{#if card.mark}` guard; render per `card.mark.kind` — `image` keeps today's `<img>`, `logo` draws a circular div in `mark.hex` with an inline `<svg>` of `mark.path` filled per `markColor.ts`, `flag` renders `<CountryFlag code={mark.countryCode} />`, `family` draws a circular div in `mark.color` with the matching Lucide icon centered, white glyph

## 5. Verification

- [ ] 5.1 `pnpm check` (svelte-check) and `pnpm lint` clean on all touched/new files
- [ ] 5.2 `pnpm test` (vitest) green, including the new `markColor`/`seeAlsoMark` suites
- [ ] 5.3 Visual verification: run the dev server, open a job page whose facets hit all four mark kinds in one row (e.g. a senior Python AWS remote-Germany job), confirm no broken images and legible glyph contrast in both light and dark theme
