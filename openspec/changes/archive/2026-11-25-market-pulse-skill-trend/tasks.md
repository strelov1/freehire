## 1. Data layer

- [x] 1.1 Add a new migration creating `insights_skill_history(skill text,
      week_start date, open_count integer, PRIMARY KEY (skill, week_start))`
      plus an index serving "all weeks for a set of skills, newest first"
      (e.g. `(skill, week_start DESC)`). Check `migrations/` for the current
      highest-numbered file before naming it (numbering has collided across
      branches before — verify against `main`, not just the local worktree).
- [x] 1.2 Add sqlc queries in `internal/db/queries/insights.sql` (or a new
      file, matching existing grouping style): an idempotent snapshot insert
      (`INSERT INTO insights_skill_history (skill, week_start, open_count)
      SELECT skill, @week_start, open_count FROM insights_skill_stats WHERE
      category = '' AND country = '' ON CONFLICT (skill, week_start) DO
      NOTHING`), a retention prune (`DELETE FROM insights_skill_history WHERE
      week_start < @cutoff`), and a read (`SELECT skill, week_start,
      open_count FROM insights_skill_history WHERE skill = ANY(@skills)
      ORDER BY skill, week_start`).
- [x] 1.3 Run `make sqlc` and confirm the generated code compiles.

## 2. Rollup worker

- [x] 2.1 In `cmd/rollup-stats/main.go`, after `RebuildInsightsSkillStatsGlobal`
      commits its rows within `rebuildInsights`, add the snapshot insert
      (current ISO week's Monday as `week_start`) and the retention prune —
      same transaction, so a mid-run failure yields a skipped week, never a
      partial/bad row (see design.md's transactional-consistency decision).
- [x] 2.2 Add/extend a test covering: (a) a fresh run inserts one row per
      skill with rows in `insights_skill_stats`; (b) a second run in the same
      week inserts nothing new (row count and values unchanged); (c) rows
      older than the retention window are pruned.

## 3. Read API

- [x] 3.1 Add `internal/handler/market_pulse.go` with a handler struct
      following the existing `register(api, mw)` pattern (see
      `internal/handler/insights.go` for the sibling public endpoint's
      style). Route: `GET /me/market-pulse`, gated by `mw.cookie`
      (`RequireAuth`) per the project's cookie-only convention for
      browser-personal surfaces.
- [x] 3.2 Implementation: load the caller's skills via
      `userprofile.Service.Get(ctx, userID).Skills`; if empty, respond
      `{"data": [], "meta": {...}}` immediately. Otherwise read the skill
      history for those skills, group by skill in Go, and compute per-skill
      `open_count` (latest week), `change_pct` (null if fewer than 2 points),
      and the full `series`.
- [x] 3.3 Register the new handler in `internal/handler/handler.go` alongside
      the other `mw.cookie`-gated per-user surfaces.
- [x] 3.4 Handler tests: authenticated caller with profile skills gets
      populated `data`; authenticated caller with an empty profile gets `data:
      []` and `200`; unauthenticated request gets `401`.

## 4. Frontend

- [x] 4.1 Add `web/src/routes/my/market-pulse/+page.svelte` (+ a `load`
      or client fetch against `GET /api/v1/me/market-pulse`, matching the
      project's existing data-fetching convention for `/my/*` pages).
- [x] 4.2 Render one card per skill: skill name, current `open_count`, an
      up/down indicator from `change_pct`, and a sparkline over `series`.
      Consult the `dataviz` skill before styling the sparkline/trend
      indicators so they read as one system with the rest of the design
      system.
- [x] 4.3 Empty state when `data` is empty: point the user at adding
      skills/uploading a CV rather than showing a bare blank page.
- [x] 4.4 Add an entry point to the new page from wherever `/my/*` personal
      surfaces are already linked (nav/profile menu) — check current
      `web/src` nav structure for the right spot rather than inventing a new
      pattern.

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`,
      `go test ./...`; run `go test -tags=integration ./...` since this
      change touches worker and handler behavior, not just a signature.
- [x] 5.2 `pnpm run lint`, `pnpm run check`, `pnpm test`, `pnpm run build` in
      `web/` before pushing.
- [x] 5.3 Manual smoke: run the app locally, sign in with a profile that has
      skills, load `/my/market-pulse`, confirm cards render; confirm an
      empty-profile account sees the empty state instead of an error.

## 6. Post-demo follow-ups (requested live, not in the original plan)

- [x] 6.1 Gate the "Market Pulse" nav entry to beta testers
      (`users.beta_tester`) while the history is thin post-launch —
      `accountNav.ts`'s `betaOnly`/`beta` fields (UI-only, matches the
      existing gating convention; the endpoint itself stays open).
- [x] 6.2 Add a client-side skill-name filter above the card grid
      (`MarketPulseView.svelte`) for profiles with many skills — plain
      substring match, no backend change.
