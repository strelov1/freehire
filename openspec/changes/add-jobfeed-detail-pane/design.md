## Context

Today the home feed (`JobsView.svelte`, mounted at `/`) is a left filter `<aside>` + a full-width, infinite-scrolling list of full-size `JobRow` cards, each a plain `<a href="/jobs/[slug]">`. `/jobs/[slug]` is a separate full page (`JobView` + `JobMatch`/`MatchAnalysisFull` + `JobDescription` + `JobApplyForm` + `JobRelated`, loaded by four independent, fault-tolerant calls in `+page.server.ts`), with sibling routes (`fit`, `copies`, `discussion`, `og.png`) directly beneath it.

A design spike (2026-08-06, scratch HTML, not in this repo) validated the content-density question: `MatchAnalysisFull`'s full report already renders cleanly at the width a list-plus-pane layout affords, because it already ships in production at a *narrower* width (`ArtifactPanel.svelte`, 340–720px, via the component's existing `stacked` prop). `JobRow.svelte` already has a `compact` presentation (tighter padding, single-line title) — built for some other context but structurally exactly what a narrow list column needs.

The codebase carries a documented bug class ([[hire-shallow-routing-back-forward-stale]]) where synchronous shallow-routing writes (`replaceState`/`pushState` from `$app/navigation`) desync `page.url` from `location.search` on browser back/forward. Any mechanism for "show a job's detail without leaving the feed" must not reintroduce that class of bug — it must use real routes, not a client-side URL rewrite.

## Goals / Non-Goals

**Goals:**
- Selecting a job on the desktop feed shows its full detail beside the list, without a full page reload and without losing the list's scroll position or active filters.
- `/jobs/[slug]` keeps its exact URL and data contract — it's reused, not duplicated, so deep links, shares, and its sibling routes keep working.
- Filters move to a horizontal bar above the list; their underlying logic (`FilterStore`, URL sync) is relocated, not rewritten.
- Mobile keeps today's behavior exactly: list only, tapping a card navigates to the full `/jobs/[slug]` page.

**Non-Goals:**
- Rewriting `JobsView`'s filter logic, `JobRow`, or any detail component — this change relays out existing pieces.
- Deciding whether the feed's sibling routes (`fit`, `copies`, `discussion`) also render inside the shared layout — flagged as an open question, not assumed.
- Any backend/API change — the pane consumes the same endpoints `/jobs/[slug]/+page.server.ts` already calls.

## Decisions

**Share a layout between `/` and `/jobs/[slug]` via a SvelteKit route group — don't add a new detail route.** Group both under a shared directory (e.g. `src/routes/(feed)/`) so a `+layout.svelte` there renders the list column + filter bar once, while `src/routes/(feed)/+page.svelte` (→ `/`, no selection) and `src/routes/(feed)/jobs/[slug]/+page.svelte` (→ `/jobs/[slug]`, unchanged URL) render inside it. A route group segment (`(feed)`) contributes nothing to the URL, so `/jobs/[slug]` keeps its exact path. Clicking a card is a plain `<a href="/jobs/[slug]">` — ordinary navigation, not `pushState`. Because both routes share the parent layout, SvelteKit re-runs only the child page's `load` on selection; the layout (list, filters, scroll position) stays mounted. This avoids [[hire-shallow-routing-back-forward-stale]] entirely: there's no synchronous client URL write to desync from `page.url`, and back/forward is native browser history against real routes.
  - *Alternative considered*: a new `/matches/[slug]`-style route mirroring `/jobs/[slug]`'s four API calls independently. Rejected — it duplicates a non-trivial load (`getJob`/`getSimilarJobs`/`getJobCopies`/`getApplyForm`) instead of reusing it, and gives the app two URLs for the same job's detail.
  - *Alternative considered (from the reference app)*: `fetch('/job/:id/detail')` + manual DOM swap + `history.pushState`. Rejected for the same reason as before — it's the shallow-routing shape this codebase has already been burned by.

**List column reuses `JobRow`'s existing `compact` mode.** No new card component; the column renders the same `JobRow` the full feed does today, with `compact` set, inside a fixed ~420–460px column (the width the spike validated `MatchAnalysisFull` at).

**Filters relocate, logic unchanged.** `JobsView`'s filter controls move from the `<aside>` sidebar into a horizontal bar docked above the list column. `FilterStore` and its URL-sync mechanism are unchanged — only the controls' layout container moves.

**No forced re-analysis on selection.** The pane seeds `MatchAnalysisFull`/`JobMatch` from the job's cached `initial` fit with `autoRun={false}` (mirroring `ArtifactPanel.svelte`), so clicking through several jobs doesn't fire a fresh billed analysis per click ([[hire-per-user-llm-key-live]]).

**List ownership moves fully into the shared layout.** `/`'s own `+page.svelte`/`+page.server.ts` shrinks to whatever the "no job selected" state needs (see Open Questions); it must not also fetch the job list, or the list fetches twice (once for the layout, once for the page).

## Risks / Trade-offs

- **[Risk]** `/jobs/[slug]`'s sibling routes (`fit`, `copies`, `discussion`, `og.png`) sit directly under `jobs/[slug]/` today; moving `jobs/[slug]` into a route group doesn't automatically decide whether siblings come along. → **Mitigation**: explicit per-route call during implementation — `og.png` (an image response) almost certainly should NOT render inside the feed layout; `fit`/`discussion` are open questions below.
- **[Risk]** Two things could double-fetch the job list (layout `load` + a leftover page `load`) if `/`'s own page isn't trimmed. → **Mitigation**: list `load` lives only in the shared layout; verify manually before merge.
- **[Risk]** Restructuring live routes (`/`, `/jobs/[slug]`) is a larger, riskier diff than adding a new route alongside them — regressions here are visible to every visitor, not opt-in. → **Mitigation**: land behind a thorough manual pass of both routes (list, selection, deep link, back/forward, mobile) before merge; no code-level feature flag is planned (see Migration Plan), so review rigor is the safety net.
- **[Risk]** `MatchAnalysisFull`'s SSE stream ([[hire-match-analysis-sse-blind-spot]]: always returns SSE 200, failures invisible outside `journalctl`) now sits behind a much higher-traffic entry point (the home feed) than today's one-job-at-a-time detail page. → **Mitigation**: no new server code path is introduced; rely on existing observability, don't scope-creep this change into fixing that blind spot.

## Migration Plan

No schema or data change. This reworks two live, high-traffic routes in place, so: implement behind a normal PR with a full manual verification pass (checklist in tasks.md); rollback is reverting the PR. No feature flag — see [[hire-assistant-rollout-gate-removed]] for the project's general stance that gates are added only when there's a concrete reason, and this change has no gradual-rollout need (it's a layout change, not a spend- or risk-bearing feature).

## Open Questions

- Do `/jobs/[slug]/fit` and `/jobs/[slug]/discussion` render inside the shared feed layout (list visible alongside) or stay full-page like today? `/jobs/[slug]/og.png` almost certainly stays outside (it's an image response, not a page).
- What does `/` show with nothing selected: auto-select the first job in the list (reference app's behavior) or an empty "pick a job" placeholder (this app's `/tailor` precedent)?
- Does filtering the list affect an already-open detail pane if the selected job no longer matches the new filter (keep it open regardless, or clear the selection)?
