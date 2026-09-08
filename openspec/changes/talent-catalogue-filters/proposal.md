## Why

The Talent Network catalogue ships with four filter rows of plain chips. Two of its seven
filters — skills and city — have no control at all, and the skills one is the filter a
recruiter actually reaches for: "who knows Go and Kubernetes" is the question, and today
the only way to ask it is to edit the URL by hand.

A chip row cannot answer it either. Skills are an open vocabulary of thousands of
canonicals; rendering them as pills is not a longer version of the same control, it is a
different one. `/jobs` already solved this: a rail of panes, a searchable facet section per
open vocabulary, live counts beside every value, and a deferred apply.

Almost all of that is reusable and already generic. `FilterModalShell` knows nothing about
jobs — it takes a rail, sections, a staged surface and a pane snippet. `FacetSection` and
`ChipFacet` drive any store satisfying `FacetStore`, eight methods, and the companies
catalogue already proved it by writing a second one. What the catalogue is missing is the
store and the counts.

## What Changes

- **New endpoint `GET /api/v1/talent/facets`**: for the current filter, the count of
  members behind every value of every facet — the same `{total, facets, stats}` shape
  `/jobs/facets` already returns, so the panes need no new vocabulary.
- **A `TalentFilterStore`** implementing `FacetStore`, over the existing `UrlSyncedState`
  primitive, with `talentQuery.ts` promoted to the pure model beneath it (types,
  serialisation, mutators) exactly as `companyFacetModel.ts` sits under
  `CompanyFilterStore`.
- **A filter modal for the catalogue**, a thin wrapper over `FilterModalShell` supplying a
  rail of Specialization, Skills, Seniority, Timezone, City and Experience. The panes are
  the existing ones, so skills and city arrive with search, counts, and multi-select for
  free.
- **The list surface adopts the jobs chrome**: `ListToolbar`, `FilterSummary` (the
  removable chips that say what is currently narrowing the list) and `Pagination`.
- **The card matches `JobRow`'s density**, so a person scanning the catalogue reads it the
  way they already read the job list.
- Not included: sorting (the catalogue has one order and it is not a preference), saved
  searches and alerts over candidates (a subscription to people is a different consent
  question), and any filter over data the card does not carry.

## Capabilities

### New Capabilities

- `talent-catalogue-facets`: the counts behind every filter value, and the rule for what a
  count may and may not reveal about a small population.

### Modified Capabilities

- `talent-network-catalog`: the filter requirement gains controls for the facets that have
  none, and the counts that make an open vocabulary searchable. The catalogue's projection
  and 404 rules are untouched.

## Impact

**Go.** `internal/candidate/talentnetwork` gains the count pass over the snapshot it
already holds — one walk of the filtered set, no index and no new storage, because the
whole membership is already in memory. `internal/api/handler` gains one route, on the
catalogue's own rate-limit budget.

**Web.** `talentQuery.ts` grows the mutators a `FacetStore` needs and keeps its tests;
`talentFilters.ts` is new and reactive; `TalentFilterModal.svelte` is new and thin. The
list route swaps its hand-rolled chip rows and pager for the shared chrome. `TalentCard`
is restyled against `JobRow`.

**Shared vocabulary, deliberately.** A candidate's skills are `skilltag` canonicals — the
same dictionary a posting's are. One chip labelled "Go" therefore means the same thing on
`/jobs` and on `/talent`, which is what would later let one side be asked about the other
("who knows what this posting wants"). Nothing here builds that; it only avoids the second
vocabulary that would make it impossible.
