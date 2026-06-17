# Design — gupy-board-discovery

## Context

`cmd/harvest-boards` today: `harvest-boards <provider> <seed.json>` →
`loadSeed` → `mapSeeds` → `newBoards` (dedup vs `sources/<provider>.yml`) →
`probeAll` (concurrent `prober.probe(slug) → (name, openJobs, err)`, keep
`openJobs>0`) → `appendEntries`. Probers live in `prober.go` behind the `prober`
interface, with optional markers `seedMapper`/`dedupKeyer` — the established
opt-in-interface idiom.

Gupy boards are numeric `companyId`s with no public seed list. They are only
found by enumerating Gupy's global feed
`https://employability-portal.gupy.io/api/v1/jobs?limit=&offset=`, which returns
postings across all employers (~25 distinct companies per 40-job page) and
paginates by `offset` past the first 100. `?companyId=<id>&limit=1` returns that
company's `careerPageName` and open-job `total`.

## Decisions

### Discovery replaces only the candidate source

The seed (`loadSeed`+`mapSeeds`) is just the *source of candidate ids*. Everything
after — dedup, concurrent probe, append — is provider-agnostic and stays as is.
So discovery is modeled as an alternative candidate source, not a parallel
pipeline. A new opt-in interface mirrors `seedMapper`/`dedupKeyer`:

```go
type discoverer interface {
    discover(ctx context.Context, c httpClient) ([]string, error)
}
```

`run()` chooses the candidate source:
- 3 args (`<provider> <seed.json>`) → seed path, unchanged.
- 2 args (`<provider>`) **and** the prober implements `discoverer` → candidates =
  `discover(...)`.
- 2 args and not a discoverer → usage error (unchanged behavior for seed-only
  providers).

Discovered ids then flow through the **same** `newBoards` → `probeAll` →
`appendEntries`. So a discovered `companyId` is still probed (gets the
authoritative name + count and re-confirms it is live), preserving the
"committed file = our validated facts" doctrine.

### gupyProber

Implements both `prober.probe` and `discoverer.discover`.

- `probe(ctx, c, companyId)`: `GET …/jobs?companyId=<id>&limit=1`, decode
  `{data:[{careerPageName}], <count>}`. Return `(careerPageName, total, nil)`;
  name falls back to `companyId`, `total==0`/error → `("",0,nil)` (skip). Same
  shape as `greenhouseProber`.
- `discover(ctx, c)`: loop `offset = 0, gupyPageSize, 2·gupyPageSize, …`,
  `GET …/jobs?limit=<gupyPageSize>&offset=<offset>`, collect each posting's
  `companyId` into an ordered de-duplicated slice (first-seen order); stop when a
  page returns zero postings or `offset` reaches `gupyMaxOffset` (safety against a
  feed that never empties). Return the distinct ids as strings.

Constants: `gupyFeedURL`, `gupyPageSize` (100), `gupyMaxOffset` (bounds the sweep;
sized well above Gupy's live-job count). The feed host is duplicated as a local
const (the tool lives outside `internal/sources`, mirroring `greenhouseBoardsAPI`).

Register `probers["gupy"] = gupyProber{}`.

### Open-job count for dedup vs. probe

Discovery yields companies that already have ≥1 open job (they are in the feed),
but probe still runs: it returns the authoritative `careerPageName` (the feed
posting carries it too, but probing is uniform and re-confirms liveness) and the
total count. The double round-trip (enumerate + probe-each) is acceptable for a
run-once host tool with 16 workers and client 429-backoff.

## Risks / Trade-offs

- **Volume.** "All companies" can be thousands of distinct ids → thousands of
  probe requests. Bounded concurrency (16) + 429 backoff keep it polite; it is a
  run-once host tool, not a cron job. `gupyMaxOffset` caps the enumeration.
- **Messy names.** `careerPageName` is sometimes a marketing slogan. We keep the
  slug-fallback doctrine (id when empty) and rely on diff review before ingest;
  no name-cleaning (YAGNI).
- **Feed pagination shape.** Assumes `offset` pages the feed to exhaustion. If the
  feed caps `offset`, the sweep ends early (fewer companies) rather than failing —
  `gupyMaxOffset` and the empty-page stop both terminate cleanly.

## Testing

`prober_test.go`-style fake `httpClient` (routed by URL substring):
- `discover`: multi-page feed with repeated companies → distinct ids in order;
  empty page stops paging; `gupyMaxOffset` bound respected.
- `probe`: `companyId` → `(careerPageName, total)`; empty/zero → `("",0,nil)`;
  missing name → id fallback.
- `run()`: discoverer branch (no seed) vs seed branch vs non-discoverer-no-seed
  usage error.

## Migration / Rollout

None automated. Run `harvest-boards gupy` on a host, review the `sources/gupy.yml`
diff, commit, deploy via sources rsync. No schema/runtime change.
