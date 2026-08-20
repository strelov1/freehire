## Context

The header's list-search input (`web/src/lib/components/HeaderListSearch.svelte`) is
a thin remote control: it proxies keystrokes into whichever filter store the active
list view registered through `web/src/lib/listSearch.svelte.ts`, reusing that store's
URL sync, debounce, and back/forward handling. It renders no dropdown today, and says
so in a comment — "No dropdown — the page's own list is the live result."

Everything this change needs already exists on the client:

- `web/src/lib/generated/contracts.ts` ships `ROLE_LABELS` (1,290 slug → label pairs,
  generated from `internal/roletag`'s catalogue) and `ROLE_ALIASES`.
- `web/src/lib/facets.ts` already matches a typed query against a role option's label
  and aliases via `optionMatches(option, query, searchAliases)`, keyed by `baseRole`.
  That is the role picker's matcher.
- `JobsView.svelte` already fetches the full facet distribution (`refreshCounts()` →
  `api.facetCounts`), which includes the `roles` distribution.
- The `role` query param already filters end to end, through the same filter store the
  header writes into.

So the work is wiring, not capability: a pure matcher/ranker, one optional member on
the bridge interface, a dropdown in the header, and one publication in the jobs view.

The change is justified by production measurement rather than intuition; the figures
and their method are recorded in the proposal. The short version: 37.6% of real
searches name a role, and the role facet is used in 1.1% of requests.

## Goals / Non-Goals

**Goals:**

- Turn the ~28% of searches that exactly name a role, and the ~10% that prefix one,
  into an exact `role` facet filter offered at the moment of typing.
- Make the role facet discoverable without moving it or redesigning the filter modal.
- Keep the free-text path fully intact, including its Enter key.
- Keep the header free of page-path branching.

**Non-Goals:**

- No change to relevance ranking, Meilisearch settings, or any index. An earlier
  investigation in this thread considered reordering `RankingRules` /
  `SearchableAttributes` to favour company-name matches; production logs showed the
  company names it would fix are searched zero times, while 17.7% of real searches
  begin with a string that is literally some company's name, so that path was
  measured and dropped. It stays dropped here.
- No new roles in the dictionary. The logs name real gaps (`interaction designer`
  307×, `product analyst` 280×, `design systems designer` 270×, `service designer`
  243×, the `ui designer` / `ux/ui designer` / `user interface designer` cluster
  ~960× combined). Those belong to `internal/roletag` and are their own change.
- No suggestions for skills, companies, or locations. Roles only.
- No suggestions on `/companies`.
- No server round-trip per keystroke.

## Decisions

### Selecting a role clears the text query

Applying `role=<slug>` **and** keeping the text would AND two filters, returning
strictly fewer results than the user's plain text search did — a suggestion that
makes the page emptier reads as a bug. The role tag is derived from the title by a
deterministic dictionary, so it is already the more precise of the two. Clearing the
text is the choice that makes the suggestion an upgrade rather than an extra
constraint.

*Alternative considered:* keep both, as a safety net against gaps in role tagging. Rejected —
it inverts the feature's value and leaves the user unable to tell which of two
active filters emptied their page. The always-present free-text row covers the gap
instead.

### The header learns nothing about pages; the page publishes a capability

`ListSearchTarget` gains one optional member:

```ts
readonly roleSuggest?: {
  counts(): FacetCounts | null;
  apply(slug: string): void;
};
```

`JobsView` publishes it, `CompaniesView` does not, and the header renders the
dropdown iff it is present. This is the pattern `filterScope` and `openFilters`
already established on the same interface — `/companies` gets no location popover for
exactly this reason. `counts` is a getter so the distribution stays reactive across
the bridge, matching `filterScope.counts`.

*Alternative considered:* have the header check `page.url.pathname`. Rejected — it
would be the first path-branch in a component built specifically to avoid them, and
would silently miss the company page's embedded jobs list, which registers a jobs
target under `/companies/:slug`.

### The matcher is a pure module, separate from the component

`web/src/lib/roleSuggest.ts` exports one function:

```ts
suggestRoles(query: string, counts: FacetCounts | null, active: readonly string[]): RoleSuggestion[]
```

It has no Svelte, no network, and no DOM, so every spec scenario about matching,
ranking, bounding, dedup, and the counts-absent case is a plain vitest case. The
component is then left with only what genuinely needs a component: rendering,
keyboard, and focus.

### Matching reuses the picker's matcher

`optionMatches` from `facets.ts` is the existing answer to "does this query name this
role", aliases included. Calling it keeps one behaviour instead of two that drift —
the trap the repo's single-legal-form-vocabulary rule exists to prevent. If its
signature does not fit a bare slug/label pair, adapt the call site; do not fork the
logic.

### Ranking is by match quality first, count second

An earlier draft ranked by open vacancies alone. Measured against the live
distribution, that was wrong: `optionMatches` is typo-tolerant and alias-aware, so
`devops` reaches Sales Specialist (147,223 jobs) through `revops` and `backend`
reaches Marketing Specialist (55,768) through `growth hacker` — and sorting by
absolute count then hands first place to the largest unrelated bucket. Match quality
(prefix, then word-boundary, then fuzzy) is the primary key; count orders within a
tier; label breaks the remaining tie.

This also rescues the unmeasured state. With no distribution every count is absent,
so a count-first rule degenerates to alphabetical over 1,290 roles — which returns
five `C-Level …` rows and, for `swe`, omits Software Engineer entirely. Under
quality-first the same call leads with the role the query names.

### One row per base role

Graded slugs outnumber ungraded ones about six to one in the live distribution, so
without collapsing, `data analyst` spends all five rows on Data Analyst's grades and
never reaches Data Engineer. `baseRole` already exists in `facets.ts` for exactly this
key. Collapsing keeps the highest-ranked variant, so naming a grade still yields that
grade — it wins tier 1 against its own ungraded sibling.

Collapsing WITHOUT the quality tiering would make things worse, not better (`backend`
would collapse to Backend Engineer plus Marketing Specialist), which is why the two
ship together.

### Enter is not captured by default

The dropdown starts with nothing highlighted. Enter then does exactly what it does
today. Only after Down/Up moves the highlight into a row does Enter activate that
row. This makes the feature purely additive for anyone who ignores it.

## Risks / Trade-offs

- **A role tag missing from a job hides it behind the role filter** → The dictionary
  emits nothing for titles it cannot place, by design. The always-present "search
  «…» as text" row is the escape hatch, and the applied role shows as a removable
  chip.
- **A dropdown over the header can cover page content or trap focus** → Follow the
  existing header popovers (`HeaderLocationFilter`) for layering and dismissal;
  Escape and outside-click both close, and the query survives Escape.
- **Reversing the documented "no dropdown" decision** → Replace the comment together
  with the behaviour, so the file does not carry a rule its code no longer follows.
- **Matching 1,750 strings per keystroke, undebounced** → `FilterStore.setQuery` uses
  `setSoon`, which updates `value` IMMEDIATELY and debounces only `applied`
  (`urlSynced.svelte.ts` — "`value` stays live so the input never lags"). The header
  binds `target.value.q`, so there is no upstream debounce protecting this path. An
  earlier draft of this document claimed there was; that was wrong. Measured at 8–11 ms
  per call on a warm desktop JIT, ~20 ms for a long query — plausibly 30–60 ms of
  synchronous main-thread work per keystroke on a mid-range phone. Task 3 must either
  debounce the suggestion computation or memoize it on `(query, distribution)`.
- **Suggestions could be measured as unused, like the facet they surface** → The jobs
  view already emits a `search` analytics event; a role selection should be
  distinguishable in it, so the 1.1% figure can be re-measured after ship.

## Migration Plan

Frontend-only, no schema, no index, no API. Ships with a normal web deploy (the
static bundle needs the web service restarted — see the repo's deploy notes). Rollback
is a revert of the same bundle; nothing persists, and any `role=` URL a user shared
in the meantime keeps working, because that param already existed.

## Open Questions

- Should the dropdown also appear on the company page's embedded jobs list? It
  registers a jobs-backed target, so publishing `roleSuggest` there comes for free and
  the specs do not forbid it. Default to yes (jobs-backed lists get it); revisit if it
  feels wrong in review.
