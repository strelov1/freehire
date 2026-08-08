## 1. Tailor workspace absorbs the fit-analysis trigger

- [ ] 1.1 In `web/src/routes/tailor/[slug]/+page.svelte`, replace the `409 "run the fit analysis
      first"` handler (currently `goto(resolve('/match/[slug]', { slug }))`) with logic that opens
      the existing fit-analysis SSE stream (`$lib/matchAnalysis.ts`) in place.
- [ ] 1.2 Render the stream's stage progress (stepper, thinking panel, progressive section fill) in
      the Job Match tab while it runs, reusing the presentation already built for `MatchAnalysisFull`
      rather than duplicating it.
- [ ] 1.3 On stream completion, retry the tailoring bootstrap so the workspace loads normally with
      the now-cached analysis. On stream failure, surface the error state the stream already emits
      (do not silently fall back to a dead workspace).

## 2. Repoint in-app links to `/tailor/[slug]`

- [ ] 2.1 Update `web/src/lib/components/MatchSummary.svelte`'s two `resolve('/match/[slug]', ...)`
      call sites to `resolve('/tailor/[slug]', ...)`.
- [ ] 2.2 Grep the rest of `web/src` for other `/match/[slug]` link targets (job cards, `JobDrawer`,
      `SwipeDeck`, `JobView`) and repoint each to `/tailor/[slug]`.

## 3. Redirects for old paths

- [ ] 3.1 Update `web/src/routes/jobs/[slug]/fit/+page.ts` to redirect (308) to
      `/tailor/${params.slug}` instead of `/match/${params.slug}`.
- [ ] 3.2 Add `web/src/routes/match/[slug]/+page.ts` (new file) redirecting (308) to
      `/tailor/${params.slug}`.

## 4. Remove the standalone page

- [ ] 4.1 Delete `web/src/routes/match/[slug]/+page.svelte` and its `+page.server.ts`, only after
      task 3.2's redirect route is in place so the path never dead-ends.

## 5. Verify

- [ ] 5.1 `pnpm check` / `pnpm build` in `web/` pass with no route-resolution errors from the
      removed page or the changed link targets.
- [ ] 5.2 Manually open `/tailor/[slug]` for a vacancy with no cached analysis; confirm the stream
      runs inline, the workspace never navigates away, and the Job Match tab ends up showing the
      completed analysis.
- [ ] 5.3 Manually open `/tailor/[slug]` for a vacancy with a fresh cached analysis; confirm it
      renders immediately from the bootstrap response with no stream.
- [ ] 5.4 Manually hit the old `/match/[some-slug]` and `/jobs/[some-slug]/fit` URLs; confirm both
      308-redirect to `/tailor/[some-slug]`.
- [ ] 5.5 Confirm the Job Match tab's internal layout (live score above the labelled fit snapshot)
      and the separate Score/ATS-delta tab are both visually unchanged from before this change —
      this change touches routing and the bootstrap trigger only, not tab layout.
