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
      on `companyRailGroups.test.ts`, with every facet hosted in another entry's
      pane on an exception list recording WHICH pane hosts it (17 of them, not the
      two this task first guessed), plus `source` recorded as a deliberate refusal.
      A second test keeps the list honest: an excepted facet must be a declared
      facet and must not also have a row of its own.
- [x] 6.2 Add the two placement tests the spec names: `posted` is a `ROLE` entry,
      `reality` has an entry of its own.

## 7. Review fixes

Raised by code review against the first implementation; each is covered by a test
that fails without the fix.

- [x] 7.1 Model the stored ordering as `JobSort | null` ("chosen" vs "not chosen")
      so typing a query cannot carry `sort=posted_at` into a text search, and a
      typed query still serializes equal to the saved search it came from.
- [x] 7.2 Move the option-availability rule out of `JobsView.svelte` into pure
      `sortOptionsFor` / `selectedSortFor`, and have the control name the ordering
      the endpoint will actually serve when the resolved one is not on offer (a
      signed-out `?sort=match` link rendered the select blank).
- [x] 7.3 Offer an off-preset freshness bound as a stop of its own
      (`freshnessOptions`), so a `posted_within_days` from a shared link or the AI
      dialog cannot leave the select blank over a live bound.
- [x] 7.4 Fix the mobile row: measured 49px of overflow at 390px, so the evergreen
      toggle drops its word below `sm` and the toolbar row wraps.

## 8. Verification

- [x] 8.1 `pnpm test` and `pnpm check` (svelte-check) clean in `web/`, and
      `pnpm lint` adds no new warning.
- [x] 8.2 Drive the real page against the live API: signed out with a text query
      the select offers Relevance/Newest; signed out with no query there is no
      select; `?q=go&sort=posted_at` preselects Newest; `?q=go&sort=match` signed
      out shows Relevance rather than a blank control; `?posted_within_days=7`
      preselects `1 week` and `=5` offers `Last 5 days` in day order;
      `?reality_exclude=likely-evergreen` renders the toggle pressed; the controls
      render on a company page; the row fits a 390px viewport (measured).
- [x] 8.3 Confirm no Go, SQL, or Meilisearch settings file is touched by the diff.
