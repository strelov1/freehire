## Context

`q` is passed to Meilisearch as-is (`internal/api/handler/search.go` → `buildSearchRequest`
in `internal/search/search/client.go`), with no field scoping — Meilisearch's
`SearchableAttributes` for the jobs index is `["title", "company", "description",
"location"]` (`client.go`, `facetSettings`). The index's `ProximityPrecision` is
`byAttribute` (set in `#1637`, see `client.go`'s comment on `facetSettings`), which is
why a quoted `q` degrades to an order-independent AND rather than a contiguous-phrase
match — Meilisearch cannot verify word adjacency without the `byWord` word-pair
proximity structure. See `proposal.md` for the motivating example (`STS Systems
Defense`) and why real exact-phrase matching is out of scope for this change.

Unrecognized query params are reported, never silently accepted or hard-errored, via
`search.UnknownParams` (`internal/search/search/query_params.go`) — it checks **param
names** against a known vocabulary (facets, their `_exclude`/`_mode` suffixes, and a
short `scalarFilters` list) and returns unmatched ones in `meta.ignored_params`. `q`
itself is not in that vocabulary; the handler passes it to `UnknownParams` via the
`alsoKnown` parameter alongside `limit`/`offset`/etc. `q_fields` needs the same
treatment, but the interesting failure mode is an unrecognized **value** inside an
otherwise-known param name (e.g. `q_fields=salary`), which `UnknownParams` does not
currently express — it only reports names it has never heard of.

## Goals / Non-Goals

**Goals:**
- Let a caller restrict `q` matching to a subset of the four searchable fields,
  query-time, with no index/reindex change.
- Document `q`'s actual matching semantics so a caller can predict results.

**Non-Goals:**
- Contiguous exact-phrase matching. Confirmed out of scope by two local spikes (see
  "Spike findings" below) — it requires `ProximityPrecision: byWord` plus a full catalog
  reindex, and the indexing-cost regression that `byAttribute` exists to avoid (`#1637`)
  is real and grows with corpus size. That is a separate, larger decision.
- Changing `q`'s OR/AND token semantics. Only field scoping is added; a caller who
  wants exact-phrase behavior still cannot get it after this change (documented as a
  known gap, not silently implied to be fixed).

## Decisions

### `q_fields` is implemented via Meilisearch's query-time `AttributesToSearchOn`

Meilisearch's `SearchRequest.AttributesToSearchOn` restricts which searchable
attributes a single query matches against, without touching stored index settings —
no reindex, no `ProximityPrecision` change, no interaction with the exact-phrase
question. `buildSearchRequest` sets it only when `q_fields` is present; when absent,
Meilisearch's default (all `SearchableAttributes`) applies, matching today's behavior
exactly. This is the cheapest of the two levers the issue asked for, and it's fully
independent of the exact-phrase decision — hence splitting it into its own change.

**Alternative considered**: a `title` boolean shortcut param (`title_only=true`)
instead of a general `q_fields`. Rejected: the issue explicitly asks for a general
field-scoping mechanism, and a single-purpose boolean would need a second parameter
the moment `company`-only scoping is wanted, which is a real use case for filtering
out reposting aggregators from a company-name search.

### Order of `AttributesToSearchOn` is fixed, not caller-supplied

When `q_fields=company,title` is given, the fields are passed to Meilisearch in the
index's canonical order (`title`, `company`, `description`, `location`), not the
order the caller wrote them. Meilisearch's relevance ranking is sensitive to
attribute order among matched fields; a caller-controlled order would make identical
restrictions (`title,company` vs `company,title`) rank differently for no reason a
caller could predict — directly the kind of unpredictability this change exists to
remove.

### An unrecognized field name invalidates the whole `q_fields` value

`q_fields=title,salary` is treated the same as `q_fields=salary`: the entire
parameter is dropped (falls back to unrestricted, all-fields matching) and reported
once via `meta.ignored_params`, rather than silently applying only the valid names.
Partial application would mean a caller who makes one typo (`titel`) alongside a
correct name (`company`) gets a silently narrower-than-intended search with no
signal beyond the general convention that whatever else IS restricting must be
"working as expected" — worse than the current "params I don't understand widen and
tell you" contract this API already keeps everywhere else (`UnknownParams`,
`search.Narrows`, `UnknownCompanyParams`). Whole-value drop keeps the failure mode
in the same class as an unrecognized facet: coarse, but honest and consistent.

### Validation lives beside `UnknownParams`, as a value check, not a name check

`query_params.go` gains a small `validQFields` set and a helper that both (a)
returns the valid `AttributesToSearchOn` list for `buildSearchRequest` and (b)
reports `q_fields` as an ignored param (reusing the existing `UnknownParam` /
`SortAndCap` shape) when any named field isn't in that set. This sits next to
`UnknownParams` rather than inside it: `UnknownParams` answers "is this param name
one I read at all", which stays true for `q_fields` (it is read) — the new check
answers "is what's inside it something I understood", a different question the
existing function doesn't ask of any other param today.

### Spike findings backing the exact-phrase non-goal

Two local, isolated Meilisearch spikes (v1.49.0, matching prod) ran during this
change's planning, never against prod infrastructure:

1. **900 real job documents** (pulled from the live public API) indexed into an
   empty index under `byAttribute` vs `byWord`: both completed in under 1.2s. This
   under-counts the real cost — an empty index has no existing
   `wordPairProximityDocids` structure to merge into, which `#1637`'s own benchmark
   identifies as the expensive part.
2. **60,000 synthetic documents**, bulk-loaded, then a 200-document incremental push
   measured on the resulting "warm" index (mirroring `search-drain`'s real push
   shape): `byWord` bulk-load was ~1.7x slower (14.0s vs 8.2s) and the warm-index
   incremental push was ~3.3x slower (0.29s vs 0.09s, Meilisearch-reported task
   duration) than `byAttribute`.

Caveat, stated plainly: the synthetic corpus's description text draws from an ~80-word
vocabulary, far narrower than real job postings. `#1637`'s own benchmark measured
against a ~317k-document **real** sample and found costs an order of magnitude higher
(~10s per 200-document batch) — a gap this spike's low lexical diversity plausibly
explains, since `wordPairProximityDocids` size depends on vocabulary richness, not
document count alone. The spike confirms the **direction and existence** of the cost
(and that it grows with index size, as expected); it does not overturn `#1637`'s
magnitude, and a decision to actually flip `ProximityPrecision` needs a measurement
against real-scale, real-vocabulary data — not bundled into this change.

## Risks / Trade-offs

- **[Risk]** A caller expects `q_fields=title` combined with quoted `q` to yield an
  exact-phrase-in-title match. → **Mitigation**: the OpenAPI doc and this proposal are
  explicit that quoting is still order-independent AND, with or without `q_fields`.
- **[Risk]** Whole-value-drop on `q_fields` surprises a caller who only mistyped one
  of several field names. → **Mitigation**: `meta.ignored_params` names `q_fields`
  explicitly, matching the existing convention callers of this API already rely on to
  catch typos in facet params.
- **[Trade-off]** Fixed attribute order (not caller order) means `q_fields` cannot be
  used to express "prefer company matches over title matches" — only which fields
  are eligible, not their relative weight. Accepted: the issue's request was for
  restriction, not custom ranking, and custom per-call ranking is a materially bigger
  feature.

## Migration Plan

No migration, no reindex, no index settings change. This is an application-code and
documentation change: deploy `internal/api/handler` and `internal/search/search`
changes together with the updated `web/static/openapi.yaml`. Rollback is a plain
revert — no data or index state depends on `q_fields` existing.
