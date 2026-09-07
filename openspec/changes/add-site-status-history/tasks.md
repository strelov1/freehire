## 1. Schema and generated queries

- [x] 1.1 Add migration `migrations/0144_site_status_daily.sql` creating
      `site_status_daily(day date PRIMARY KEY, worst_severity smallint NOT
      NULL, updated_at timestamptz NOT NULL DEFAULT now())`.
- [x] 1.2 Add `internal/platform/db/queries/site_status_daily.sql`:
      `RecordSiteStatusSample` (`:exec`, upsert-max on `worst_severity` via
      `GREATEST`) and `SiteStatusHistory` (`:many`, last 90 days, ordered by
      `day` ascending).
- [x] 1.3 Run `make sqlc` (Docker-based, confirmed available in this
      environment — no local `sqlc` binary needed) to regenerate
      `internal/platform/db`. Verify the diff is exactly the new
      query/table code, nothing else drifted.

## 2. Severity mapping and shared site-status computation (`internal/api/handler`)

- [x] 2.1 RED: write a table test for a new pure function mapping
      `providerStatus` to its severity int (0/1/2) and back, covering all
      three values and an out-of-range value on the reverse mapping.
- [x] 2.2 GREEN: implement the mapping in `status.go`, next to the existing
      `providerStatus` constants. An out-of-range severity reads as `down`
      (alarming, not reassuring) rather than `operational`.
- [x] 2.3 / 2.4: extracted `(h *statsHandlers) currentSiteHealth(ctx) (siteHealth,
      bool)` (the bool is `dbUp`, needed by the caller to decide the
      short-circuit) from the assembly previously inlined in `IngestStatus`.
      No separate unit test for the extraction itself — it is exercised by
      the existing `status_integration_test.go` (unchanged behavior) plus
      the new history assertions added there in task 3.4.

## 3. History read (`internal/api/handler/status.go`)

- [x] 3.1 RED: extend `status_test.go` for `siteHistoryFromRows`, covering
      empty input (non-nil empty slice) and mapping day+severity in order.
- [x] 3.2 GREEN: implement `siteHistoryFromRows` and `siteHistoryEntry`; add
      `History []siteHistoryEntry` to `siteHealth`.
- [x] 3.3 Wire `IngestStatus` to query `SiteStatusHistory` (only on the
      dbUp path) and populate `site.history`; the dbUp==false path sets an
      explicit empty slice so the wire shape is always `[]`, never `null`.
- [x] 3.4 Extended `status_integration_test.go`: seeded `today-2` (down),
      left `today-1` unsampled (the gap), seeded `today` (degraded); asserts
      exactly 2 history entries in ascending order and that the gap day is
      absent, not `"operational"`. Passes against a real Postgres.

## 4. Daily sampler ticker (`cmd/server/main.go`)

- [x] 4.1 Skipped as redundant: severity computation is exactly
      `severityFromStatus(deriveSiteStatus(...))`, already covered by 2.1's
      table tests.
- [x] 4.2 GREEN: added `handler.StartSiteStatusSampler(ctx, pool, queries,
      interval)` in `internal/api/handler/site_status_sampler.go` (exported
      from the handler package, not `cmd/server`, since it needs
      `currentSiteHealth` and `statsHandlers` — both unexported). Shaped
      like `startSuggestRefresh`: samples immediately, ticks on the given
      interval, logs (does not panic) on a failed record, returns on
      `ctx.Done()`. Covered by an integration test
      (`site_status_sampler_integration_test.go`, real Postgres, a 50ms
      interval) asserting the immediate sample, a later tick advancing
      `updated_at`, and the ticker actually stopping after `cancel()`.
