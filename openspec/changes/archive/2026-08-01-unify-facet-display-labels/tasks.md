## 1. Bind the category map to the generated vocabulary

- [x] 1.1 Add `web/src/lib/labels.test.ts` pinning the settled wordings
  (`ai_engineering` → "AI Engineering", `fullstack` → "Full-Stack",
  `RELOCATION_LABELS.not_supported` → "Not supported") and the title-cased fallback.
- [x] 1.2 Extend `CATEGORY_LABELS` in `web/src/lib/labels.ts` to cover every `CATEGORY_VALUES`
  entry including `other`, applying the settled wordings; add `RELOCATION_LABELS`. Amend the
  file's opening comment to record why this one map is exhaustive while its siblings stay
  override-only.
- [x] 1.3 Constrain the map with `satisfies Record<Category, string>` so exhaustiveness is a
  compile error in both directions, and delete the runtime coverage assertion it replaces.
  (Adopted after review — verified: `TS1360` names an unlabelled code, `TS2353` names a stale
  one, and `pnpm run check` runs in the required `web` CI job.)

## 2. Give the label lookup one home

- [x] 2.1 Move `facets.ts`'s `humanize` into `labels.ts` as the exported `titleCase`, and
  `facets.ts`'s `categoryLabel` into `labels.ts` alongside it. Import `titleCase` back into
  `facets.ts` for `options()`, `companyLabel`, `roleLabel` and `sourceLabel`.
- [x] 2.2 Point `web/src/lib/filterSections.ts` and `web/src/lib/components/ProfileForm.svelte`
  at `$lib/labels` for `categoryLabel`. No re-export shim in `facets.ts`.
- [x] 2.3 Replace `facets.ts`'s inline relocation overrides with `RELOCATION_LABELS`.

## 3. Retire the `/insights` fork

- [x] 3.1 Delete `CATEGORY_LABELS`, `SENIORITY_LABELS` and the body of `categoryLabel` from
  `web/src/lib/insights.ts`; reduce `seniorityLabel` to the empty-string band plus a delegation
  to the shared map and `titleCase`.
- [x] 3.2 Point the three `/insights` loaders
  (`routes/insights/{roles,salary,skills}/[category]/+page.server.ts`) at `$lib/labels` for
  `categoryLabel`.
- [x] 3.3 Move the `categoryLabel` cases out of `web/src/lib/insights.test.ts` (they now live in
  `labels.test.ts`) and add cases pinning `seniorityLabel('')` to "All levels" and a real token
  to its shared-vocabulary label. Confirm the intro-sentence and `coveredCategories` tests still
  pass unchanged.

## 4. Align the job-detail facet rows

- [x] 4.1 Replace `enrichment.ts`'s module-level `RELOCATION` with `RELOCATION_LABELS` from
  `labels.ts`.
- [x] 4.2 Rename `enrichment.ts`'s `humanize` to `sentenceCase` and correct its doc comment,
  which claimed title case and demonstrated sentence case. Do not merge it with `titleCase` —
  the other facets on that page depend on it.

## 5. Close the fourth surface (found in review)

- [x] 5.1 `routes/open/+page.svelte` labels its seniority and work-mode distributions with a
  private fallback, so the sitemap'd `/open` page reads "C level" and "Onsite". Point those two
  buckets at `SENIORITY_LABELS` / `WORK_MODE_LABELS`; leave skills on the local fallback, being
  an open vocabulary with no shared map.
- [x] 5.2 Narrow the first spec requirement: a surface must not declare a competing map or
  label a code without consulting the shared one, but may keep its own fallback for codes
  outside it — which is what `enrichment.ts` does by design, and what the requirement as first
  written wrongly forbade.

## 6. Verify and finish

- [x] 6.1 Run the web gate: `pnpm test`, `pnpm check`, `pnpm lint`, `pnpm build`.
- [x] 6.2 Grep the tree for any surviving second declaration of a category or relocation label
  map, and for any remaining importer of `categoryLabel` from `$lib/facets` or `$lib/insights`.
- [ ] 6.3 Visually confirm the converged surfaces render the settled strings: a category in the
  filter panel, the Category and Relocation rows of a job detail page, an
  `/insights/roles/<category>` H1, and the `/open` seniority bucket.
