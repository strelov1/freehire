## Why

Agents that call the search API want several role names at once and a title-only match.
The busiest client is the `freehire-search` skill in MadsLorentzen/ai-job-search. Today
`OR` is an ordinary word: `produktchef OR "product owner"` returns exactly what
`"product owner"` alone returns (10,352). So the skill sends one call per term and merges
by URL (its #653). The `Q` description in `web/static/openapi.yaml` also misstates plain
words as an OR. Measured, they follow Meilisearch's `matchingStrategy: last`, and word
order changes the result.

## What Changes

- `q` accepts `OR` (uppercase, standalone, outside quotes) between alternatives, and
  `title:` at the start of an alternative to match it in the title only.
- One alternative is the single-index request the search sends today. Two or more go out
  as one federated `/multi-search`. Hits, paging, sort and facet counts work over the
  merged set, and a posting matched twice appears once.
- More than 5 alternatives is a 400: `q has more than 5 OR alternatives`.
- The `Q` OpenAPI description is corrected, and quotes, `-`, `OR` and `title:` are
  documented.
- Not changed: what plain spaces mean, the UI, and the index settings (no reindex).

## Capabilities

### New Capabilities

### Modified Capabilities

- `job-search`: the public search query gains `OR` and `title:` operators.

## Impact

`internal/search/search` (`Client.Search`, the facet paths, a new parser and federated
builder), `internal/api/handler/errors.go`, `web/static/openapi.yaml`, and the
`search_integration_test.go` engine image (v1.13 → v1.49.0, the version prod runs).
Every jobs-search caller gets the operators: web, agent API, OJCP, MCP and the assistant.
