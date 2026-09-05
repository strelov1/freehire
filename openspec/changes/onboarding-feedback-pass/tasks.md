## 1. Rotating role placeholder

- [x] 1.1 Add `web/src/lib/placeholderRoles.ts`: a `Category[]` list (`backend`,
      `frontend`, `devops`, `qa`, `data_science`, `product`) and `rolePlaceholders()`
      composing `Search jobs — e.g. <label>` through `categoryLabel()`.
- [x] 1.2 Unit tests (`placeholderRoles.test.ts`): list non-empty and unique, every key
      resolves to a non-empty label, every composed string carries its label.
- [x] 1.3 Split `HeaderSearch.svelte`'s `placeholder` prop into `placeholder` (may move)
      and required `label` (static `aria-label`); update both call sites (`TopBar.svelte`,
      `HomeLandingView.svelte`) so the accessible name stops tracking the example.
- [x] 1.4 (revised in review) Add the timer. The planned `rotating?: string[]` prop became a
      union on `placeholder` (`string | string[]`) instead: a separate override always won,
      leaving the static prop dead at every rotating call site. Advance every 2.5s, stop
      permanently on first focus or input: advance every 2.5s, stop
      permanently on first focus or input, hold at index 0 under `prefers-reduced-motion`,
      fade via a `::placeholder` colour transition.
- [x] 1.5 Pass `rolePlaceholders()` from the jobs call sites only; `/companies` keeps its
      static placeholder.
- [x] 1.6 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 2. AI filter moves into the filter modal

- [x] 2.1 Remove `AiFilterButton` from `filters/FilterSummary.svelte` (its `beforeButton`
      snippet) and drop the now-unused import.
- [x] 2.2 Render it inside `filters/FilterModal.svelte` so it is reachable at every
      viewport width, including sub-`md` where the sidebar does not exist.
- [x] 2.3 Verify the sidebar has no orphaned wrapper/snippet left behind.
- [x] 2.4 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 3. Save-to-board as the primary CTA

- [x] 3.1 Test first: `columnOf` maps a saved row with no stage to `preparing`, and an
      explicit stage still wins.
- [x] 3.2 Change `columnOf` in `JobBoard.svelte` accordingly; remove the `continue` that
      dropped saved-only rows in `build()`.
- [x] 3.3 Update the existing 500-row-cap comment in `JobBoard.svelte` in place — do not
      add a second note elsewhere.
- [x] 3.4 Replace `JobRow.svelte`'s icon-only bookmark overlay with a labelled button in
      the card footer, keeping it outside the `<a>` so it never navigates.
- [x] 3.5 Check the surfaces that reuse `JobRow` without the feed's affordances (saved
      list, hidden list, tracking views) still render correctly.
- [x] 3.6 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 4. Account-completeness card

- [x] 4.1 Add `web/src/lib/accountCompleteness.ts`: a pure function taking the résumé,
      profile and saved-search inputs and returning the five steps with a done flag and a
      link each.
- [x] 4.2 Unit tests: each step's predicate, a full account reporting complete, an empty
      account reporting every step open.
- [x] 4.3 Render the card at the top of `/my/tracking`; it disappears when complete.
- [x] 4.4 Add the quiet dot on the header avatar in `HeaderMenu.svelte` while any step is
      open — no count, so it never competes with the notification bell.
- [x] 4.5 simplify → tests green → `requesting-code-review` + `/code-review` → fix
      Critical/Important.

## 5. Ship

- [x] 5.1 `pnpm --dir web lint`, `pnpm --dir web check`, `pnpm --dir web test` all green.
- [ ] 5.2 Verify in a browser: rotation and its three guards, the AI filter on a narrow
      viewport, save-to-board round trip, the card and dot appearing and clearing.
- [ ] 5.3 PR → CI green → merge.
- [ ] 5.4 Deploy and verify on production.
- [x] 5.5 `PUBLIC_MATCH_SORT=1` IS set on production — in `/opt/freehire/env/hire-web.env`,
      not `/opt/freehire/.env` (the two-file env split). So the match sort is live and
      selectable; what remains unaddressed from the reviewer's first note is that
      `defaultSortFor('')` still returns `newest`, so it is never the DEFAULT ordering.
      Changing that default is a separate decision, deliberately not taken here.
