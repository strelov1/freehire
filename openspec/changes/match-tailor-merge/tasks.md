## 1. Tailor workspace absorbs the fit-analysis trigger

- [x] 1.1 In `web/src/routes/tailor/[slug]/+page.svelte`, replace the `409 "run the fit analysis
      first"` handler (currently `goto(resolve('/match/[slug]', { slug }))`) with logic that opens
      the existing fit-analysis SSE stream (`$lib/matchAnalysis.ts`) in place.
- [x] 1.2 Render the stream's stage progress (stepper, thinking panel, progressive section fill) in
      the Job Match tab while it runs, reusing the presentation already built for `MatchAnalysisFull`
      rather than duplicating it. (Reused `MatchAnalysisFull` itself, with a new optional `onDone`
      prop, rather than reimplementing its stream-consumption UI.)
- [x] 1.3 On stream completion, retry the tailoring bootstrap so the workspace loads normally with
      the now-cached analysis. On stream failure, surface the error state the stream already emits
      (do not silently fall back to a dead workspace).

## 2. Repoint in-app links to `/tailor/[slug]`

- [x] 2.1 Update `web/src/lib/components/MatchSummary.svelte`'s two `resolve('/match/[slug]', ...)`
      call sites to `resolve('/tailor/[slug]', ...)`.
- [x] 2.2 Grep the rest of `web/src` for other `/match/[slug]` link targets (job cards, `JobDrawer`,
      `SwipeDeck`, `JobView`) and repoint each to `/tailor/[slug]`. (Job cards/drawer/swipe-deck all
      route through `MatchSummary`, already covered by 2.1; found and fixed one more direct link in
      `AnalysesView.svelte`, the `/my/activity/matches` history list.)

## 3. Redirects for old paths

- [x] 3.1 Update `web/src/routes/jobs/[slug]/fit/+page.ts` to redirect (308) to
      `/tailor/${params.slug}` instead of `/match/${params.slug}`.
- [x] 3.2 Add `web/src/routes/match/[slug]/+page.ts` (new file) redirecting (308) to
      `/tailor/${params.slug}`. (Implemented by rewriting the existing `+page.server.ts`'s load
      function to redirect instead — its old data-fetching load became dead code the moment the
      page it fed is deleted, so replacing it in place avoided leaving two load files where one
      does nothing.)

## 4. Remove the standalone page

- [x] 4.1 Delete `web/src/routes/match/[slug]/+page.svelte` and its `+page.server.ts`, only after
      task 3.2's redirect route is in place so the path never dead-ends. (`+page.svelte` deleted;
      `+page.server.ts` kept as the redirect file per 3.2's note above, rather than deleted and
      re-added as `+page.ts`.)

## 5. Verify

- [x] 5.1 `pnpm check` / `pnpm build` in `web/` pass with no route-resolution errors from the
      removed page or the changed link targets. (0 errors, same 18 pre-existing baseline warnings.)
- [ ] 5.2 Manually open `/tailor/[slug]` for a vacancy with no cached analysis; confirm the stream
      runs inline, the workspace never navigates away, and the Job Match tab ends up showing the
      completed analysis. **NOT VERIFIED LIVE** — needs an authenticated user with a stored CV and
      a configured LLM proxy (`LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`), neither available in the
      sandboxed worktree stack used for this change. See note below.
- [ ] 5.3 Manually open `/tailor/[slug]` for a vacancy with a fresh cached analysis; confirm it
      renders immediately from the bootstrap response with no stream. **NOT VERIFIED LIVE** — same
      blocker as 5.2.
- [x] 5.4 Manually hit the old `/match/[some-slug]` and `/jobs/[some-slug]/fit` URLs; confirm both
      308-redirect to `/tailor/[some-slug]`. Verified live against the worktree's own Docker stack:
      both return `308` with `location: /tailor/some-slug`.
- [x] 5.5 Confirm the Job Match tab's internal layout (live score above the labelled fit snapshot)
      and the separate Score/ATS-delta tab are both visually unchanged from before this change.
      Verified by diff: `web/src/lib/tailor/ArtifactPanel.svelte` has zero changes in this branch.
