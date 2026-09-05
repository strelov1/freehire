## Why

An outside reviewer walked the product and left five notes. Four describe surfaces that
already exist but cannot be reached, or reach only one hand:

1. Search is not prominent enough, and the default feed is not relevant until a visitor
   sets filters.
2. The search box gives no example of what to type.
3. Navigation is confusing; the AI filter eats the last column.
4. There is no "how complete is my account" funnel after onboarding.
5. There is no way to track a job *before* applying to it.

Note 5 is the instructive one: the state it asks for already exists. `preparing` is a
real stage and the board's first column (`internal/application/userjob/groups.go`),
added for CV tailoring. Nothing in the jobs feed can put a posting into it.

Note 3 hides a second defect. `AiFilterButton` renders inside the filters sidebar, and
that sidebar is `<aside class="hidden w-72 shrink-0 md:block">` — so the AI filter does
not exist on a phone at all today. Moving it into the modal frees the column the note
asks about *and* gives the feature to mobile for the first time.

Note 1 splits in two. The homepage search already IS the hero, full-bleed and
autofocused. What remains is the default ordering: `defaultSortFor('')` returns `newest`
for everyone. The match sort that would fix it is written and gated behind
`PUBLIC_MATCH_SORT`. Flipping that flag is an operational question, not a code change,
and is deliberately out of scope here.

## What Changes

- **A rotating role placeholder.** The jobs search box shows `Search jobs — e.g. Backend`
  and cycles the trailing word. The rotation stops permanently at the first focus or
  keystroke, is disabled under `prefers-reduced-motion`, and never moves the field's
  accessible name — `HeaderSearch` currently passes one prop to both `placeholder` and
  `aria-label`, so the props split.
- **The AI filter moves into the filter modal** and leaves the sidebar.
- **Saving a job puts it on the tracking board.** The board's client-side `build()` is
  what discards saved-only rows today, so this is a read-side change: no migration, no
  new write, and it applies to every bookmark ever made. The card's icon-only bookmark
  becomes a labelled primary button.
- **An account-completeness card** at the top of `/my/tracking`, plus a quiet dot on the
  header avatar while any step is open. Five steps, all readable from stores that already
  exist — no new API endpoint.

## Impact

- Affected specs: `search-role-placeholder` (new), `ai-filter-entry`,
  `user-job-tracking`, `account-completeness` (new)
- Affected code: `web/src/lib/components/HeaderSearch.svelte`, `TopBar.svelte`,
  `HomeLandingView.svelte`, `JobRow.svelte`, `JobBoard.svelte`,
  `filters/FilterSummary.svelte`, `filters/FilterModal.svelte`, `HeaderMenu.svelte`,
  `web/src/routes/my/tracking/`, plus two new pure modules under `web/src/lib/`
- No backend, no schema, no new endpoint
