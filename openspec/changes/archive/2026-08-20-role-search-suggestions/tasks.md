## 1. Matcher and ranker (pure module)

- [x] 1.1 Add `web/src/lib/roleSuggest.ts` exporting `RoleSuggestion` and
  `suggestRoles(query, counts, active)`, matching a query against the role catalogue
  by label and alias through `facets.ts`'s existing `optionMatches` (adapt the call
  site if its signature needs it; do not fork the matching logic). Cover with
  `roleSuggest.test.ts`: label match, alias match (`swe` → `software_engineer`),
  prefix match (`data an` → `data_analytics`), and one role reached through two
  aliases offered once.
- [x] 1.2 Rank matches by open-vacancy count from the passed `FacetCounts`, break ties
  by label, and cap at five. Tests: higher count ranks first, equal counts order by
  label and do not reshuffle across repeated calls, nine matches yield exactly five.
  (Superseded by 1.5: count is the WITHIN-TIER key, not the primary one. Left as
  written because it is what this step actually built, and 1.5 records why it was
  wrong.)
- [x] 1.3 Handle the three absence cases: a null/absent distribution offers the matches
  ordered by label with no count (never a zero); a role missing from a PRESENT
  distribution is dropped entirely (measured zero — a suggestion to an empty page); and
  a role already present in `active` is not offered. Tests for all three.
- [x] 1.4 Reject queries shorter than two characters with no matches, so the component
  has no threshold logic of its own. Test the one-character case.

- [x] 1.5 Rank by match-quality tier before count (prefix → word-boundary → fuzzy),
  and collapse graded variants to one row per `baseRole`, keeping the best-ranked
  variant. Both rules exist because ranking by count alone was measured wrong on the
  live catalogue: `devops` led with Sales Specialist, `backend` with Marketing
  Specialist, and `data analyst` spent all five rows on grades of one role.
- [x] 1.6 Re-test group 1 against the REAL catalogue and a REAL production
  distribution, asserting RANK (`found[0]`), not mere presence — every existing
  fixture reduces the 1,290-role catalogue to a handful, which is why the suite stayed
  green through all of the above. Cover the `counts === null` path the same way.

## 2. Bridge capability

- [x] 2.1 Add the optional `roleSuggest` member to `ListSearchTarget` in
  `web/src/lib/listSearch.svelte.ts` — `counts()`, `active()` and `apply(slug)`,
  documented in the file's existing comment style as the jobs-only capability it is.
- [x] 2.2 Publish `roleSuggest` from the target `JobsView.svelte` already registers,
  backed by `filtersWithRole` (pure, tested in facetModel) and `FilterStore.applyRole`,
  which commits the cleared text and the role facet in ONE `setNow` so the pending
  keystroke debounce cannot land afterwards. `CompaniesView.svelte` publishes nothing
  new.
- [x] 2.3 Measure the role distribution WITHOUT the text query, in its own single-facet
  request refreshed only when a non-text filter moves. Scoped by `q` the numbers would
  answer "jobs matching what you typed AND this role", lag a debounce behind the input,
  and make roles appear and vanish mid-word through the measured-zero drop rule.

## 3. Header dropdown

- [x] 3.1 Render the dropdown in `HeaderListSearch.svelte` when `target.roleSuggest`
  exists and `suggestRoles` returns matches: up to five role rows with label and
  count, then a final "search «…» as text" row. Replace the file's now-false
  "No dropdown — the page's own list is the live result" comment with one describing
  the actual rule.
- [x] 3.2 Wire selection: activating a role row calls `roleSuggest.apply(slug)` and
  closes the dropdown; activating the text row leaves today's free-text behaviour
  exactly as it is.
- [x] 3.3 Add keyboard handling — Down/Up move the highlight, Enter activates the
  highlighted row, Enter with nothing highlighted falls through to the existing
  free-text search, Escape closes and keeps the typed text, outside click closes.
  Start with nothing highlighted so Enter is never captured by default.
- [x] 3.4 Add the combobox/listbox roles and `aria-activedescendant`, and match the
  layering and dismissal of the existing `HeaderLocationFilter` popover so the
  dropdown cannot cover page content or trap focus.

## 4. Verify

- [x] 4.1 Run the web checks — `pnpm test`, `pnpm lint`, `pnpm check` (or this repo's
  equivalents) — and confirm they pass.
- [x] 4.2 Drive the real app: on the jobs feed type `data an`, pick `Data Analyst`,
  and confirm the URL carries `role=data_analytics` with no `q`, the input is empty,
  the chip is present and removable, and the counts reload. Then confirm `/companies`
  shows no dropdown, and that typing `revolut` + Enter still runs a free-text search.
- [x] 4.3 Emit a `role_suggestion` analytics event on selection — its own event rather
  than a flag on `search`, since the question is how often the dropdown is what puts
  the role facet on. The role facet measured 1.1% of searches before it existed.
- [x] 4.4 Give the `role` facet a chip group in `FilterSummary.svelte`. It had none: a
  role counted towards the All-filters badge but drew no chip, so the only way to lift
  it was Reset all. Latent while the facet lived in the modal; this change puts it one
  click from the search box.
