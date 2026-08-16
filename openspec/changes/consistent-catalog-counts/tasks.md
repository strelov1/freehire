## 1. Correct the estimate

- [x] 1.1 Add `migrations/0109_estimate_open_jobs_full_predicate.sql` replacing
      `estimate_open_jobs()` so its `EXPLAIN` applies the full listing predicate
      (`closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private`). Verify
      the next free number at write time — `migrations/` already carries
      duplicate numbers, so `ls migrations/ | tail` rather than assuming.
      (Landed as 0109: main had moved past 0102 to 0108.)
- [x] 1.2 Add an integration test (`//go:build integration`, testdb) seeding
      open, closed, duplicate-suppressed, and private jobs, asserting the
      function's result excludes the last three. Assert *which set* is estimated
      — the result must sit nearer the paginated count than the not-closed one —
      rather than a tolerance around the exact count: on a small table the
      planner's selectivity arithmetic carries error that is not a defect.

## 2. `internal/cache` — the layer

- [x] 2.1 Write the failing test for `Memory`: `Set` then `Get` round-trips,
      a missing key reports `found == false` with no error, and an entry past
      its TTL reports a miss.
- [x] 2.2 Define `Cache` (`Get(ctx, key) ([]byte, bool, error)`,
      `Set(ctx, key, val []byte, ttl time.Duration) error`) and implement
      `Memory` (map + mutex). Document on the interface that a returned error is
      the caller's cue to treat the read as a miss — mirroring how
      `ratelimit.Throttler` leaves the fail-open decision to one caller.
- [x] 2.3 Write the failing test for `RedisCache` against `miniredis` (already a
      dependency): round-trip, miss, TTL honoured, and an error surfaced when
      the backend is closed.
- [x] 2.4 Implement `RedisCache` over the existing `*redis.Client`.
- [x] 2.5 Write the failing test for `GetJSON[T]`/`SetJSON`: round-trip of a
      struct, a miss on an absent key, and a decode failure reported as a miss
      rather than a hard error (a stale incompatible payload must not wedge a
      caller).
- [x] 2.6 Implement `GetJSON`/`SetJSON` as free generic functions — Go does not
      allow type parameters on methods.

## 3. `internal/catalogstats` — the figures

- [x] 3.1 Write the failing unit test for the derived counts: the ATS-platform
      count comes from `sources.Taxonomy()` and the Telegram-channel count from
      the parsed channel config, so adding an entry moves the number with no
      literal to edit.
- [x] 3.2 Define `Snapshot` (open jobs, companies, sources, ATS platforms,
      Telegram channels, `ComputedAt`) and the derived-count half of `Compute`.
      `Sources` was added on top of the planned fields: /open's "166 ATS
      platforms" was really the whole registry mislabelled, and the honest ATS
      count is 93 — so the strip leads with total reach (227) under an accurate
      label and the narrower figure stays available in the API.
- [x] 3.3 Write the failing integration test for the exact counts: seed open,
      closed, duplicate-suppressed and private jobs plus their companies, assert
      `Compute` counts exactly the set `GET /api/v1/jobs` paginates.
- [x] 3.4 Add the sqlc query for the exact open-job and company counts and wire
      it into `Compute` (`make sqlc`; edit `internal/db/queries/*.sql`, never the
      generated code).
- [x] 3.5 Write the failing test for `Load`: a cached snapshot returns exact; an
      empty cache and an erroring cache both return the estimate flagged as
      degraded; neither returns an error to the caller.
- [x] 3.6 Implement `Load` and `Store`. `Load` must never invoke `Compute` —
      made structural instead of asserted: `Load` takes no `ExactCounter`, so no
      read path can reach the catalogue-wide scan even by mistake. A runtime
      assertion would have been the weaker guarantee.

## 4. Publish the snapshot

- [x] 4.1 Write the failing test that `rollup-stats`'s snapshot step stores a
      computed snapshot through the cache with the intended TTL. The TTL is
      asserted in `catalogstats` against a recording cache — the retention
      decision belongs next to the constant that encodes it, not in the worker.
- [x] 4.2 Call `catalogstats.Compute` + `Store` from `cmd/rollup-stats`, after
      the existing rollups. A snapshot failure must not fail the run's exit code
      — the rollups are the worker's primary job and already have their own
      transaction semantics.
- [x] 4.3 Construct the cache in `cmd/rollup-stats` from `cfg.RedisURL`
      (`cmd/server` already builds a client for rate limiting — follow that
      wiring, do not duplicate parsing logic).

## 5. Serve it

- [x] 5.1 Write the failing handler test for `GET /api/v1/stats/catalog`:
      `{"data": {...}}` carrying all four figures plus `computed_at`, and
      unauthenticated access.
- [x] 5.2 Implement the handler and register the route alongside the existing
      `/stats/*` routes in `internal/handler/stats.go`.
- [x] 5.3 Write the failing test that `GET /api/v1/jobs` `meta.total` prefers the
      snapshot's exact count and falls back to the estimate when the cache is
      empty or unreachable, returning 200 in both cases.
- [x] 5.4 Switch the jobs-list total to `catalogstats.Load`.
- [x] 5.5 Construct the cache in `cmd/server` from the `*redis.Client` it already
      builds, and inject it into the handlers.

## 6. Frontend

- [x] 6.1 Add the `/api/v1/stats/catalog` client method (check whether the
      contract is codegen'd — `cmd/gen-contracts` — and regenerate rather than
      hand-writing the type if so).
- [x] 6.2 Switch `web/src/routes/about/+page.server.ts` to the single snapshot
      call, replacing the two `limit=1` list reads.
- [x] 6.3 Switch `web/src/routes/open/+page.server.ts` to the snapshot call and
      delete `ATS_PLATFORMS = 166` / `TELEGRAM_CHANNELS = 88` from
      `+page.svelte`, reading both from the snapshot.
- [x] 6.4 Update the API-down fallbacks in `web/src/lib/components/HomeView.svelte`
      (`3.4M+` → `3.3M+`, `200K+` → `290K+`) and take the ATS-platform figure
      from the snapshot instead of the hardcoded `'166'`.
- [x] 6.5 Verify `/about` and `/open` render the same figures, headless, against
      a running stack — the numbers agreeing is the whole point of the change,
      so it needs looking at rather than asserting. Done against a local stack
      seeded with 40 listed + 40 excluded postings: API, /about and /open all
      reported 40/1/227/95. The degraded path was exercised too (cache flushed,
      then Redis stopped): both endpoints stayed 200, and it caught a real bug —
      /open rendered a static "290K+" companies figure when the backend had said
      it could not measure one. Fixed by mapping database-only figures to null
      instead of zero, so "not measured" stops looking like a value.

## 7. Finish

- [x] 7.1 `gofmt -w` the touched Go files, then `go vet ./...`, `go test ./...`,
      and `go vet -tags=integration ./...`.
- [x] 7.2 Run the tagged suite for the packages whose behaviour changed
      (`go test -tags=integration ./internal/db/ ./internal/handler/
      ./internal/catalogstats/`).
- [x] 7.3 Update `internal/handler/AGENTS.md` with the new route, and note the
      snapshot's ownership and fallback rule wherever the module map wants it.
      Recorded as a root AGENTS.md convention rather than a new per-package
      AGENTS.md: the rule is cross-cutting (every scale figure reads one
      snapshot, nothing counts on a request path) and `internal/catalogstats` is
      too small to carry a module file of its own.
