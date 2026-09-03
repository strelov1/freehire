## Context

The brainstormed design this change implements is committed at
[docs/superpowers/specs/2026-09-02-search-suggestions-service-design.md](../../../docs/superpowers/specs/2026-09-02-search-suggestions-service-design.md).
That document carries the production measurements behind every number quoted
here; this one records the decisions and their alternatives.

Current state: the homepage `/` **is** the jobs feed, so it renders
`HeaderListSearch`, not the `HeaderSearch` launcher. Its dropdown already offers
roles, matched entirely in the browser by `web/src/lib/roleSuggest.ts` against
1,830 generated role labels and 1,532 aliases, behind a 120 ms debounce because a
pass costs ~10 ms. Every keystroke is also pushed into the list filter, so the
feed refetches while the visitor is still typing.

Constraints that shaped this:

- Layering. A new package sits in exactly one block and may import only blocks
  below it, enforced twice (`depguard` and `internal/platform/arch/layering`).
- Meilisearch runs **one serial task queue**. A second index does not get its own.
- The catalogue is sales- and management-heavy: Management 266,883, Sales 179,993,
  Support 127,110. Any "most popular" ordering surfaces those.

## Goals / Non-Goals

**Goals:**

- A visitor who does not know what to type is given somewhere to start.
- `java developer`, `nodejs developer` — phrases that name no role — produce
  useful suggestions.
- A typo produces the suggestion it was reaching for.
- `senior software engineer go` completes to Google, applying role and company
  together.
- The dropdown shows real postings, with company logos, alongside the completions.

**Non-Goals:**

- Personalised suggestions from the signed-in user's profile. The seam exists (the
  endpoint could take the viewer's specializations) and is deliberately not built.
- Suggestions on the `HeaderSearch` launcher — the non-list pages. The homepage is
  the surface this change is scoped to; the launcher keeps its current behaviour.
- Semantic or embedding-based suggestion. The dictionary is finite and curated;
  this is the same "never guess" discipline the facet dictionaries hold.
- Multi-language suggestion surfaces. Titles are mined as they are written.

## Decisions

### A separate index, not a facet on `jobs`

`title` is in `SearchableAttributes` but not `FilterableAttributes`
(`internal/search/search/client.go:573`). Promoting it would not work: distinct
titles number in the millions, `MaxValuesPerFacet` truncates the distribution, and
`SortFacetValuesBy: count` decides what survives. Confirmed against production —
`GET /jobs/facets?facets=title` answers `unknown facet: title`, and all 26 facets
that do exist are closed vocabularies.

**Alternative considered:** a normalised-title attribute derived at index time,
the way `roles` and `role_type` already are. Rejected because it puts a
high-cardinality filterable attribute on the 8M-document index to serve a
dictionary of tens of thousands, and because adding a filterable attribute there
carries the settings-before-binary hazard that a separate index does not.

### The index replaces the client matcher rather than joining it

The smaller fix — patch `roleSuggest.ts` to admit the typo tier as a fallback,
rank it by edit distance, add an empty state, make the pass two-phase so the
debounce can go — is work Meilisearch does natively over a corpus we choose.
Keeping both leaves two matchers to drift apart, and the drift is silent: a
suggestion that differs between them looks like a bug in neither.

### Two mechanisms for the query parse, deliberately not merged

The **recognised prefix** needs exact matching against normalised phrases, so it
runs against an in-process phrase set the API loads from the index at startup and
refreshes on a ticker. No typo tolerance is wanted: a mistyped phrase must fall
into the fragment, not be silently consumed as recognised.

The **fragment** needs typo tolerance and relevance ranking, so it stays a query
against Meilisearch.

**Alternative considered:** issuing progressively shorter prefix queries to
Meilisearch to find the longest match. Rejected — it is N round-trips per
keystroke to answer a question a hash lookup answers.

**Alternative considered:** doing the parse in the browser against the generated
contracts. Rejected — it is the second matcher this change exists to remove, and
companies are not in the generated contracts at all.

### A category is dropped when a role shares its slug

Measured: role `devops` 53,250 against category `devops` 53,251; role
`data_analytics` 77,367 against category 77,375. They are the same postings. The
role wins because "DevOps Engineer" names a job while "DevOps" names a department.

