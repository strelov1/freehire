## 1. Bind the category map to the generated vocabulary

- [ ] 1.1 Add `web/src/lib/labels.test.ts` with a failing coverage test: every value of
  `CATEGORY_VALUES` (from `$lib/generated/contracts`) is a key of `CATEGORY_LABELS`, and the
  failure message names the missing codes. Also assert the three settled wordings
  (`ai_engineering` → "AI Engineering", `fullstack` → "Full-Stack") and that
  `RELOCATION_LABELS.not_supported` is "Not supported".
- [ ] 1.2 Make it pass: extend `CATEGORY_LABELS` in `web/src/lib/labels.ts` to cover every
  `CATEGORY_VALUES` entry including `other`, applying the settled wordings; add
  `RELOCATION_LABELS`. Amend the file's opening comment to record why this one map is
  exhaustive while its siblings stay override-only.

## 2. Give the label lookup one home

- [ ] 2.1 Move `facets.ts`'s `humanize` into `labels.ts` as the exported `titleCase`, and
  `facets.ts`'s `categoryLabel` into `labels.ts` alongside it. Import `titleCase` back into
  `facets.ts` for `options()` and `sourceLabel`. Cover the fallback
  (`categoryLabel('quantum_widgets')` → "Quantum Widgets") in `labels.test.ts`.
- [ ] 2.2 Point `web/src/lib/filterSections.ts` and `web/src/lib/components/ProfileForm.svelte`
  at `$lib/labels` for `categoryLabel`. No re-export shim in `facets.ts`.
- [ ] 2.3 Replace `facets.ts`'s inline relocation overrides with `RELOCATION_LABELS`.

## 3. Retire the `/insights` fork

- [ ] 3.1 Delete `CATEGORY_LABELS`, `SENIORITY_LABELS` and the body of `categoryLabel` from
  `web/src/lib/insights.ts`; reduce `seniorityLabel` to the empty-string band plus a delegation
  to the shared map and `titleCase`.
- [ ] 3.2 Point the three `/insights` loaders
  (`routes/insights/{roles,salary,skills}/[category]/+page.server.ts`) at `$lib/labels` for
  `categoryLabel`.
- [ ] 3.3 Move the `categoryLabel` cases out of `web/src/lib/insights.test.ts` (they now live in
  `labels.test.ts`) and add a case pinning `seniorityLabel('')` to "All levels". Confirm the
  intro-sentence and `coveredCategories` tests still pass unchanged.

## 4. Align the job-detail facet rows

- [ ] 4.1 Replace `enrichment.ts`'s module-level `RELOCATION` with `RELOCATION_LABELS` from
  `labels.ts`.
- [ ] 4.2 Rename `enrichment.ts`'s `humanize` to `sentenceCase` and correct its doc comment,
  which currently claims title case and demonstrates sentence case. Do not merge it with
  `titleCase` — the other facets on that page depend on it.

## 5. Verify and finish

- [ ] 5.1 Run the web checks: `pnpm -C web test`, `pnpm -C web check`, `pnpm -C web build`.
- [ ] 5.2 Grep the tree for any surviving second declaration of a category or relocation label
  map, and for any remaining importer of `categoryLabel` from `$lib/facets` or `$lib/insights`.
- [ ] 5.3 Visually confirm the three converged surfaces render the settled strings: a category
  in the filter panel, the Category and Relocation rows of a job detail page, and an
  `/insights/roles/<category>` H1.
