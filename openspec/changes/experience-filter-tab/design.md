## Context

The filter modal (`web/src/lib/components/filters/FilterModal.svelte`) renders a
left rail of entries declared in `web/src/lib/filterSections.ts` as `RAIL`, each
carrying a `RailKind` that selects which pane markup runs. Facet-shaped entries
(`kind: 'facet'`) render a `FacetDef` from `web/src/lib/facets.ts`; the rest are
bespoke panes with hand-written markup.

Filter state is a `JobFilters` object (`web/src/lib/facetModel.ts`): a `q` string, a
`Record<string, FacetState>` of the string-enum facets, and a handful of scalar
fields — `visa`, `salaryMin`, `postedWithinDays` — each serialized to a URL param by
`filtersToParams` and parsed back by `filtersFromParams`. Two stores wrap it: the
live URL-synced `filters.ts` and the deferred `stagedFilters.svelte.ts` the modal
edits.

On the backend, `internal/search/query_params.go` allow-lists the query params the
search accepts and `query_filter.go` turns them into Meilisearch filter groups.
`enrichment.experience_years_min` is already a filterable attribute on the index and
already has a floor filter; it has no ceiling.

Production measurement on 1,349,667 searchable postings: 645,723 (48%) carry a stated
experience requirement. Of those, 7,440 are `0`, 65,857 are `1`, 101,638 are `2`,
165,177 are `3–4`, 201,055 are `5–7`, 36,358 are `8–9`, and 68,198 are `10+`.

## Goals / Non-Goals

**Goals:**
- Make experience a first-class pane in the filter rail, with seniority living there.
- Let a candidate express "I have N years" and see what they can apply to.
- Detect the prose form of "no prior experience required" so the entry-level
  population is measurable instead of invisible.
- Ship without a migration, a `cmd/backfill-derive` run, or a reindex.

**Non-Goals:**
- The `role_type` facet (Individual Contributor vs People Manager). Separate change.
- A lower-bound years control in the UI. `experience_years_min` stays API-only; the
  seniority pills already serve "not a junior posting" for the browsing case.
- A dual-handle range slider. No such control exists in this codebase or in
  `design-system/`, and nothing here needs one.
- Adding a value to `vocab.SeniorityValues`. See the decision below.

## Decisions

### The years control is a preset-stop slider, not a raw-years slider

The pane renders one `<input type="range">` whose `value` is an **index into a preset
array**, exactly as the freshness control does (`FilterModal.svelte`, `kind: 'posted'`,
driven by `FRESHNESS_PRESETS`). Stops: no-experience, 1, 2, 3, 5, 8, 10, Any.

*Why:* the production distribution is heavily skewed — 5–7 years alone holds 201k
postings while 8–9 holds 36k — so a linear 0–50 scale would spend most of its travel
on a tail nobody filters into. Index-into-presets also makes the leftmost stop mean
"no experience required" without a second control, and it reuses a native input, so
there is no drag/touch/a11y work to write.

*Alternatives considered.* A linear slider like `salaryMin`: simpler to read but
mis-scaled against the data, and it cannot express "no experience" distinctly from
"0 or more". A chip row of ranges: fits the surrounding UI but each chip would be a
bespoke non-facet control fighting the include/exclude machinery `FacetDef` provides
for string enums. A dual-handle range: matches the reference design, but means
writing a slider component for a lower bound this change has no demand for.

### "No prior experience" is a value of the existing column, not a new seniority value

The detector writes `0` into the existing `jobs.experience_years_min`. It does **not**
add a `no_experience` member to `vocab.SeniorityValues`.

*Why:* `SeniorityValues` is the closed vocabulary shared by the enrichment contract,
the LLM prompt schema, `internal/classify`'s title dictionary, and the facet config.
Adding a member obliges all of them, plus a backfill and a reindex, to express
something the numeric column already expresses exactly. `0` is not a workaround here —
"requires zero years" is the literal fact.

*Trade-off:* the entry-level filter therefore lives on the years control rather than
next to the seniority pills, which is where the reference design puts it. The two sit
in the same pane, so the grouping is preserved even though the control is not shared.

### `experience_years_max` is additive; `experience_years_min` is untouched

`experience_years_min=N` keeps meaning `enrichment.experience_years_min >= N`. The new
`experience_years_max=N` means `enrichment.experience_years_min <= N`. Both bound the
same attribute; together they express a closed range.

*Why not rename.* `web/static/openapi.yaml` is the integration contract — the CLI, the
MCP surface, and the ChatGPT action all read it. The existing name is live and its
floor semantics are documented; a rename would break callers to buy nothing.

*Acknowledged awkwardness:* `experience_years_max` upper-bounds a field named
`..._min`. The name is right from the caller's side (it is the maximum they will
accept) and wrong from the field's side. The parameter description in `openapi.yaml`
must say plainly which attribute it bounds, or the pair reads as a min/max over two
different fields.

### Both bounds exclude postings with no stated requirement

Meilisearch numeric comparisons match only documents where the attribute is present,
so any bound drops the ~52% of postings with no stated requirement. This is correct —
"asks for at most 3 years" is not something an unstated requirement satisfies — but it
is invisible at the UI, where it looks like the result count collapsed for no reason.
The pane therefore carries a permanent note, not a conditional one.

### The phrase detector keeps the conservative floor

`ExperienceYearsMin` already takes the **smallest** year figure in the description. An
explicit no-experience statement resolves to `0`, which is the smallest possible
value, so it naturally wins under the existing rule with no special-casing. A
description saying "no prior experience required" and separately "3 years with Go is a
plus" yields `0`, which is the honest reading.

## Risks / Trade-offs

**The 48% coverage makes any bound look broken.** → The permanent coverage note in the
pane, plus keeping the default at **Any** so the control is never bounded unless the
user acts.

**The phrase list over-fires on negated or unrelated prose** ("no prior experience with
Kubernetes is required" means the *tool* is optional, not the job). → Phrases are
matched on word boundaries via the package's existing `wordmatch`, and the list is
anchored to bare statements. A phrase followed by "with"/"in" naming a technology is
the known false positive; the test suite covers it explicitly and the list stays
precision-first — a missed entry-level posting is cheaper than a mislabelled senior one.

**Moving seniority out of the `Role` pane splits it from the role picker,** whose slugs
carry a grade prefix (`senior_backend`). A user can select "Senior Backend" in one pane
and not see the Senior pill light up in another. → The two were already independent
params with overlapping meaning; the rail's per-entry staged count makes the second
selection visible without opening the pane. No behaviour changes, only adjacency.

**Existing tests assert the rail's shape.** `filterSections` has a test requiring every
facet param to be reachable in the UI, and the filter-modal tests assert pane contents.
→ Both are updated as part of the tasks that change them; this is a signal the move is
covered, not a hazard.

## Migration Plan

No schema change, no new index attribute, no backfill, no reindex. Deploy is the
ordinary web + API release.

The phrase detector reaches new ingests immediately and existing rows on the next
scheduled `cmd/backfill-derive` run — until then the entry-level stop matches only the
7,440 postings that already carry `0`. That is a smaller set than the feature will
eventually serve, not a wrong one.

Rollback is a revert: the new param is additive, so an older API paired with a newer
web build simply ignores `experience_years_max` (`/jobs` drops unknown filters
silently), and a newer API with an older web build is never asked for it.

## Open Questions

None. The control shape, the vocabulary question, and the parameter naming were
settled above.
