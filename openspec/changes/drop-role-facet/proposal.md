## Why

The `role` facet is a cross-product of two facets the site already has, and almost
nobody uses it.

Measured on two days of production access logs — 54,870 job searches:

| Filter | Applied | Share |
|---|---|---|
| `category` | 8,782 | 16.0% |
| `seniority` | 4,847 | 8.8% |
| `skills` | 4,407 | 8.0% |
| **`role`** | **894** | **1.6%** |

Specialization is used **ten times more often**. And 1.6% is the same figure the
`role-search-suggestions` spec recorded a year ago (1.1%) — a year of dictionary work
did not move it.

Measured against the live catalogue, the facet is also redundant. Of its 1,200 values:

- **47** are a specialization spelled identically. Not approximately: all 47 return the
  same posting count to the digit — `design` 40,769 both ways, `sales` 191,352 both
  ways, zero divergence across the whole vocabulary.
- **979** are specialization × seniority, which two existing facets already express.
- **8** are a bare grade (`senior`), which is the Experience facet.
- **166** are genuinely their own names (`ios_developer`, `accountant`, `2d_artist`).

So 86% of the vocabulary is a mechanical join of two axes, and the axis it joins on is
thin: seniority resolves for **24.3%** of the catalogue, against 97.5% for
specialization. The graded half of the facet is blind to three postings in four.

The 166 named roles are the only thing the facet adds, and the suggestions dictionary
that shipped this week already carries them better: 21,176 mined posting titles,
including "iOS Developer" and "Accountant", written the way the market writes them
rather than the way a curated list guesses.

## What Changes

- **BREAKING (public API):** the `role` filter and its `role_exclude` / `role_mode`
  twins are removed from `GET /api/v1/jobs/search` and `GET /api/v1/jobs/facets`. A
  request still carrying `role=` is not refused — it lands in `meta.ignored_params`,
  the mechanism that already reports a dropped filter, so a stale saved search or
  shared link says what happened instead of silently widening.
- The Role pane leaves the filter modal. Specialization and Experience stay, and
  together they say everything a role slug said.
- `roles` leaves the search index document, and `internal/dict/roletag` is deleted with
  everything derived from it: `ROLE_LABELS`, `ROLE_ALIASES` and the `roleRelated`
  adjacency map in the web contracts.
- The AI filter (`searchintent`) stops emitting `role` and emits `category` +
  `seniority` instead — which is what it should have been saying all along, since that
  pair is what the role slug decomposed into.
- Suggestions stop offering the `role` kind. The 166 named roles are already in the
  dictionary as mined titles, so what a visitor typing "ios developer" gets does not
  get worse; the `category`-vs-`role` de-duplication in the builder is deleted with the
  kind it existed to resolve, which is what lets specializations appear as suggestions
  at all — today the dictionary holds **zero** of them, because every one collided.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `role-facet`: the facet is removed. Its questions are answered by `category` and
  `seniority`, which the same request already accepts.
- `job-search`: `role` leaves the filter vocabulary and the index document.
- `search-suggestions`: the `role` kind is no longer built, and the category-vs-role
  de-duplication goes with it.
- `role-search-suggestions`: the dropdown no longer applies a role facet.
- `ai-filter-from-text`: the interpreter emits `category` + `seniority` where it used
  to emit `role`.

## Impact

- **Deleted:** `internal/dict/roletag`, `web/src/lib/roleRelated.ts`, the Role pane in
  the filter modal, `ROLE_LABELS` / `ROLE_ALIASES` in the generated contracts.
- **Modified:** `internal/search/search` (the filter table and the index document),
  `internal/search/searchintent`, `internal/search/suggest`, `cmd/gen-contracts`,
  `cmd/build-suggestions`, `internal/dict/classify` (which reads roletag's tables),
  `web/static/openapi.yaml`, and the web files that name the facet —
  `facets.ts`, `filterSections.ts`, `seeAlsoMark.ts`, `familymarks.ts`,
  `saveSearchAlert.ts`, `cv.ts`.
- **Untouched, checked:** `internal/ai/aiarchetype` and `internal/dict/roletype` name
  roletag only in a comment about shared doctrine — neither calls it. The
  `/roles/[category]` landing pages are keyed on **category**, not on this facet, and
  keep working; only their name suggests otherwise.
- **Operational:** removing `roles` from the index document needs a reindex before the
  facet stops being served, or the facets endpoint answers 500 for a value the live
  index still declares filterable. Settings first, binary second — the same order the
  clearance facet needed.