### The empty state is curated, not ranked

Ordered by `CATEGORY_GROUP_ORDER` (`web/src/lib/filterSections.ts:19`) —
Engineering first, consumer industries last. Ranking by count would lead with
Management, Sales and Support, which reads as a different website.

**Alternative considered:** a hand-written shortlist of tech roles. Rejected — the
category groups are already curated, already ordered, and already maintained under
a compile-time completeness check, so a second list would be a second thing to
keep current.

### A frequency floor plus a generic-title rule

The floor alone is not enough: bare `manager` occurs 44 times in a 2,000-title
sample and bare `director` 18, both above any sane floor and both useless as
suggestions. Titles reducing to a bare grade or bare generic are dropped
regardless of count; the role and category dictionaries carry those axes properly.

### Frequency is recorded in a table, not read from access logs

The five-day measurement quoted in the existing `role-search-suggestions` spec
came from nginx access logs, and `internal/application/viewlog` already parses
those — so the data could be mined rather than written.

Chosen the table anyway: a direct upsert is one code path with one owner, whereas
the log route couples suggestion quality to log shipping, retention and a parser
whose contract is nginx's format. The write fails open and never delays a
response, so the coupling it adds to the search path is bounded.

## Risks / Trade-offs

**The dropdown becomes a page.** Five completions, five postings and three
companies is thirteen rows plus headers → the visitor has more to read, not less,
which is the opposite of what "less confusing" means. → Sections are capped and
ordered by decreasing abstraction (what to search, then what exists, then who is
hiring); the empty state shows one section only.

**Losing search-as-you-type is a regression for some.** Anyone who ignores the
dropdown today watches the list narrow as they type. → It is the change that was
explicitly asked for, and the postings section restores live evidence inside the
dropdown. Reversible in one commit if it reads badly.

**Per-keystroke requests multiply load.** `/suggest` plus the postings and
companies queries is three requests per settled keystroke where the homepage
previously issued one. → `/suggest` gets its own rate-limit bucket; the debounce
and the stale-response token bound the in-flight set. The prior incident where
`c.IP()` returned empty and put the whole site in one bucket is the failure mode
to avoid when keying it.

**A stale dictionary suggests dead searches.** The index is rebuilt on a cron, so
a suggestion can outlive its postings. → The endpoint withholds a suggestion whose
count has reached zero; the count is refreshed by the rebuild, and the rebuild is
cheap because the index is small.

**Meilisearch's serial task queue.** A suggestions build queues behind a running
`jobs` rebuild and looks like a hang. → The index is small so the wait is the cost
rather than the build; the timer must not be scheduled over `freehire-reindexw`.

**Title mining quality is unproven at full scale.** The 2,000-title sample says
the distribution is concentrated, but the normalisation was measured on a sample,
not the catalogue. → The first build is inspected before the endpoint is wired to
it; the floor is a tunable, and a bad floor produces a small index rather than a
wrong one.

## Migration Plan

Staged, each stage independently shippable:

1. **Frontend only.** Enter-to-search, the empty-state dropdown, the postings and
   companies sections with logos. No Go, no migration, no index. Reversible by
   revert.
2. **The index and the endpoint.** Migration-free. Ship the builder, run it by
   hand, inspect the dictionary, add the systemd unit and timer, then wire the
   client and delete `roleSuggest.ts`. The endpoint is dead code until the client
   points at it, so the two can land in either order.
3. **Frequency.** Migration `0125` applied before the binary that writes
   `search_queries` rolls out. Ranking by demand is inert until the table has
   rows, so nothing changes on the day it ships.

Rollback: stage 1 by revert; stage 2 by pointing the client back at the previous
build (the `jobs` index is untouched, so nothing else regresses); stage 3 by
dropping the ranking term, leaving the table harmlessly collecting.

## Open Questions

- The exact frequency floor for mined titles, and the posting floor for companies.
  Both are tunables to be set from the first real build rather than guessed here.
- How often the builder should run. Daily is the starting assumption, matching
  `cmd/rollup-facets`, since neither titles nor company sizes move hourly.
- Whether the in-process phrase set refreshes on a ticker or on a build
  notification. Ticker first; a notification is only worth it if the staleness
  window proves visible.
