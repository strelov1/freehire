## Why

Nothing public says how much of the catalogue carries a semantic embedding, or how
fast the rest is being covered — so a backlog is invisible to everyone but an operator
with `psql`, and a volunteer has no reason to run the client
([freehire-volunteer-embed](https://github.com/strelov1/freehire-volunteer-embed)) and
no way to see that it helped. It also went unnoticed on 2026-10-19 that nothing was
enqueuing new postings at all (fixed in `freehire-ops` with `freehire-embed.timer`): a
visible coverage figure is what would have shown it.

## What Changes

- A coverage snapshot — how many postings are eligible for an embedding and how many
  carry a current one — measured by a new pass inside `cmd/rollup-stats` and published
  to the cache, because the count takes over a minute on production and can never run
  on a request path.
- An in-process sampler in `cmd/server` that reads the `semantic_outbox` depth every
  15 seconds and keeps the recent samples in memory, from which the rate and an ETA
  are derived.
- `GET /api/v1/stats/embeddings`: the current progress as one JSON document.
- `GET /api/v1/stats/embeddings/stream`: the same document as server-sent events, one
  event per sample.
- A public `/embeddings` page: server-rendered from the JSON endpoint, then updated
  live from the stream; it links the volunteer client for anyone who wants to help.

## Capabilities

### New Capabilities

- `embedding-progress`: the public measure of semantic-embedding coverage — what is
  counted, how it is kept fresh, the JSON and stream endpoints, and the page.

### Modified Capabilities
<!-- none -->

## Impact

- New package `internal/search/embedprogress` (search block: it needs
  `search.CurrentEmbedderModel()`), added to the layering table.
- New SQL queries in `internal/platform/db/queries/` (coverage counts, claimable depth)
  and `make sqlc`.
- `cmd/rollup-stats`: one more best-effort pass after the rollups, like the catalogue
  and source snapshots.
- `cmd/server`: the sampler goroutine and its broadcaster, wired like the recent-jobs
  feed.
- `internal/api/handler`: two routes, one of them SSE with a per-IP connect limit.
- `web/`: the `/embeddings` route and an API client method; `web/static/openapi.yaml`
  for the public JSON endpoint.
- Depends on `freehire-embed.timer` (`freehire-ops`) running hourly: the queue depth
  only stands for "not yet covered" while something keeps enqueuing.
