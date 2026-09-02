## 1. The sort model (pure, `facetModel.ts`)

- [x] 1.1 Extend `JobSort` to `'relevance' | 'newest' | 'match'` and replace the
      `DEFAULT_JOB_SORT` constant with `defaultSortFor(q)` returning `relevance`
      under query text and `newest` without. Update the doc comment on
      `JobFilters.sort`, which currently states there are only two values.
- [x] 1.2 Add the exported pure resolver that collapses `relevance` to `newest`
      when the query is empty, and route both `filtersToParams` and the control's
      value through it.
- [x] 1.3 Teach `filtersToParams` the wire mapping: omit `sort` at the contextual
      default, emit `sort=posted_at` for a non-default `newest`, `sort=match` for
      `match`.
- [x] 1.4 Teach `filtersFromParams` the reverse: `posted_at` → `newest`, `match` →
      `match`, absent or unrecognised → `defaultSortFor(q)`.
- [x] 1.5 Confirm `emptyFilters()` and every other `DEFAULT_JOB_SORT` reader still
      compile and mean the same thing (`svelte-check`).

## 2. The freshness setter (`filters.ts`)

- [x] 2.1 Add a `setNow`-based freshness setter beside the existing `setSoon` one,
      documenting why the two differ (discrete select vs dragged slider), and
      mirror it in `stagedFilters.svelte.ts` if the staged store needs it.

## 3. The toolbar slot (`ListToolbar.svelte`, `CompaniesView.svelte`)

- [x] 3.1 Rename the `sortControl` prop to `controls` and update its comment to
      say the slot carries however many controls the view passes.
- [x] 3.2 Change the desktop row's condition so the total stays gated on
      `showDesktopTotal` while the controls render whenever they are passed.
- [x] 3.3 Update the `CompaniesView` call site.

## 4. The three controls (`JobsView.svelte`)

- [x] 4.1 Replace `sortControlVisible` with per-option availability: `newest`
      always, `relevance` under query text, `match` under the existing
      `matchFilterAvailable && matchSortEnabled(env)` gate. Render the select only
      when more than one option is available.
- [x] 4.2 Add the Posted select over `FRESHNESS_PRESETS`, writing through the new
      `setNow` freshness setter.
- [x] 4.3 Add the Hide-evergreen toggle writing `likely-evergreen` into the
      `reality` facet's exclude set, reflecting current state, and clearing only
      that exclusion when released.
- [x] 4.4 Pass all three through the renamed `controls` slot and check the row on
      a narrow viewport.

## 5. The modal rail (`filterSections.ts`)

- [x] 5.1 Move the `posted` entry into the `ROLE` section, immediately after
      `experience`.
- [x] 5.2 Add the `reality` entry (`kind: 'facet'`, `facetParam: 'reality'`,
      label `Posting reality`) after `posted`.

## 6. The completeness guard (`filterSections.test.ts`)

- [x] 6.1 Add the rail completeness test over the job `FACETS` vocabulary, modelled
      on `companyRailGroups.test.ts`, with `seniority` and `ai_archetype` on an
      exception list recording which pane hosts each.
- [x] 6.2 Add the two placement tests the spec names: `posted` is a `ROLE` entry,
      `reality` has an entry of its own.

## 7. Verification

- [ ] 7.1 `pnpm test` and `pnpm check` (svelte-check) clean in `web/`.
- [ ] 7.2 Drive the real page: signed out with a text query the select offers
      Relevance/Newest and switching reorders; signed out with no query there is
      no select; the Posted select and the evergreen toggle each move the URL and
      the list; the modal shows the same freshness value; the controls render on a
      company page.
- [ ] 7.3 Confirm no Go, SQL, or Meilisearch settings file is touched by the diff.
