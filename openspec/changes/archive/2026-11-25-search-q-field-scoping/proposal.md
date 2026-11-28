## Why

`q` on `GET /api/v1/jobs/search` and `GET /api/v1/agent/jobs/search` is documented only
as "full-text query over title, company, and description" — its actual matching
semantics (unquoted is OR-of-tokens, quoted is order-independent AND-of-tokens, and it
also reaches `location`) are undocumented and unpredictable to callers (issue #2671).
Reproduced live against prod on 2026-09-09: `q="engineer systems"` (quoted, reversed
word order) surfaces "Engineer – Asset Manager" at "STS Systems Defense" — a non-tech,
security-clearance defense role — because "systems" only appears in the company name,
not the title. A caller who wants systems-engineering roles cannot exclude this today.

## What Changes

- Document `q`'s actual matching semantics for both endpoints in `web/static/openapi.yaml`:
  unquoted `q` is OR-of-stemmed-tokens, quoted `q` is AND-of-stemmed-tokens irrespective
  of word order (**not** a contiguous-phrase match), and `q` searches `title`, `company`,
  `description`, and `location`. Also answers issue #2671's point 4 directly: the default
  relevance order (no `sort`) does **not** reliably rank a contiguous match above a
  scattered-token one, since `ProximityPrecision: byAttribute` (`#1637`) gives the
  `proximity` ranking rule only attribute-level, not word-level, distance data.
- Add a `q_fields` parameter to both endpoints restricting which of those four fields `q`
  matches against (e.g. `q_fields=title`), implemented via Meilisearch's query-time
  `AttributesToSearchOn` — no reindex required, no change to stored index settings.
- An unrecognized `q_fields` value is dropped and reported through the existing
  `meta.ignored_params` convention (`search.UnknownParams`), consistent with how every
  other unrecognized search param is handled — never a hard error, never a silent widen.
- **Explicitly out of scope: exact contiguous-phrase matching.** Two local spikes (see
  `design.md`) confirmed that enabling it requires switching Meilisearch's
  `ProximityPrecision` from `byAttribute` to `byWord` plus a full catalog reindex, and
  that the indexing-cost regression `byAttribute` was chosen to avoid (`#1637`) is real
  and grows with corpus size — directionally confirmed at 60k-document scale (bulk load
  1.7x slower, warm-index incremental push 3.3x slower). Deciding whether to pay that
  cost needs its own change with a real-data-scale measurement, not a bundled guess here.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `job-search`: the "Public job search endpoint" requirement gains a documented `q`
  semantics contract and the new `q_fields` parameter.
- `agent-jobs-search`: the "Agent job search endpoint" requirement's "runs the same
  search" contract is made explicit for `q_fields` — it passes through identically to
  the public endpoint.

## Impact

- `internal/api/handler/search.go` — read the new `q_fields` query param for both
  `SearchJobs` and `AgentSearchJobs`.
- `internal/search/search/client.go` (`buildSearchRequest`) — set Meilisearch's
  `AttributesToSearchOn` when `q_fields` is present.
- `internal/search/search/query_params.go` (`UnknownParams`) — validate `q_fields`
  values against the four searchable fields and report unrecognized ones.
- `web/static/openapi.yaml` — document `q` semantics and the new `q_fields` parameter
  for both endpoints (linted by the `artifacts` CI job's OpenAPI validation).
- No migration, no reindex, no change to Meilisearch index settings.
