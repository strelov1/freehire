## Why

Experience is the axis a candidate filters on first — "am I even eligible?" — and
it is the one axis the filter modal does not offer. The catalogue already carries
the data: `experience_years_min` is a `jobs` column, a Meilisearch filterable
attribute, and a documented query param, and 645,723 of the 1,349,667 searchable
postings (48%) have a value. None of it is reachable from the UI. The one control
that does speak to experience — the seniority pills — is buried inside the `Role`
pane next to the role picker and the specialization chips, where it reads as a
qualifier on the role rather than a filter of its own.

Two gaps make the data less useful than it looks. The API exposes only a **floor**
(`experience_years_min=3` means "the posting asks for at least 3 years"), which
serves a senior avoiding junior postings but not a candidate with 3 years looking
for what they can actually apply to — that needs a ceiling, which does not exist.
And the description parser reads only a digit adjacent to a year word, so
"no prior experience required" — the phrasing entry-level postings actually use —
parses as *nothing stated*, indistinguishable from silence. Only 7,440 postings
(0.5%) currently carry `experience_years_min = 0`, which understates the
entry-level population rather than measuring it.

## What Changes

- The filter modal's `ROLE` rail gains an **Experience** pane holding two controls:
  the seniority pills and a years-of-experience ceiling. The entry-level case is the
  ceiling's leftmost stop rather than a toggle of its own — see `design.md` for why
  one control beats two here.
- The seniority pills **move** out of the `Role` pane into the new Experience
  pane. They remain the same `seniority` URL param with the same values and
  exclusion behaviour — only their home in the rail changes.
- A new `experience_years_max` query param bounds
  `enrichment.experience_years_min` from above, making the pair a range. The
  existing `experience_years_min` keeps its current floor semantics unchanged;
  this is additive, not a rename. **Not breaking.**
- `jobfacts` gains a precision-first phrase list that resolves an explicit
  "no prior experience" statement to `experience_years_min = 0`. Absent an
  explicit statement it continues to emit nothing — the dict-only rule holds, and
  a missing value is never inferred from silence.
- The years range control states its own coverage: moving it away from "Any"
  drops the ~52% of postings whose experience is unstated, and the pane says so
  rather than letting the result count silently collapse.

Explicitly **out of scope**: the `role_type` facet (Individual Contributor vs
People Manager). It needs a new column, a new dictionary, a Meilisearch facet, and
a `cmd/backfill-derive` + reindex pass, so it is a separate change. This change
leaves room for it in the Experience pane and adds nothing on its behalf.

## Capabilities

### New Capabilities

None. Every behaviour here extends an existing capability.

### Modified Capabilities

- `filter-modal`: the `ROLE` rail gains an `Experience` entry; the seniority pills
  move there from the specialization pane, so the requirement that consolidates
  "seniority within specialization" no longer holds.
- `job-search`: the searchable index gains an `experience_years_max` query filter
  that upper-bounds `enrichment.experience_years_min`.
- `deterministic-facets`: `experience_years_min` additionally resolves to `0` from
  an explicit "no prior experience required" statement in the description, while
  still emitting nothing when the description is silent.
- `api-documentation`: the documented numeric filter set gains
  `experience_years_max`, and `web/static/openapi.yaml` — the integration
  contract — declares the new parameter on every endpoint that accepts
  `experience_years_min`.

## Impact

**Go (backend):**
- `internal/search/query_params.go` — allow `experience_years_max`.
- `internal/search/query_filter.go` — emit `enrichment.experience_years_min <= N`.
- `internal/jobfacts/jobfacts.go` — the no-experience phrase list feeding
  `ExperienceYearsMin`.

**Web (SPA):**
- `web/src/lib/filterSections.ts` — the new `experience` rail entry and its kind.
- `web/src/lib/facets.ts` — the years-range and no-experience controls.
- The filter modal pane component rendering `kind: 'experience'`.
- `web/static/openapi.yaml`, `web/src/lib/docs/filters.ts`,
  `web/src/lib/docs/api-spec.ts` — the new param in the public contract and docs.

**Not affected:** no migration, no new `jobs` column, no new Meilisearch
filterable attribute (`enrichment.experience_years_min` is already one), and
therefore **no `cmd/backfill-derive` run and no reindex** are required to ship
this. The phrase detector reaches existing rows on the next scheduled backfill
and applies to new ingests immediately.
