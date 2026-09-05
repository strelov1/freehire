## 1. Rotating role placeholder

- [x] 1.1 Add `web/src/lib/placeholderRoles.ts`: a `Category[]` list (`backend`,
      `frontend`, `devops`, `qa`, `data_science`, `product`) and `rolePlaceholders()`
      composing `Search jobs — e.g. <label>` through `categoryLabel()`.
- [x] 1.2 Unit tests (`placeholderRoles.test.ts`): list non-empty and unique, every key
      resolves to a non-empty label, every composed string carries its label.
- [x] 1.3 Split `HeaderSearch.svelte`'s `placeholder` prop into `placeholder` (may move)
      and required `label` (static `aria-label`); update both call sites (`TopBar.svelte`,
      `HomeLandingView.svelte`) so the accessible name stops tracking the example.
- [x] 1.4 Add the `rotating?: string[]` prop and the timer: advance every 2.5s, stop
      permanently on first focus or input, hold at index 0 under `prefers-reduced-motion`,
      fade via a `::placeholder` colour transition.
- [x] 1.5 Pass `rolePlaceholders()` from the jobs call sites only; `/companies` keeps its
      static placeholder.
- [x] 1.6 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 2. AI filter moves into the filter modal

- [ ] 2.1 Remove `AiFilterButton` from `filters/FilterSummary.svelte` (its `beforeButton`
      snippet) and drop the now-unused import.
- [ ] 2.2 Render it inside `filters/FilterModal.svelte` so it is reachable at every
      viewport width, including sub-`md` where the sidebar does not exist.
- [ ] 2.3 Verify the sidebar has no orphaned wrapper/snippet left behind.
- [ ] 2.4 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 3. Save-to-board as the primary CTA

- [ ] 3.1 Test first: `columnOf` maps a saved row with no stage to `preparing`, and an
      explicit stage still wins.
- [ ] 3.2 Change `columnOf` in `JobBoard.svelte` accordingly; remove the `continue` that
      dropped saved-only rows in `build()`.
- [ ] 3.3 Update the existing 500-row-cap comment in `JobBoard.svelte` in place — do not
      add a second note elsewhere.
- [ ] 3.4 Replace `JobRow.svelte`'s icon-only bookmark overlay with a labelled button in
      the card footer, keeping it outside the `<a>` so it never navigates.
- [ ] 3.5 Check the surfaces that reuse `JobRow` without the feed's affordances (saved
      list, hidden list, tracking views) still render correctly.
- [ ] 3.6 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 4. Account-completeness card

- [ ] 4.1 Add `web/src/lib/accountCompleteness.ts`: a pure function taking the résumé,
      profile and saved-search inputs and returning the five steps with a done flag and a
      link each.
- [ ] 4.2 Unit tests: each step's predicate, a full account reporting complete, an empty
      account reporting every step open.
- [ ] 4.3 Render the card at the top of `/my/tracking`; it disappears when complete.
- [ ] 4.4 Add the quiet dot on the header avatar in `HeaderMenu.svelte` while any step is
      open — no count, so it never competes with the notification bell.
- [ ] 4.5 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 5. Ship

- [ ] 5.1 `pnpm --dir web lint`, `pnpm --dir web check`, `pnpm --dir web test` all green.
- [ ] 5.2 Verify in a browser: rotation and its three guards, the AI filter on a narrow
      viewport, save-to-board round trip, the card and dot appearing and clearing.
- [ ] 5.3 PR → CI green → merge.
- [ ] 5.4 Deploy and verify on production.
- [ ] 5.5 Check whether `PUBLIC_MATCH_SORT` is set on the host and report — out of scope
      to change, in scope to answer.