- [x] 4.3 Wired into `cmd/server/main.go` right after `handler.Register`,
      unconditional (no external dependency to gate on, unlike
      `startSuggestRefresh`'s Meili-key gate). Interval:
      `siteStatusSampleInterval = 5 * time.Minute`, a named constant next to
      `suggestRefreshInterval`.
- [x] 4.4 Superseded by the integration test in 4.2, which exercises the
      real ticker end-to-end (immediate sample + periodic re-sample +
      shutdown) rather than only the SQL upsert semantics — turned out to be
      straightforward to test directly with a short interval, so the
      "awkward to unit-test" concern in the original plan did not
      materialize.

## 5. Frontend (`web/`)

- [x] 5.1 Added `SiteHistoryEntry` (`day`, `status`) and `history:
      SiteHistoryEntry[]` on `SiteHealth` in `web/src/lib/types.ts`.
- [x] 5.2 Added the daily history strip to `StatusBoard.svelte`: 90 tiles
      anchored on the response's own `generated_at` (UTC), not the visitor's
      local clock (see the `historyTiles` derivation) — a gap between the
      anchor and a stale local clock would otherwise misalign "today". Tiles
      reuse `STATUS_META`'s dot classes; a new `HISTORY_TILE_META['no-data']`
      (`bg-border`) covers the gap case. Native `title` attribute for the
      per-day tooltip (no library). Wrapped in `overflow-x-auto` for narrow
      viewports.
- [x] 5.3 `svelte-check`: 0 errors. `pnpm check:dead` and
      `design-system`'s `check:tokens` both clean (no new raw-utility
      instances — `bg-border`, `h-6`, `w-1.5`, etc. are all existing scale
      utilities, not arbitrary values). Manual dev-server + screenshot
      verification moved to §6.2 alongside the backend end-to-end check
      (needs seeded `site_status_daily` rows to show anything besides an
      all-gray strip).

## 6. Finish

- [x] 6.1 `gofmt -l .` clean, `go vet ./...`, `go vet -tags=integration
      ./...`, `go test ./...` green, and `go test -tags=integration
      ./internal/api/handler/... ./internal/platform/...` green (including
      `internal/platform/arch/layering` and `internal/platform/db`/migrate
      suites) against real Postgres containers.
- [x] 6.2 Manually verified end-to-end: real local Postgres + `cmd/server`,
      seeded `site_status_daily` spanning 10 days with 3 deliberate gaps,
      confirmed `GET /api/v1/status`'s `site.history` matches exactly
      (gaps absent, ascending order) and that today was already present —
      the sampler's immediate-on-boot sample wrote it before the first curl.
      Screenshot-verified `/status` renders the strip with the right colors
      in the right positions, including the gray "no data" gaps.
- [x] 6.3 Ran `simplify` pass: removed a one-key `Record<'no-data', ...>`
      wrapper in `StatusBoard.svelte` in favor of a plain constant
      (`NO_DATA_TILE_META`) — the rest of the diff was already minimal.
- [x] 6.4 Code review pass on the full diff. Fixed: `CURRENT_DATE` (session
      timezone) → `(now() AT TIME ZONE 'utc')::date` in both queries,
      matching the `social_digest.sql` precedent for the identical bug
      class; the 90-day window was actually 91 days (`>=` → `>` against the
      interval); `StartSiteStatusSampler`'s throwaway partially-populated
      `&statsHandlers{}` replaced by making `currentSiteHealth` a plain
      function over `*pgxpool.Pool`; `severityFromStatus`/`severityToStatus`
      unified onto one `severityOrder` source of truth instead of two
      hand-kept switches; the hardcoded `"2006-01-02"` now reuses the
      existing `dateLayout` constant, and the DB-down short-circuit reuses
      `siteHistoryFromRows(nil)` instead of a second hand-built empty slice.
      Left as deliberate non-fixes (see design.md's "Code review outcome"):
      sequential (not concurrent) DB round trips in `IngestStatus`, and no
      exhaustiveness-check tooling for `providerStatus` switches (consistent
      with the rest of the package). Re-verified manually: the 90-day
      boundary is now exactly 90 days (confirmed `today-90` excluded,
      `today-89` included against a real Postgres), and the full
      integration suite re-ran green.
- [ ] 6.5 PR, merge, deploy via `release.sh`, verify live (`GET
      /api/v1/status` carries `site.history`, `/status` renders the tile
      row, no unexpected 5xx after the flip). Note: history will be empty on
      first deploy until the sampler has run a few times — this is expected
      per the "absent days are absent" design decision, not a bug to chase.
- [ ] 6.6 Finish the branch, archive and sync the OpenSpec change.
