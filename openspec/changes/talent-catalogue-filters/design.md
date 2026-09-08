## Context

The catalogue shipped with four rows of plain chips. It works for the closed vocabularies —
seniority is eight values, timezone region is eight — and it cannot work for the open ones.
Skills are thousands of `skilltag` canonicals; a chip row over that is not a longer control,
it is the wrong one. City is the same shape. Both are currently reachable only by editing
the URL, which is the "backend can do it, the UI cannot reach it" failure this whole feature
exists to have fixed.

`/jobs` solved this already, and — this is the part that makes the change small — solved it
generically:

- `FilterModalShell` takes a rail, sections, a staged surface and a pane snippet. It knows
  nothing about jobs.
- `FacetSection` and `ChipFacet` drive anything satisfying `FacetStore`: eight methods.
- `CompanyFilterStore` is the proof, written for the companies catalogue against the same
  contract, with a pure model (`companyFacetModel.ts`) beneath a reactive
  `UrlSyncedState` wrapper.

What is missing for the catalogue is that store, and the counts the panes render.

## Goals / Non-Goals

**Goals:**

- Every filter the API reads has a control a visitor can reach with a mouse.
- Skills and city are searchable, with the number of members behind each value.
- The list reads like `/jobs`: the same toolbar, the same removable summary chips, the same
  pager, a card of the same density.
- One filter vocabulary, shared by the URL, the store, the modal and the API.

**Non-Goals:**

- Sorting. The catalogue has one order — freshness of the extract — and it is not a
  preference; offering a choice would imply the others are equally meaningful.
- Saved searches and alerts over candidates. Subscribing to *people* is a different consent
  question from subscribing to postings, and it is not answered by reusing a component.
- Any filter over data the card does not carry. A control for something invisible is a
  control that cannot be judged.

## Decisions

### Counts come from the snapshot, not from a new index

`GET /api/v1/talent/facets` answers with `{total, facets, stats}` — the shape `/jobs/facets`
already returns, so the panes need no new vocabulary and no new client code.

It is computed by walking the filtered snapshot the catalogue already holds in memory and
tallying each facet. At a membership in the hundreds that is microseconds and no storage.

*Alternative considered — the whole skill dictionary as static options.* Rejected: it would
offer thousands of skills nobody in the catalogue has, so every other search would end in an
empty list, and the visitor could not tell a missing person from a missing skill.

*Alternative considered — Meilisearch facet distribution, as the jobs list uses.* Rejected
for the same reason the catalogue is not indexed at all: an index, a drain and a second way
to serve somebody who has already left, for a set that fits in memory.

**One count is computed differently, and the difference is the point.** A facet's own count
is taken with that facet's selection REMOVED — otherwise picking "Go" collapses the skills
pane to "Go (3)" and a visitor can never add a second skill, because every other value reads
zero. This is what `/jobs` does and it is not obvious from the outside.

### A count must not become a way to name somebody

A count over a small population is a disclosure in a way a count over eight million postings
is not: "senior · backend · Berlin · Go · 1" plus a card is a person, and enough narrow
counts is a way to walk the space.

Two rules follow. Counts are reported for the CURRENT filter only, never as a global
histogram of the membership. And a facet value with a count below a floor is reported as
present without its number, so the pane still offers it — withholding the option would be a
worse answer, since the member is in the catalogue either way and the list already shows
them.

**The floor is not a privacy guarantee.** It is friction. The honest guarantee is the
projection: the card carries no name and no employer, so narrowing to one member yields one
anonymous card, which is what the member agreed to.

### The store is the companies pattern, not a new one

`talentQuery.ts` is already the pure model — types, parse, serialise. It grows the mutators
`FacetStore` needs (toggle, add, remove, clear per facet) and keeps its tests, staying free
of `$app` so it can be unit-tested. `talentFilters.ts` wraps it in `UrlSyncedState` and
implements the eight methods.

Like companies, the catalogue has no `_exclude` and no AND/OR modes, so `cycle` and `pick`
collapse to one include toggle and `toggleSign`/`setMatchAll` are inert. That is the second
time the contract has been satisfied by a store that does not need half of it — worth
noticing, not worth generalising over two examples.

### The URL stays the single source of truth

Every control still navigates. The modal stages changes and commits them to the URL on
apply, which is what `FilterModalShell` already does; the page reloads from the URL. A
narrowed catalogue therefore remains a shareable link, works with the back button, and shows
a crawler what it shows a person.

## Risks / Trade-offs

- **Two counting paths — the endpoint's and the list's — can disagree.** → Both walk the
  same snapshot through the same filter, and a test asserts the total the facets endpoint
  reports equals the total the list reports for the same query.
- **The facets endpoint doubles the requests a filtering visitor makes.** → It shares the
  catalogue's own rate-limit budget rather than the site-wide one, so a burst of filtering
  cannot exhaust the reads the rest of the site depends on.
- **A count could narrow to one member.** → See above: it can, and what it yields is one
  anonymous card. The floor hides the number, not the person, and the projection is what
  makes that acceptable.
- **`JobRow`'s density on a card with a third of the fields may read thin.** → The catalogue
  card carries less by construction, and matching the row's rhythm is worth more than
  filling it; a page that looks like the job list is one a visitor already knows how to read.
